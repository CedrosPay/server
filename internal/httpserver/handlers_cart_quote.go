package httpserver

import (
	"net/http"
	"time"

	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/paywall"
	"github.com/CedrosPay/server/pkg/responders"
)

// requestCartQuote handles POST /request-cart-quote - generates x402 quote for multiple items.
func (h *handlers) requestCartQuote(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Track cart quote generation timing
	quoteStart := time.Now()

	var req paywall.CartQuoteRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().
			Err(err).
			Msg("cart.quote.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	// Validate request
	if len(req.Items) == 0 {
		log.Warn().
			Msg("cart.quote.empty_cart")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeEmptyCart, "at least one item required")
		return
	}

	// Generate cart quote
	resp, err := h.paywall.GenerateCartQuote(r.Context(), req)
	if err != nil {
		log.Error().
			Err(err).
			Int("item_count", len(req.Items)).
			Str("coupon_code", req.CouponCode).
			Msg("cart.quote.generation_failed")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, err.Error())
		return
	}

	// Record cart quote generation timing
	quoteDuration := time.Since(quoteStart)
	if h.metrics != nil {
		// Track cart quote timing with item count in resource field
		h.metrics.ObserveCartCheckout("quote", len(req.Items))
		h.metrics.ObservePayment("cart_quote", resp.CartID, false, quoteDuration, 0, "")
	}

	log.Info().
		Str("cart_id", resp.CartID).
		Int("item_count", len(req.Items)).
		Str("coupon_code", req.CouponCode).
		Msg("cart.quote.generated")

	// Return HTTP 402 Payment Required with x402 format (not 200 OK)
	// The quote contains the payment requirement that must be satisfied
	responders.JSON(w, http.StatusPaymentRequired, resp)
}
