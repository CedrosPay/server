package paywall

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CedrosPay/server/internal/callbacks"
	"github.com/CedrosPay/server/internal/coupons"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/money"
	"github.com/CedrosPay/server/internal/storage"
	"github.com/CedrosPay/server/pkg/x402"
)

// Authorize attempts to grant access using Stripe or x402 proof headers.
func (s *Service) Authorize(ctx context.Context, resourceID, stripeSessionID, paymentHeader, couponCode string) (AuthorizationResult, error) {
	// Check if this is a cart payment (resourceID starts with "cart_")
	if strings.HasPrefix(resourceID, "cart_") {
		return s.authorizeCart(ctx, resourceID, paymentHeader, couponCode)
	}

	// Check if this is a refund verification (resourceID starts with "refund_")
	if strings.HasPrefix(resourceID, "refund_") {
		return s.authorizeRefund(ctx, resourceID, paymentHeader)
	}

	resource, err := s.ResourceDefinition(ctx, resourceID)
	if err != nil {
		return AuthorizationResult{}, err
	}

	// Check Stripe session if provided
	if stripeSessionID != "" {
		sessionSignature := fmt.Sprintf("stripe:%s", stripeSessionID)
		payment, err := s.store.GetPayment(ctx, sessionSignature)
		if err == nil {
			if payment.ResourceID != resourceID {
				return AuthorizationResult{}, fmt.Errorf("stripe session belongs to %s, not %s", payment.ResourceID, resourceID)
			}
			return AuthorizationResult{Granted: true, Method: "stripe", Wallet: payment.Wallet}, nil
		}
		if err != storage.ErrNotFound {
			return AuthorizationResult{}, fmt.Errorf("lookup stripe session: %w", err)
		}
		return AuthorizationResult{}, ErrStripeSessionPending
	}

	if paymentHeader != "" {
		proof, err := x402.ParsePaymentProof(paymentHeader)
		if err != nil {
			return AuthorizationResult{}, fmt.Errorf("parse payment header: %w", err)
		}
		// Validate scheme and network match configuration
		if proof.Scheme != "solana-spl-transfer" && proof.Scheme != "solana" {
			return AuthorizationResult{}, fmt.Errorf("unsupported scheme: %s", proof.Scheme)
		}
		if proof.Network != s.cfg.X402.Network {
			return AuthorizationResult{}, fmt.Errorf("network mismatch: expected %s, got %s", s.cfg.X402.Network, proof.Network)
		}

		// Verify crypto pricing is configured
		if resource.CryptoAtomicAmount <= 0 {
			return AuthorizationResult{}, fmt.Errorf("resource has no crypto pricing configured")
		}

		// Get asset for Money conversion
		cryptoAsset, err := money.GetAsset(resource.CryptoToken)
		if err != nil {
			return AuthorizationResult{}, fmt.Errorf("get asset for token %s: %w", resource.CryptoToken, err)
		}

		// Validate manual coupon if provided
		manualCoupon := s.validateManualCoupon(ctx, couponCode, resourceID, "")

		// IMPORTANT: For single product authorization, apply ALL coupons (catalog + checkout)
		// Since there's no separate cart step, the single product IS the cart
		// Must match quote generation logic to avoid verification failures
		var applicableCoupons []coupons.Coupon
		if s.coupons != nil {
			catalogCoupons := SelectCouponsForPayment(ctx, s.coupons, resourceID, coupons.PaymentMethodX402, manualCoupon, ScopeCatalog)
			checkoutCoupons := SelectCouponsForPayment(ctx, s.coupons, "", coupons.PaymentMethodX402, nil, ScopeCheckout)
			applicableCoupons = append(catalogCoupons, checkoutCoupons...)
		}

		// Use atomic amount directly (Money type)
		expectedMoney := money.Money{Asset: cryptoAsset, Atomic: resource.CryptoAtomicAmount}

		// Apply stacked coupons using precise Money arithmetic (catalog first, then checkout)
		if len(applicableCoupons) > 0 {
			expectedMoney, err = StackCouponsOnMoney(expectedMoney, applicableCoupons)
			if err != nil {
				return AuthorizationResult{}, fmt.Errorf("apply coupons to expected amount: %w", err)
			}
		}

		// IMPORTANT: Round to cents precision (2 decimals) using precise integer arithmetic
		// This ensures authorization compares against the same rounded amount as the quote
		// Example: $0.184 (after coupons) → $0.19 (ceiling)
		expectedMoney = expectedMoney.RoundUpToCents()

		// Convert back to float64 for x402 verification (external API boundary)
		expectedAmount, _ := strconv.ParseFloat(expectedMoney.ToMajor(), 64)

		// For verification, we need the actual token account to check the transaction
		recipientTokenAccount := resource.CryptoAccount
		if recipientTokenAccount == "" {
			recipientTokenAccount = deriveTokenAccountSafe(s.cfg.X402.PaymentAddress, s.cfg.X402.TokenMint)
		}

		requirement := x402.Requirement{
			ResourceID:            resourceID,
			RecipientOwner:        s.cfg.X402.PaymentAddress,
			RecipientTokenAccount: recipientTokenAccount,
			TokenMint:             s.cfg.X402.TokenMint,
			Amount:                expectedAmount, // Use discounted amount
			Network:               s.cfg.X402.Network,
			TokenDecimals:         s.cfg.X402.TokenDecimals,
			AllowedTokens:         s.cfg.X402.AllowedTokens,
			QuoteTTL:              s.cfg.Paywall.QuoteTTL.Duration,
			SkipPreflight:         s.cfg.X402.SkipPreflight,
			Commitment:            s.cfg.X402.Commitment,
		}

		// CRITICAL: Atomically claim this signature BEFORE verification to prevent TOCTOU race
		// For non-gasless transactions, the signature is already known from the X-PAYMENT header
		// For gasless transactions, the signature is determined after the backend co-signs and submits
		// This prevents concurrent requests from both passing the check and wasting RPC calls
		log := logger.FromContext(ctx)
		now := time.Now()

		// For gasless transactions, skip optimistic recording since signature doesn't exist yet
		// The actual signature is only known after the backend co-signs and Solana accepts the transaction
		isGasless := proof.FeePayer != ""
		if !isGasless {
			// Optimistically record the payment with placeholder data
			// If verification fails, we don't clean this up (intentional - prevents replay)
			placeholderTx := storage.PaymentTransaction{
				Signature:  proof.Signature,
				ResourceID: resourceID,
				Wallet:     "",                      // Will be updated after verification
				Amount:     money.Zero(cryptoAsset), // Will be updated after verification
				CreatedAt:  now,
				Metadata:   map[string]string{"status": "verifying"},
			}

			if err := s.store.RecordPayment(ctx, placeholderTx); err != nil {
				// Signature already exists - either replay attack or concurrent request
				originalTx, _ := s.store.GetPayment(ctx, proof.Signature)
				log.Warn().
					Err(err).
					Str("signature", logger.TruncateAddress(proof.Signature)).
					Str("original_resource_hash", hashResourceID(originalTx.ResourceID)).
					Str("attempted_resource_hash", hashResourceID(resourceID)).
					Msg("authorize.replay_attack_detected")

				if s.metrics != nil {
					s.metrics.ObservePaymentFailure("x402", resourceID, "replay_attack")
				}

				return AuthorizationResult{}, fmt.Errorf("payment proof has already been used (originally for resource: %s)", originalTx.ResourceID)
			}
		}

		// Track payment attempt timing
		paymentStart := time.Now()

		result, err := s.verifier.Verify(ctx, proof, requirement)
		paymentDuration := time.Since(paymentStart)

		if err != nil {
			// Record failed payment metric
			if s.metrics != nil {
				reason := "verification_failed"
				if vErr, ok := err.(x402.VerificationError); ok {
					reason = string(vErr.Code)
				}
				s.metrics.ObservePaymentFailure("x402", resourceID, reason)
			}

			if vErr, ok := err.(x402.VerificationError); ok {
				// Log technical details for debugging (resource ID hashed for security)
				log.Error().
					Err(vErr.Err).
					Str("resource_hash", hashResourceID(resourceID)).
					Str("error_code", string(vErr.Code)).
					Msg("authorize.x402_verification_failed")
				// Return user-friendly message
				return AuthorizationResult{}, fmt.Errorf("%s", vErr.Message)
			}
			return AuthorizationResult{}, err
		}

		// SECURITY: Enforce exact amount matching to prevent frontend bugs and user error
		// The Solana verifier allows overpayment (for tips), but we require exact match
		// Use a separate product/resource for tips/donations if overpayment is desired
		if result.Amount != expectedAmount {
			// Record amount mismatch failure
			if s.metrics != nil {
				s.metrics.ObservePaymentFailure("x402", resourceID, "amount_mismatch")
			}

			log.Error().
				Str("resource_hash", hashResourceID(resourceID)).
				Float64("quoted_amount", expectedAmount).
				Float64("paid_amount", result.Amount).
				Str("wallet", logger.TruncateAddress(result.Wallet)).
				Msg("authorize.payment_amount_mismatch")
			return AuthorizationResult{}, fmt.Errorf("payment amount (%.6f %s) does not match required amount (%.6f %s). Please ensure you're paying the exact quoted amount.",
				result.Amount, cryptoAsset.Code, expectedAmount, cryptoAsset.Code)
		}

		// For gasless transactions, the actual signature comes from the verifier (after co-signing + submission)
		// For non-gasless transactions, use the signature from the proof (pre-submitted by user)
		actualSignature := result.Signature
		if !isGasless && proof.Signature != "" {
			actualSignature = proof.Signature
		}

		// Build metadata with coupon information BEFORE recording payment
		// This ensures coupon codes are persisted in the database
		// Merge: resource metadata → proof metadata → payment-specific metadata
		paymentMetadata := mergeMetadata(resource.Metadata, proof.Metadata)
		if paymentMetadata == nil {
			paymentMetadata = make(map[string]string)
		}
		paymentMetadata["status"] = "verified"
		paymentMetadata["network"] = s.cfg.X402.Network

		if len(applicableCoupons) > 0 {
			// Store all applied coupon codes (comma-separated)
			var couponCodes []string
			for _, c := range applicableCoupons {
				couponCodes = append(couponCodes, c.Code)
			}
			paymentMetadata["coupon_codes"] = strings.Join(couponCodes, ",")
			paymentMetadata["original_amount"] = money.Money{Asset: cryptoAsset, Atomic: resource.CryptoAtomicAmount}.ToMajor()
			paymentMetadata["discounted_amount"] = fmt.Sprintf("%.6f", expectedAmount)
		}

		// Payment signature was already recorded before verification (atomic claim) for non-gasless
		// For gasless, this is the first time we're recording since we didn't know the signature before
		// Now update/create the record with verified payment details
		finalPaymentTx := storage.PaymentTransaction{
			Signature:  actualSignature,
			ResourceID: resourceID,
			Wallet:     result.Wallet,
			Amount:     expectedMoney,
			CreatedAt:  now,
			Metadata:   paymentMetadata, // Now includes coupon info
		}
		if err := s.store.RecordPayment(ctx, finalPaymentTx); err != nil {
			// For gasless: might be a race where same tx was submitted twice
			// For non-gasless: should not happen since we claimed the signature earlier
			if isGasless {
				// Check if this signature was already used
				originalTx, _ := s.store.GetPayment(ctx, actualSignature)
				log.Warn().
					Err(err).
					Str("signature", logger.TruncateAddress(actualSignature)).
					Str("original_resource_hash", hashResourceID(originalTx.ResourceID)).
					Str("attempted_resource_hash", hashResourceID(resourceID)).
					Msg("authorize.gasless_replay_detected")
				return AuthorizationResult{}, fmt.Errorf("payment proof has already been used (originally for resource: %s)", originalTx.ResourceID)
			}
			// Log error but don't fail - payment was already verified on-chain
			log.Error().
				Err(err).
				Str("signature", logger.TruncateAddress(actualSignature)).
				Msg("authorize.failed_to_finalize_payment_record")
		}

		// Convert amount to cents for metrics (stored as float64 in USD)
		amountCents := int64(result.Amount * 100)

		// Record successful payment metrics
		if s.metrics != nil {
			s.metrics.ObservePayment("x402", resourceID, true, paymentDuration, amountCents, resource.CryptoToken)
			// Settlement duration = time from payment verification request to on-chain confirmation
			// This is the same as paymentDuration since Verify() waits for confirmation
			s.metrics.ObserveSettlement(s.cfg.X402.Network, paymentDuration)
		}

		// Increment usage for all applied coupons
		if len(applicableCoupons) > 0 && s.coupons != nil {
			for _, coupon := range applicableCoupons {
				if err := s.coupons.IncrementUsage(ctx, coupon.Code); err != nil {
					// Log error but don't fail - payment was successful
					log.Warn().
						Err(err).
						Str("coupon_code", coupon.Code).
						Msg("authorize.coupon_increment_failed")
				}
			}
		}

		// Use paymentMetadata for callback (already includes resource, proof, and coupon metadata)
		s.notifier.PaymentSucceeded(ctx, callbacks.PaymentEvent{
			ResourceID:         resourceID,
			Method:             "x402",
			CryptoAtomicAmount: expectedMoney.Atomic, // Use discounted amount paid
			CryptoToken:        resource.CryptoToken,
			Wallet:             result.Wallet,
			ProofSignature:     actualSignature,
			Metadata:           paymentMetadata, // Includes coupon codes, original/discounted amounts
			PaidAt:             now.UTC(),
		})

		// Build settlement response following x402 spec
		// Reference: https://github.com/coinbase/x402
		networkID := s.cfg.X402.Network
		settlement := &SettlementResponse{
			Success:   true,
			Error:     nil,
			TxHash:    &result.Signature, // Use actual signature from verification result
			NetworkID: &networkID,
		}

		return AuthorizationResult{
			Granted:    true,
			Method:     "x402",
			Wallet:     result.Wallet,
			Settlement: settlement,
		}, nil
	}

	quote, err := s.GenerateQuote(ctx, resourceID, couponCode)
	if err != nil {
		return AuthorizationResult{}, err
	}
	return AuthorizationResult{
		Granted: false,
		Quote:   &quote,
	}, nil
}
