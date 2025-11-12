package paywall

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/CedrosPay/server/internal/callbacks"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/money"
	"github.com/CedrosPay/server/internal/storage"
	"github.com/CedrosPay/server/pkg/x402"
	"github.com/gagliardetto/solana-go"
)

// RefundQuoteRequest represents a request to generate a refund quote.
type RefundQuoteRequest struct {
	OriginalPurchaseID string            `json:"originalPurchaseId"` // Reference to original purchase
	RecipientWallet    string            `json:"recipientWallet"`    // Wallet to receive the refund
	Amount             float64           `json:"amount"`             // Amount to refund
	Token              string            `json:"token"`              // Token symbol
	Reason             string            `json:"reason,omitempty"`   // Optional reason
	Metadata           map[string]string `json:"metadata,omitempty"` // Optional metadata
}

// RefundQuoteResponse contains the generated refund quote.
type RefundQuoteResponse struct {
	RefundID  string       `json:"refundId"`  // Unique refund identifier
	Quote     *CryptoQuote `json:"quote"`     // x402 requirement for the refund
	ExpiresAt time.Time    `json:"expiresAt"` // When this refund quote expires
}

// CreateRefundRequest creates a refund request without generating an x402 quote.
// The quote is generated later when admin approves the refund via RegenerateRefundQuote.
// This is the correct flow: user requests → admin reviews → admin approves → quote generated → admin executes.
func (s *Service) CreateRefundRequest(ctx context.Context, req RefundQuoteRequest) (storage.RefundQuote, error) {
	// Validate request
	if req.OriginalPurchaseID == "" {
		return storage.RefundQuote{}, fmt.Errorf("paywall: originalPurchaseId required")
	}
	if req.RecipientWallet == "" {
		return storage.RefundQuote{}, fmt.Errorf("paywall: recipientWallet required")
	}
	if req.Amount <= 0 {
		return storage.RefundQuote{}, fmt.Errorf("paywall: amount must be positive")
	}
	if req.Token == "" {
		return storage.RefundQuote{}, fmt.Errorf("paywall: token required")
	}

	// Validate recipient wallet address format
	_, err := solana.PublicKeyFromBase58(req.RecipientWallet)
	if err != nil {
		return storage.RefundQuote{}, fmt.Errorf("paywall: invalid recipient wallet address: %w", err)
	}

	// SECURITY: Enforce one-refund-per-signature limit
	// Check if a refund already exists for this transaction signature
	existingRefund, err := s.store.GetRefundQuoteByOriginalPurchaseID(ctx, req.OriginalPurchaseID)
	if err == nil {
		// Refund already exists for this signature
		return storage.RefundQuote{}, fmt.Errorf("paywall: refund already exists for this transaction (refund ID: %s)", existingRefund.ID)
	}
	if err != storage.ErrNotFound {
		// Unexpected error
		return storage.RefundQuote{}, fmt.Errorf("paywall: check existing refund: %w", err)
	}

	// Generate unique refund ID
	refundID, err := storage.GenerateRefundID()
	if err != nil {
		return storage.RefundQuote{}, fmt.Errorf("paywall: generate refund id: %w", err)
	}

	// Store refund request (note: ExpiresAt is set far in future since quote isn't generated yet)
	now := time.Now()
	refundTTL := s.cfg.Storage.RefundQuoteTTL.Duration
	if refundTTL == 0 {
		refundTTL = 15 * time.Minute // Fallback default
	}
	expiresAt := now.Add(refundTTL) // Will be updated when admin approves

	// Convert float64 amount to Money for storage
	asset, err := money.GetAsset(req.Token)
	if err != nil {
		return storage.RefundQuote{}, fmt.Errorf("paywall: get asset for token %s: %w", req.Token, err)
	}
	refundAmount, err := money.FromMajor(asset, strconv.FormatFloat(req.Amount, 'f', -1, 64))
	if err != nil {
		return storage.RefundQuote{}, fmt.Errorf("paywall: convert refund amount to Money: %w", err)
	}

	refundQuote := storage.RefundQuote{
		ID:                 refundID,
		OriginalPurchaseID: req.OriginalPurchaseID,
		RecipientWallet:    req.RecipientWallet,
		Amount:             refundAmount, // Store as Money
		Reason:             req.Reason,
		Metadata:           req.Metadata,
		CreatedAt:          now,
		ExpiresAt:          expiresAt,
	}

	if err := s.store.SaveRefundQuote(ctx, refundQuote); err != nil {
		return storage.RefundQuote{}, fmt.Errorf("paywall: save refund quote: %w", err)
	}

	return refundQuote, nil
}

