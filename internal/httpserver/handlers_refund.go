package httpserver

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"

	"github.com/CedrosPay/server/internal/auth"
	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/paywall"
	"github.com/CedrosPay/server/internal/storage"
	"github.com/CedrosPay/server/pkg/responders"
)

// requestRefundRequest captures the refund request from any user (publicly accessible).
type requestRefundRequest struct {
	OriginalPurchaseID string            `json:"originalPurchaseId"` // Reference to original purchase (resourceID, cartID, etc.)
	RecipientWallet    string            `json:"recipientWallet"`    // Wallet to receive the refund
	Amount             float64           `json:"amount"`             // Amount to refund
	Token              string            `json:"token"`              // Token symbol (USDC, etc.)
	Reason             string            `json:"reason,omitempty"`   // Optional refund reason
	Metadata           map[string]string `json:"metadata,omitempty"` // Optional metadata
}

// requestRefund handles POST /request-refund - generates x402 quote for issuing a refund.
// SECURITY REQUIREMENTS:
// 1. originalPurchaseId MUST be a Solana transaction signature that exists in our payment records
// 2. recipientWallet MUST match the wallet that made the original payment
// 3. Request MUST be signed by EITHER:
//    - The recipientWallet (user requesting their own refund), OR
//    - The payTo wallet (admin issuing refund on behalf of user)
// 4. Only one refund request can be created per transaction signature
// Admin reviews and approves/denies requests via separate management endpoints.
func (h *handlers) requestRefund(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	// Parse and validate request body first
	var req requestRefundRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().
			Err(err).
			Msg("refund.request.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	// Validate request
	if req.OriginalPurchaseID == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "originalPurchaseId required (must be a Solana transaction signature)")
		return
	}
	if req.RecipientWallet == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "recipientWallet required")
		return
	}
	if req.Amount <= 0 {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidAmount, "amount must be positive")
		return
	}
	if req.Token == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "token required")
		return
	}

	// SECURITY: Validate that originalPurchaseId is a valid Solana signature
	// This prevents arbitrary refund requests for non-existent payments
	_, err := solana.SignatureFromBase58(req.OriginalPurchaseID)
	if err != nil {
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeInvalidSignature,
			"originalPurchaseId must be a valid Solana transaction signature",
			"hint", "expected base58-encoded signature from a completed payment")
		return
	}

	// SECURITY: Verify the signature exists in our payment records and get payment details
	// This ensures refunds can only be requested for actual completed payments
	payment, err := h.paywall.GetPayment(r.Context(), req.OriginalPurchaseID)
	if err != nil {
		if err.Error() == "paywall: payment not found" || err.Error() == "paywall: storage: not found" {
			apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeResourceNotFound,
				"payment not found",
				"hint", "originalPurchaseId must correspond to a completed payment processed by this server")
			return
		}
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "failed to verify payment")
		return
	}

	// SECURITY: Validate that recipientWallet matches the wallet that made the original payment
	// This prevents someone from requesting a refund to a different wallet
	if payment.Wallet != req.RecipientWallet {
		apierrors.WriteError(w, apierrors.ErrCodeInvalidRecipient,
			"recipientWallet must match the wallet that made the original payment",
			map[string]interface{}{
				"hint":           "refunds can only be issued to the wallet that paid for the transaction",
				"expectedWallet": payment.Wallet,
				"providedWallet": req.RecipientWallet,
			})
		return
	}

	// SECURITY: Validate that refund amount does not exceed original payment amount
	// This prevents users from requesting refunds larger than what they paid
	// Convert payment.Amount (Money) to float64 for comparison
	paymentAmountFloat, err := strconv.ParseFloat(payment.Amount.ToMajor(), 64)
	if err != nil {
		apierrors.WriteError(w, apierrors.ErrCodeAmountMismatch,
			"failed to parse payment amount",
			map[string]interface{}{
				"hint": "internal error processing refund request",
			})
		return
	}

	if req.Amount > paymentAmountFloat {
		apierrors.WriteError(w, apierrors.ErrCodeAmountMismatch,
			"refund amount exceeds original payment amount",
			map[string]interface{}{
				"hint":            "refund amount must be equal to or less than the original payment",
				"requestedAmount": req.Amount,
				"originalAmount":  paymentAmountFloat,
				"maxRefundable":   paymentAmountFloat,
			})
		return
	}

	// SECURITY: Validate that token matches the original payment token
	// This prevents requesting a refund in a different token than the original payment
	if req.Token != payment.Amount.Asset.Code {
		apierrors.WriteError(w, apierrors.ErrCodeInvalidTokenMint,
			"refund token must match original payment token",
			map[string]interface{}{
				"hint":           "refunds must be issued in the same token as the original payment",
				"requestedToken": req.Token,
				"originalToken":  payment.Amount.Asset.Code,
			})
		return
	}

	// SECURITY: Verify the signer is either:
	// 1. The recipient wallet (user requesting their own refund), OR
	// 2. The payTo wallet (admin issuing refund on behalf of user)
	verifier := auth.NewSignatureVerifier()
	allowedSigners := []string{req.RecipientWallet, h.cfg.X402.PaymentAddress}
	expectedMessage := "request-refund:" + req.OriginalPurchaseID

	if err := verifier.VerifyUserRequest(r, allowedSigners, expectedMessage); err != nil {
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeInvalidSignature,
			err.Error(),
			"hint", "sign message 'request-refund:<originalPurchaseId>' with your wallet")
		return
	}

	// Create refund request (service layer enforces one-refund-per-signature limit)
	// Note: This does NOT generate the x402 quote - that happens when admin approves
	refund, err := h.paywall.CreateRefundRequest(r.Context(), paywall.RefundQuoteRequest{
		OriginalPurchaseID: req.OriginalPurchaseID,
		RecipientWallet:    req.RecipientWallet,
		Amount:             req.Amount,
		Token:              req.Token,
		Reason:             req.Reason,
		Metadata:           req.Metadata,
	})
	if err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	// Return simple confirmation, NOT an x402 quote
	// Convert Money to float64 for response
	refundAmountFloat, _ := strconv.ParseFloat(refund.Amount.ToMajor(), 64)

	responders.JSON(w, http.StatusOK, map[string]any{
		"refundId":           refund.ID,
		"status":             "pending",
		"originalPurchaseId": refund.OriginalPurchaseID,
		"recipientWallet":    refund.RecipientWallet,
		"amount":             refundAmountFloat,
		"token":              refund.Amount.Asset.Code,
		"reason":             refund.Reason,
		"createdAt":          refund.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"message":            "Refund request submitted successfully. An admin will review and process your request.",
	})
}

