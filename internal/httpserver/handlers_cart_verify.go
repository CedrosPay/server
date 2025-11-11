package httpserver

import (
	"net/http"

	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/go-chi/chi/v5"
)

// verifyCartPayment handles GET /paywall/v1/cart/{cartId} - verifies cart payment via X-PAYMENT header.
// This mirrors the single-item endpoint (GET /paywall/{resource}) for consistency.
func (h *handlers) verifyCartPayment(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	cartID := chi.URLParam(r, "cartId")
	if cartID == "" {
		log.Warn().
			Msg("cart.verify.missing_cart_id")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "cart ID required")
		return
	}

	// Cart IDs already include "cart_" prefix from GenerateCartID()
	// Client calls GET /paywall/cart/cart_abc123 where cartID = "cart_abc123"
	resourceID := cartID

	// Extract X-PAYMENT header (same as single-item flow)
	paymentHeader := r.Header.Get("X-PAYMENT")
	if paymentHeader == "" {
		// No payment proof provided - this is a 402 Payment Required
		paymentRequiredResponse(w, "Please provide X-PAYMENT header with payment proof", cartID, "cart")
		return
	}

	// Extract coupon code from query params (optional)
	couponCode := r.URL.Query().Get("couponCode")

	// Verify cart payment using the Authorize method (which detects cart_ prefix)
	result, err := h.paywall.Authorize(r.Context(), resourceID, "", paymentHeader, couponCode)
	if err != nil {
		log.Error().
			Err(err).
			Str("cart_id", cartID).
			Msg("cart.verify.authorization_failed")
		paymentVerificationFailedResponse(w, err, cartID, "cart")
		return
	}

	if !result.Granted {
		log.Warn().
			Str("cart_id", cartID).
			Str("method", result.Method).
			Msg("cart.verify.payment_failed")
		paymentNotGrantedResponse(w, "Payment could not be verified", cartID, "cart")
		return
	}

	log.Info().
		Str("cart_id", cartID).
		Str("method", result.Method).
		Str("wallet", logger.TruncateAddress(result.Wallet)).
		Msg("cart.verify.success")

	// Payment verified successfully
	paymentSuccessResponse(w, cartID, "cart", result)
}

// verifyCartPaymentInternal handles cart payment verification from the unified /paywall/verify endpoint.
func (h *handlers) verifyCartPaymentInternal(w http.ResponseWriter, r *http.Request, cartID, paymentHeader string) {
	log := logger.FromContext(r.Context())
	// Cart IDs should include "cart_" prefix
	resourceID := cartID

	// Extract coupon code from payment metadata if present
	var couponCode string

	// Verify cart payment using the Authorize method (which detects cart_ prefix)
	result, err := h.paywall.Authorize(r.Context(), resourceID, "", paymentHeader, couponCode)
	if err != nil {
		log.Error().
			Err(err).
			Str("cart_id", cartID).
			Msg("cart.verify_internal.authorization_failed")
		paymentVerificationFailedResponse(w, err, cartID, "cart")
		return
	}

	if !result.Granted {
		log.Warn().
			Str("cart_id", cartID).
			Str("method", result.Method).
			Msg("cart.verify_internal.payment_failed")
		paymentNotGrantedResponse(w, "Payment could not be verified", cartID, "cart")
		return
	}

	log.Info().
		Str("cart_id", cartID).
		Str("method", result.Method).
		Str("wallet", logger.TruncateAddress(result.Wallet)).
		Msg("cart.verify_internal.success")

	// Payment verified successfully
	paymentSuccessResponse(w, cartID, "cart", result)
}