// buildRefundX402Quote creates an x402 quote for a refund transaction.
// Unlike regular quotes, the payTo field is the CUSTOMER wallet (recipient).
// NOTE: Refunds do NOT use gasless mode - admin pays both refund amount AND network fees.
func (s *Service) buildRefundX402Quote(refundID, recipientWallet string, atomicAmount uint64, token string, expiresAt time.Time) (*CryptoQuote, error) {
	// Convert atomic units to major units for display only
	asset, _ := money.GetAsset(token)
	displayAmount := money.New(asset, int64(atomicAmount)).ToMajor()

	return s.buildX402Quote(x402QuoteOptions{
		ResourceID:            fmt.Sprintf("refund:%s", refundID),
		AtomicAmount:          atomicAmount,
		Token:                 token,
		PayToAddress:          recipientWallet, // Customer wallet receives refund
		RecipientTokenAccount: deriveTokenAccountSafe(recipientWallet, s.cfg.X402.TokenMint),
		Description:           fmt.Sprintf("Refund (%s %s)", displayAmount, token),
		ExpiresAt:             expiresAt,
		IncludeFeePayer:       false, // Refunds do NOT support gasless (admin pays all fees)
	})
}

// authorizeRefund handles x402 payment verification for refund transactions.
// Only the configured payTo wallet (server wallet) can execute refunds.
func (s *Service) authorizeRefund(ctx context.Context, refundID, paymentHeader string) (AuthorizationResult, error) {
	// Payment header is required for refund transactions
	if paymentHeader == "" {
		return AuthorizationResult{}, fmt.Errorf("payment proof required for refund %s", refundID)
	}

	// Parse payment proof
	proof, err := x402.ParsePaymentProof(paymentHeader)
	if err != nil {
		return AuthorizationResult{}, fmt.Errorf("parse payment header: %w", err)
	}

	// Validate scheme and network
	if proof.Scheme != "solana-spl-transfer" && proof.Scheme != "solana" {
		return AuthorizationResult{}, fmt.Errorf("unsupported scheme: %s", proof.Scheme)
	}
	if proof.Network != s.cfg.X402.Network {
		return AuthorizationResult{}, fmt.Errorf("network mismatch: expected %s, got %s", s.cfg.X402.Network, proof.Network)
	}

	// Lookup refund quote
	refund, err := s.store.GetRefundQuote(ctx, refundID)
	if err != nil {
		if err == storage.ErrNotFound {
			return AuthorizationResult{}, fmt.Errorf("refund not found: %s", refundID)
		}
		return AuthorizationResult{}, fmt.Errorf("get refund quote: %w", err)
	}

	// Check if refund quote is expired (transaction execution window)
	// NOTE: Expired quotes remain in storage and can be re-quoted by admin
	now := time.Now()

	// Get refund TTL from config with fallback
	refundTTL := s.cfg.Storage.RefundQuoteTTL.Duration
	if refundTTL == 0 {
		refundTTL = 15 * time.Minute // Fallback default
	}

	if refund.IsExpiredAt(now) {
		return AuthorizationResult{}, fmt.Errorf("refund quote expired, please request a new quote")
	}

	// Check if refund was already processed
	if refund.IsProcessed() {
		return AuthorizationResult{}, fmt.Errorf("refund already processed: %s", refundID)
	}

	// Verify payment amount matches refund amount EXACTLY
	// For refunds, we verify the transaction sent TO the customer
	// Convert refund.Amount (Money) to float64 for x402 verification
	refundAmountFloat, err := strconv.ParseFloat(refund.Amount.ToMajor(), 64)
	if err != nil {
		return AuthorizationResult{}, fmt.Errorf("parse refund amount: %w", err)
	}

	// Get token mint and decimals from Money asset metadata
	tokenMint := refund.Amount.Asset.Metadata.SolanaMint
	tokenDecimals := refund.Amount.Asset.Decimals

	requirement := x402.Requirement{
		ResourceID:            refundID,
		RecipientOwner:        refund.RecipientWallet,                                    // Customer receives refund
		RecipientTokenAccount: deriveTokenAccountSafe(refund.RecipientWallet, tokenMint), // Customer's token account
		TokenMint:             tokenMint,                                                 // From asset metadata
		Amount:                refundAmountFloat,                                         // Convert Money to float64
		Network:               s.cfg.X402.Network,
		TokenDecimals:         tokenDecimals,                      // From asset metadata
		AllowedTokens:         []string{refund.Amount.Asset.Code}, // Token symbol from asset
		QuoteTTL:              refundTTL,
		SkipPreflight:         s.cfg.X402.SkipPreflight,
		Commitment:            s.cfg.X402.Commitment,
	}

	// CRITICAL: Atomically claim this signature BEFORE verification to prevent TOCTOU race
	// The signature is already known from the X-PAYMENT header (Solana tx signature)
	// This prevents concurrent requests from both passing the check and wasting RPC calls
	log := logger.FromContext(ctx)

	// Optimistically record the payment with placeholder data
	// If verification fails, we don't clean this up (intentional - prevents replay)
	placeholderTx := storage.PaymentTransaction{
		Signature:  proof.Signature,
		ResourceID: refundID,
		Wallet:     "",                              // Will be updated after verification
		Amount:     money.Zero(refund.Amount.Asset), // Will be updated after verification
		CreatedAt:  now,
		Metadata:   map[string]string{"status": "verifying"},
	}

	if err := s.store.RecordPayment(ctx, placeholderTx); err != nil {
		// Signature already exists - check if it's for the same refund (idempotent retry) or different resource (replay attack)
		originalTx, getErr := s.store.GetPayment(ctx, proof.Signature)
		if getErr != nil {
			// Can't retrieve original transaction to verify - fail closed
			log.Error().
				Err(err).
				Str("signature", logger.TruncateAddress(proof.Signature)).
				Msg("refund.signature_conflict_lookup_failed")
			return AuthorizationResult{}, fmt.Errorf("payment signature conflict: %w", err)
		}

		// Check if the existing payment is for THIS exact refund (idempotent retry scenario)
		if originalTx.ResourceID == refundID {
			// Same refund - this is an idempotent retry (frontend timeout -> user retries)
			// Check if refund was already processed successfully
			if originalTx.Wallet != "" && originalTx.Metadata["status"] == "verified" {
				log.Info().
					Str("signature", logger.TruncateAddress(proof.Signature)).
					Str("refund_id", refundID).
					Msg("refund.idempotent_retry_already_verified")

				// Return success with the already-verified transaction details
				networkID := s.cfg.X402.Network
				settlement := &SettlementResponse{
					Success:   true,
					Error:     nil,
					TxHash:    &proof.Signature,
					NetworkID: &networkID,
				}
				return AuthorizationResult{
					Granted:    true,
					Method:     "x402-refund",
					Wallet:     originalTx.Wallet,
					Settlement: settlement,
				}, nil
			}
			// Payment exists but not verified yet - let verification proceed (might be stuck in 'verifying' state)
			log.Info().
				Str("signature", logger.TruncateAddress(proof.Signature)).
				Str("refund_id", refundID).
				Msg("refund.idempotent_retry_continue_verification")
			// Continue to verification step below
		} else {
			// Different resource - this is a true replay attack
			log.Warn().
				Err(err).
				Str("signature", logger.TruncateAddress(proof.Signature)).
				Str("original_resource_hash", hashResourceID(originalTx.ResourceID)).
				Str("attempted_refund_hash", hashResourceID(refundID)).
				Msg("refund.replay_attack_detected")

			if s.metrics != nil {
				s.metrics.ObserveRefund("failed", 0, refund.Amount.Asset.Code, 0, "crypto")
			}

			return AuthorizationResult{}, fmt.Errorf("payment proof has already been used (originally for: %s)", originalTx.ResourceID)
		}
	}

	// Track refund processing timing
	refundStart := time.Now()

	result, err := s.verifier.Verify(ctx, proof, requirement)
	refundDuration := time.Since(refundStart)

	if err != nil {
		// Record failed refund metric
		if s.metrics != nil {
			s.metrics.ObserveRefund("failed", 0, refund.Amount.Asset.Code, refundDuration, "crypto")
		}

		if vErr, ok := err.(x402.VerificationError); ok {
			log.Error().
				Err(vErr.Err).
				Str("refund_hash", hashResourceID(refundID)).
				Str("error_code", string(vErr.Code)).
				Msg("refund.x402_verification_failed")
			return AuthorizationResult{}, fmt.Errorf("%s", vErr.Message)
		}
		return AuthorizationResult{}, err
	}

	// IMPORTANT: Verify that the payer is the configured payTo wallet (only server can issue refunds)
	// We check the wallet from the verification result, not from the proof, as it's extracted from the actual transaction
	if result.Wallet != s.cfg.X402.PaymentAddress {
		return AuthorizationResult{}, fmt.Errorf("unauthorized: only payment address %s can issue refunds (got %s)", s.cfg.X402.PaymentAddress, result.Wallet)
	}

	// Payment signature was already recorded before verification (atomic claim)
	// Now update the placeholder record with verified refund details
	finalRefundTx := storage.PaymentTransaction{
		Signature:  proof.Signature,
		ResourceID: refundID,
		Wallet:     result.Wallet,
		Amount:     refund.Amount,
		CreatedAt:  now,
		Metadata: map[string]string{
			"status":  "verified",
			"network": s.cfg.X402.Network,
			"type":    "refund",
		},
	}
	if err := s.store.RecordPayment(ctx, finalRefundTx); err != nil {
		// Log error but don't fail - refund was already verified on-chain
		log.Error().
			Err(err).
			Str("signature", logger.TruncateAddress(proof.Signature)).
			Msg("refund.failed_to_finalize_payment_record")
	}

	// Mark refund as processed (processedBy = the server wallet that sent it)
	if err := s.store.MarkRefundProcessed(ctx, refundID, result.Wallet, proof.Signature); err != nil {
		return AuthorizationResult{}, fmt.Errorf("mark refund processed: %w", err)
	}

	// Use atomic units directly for metrics (no float64 conversion)
	amountCents := refund.Amount.Atomic

	// Record successful refund metrics
	if s.metrics != nil {
		s.metrics.ObserveRefund("success", amountCents, refund.Amount.Asset.Code, refundDuration, "crypto")
	}

	// Build callback event with refund details
	metadata := mergeMetadata(refund.Metadata, proof.Metadata)
	metadata["original_purchase_id"] = refund.OriginalPurchaseID
	metadata["recipient_wallet"] = refund.RecipientWallet
	metadata["reason"] = refund.Reason

	// Fire refund succeeded callback
	s.notifier.RefundSucceeded(ctx, callbacks.RefundEvent{
		RefundID:           refundID,
		OriginalPurchaseID: refund.OriginalPurchaseID,
		RecipientWallet:    refund.RecipientWallet,
		AtomicAmount:       refund.Amount.Atomic,
		Token:              refund.Amount.Asset.Code,
		ProcessedBy:        result.Wallet,
		Signature:          proof.Signature,
		Reason:             refund.Reason,
		Metadata:           metadata,
		RefundedAt:         time.Now().UTC(),
	})

	// Build settlement response
	networkID := s.cfg.X402.Network
	settlement := &SettlementResponse{
		Success:   true,
		Error:     nil,
		TxHash:    &proof.Signature,
		NetworkID: &networkID,
	}

	return AuthorizationResult{
		Granted:    true,
		Method:     "x402-refund",
		Wallet:     result.Wallet, // Server wallet that processed refund
		Settlement: settlement,
	}, nil
}