// getRefundQuoteRequest captures the request to get a fresh refund quote.
type getRefundQuoteRequest struct {
	RefundID string `json:"refundId"` // ID of the refund to get a fresh quote for
}

// getRefundQuote handles POST /paywall/v1/refunds/approve - generates fresh x402 quote for existing refund.
// Allows admin to get new quote if original expired (blockhash becomes stale after 15 min).
// Requires signature from payTo wallet.
func (h *handlers) getRefundQuote(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Track refund quote generation timing
	quoteStart := time.Now()

	var req getRefundQuoteRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().Err(err).Msg("refund.quote.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	if req.RefundID == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "refundId required")
		return
	}

	refundID := req.RefundID

	// Verify signature from payTo wallet (admin only)
	verifier := auth.NewSignatureVerifier()
	expectedMessage := "approve-refund:" + refundID
	if err := verifier.VerifyAdminRequest(r, h.cfg.X402.PaymentAddress, expectedMessage); err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidSignature, err.Error())
		return
	}

	// Get the existing refund request
	refund, err := h.paywall.GetRefundQuote(r.Context(), refundID)
	if err != nil {
		if err.Error() == "paywall: storage: not found" || err.Error() == "paywall: not found" {
			apierrors.WriteSimpleError(w, apierrors.ErrCodeRefundNotFound, "refund not found")
			return
		}
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, err.Error())
		return
	}

	// Check if already processed
	if refund.IsProcessed() {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeRefundAlreadyProcessed, "refund already processed")
		return
	}

	// Generate fresh quote for this refund
	resp, err := h.paywall.RegenerateRefundQuote(r.Context(), refundID)
	if err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, err.Error())
		return
	}

	// Record refund quote generation timing
	quoteDuration := time.Since(quoteStart)
	if h.metrics != nil {
		// Track refund quote timing
		h.metrics.ObserveRefund("quote", 0, "", quoteDuration, "crypto")
	}

	responders.JSON(w, http.StatusOK, resp)
}

// denyRefundRequest captures the request to deny a refund.
type denyRefundRequest struct {
	RefundID string `json:"refundId"` // ID of the refund to deny
}

