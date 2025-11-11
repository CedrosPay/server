package httpserver

import (
	"net/http"

	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/go-chi/chi/v5"
)

// verifyRefundPayment handles GET /paywall/v1/refunds/{refundId} - verifies refund execution via X-PAYMENT header.
// This endpoint verifies that the server wallet has successfully executed the refund transaction.
func (h *handlers) verifyRefundPayment(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	refundID := chi.URLParam(r, "refundId")
	if refundID == "" {
		log.Warn().
			Msg("refund.verify.missing_refund_id")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "refund ID required")
		return
	}

	// Refund IDs already include "refund_" prefix from GenerateRefundID()
	// Client calls GET /paywall/refund/refund_xyz789 where refundID = "refund_xyz789"
	resourceID := refundID

	// Extract X-PAYMENT header (same as cart flow)
	paymentHeader := r.Header.Get("X-PAYMENT")
	if paymentHeader == "" {
		// No payment proof provided - this is a 402 Payment Required
		paymentRequiredResponse(w, "Please provide X-PAYMENT header with refund transaction proof", refundID, "refund")
		return
	}

	// Verify refund execution using the Authorize method (which detects refund_ prefix)
	result, err := h.paywall.Authorize(r.Context(), resourceID, "", paymentHeader, "")
	if err != nil {
		log.Error().
			Err(err).
			Str("refund_id", refundID).
			Msg("refund.verify.authorization_failed")
		paymentVerificationFailedResponse(w, err, refundID, "refund")
		return
	}

	if !result.Granted {
		log.Warn().
			Str("refund_id", refundID).
			Str("method", result.Method).
			Msg("refund.verify.transaction_failed")
		paymentNotGrantedResponse(w, "Refund transaction could not be verified", refundID, "refund")
		return
	}

	log.Info().
		Str("refund_id", refundID).
		Str("wallet", logger.TruncateAddress(result.Wallet)).
		Msg("refund.verify.success")

	// Refund verified successfully
	paymentSuccessResponse(w, refundID, "refund", result)
}

// verifyRefundPaymentInternal handles refund payment verification from the unified /paywall/verify endpoint.
func (h *handlers) verifyRefundPaymentInternal(w http.ResponseWriter, r *http.Request, refundID, paymentHeader string) {
	log := logger.FromContext(r.Context())
	// Refund IDs should include "refund_" prefix
	resourceID := refundID

	// Verify refund execution using the Authorize method (which detects refund_ prefix)
	result, err := h.paywall.Authorize(r.Context(), resourceID, "", paymentHeader, "")
	if err != nil {
		log.Error().
			Err(err).
			Str("refund_id", refundID).
			Msg("refund.verify_internal.authorization_failed")
		paymentVerificationFailedResponse(w, err, refundID, "refund")
		return
	}

	if !result.Granted {
		log.Warn().
			Str("refund_id", refundID).
			Str("method", result.Method).
			Msg("refund.verify_internal.transaction_failed")
		paymentNotGrantedResponse(w, "Refund transaction could not be verified", refundID, "refund")
		return
	}

	log.Info().
		Str("refund_id", refundID).
		Str("wallet", logger.TruncateAddress(result.Wallet)).
		Msg("refund.verify_internal.success")

	// Refund verified successfully
	paymentSuccessResponse(w, refundID, "refund", result)
}