// GetRefundQuote retrieves an existing refund quote by ID.
func (s *Service) GetRefundQuote(ctx context.Context, refundID string) (storage.RefundQuote, error) {
	return s.store.GetRefundQuote(ctx, refundID)
}

// RegenerateRefundQuote generates a fresh x402 quote for an existing refund request.
// This is used when the original quote expires (blockhash becomes stale after 15 min).
func (s *Service) RegenerateRefundQuote(ctx context.Context, refundID string) (RefundQuoteResponse, error) {
	// Get existing refund
	refund, err := s.store.GetRefundQuote(ctx, refundID)
	if err != nil {
		return RefundQuoteResponse{}, fmt.Errorf("paywall: get refund: %w", err)
	}

	// Check if already processed
	if refund.IsProcessed() {
		return RefundQuoteResponse{}, fmt.Errorf("paywall: refund already processed")
	}

	// Generate fresh quote with new expiry
	now := time.Now()
	refundTTL := s.cfg.Storage.RefundQuoteTTL.Duration
	if refundTTL == 0 {
		refundTTL = 15 * time.Minute // Fallback default
	}
	expiresAt := now.Add(refundTTL)

	// Update expiry in storage
	refund.ExpiresAt = expiresAt
	if err := s.store.SaveRefundQuote(ctx, refund); err != nil {
		return RefundQuoteResponse{}, fmt.Errorf("paywall: update refund expiry: %w", err)
	}

	// Build fresh x402 quote
	// Pass atomic units directly from Money type (no float64 conversion)
	quote, err := s.buildRefundX402Quote(refundID, refund.RecipientWallet, uint64(refund.Amount.Atomic), refund.Amount.Asset.Code, expiresAt)
	if err != nil {
		return RefundQuoteResponse{}, fmt.Errorf("paywall: build x402 quote: %w", err)
	}

	return RefundQuoteResponse{
		RefundID:  refundID,
		Quote:     quote,
		ExpiresAt: expiresAt,
	}, nil
}

