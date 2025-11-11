package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"

	"github.com/CedrosPay/server/internal/coupons"
	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/money"
	"github.com/CedrosPay/server/internal/paywall"
	"github.com/CedrosPay/server/pkg/responders"
	x402solana "github.com/CedrosPay/server/pkg/x402/solana"
)

// buildGaslessTransaction constructs a complete unsigned transaction for gasless payments.
// The frontend will partially sign this transaction (user signs transfer authority),
// then send it back to the backend which will co-sign as fee payer and submit.
func (h *handlers) buildGaslessTransaction(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Track gasless transaction building timing
	gaslessStart := time.Now()

	// Only allow POST with JSON body
	if r.Method != http.MethodPost {
		log.Warn().
			Str("method", r.Method).
			Msg("gasless.method_not_allowed")
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse request
	var req struct {
		ResourceID string `json:"resourceId"`
		UserWallet string `json:"userWallet"`
		FeePayer   string `json:"feePayer,omitempty"`   // Optional: specific server wallet to use
		CouponCode string `json:"couponCode,omitempty"` // Optional: coupon code for discount
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().
			Err(err).
			Msg("gasless.invalid_request")
		respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	if req.ResourceID == "" {
		log.Warn().
			Msg("gasless.missing_resource_id")
		respondError(w, http.StatusBadRequest, "resourceId required")
		return
	}
	if req.UserWallet == "" {
		log.Warn().
			Msg("gasless.missing_user_wallet")
		respondError(w, http.StatusBadRequest, "userWallet required")
		return
	}

	// Check if gasless is enabled
	if !h.cfg.X402.GaslessEnabled {
		log.Warn().
			Msg("gasless.not_enabled")
		respondError(w, http.StatusBadRequest, "gasless transactions not enabled")
		return
	}

	// Parse user wallet
	userWallet, err := solana.PublicKeyFromBase58(req.UserWallet)
	if err != nil {
		log.Warn().
			Err(err).
			Str("user_wallet", req.UserWallet).
			Msg("gasless.invalid_wallet")
		respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid userWallet: %v", err))
		return
	}

	// Parse token mint
	tokenMint, err := solana.PublicKeyFromBase58(h.cfg.X402.TokenMint)
	if err != nil {
		log.Error().
			Err(err).
			Str("token_mint", h.cfg.X402.TokenMint).
			Msg("gasless.invalid_token_mint_config")
		respondError(w, http.StatusInternalServerError, "invalid token mint configuration")
		return
	}

	// Determine if this is a cart or a regular resource
	var atomicAmount uint64
	var memo string
	var recipientTokenAccount solana.PublicKey

	if strings.HasPrefix(req.ResourceID, "cart_") {
		// Handle cart payment
		log.Debug().
			Str("resource_id", req.ResourceID).
			Msg("gasless.cart_payment")
		cartQuote, err := h.paywall.GetCartQuote(r.Context(), req.ResourceID)
		if err != nil {
			log.Error().
				Err(err).
				Str("resource_id", req.ResourceID).
				Msg("gasless.cart_not_found")
			respondError(w, http.StatusNotFound, fmt.Sprintf("cart not found: %v", err))
			return
		}
		now := time.Now()
		if cartQuote.IsExpiredAt(now) {
			log.Warn().
				Str("resource_id", req.ResourceID).
				Time("expired_at", cartQuote.ExpiresAt).
				Msg("gasless.cart_expired")
			respondError(w, http.StatusBadRequest, "cart quote has expired")
			return
		}
		// Use atomic units directly from Money type (no float64 conversion)
		atomicAmount = uint64(cartQuote.Total.Atomic)
		memo = fmt.Sprintf("cart:%s", req.ResourceID)

		// Derive recipient token account from payment address for cart
		ownerKey, err := solana.PublicKeyFromBase58(h.cfg.X402.PaymentAddress)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "invalid payment address")
			return
		}
		recipientTokenAccount, _, err = solana.FindAssociatedTokenAddress(ownerKey, tokenMint)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to derive recipient token account: %v", err))
			return
		}
	} else {
		// Handle regular resource payment
		log.Debug().
			Str("resource_id", req.ResourceID).
			Msg("gasless.regular_payment")
		resource, err := h.paywall.ResourceDefinition(r.Context(), req.ResourceID)
		if err != nil {
			log.Error().
				Err(err).
				Str("resource_id", req.ResourceID).
				Msg("gasless.resource_not_found")
			respondError(w, http.StatusNotFound, fmt.Sprintf("resource not found: %v", err))
			return
		}

		// IMPORTANT: Apply ALL coupons (catalog + checkout) for single product gasless transactions
		// Since there's no separate cart step, the single product IS the cart
		// This ensures gasless transactions use the same discounted price as quotes

		// Validate manual coupon if provided
		var manualCoupon *coupons.Coupon
		if req.CouponCode != "" && h.couponRepo != nil {
			coupon, err := h.couponRepo.GetCoupon(r.Context(), req.CouponCode)
			if err == nil && coupon.IsValid() == nil &&
				coupon.AppliesToProduct(req.ResourceID) &&
				coupon.AppliesToPaymentMethod(coupons.PaymentMethodX402) {
				manualCoupon = &coupon
			}
			// Note: Silently ignore invalid coupons (matches Stripe behavior)
		}

		// Get catalog-level auto-apply coupons + optional manual coupon
		catalogCoupons := paywall.SelectCouponsForPayment(r.Context(), h.couponRepo, req.ResourceID, coupons.PaymentMethodX402, manualCoupon, paywall.ScopeCatalog)

		// Get checkout-level auto-apply coupons
		checkoutCoupons := paywall.SelectCouponsForPayment(r.Context(), h.couponRepo, "", coupons.PaymentMethodX402, nil, paywall.ScopeCheckout)

		// Combine catalog + checkout coupons
		var applicableCoupons []coupons.Coupon
		applicableCoupons = append(applicableCoupons, catalogCoupons...)
		applicableCoupons = append(applicableCoupons, checkoutCoupons...)

		// Use atomic amount directly (Money type)
		cryptoAsset, err := money.GetAsset(resource.CryptoToken)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("get crypto asset: %v", err))
			return
		}
		cryptoMoney := money.Money{Asset: cryptoAsset, Atomic: resource.CryptoAtomicAmount}

		// Apply stacked coupons using precise Money arithmetic (percentage first, then fixed)
		if len(applicableCoupons) > 0 {
			cryptoMoney, err = paywall.StackCouponsOnMoney(cryptoMoney, applicableCoupons)
			if err != nil {
				respondError(w, http.StatusInternalServerError, fmt.Sprintf("apply coupons: %v", err))
				return
			}
		}

		// IMPORTANT: Round to cents precision (2 decimals) using precise integer arithmetic
		// This ensures gasless transactions use the same price as quotes ($0.184 → $0.19)
		cryptoMoney = cryptoMoney.RoundUpToCents()

		// Use atomic units directly from Money type (no float64 conversion)
		atomicAmount = uint64(cryptoMoney.Atomic)

		memo = h.paywall.InterpolateMemo(resource.MemoTemplate, req.ResourceID)

		// Parse recipient token account
		if resource.CryptoAccount != "" {
			recipientTokenAccount, err = solana.PublicKeyFromBase58(resource.CryptoAccount)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "invalid recipient token account")
				return
			}
		} else {
			// Derive recipient token account from payment address
			ownerKey, err := solana.PublicKeyFromBase58(h.cfg.X402.PaymentAddress)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "invalid payment address")
				return
			}
			recipientTokenAccount, _, err = solana.FindAssociatedTokenAddress(ownerKey, tokenMint)
			if err != nil {
				respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to derive recipient token account: %v", err))
				return
			}
		}
	}

	// Get cached recent blockhash (shares cache with /recent-blockhash endpoint)
	blockhash, valid := h.rpcProxy.getCachedBlockhash()
	if !valid {
		// Cache miss or expired - fetch fresh blockhash
		var err error
		blockhash, err = h.rpcProxy.fetchAndCacheBlockhash(r.Context())
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get recent blockhash: %v", err))
			return
		}
	}

	// Parse optional fee payer
	var feePayer *solana.PublicKey
	if req.FeePayer != "" {
		parsed, err := solana.PublicKeyFromBase58(req.FeePayer)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid feePayer: %v", err))
			return
		}
		feePayer = &parsed
	}

	// Build the transaction using the verifier (which has access to server wallets)
	verifier, ok := h.verifier.(interface {
		BuildGaslessTransaction(ctx context.Context, req x402solana.GaslessTxRequest) (x402solana.GaslessTxResponse, error)
	})
	if !ok {
		respondError(w, http.StatusInternalServerError, "verifier does not support gasless transactions")
		return
	}

	txResp, err := verifier.BuildGaslessTransaction(r.Context(), x402solana.GaslessTxRequest{
		PayerWallet:           userWallet,
		FeePayer:              feePayer,
		RecipientTokenAccount: recipientTokenAccount,
		TokenMint:             tokenMint,
		Amount:                atomicAmount,
		Decimals:              h.cfg.X402.TokenDecimals,
		Memo:                  memo,
		ComputeUnitLimit:      h.cfg.X402.ComputeUnitLimit,
		ComputeUnitPrice:      h.cfg.X402.ComputeUnitPriceMicroLamports,
		Blockhash:             blockhash,
	})
	if err != nil {
		// Record failed gasless transaction build
		if h.metrics != nil {
			h.metrics.ObservePaymentFailure("gasless", req.ResourceID, "tx_build_failed")
		}
		log.Error().
			Err(err).
			Str("resource_id", req.ResourceID).
			Str("user_wallet", logger.TruncateAddress(req.UserWallet)).
			Msg("gasless.build_failed")
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to build transaction: %v", err))
		return
	}

	// Record successful gasless transaction build
	gaslessDuration := time.Since(gaslessStart)
	// Convert atomic units to cents for metrics (assuming atomic units ARE cents for USDC-like tokens)
	amountCents := int64(atomicAmount)
	if h.metrics != nil {
		// Note: This is just the build phase, actual payment happens when user signs and submits
		// Use token mint address as currency identifier for gasless transactions
		h.metrics.ObservePayment("gasless", req.ResourceID, false, gaslessDuration, amountCents, h.cfg.X402.TokenMint)
	}

	// Calculate display amount from atomic units for logging
	displayAmount := float64(atomicAmount) / float64(pow10(h.cfg.X402.TokenDecimals))

	log.Info().
		Str("resource_id", req.ResourceID).
		Str("user_wallet", logger.TruncateAddress(req.UserWallet)).
		Float64("amount", displayAmount).
		Uint64("amount_lamports", atomicAmount).
		Str("memo", memo).
		Msg("gasless.transaction_built")

	responders.JSON(w, http.StatusOK, txResp)
}

// pow10 calculates 10^n for converting atomic units to decimal amounts.
func pow10(n uint8) uint64 {
	result := uint64(1)
	for i := uint8(0); i < n; i++ {
		result *= 10
	}
	return result
}

// respondError writes an error response with the given status code and message.
func respondError(w http.ResponseWriter, status int, message string) {
	// Map status codes to error codes
	var errorCode apierrors.ErrorCode
	switch status {
	case http.StatusBadRequest:
		errorCode = apierrors.ErrCodeInvalidField
	case http.StatusNotFound:
		errorCode = apierrors.ErrCodeResourceNotFound
	case http.StatusMethodNotAllowed:
		errorCode = apierrors.ErrCodeInvalidField
	default:
		errorCode = apierrors.ErrCodeInternalError
	}
	apierrors.WriteSimpleError(w, errorCode, message)
}