// denyRefund handles POST /paywall/v1/refunds/deny - admin denies/cancels a pending refund request.
// Only unprocessed refunds can be denied. Requires signature from payTo wallet.
func (h *handlers) denyRefund(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	var req denyRefundRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().Err(err).Msg("refund.deny.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	if req.RefundID == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "refundId required")
		return
	}

	refundID := req.RefundID

	// Verify signature from payTo wallet (admin only)
	verifier := auth.NewSignatureVerifier()
	expectedMessage := "deny-refund:" + refundID
	if err := verifier.VerifyAdminRequest(r, h.cfg.X402.PaymentAddress, expectedMessage); err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidSignature, err.Error())
		return
	}

	// Deny the refund
	if err := h.paywall.DenyRefund(r.Context(), refundID); err != nil {
		// Check for specific errors
		if err.Error() == "paywall: storage: not found" || err.Error() == "paywall: not found" {
			apierrors.WriteSimpleError(w, apierrors.ErrCodeRefundNotFound, "refund not found")
			return
		}
		if err.Error() == "paywall: cannot deny already processed refund" {
			apierrors.WriteSimpleError(w, apierrors.ErrCodeRefundAlreadyProcessed, "cannot deny already processed refund")
			return
		}

		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, err.Error())
		return
	}

	responders.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "refund denied",
	})
}

// generateNonceRequest captures the nonce generation request.
type generateNonceRequest struct {
	Purpose string `json:"purpose"` // Purpose of the nonce (e.g., "list-pending-refunds")
}

// generateNonce handles POST /paywall/v1/nonce - generates a one-time nonce for admin actions.
// This allows the frontend to request a nonce, sign it, then make the actual request.
// SECURITY: Open endpoint, but nonces expire after 5 minutes and can only be used once.
func (h *handlers) generateNonce(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	var req generateNonceRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().Err(err).Msg("refund.nonce.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	if req.Purpose == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "purpose required")
		return
	}

	// Generate nonce
	nonceID, err := storage.GenerateNonceID()
	if err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "failed to generate nonce")
		return
	}

	now := time.Now()
	nonce := storage.AdminNonce{
		ID:        nonceID,
		Purpose:   req.Purpose,
		CreatedAt: now,
		ExpiresAt: now.Add(storage.NonceTTL),
	}

	if err := h.paywall.CreateNonce(r.Context(), nonce); err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, fmt.Sprintf("failed to store nonce: %v", err))
		return
	}

	responders.JSON(w, http.StatusOK, map[string]any{
		"nonce":     nonceID,
		"expiresAt": nonce.ExpiresAt.Unix(),
		"purpose":   req.Purpose,
	})
}

// listPendingRefunds handles POST /paywall/v1/refunds/pending - returns all pending refund requests.
// This endpoint is admin-only and requires signature from payTo wallet.
// SECURITY: Uses nonce-based replay protection - each nonce can only be used once.
func (h *handlers) listPendingRefunds(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	log.Debug().Msg("refund.list_pending.requested")
	// Extract and verify signature headers
	verifier := auth.NewSignatureVerifier()
	headers, err := verifier.ExtractHeaders(r)
	if err != nil {
		log.Warn().Err(err).Msg("refund.list_pending.unauthorized")
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeInvalidSignature,
			err.Error(),
			"hint", "sign message 'list-pending-refunds:<nonce>' with payTo wallet")
		return
	}

	// SECURITY: Verify the message includes nonce for replay protection
	// Expected format: "list-pending-refunds:<nonce>"
	if len(headers.Message) < 22 || headers.Message[:21] != "list-pending-refunds:" {
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeInvalidField,
			"invalid message format",
			"hint", "expected format: 'list-pending-refunds:<nonce>'")
		return
	}

	// Extract nonce from message
	nonce := headers.Message[21:]
	if nonce == "" {
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeMissingField,
			"nonce required in message",
			"hint", "obtain nonce from POST /paywall/v1/nonce first")
		return
	}

	// CRITICAL: Verify cryptographic signature BEFORE checking signer identity
	// This prevents timing attacks and information disclosure vulnerabilities
	if err := verifier.VerifySignature(headers); err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidSignature, err.Error())
		return
	}

	// SECURITY: Now that signature is verified, check that signer is the configured payTo wallet
	// This order ensures cryptographic verification happens before identity checks
	if headers.Signer != h.cfg.X402.PaymentAddress {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeUnauthorizedRefundIssuer,
			"unauthorized: only payment address can view pending refunds")
		return
	}

	// SECURITY: Consume the nonce to prevent replay attacks
	// This ensures each nonce can only be used once
	if err := h.paywall.ConsumeNonce(r.Context(), nonce); err != nil {
		apierrors.WriteError(w, apierrors.ErrCodeInvalidSignature,
			fmt.Sprintf("nonce validation failed: %v", err),
			map[string]interface{}{
				"hint": "nonce may be expired, already used, or invalid - request a new nonce",
			})
		return
	}

	// Get pending refunds from service
	refunds, err := h.paywall.ListPendingRefunds(r.Context())
	if err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, err.Error())
		return
	}

	responders.JSON(w, http.StatusOK, map[string]any{
		"refunds": refunds,
		"count":   len(refunds),
	})
}