// ListPendingRefunds returns all unprocessed refund quotes.
// This is used by admin to review pending refund requests.
func (s *Service) ListPendingRefunds(ctx context.Context) ([]storage.RefundQuote, error) {
	return s.store.ListPendingRefunds(ctx)
}

// DenyRefund deletes a pending refund quote, effectively denying the refund request.
// Only unprocessed refunds can be denied. Returns ErrNotFound if the refund doesn't exist.
func (s *Service) DenyRefund(ctx context.Context, refundID string) error {
	if refundID == "" {
		return fmt.Errorf("paywall: refund ID required")
	}

	// Check if refund exists and is not yet processed
	quote, err := s.store.GetRefundQuote(ctx, refundID)
	if err != nil {
		return fmt.Errorf("paywall: %w", err)
	}

	// Prevent deleting already processed refunds
	if quote.IsProcessed() {
		return fmt.Errorf("paywall: cannot deny already processed refund")
	}

	// Delete the refund quote
	if err := s.store.DeleteRefundQuote(ctx, refundID); err != nil {
		return fmt.Errorf("paywall: failed to delete refund: %w", err)
	}

	log := logger.FromContext(ctx)
	log.Info().
		Str("refund_id", refundID).
		Str("reason", quote.Reason).
		Msg("refund.denied")
	return nil
}
