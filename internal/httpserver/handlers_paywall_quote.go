package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/paywall"
	"github.com/CedrosPay/server/pkg/responders"
	"github.com/CedrosPay/server/pkg/x402"
)

// QuoteRequest represents a request to generate a payment quote.
type QuoteRequest struct {
	Resource   string  `json:"resource"`
	CouponCode *string `json:"couponCode,omitempty"`
}

// paywallQuote generates a payment quote without exposing resource ID in URL.
// POST /paywall/v1/quote
func (h *handlers) paywallQuote(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Track quote generation timing
	quoteStart := time.Now()

	var req QuoteRequest

	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().
			Err(err).
			Msg("paywall.quote.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "Invalid request body")
		return
	}

	if req.Resource == "" {
		log.Warn().
			Msg("paywall.quote.missing_resource")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "resource field is required")
		return
	}

	// Extract coupon code
	var couponCode string
	if req.CouponCode != nil {
		couponCode = *req.CouponCode
	}

	// Generate quote using existing paywall service
	quote, err := h.paywall.GenerateQuote(r.Context(), req.Resource, couponCode)
	if err != nil {
		// Distinguish between resource not found vs actual errors
		if errors.Is(err, paywall.ErrResourceNotConfigured) {
			log.Warn().
				Str("resource_id", req.Resource).
				Msg("paywall.quote.resource_not_found")
			apierrors.WriteError(w, apierrors.ErrCodeResourceNotFound,
				"Resource not found",
				map[string]interface{}{
					"resourceId": req.Resource,
					"hint":       "The requested resource does not exist or is not configured for payments",
				})
			return
		}

		// Actual internal error (RPC failure, config issue, etc.)
		log.Error().
			Err(err).
			Str("resource_id", req.Resource).
			Str("coupon_code", couponCode).
			Msg("paywall.quote.generation_failed")
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeInternalError, "failed to generate quote", "resourceId", req.Resource)
		return
	}

	// Return 402 Payment Required with quote
	// The quote.Crypto field contains the x402 quote
	if quote.Crypto == nil {
		log.Error().
			Str("resource_id", req.Resource).
			Msg("paywall.quote.crypto_unavailable")
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeInternalError, "failed to generate quote", "resourceId", req.Resource)
		return
	}

	response := map[string]any{
		"x402Version": 0,
		"accepts": []any{
			map[string]any{
				"scheme":            quote.Crypto.Scheme,
				"network":           quote.Crypto.Network,
				"maxAmountRequired": quote.Crypto.MaxAmountRequired,
				"resource":          quote.Crypto.Resource,
				"description":       quote.Crypto.Description,
				"mimeType":          quote.Crypto.MimeType,
				"payTo":             quote.Crypto.PayTo,
				"maxTimeoutSeconds": quote.Crypto.MaxTimeoutSeconds,
				"asset":             quote.Crypto.Asset,
				"extra":             quote.Crypto.Extra,
			},
		},
	}

	// Record quote generation timing (using payment observation with settled=false)
	quoteDuration := time.Since(quoteStart)
	if h.metrics != nil {
		// Track quote as a "quote" method with 0 amount since no payment yet
		h.metrics.ObservePayment("quote", req.Resource, false, quoteDuration, 0, "")
	}

	log.Info().
		Str("resource_id", req.Resource).
		Str("coupon_code", couponCode).
		Str("amount_required", quote.Crypto.MaxAmountRequired).
		Msg("paywall.quote.generated")

	responders.JSON(w, http.StatusPaymentRequired, response)
}

// VerifyRequest represents the internal structure for verify endpoint.
// The resource and resourceType are extracted from the X-PAYMENT header payload.
type VerifyRequest struct {
	// No body needed - everything comes from X-PAYMENT header
}

// paywallVerify verifies payment without exposing resource ID in URL.
// POST /paywall/v1/verify
// X-PAYMENT: <base64-encoded-payment-payload>
func (h *handlers) paywallVerify(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	paymentHeader := r.Header.Get("X-PAYMENT")
	if paymentHeader == "" {
		log.Warn().
			Msg("paywall.verify.missing_payment_header")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "Missing X-PAYMENT header")
		return
	}

	// Decode and parse payment proof
	proof, err := parsePaymentProof(paymentHeader)
	if err != nil {
		log.Warn().
			Err(err).
			Msg("paywall.verify.invalid_payment_proof")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidPaymentProof, err.Error())
		return
	}

	// Extract resource and type from payload
	resource := proof.Resource
	resourceType := proof.ResourceType

	if resource == "" {
		log.Warn().
			Msg("paywall.verify.missing_resource")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "resource field required in payment payload")
		return
	}

	// Default to "regular" if not specified
	if resourceType == "" {
		resourceType = "regular"
	}

	log.Debug().
		Str("resource", resource).
		Str("resource_type", resourceType).
		Msg("paywall.verify.routing")

	// Route based on resource type
	switch resourceType {
	case "cart":
		h.verifyCartPaymentInternal(w, r, resource, paymentHeader)
	case "refund":
		h.verifyRefundPaymentInternal(w, r, resource, paymentHeader)
	case "regular":
		h.verifyRegularPaymentInternal(w, r, resource, paymentHeader)
	default:
		log.Warn().
			Str("resource_type", resourceType).
			Msg("paywall.verify.invalid_resource_type")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "Invalid resourceType (must be: regular, cart, or refund)")
	}
}

// parsePaymentProof is a lightweight parser to extract resource info without full verification.
func parsePaymentProof(header string) (struct {
	Resource     string
	ResourceType string
}, error) {
	result := struct {
		Resource     string
		ResourceType string
	}{}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		// Try raw JSON for testing
		decoded = []byte(header)
	}

	// Parse outer payload
	var outer struct {
		Payload struct {
			Resource     string `json:"resource"`
			ResourceType string `json:"resourceType"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(decoded, &outer); err != nil {
		return result, err
	}

	result.Resource = outer.Payload.Resource
	result.ResourceType = outer.Payload.ResourceType
	return result, nil
}

// verifyRegularPaymentInternal handles regular single-item payment verification.
func (h *handlers) verifyRegularPaymentInternal(w http.ResponseWriter, r *http.Request, resourceID, paymentHeader string) {
	log := logger.FromContext(r.Context())
	// Extract coupon code from payment metadata if present
	var couponCode string

	// Parse the payment proof to extract coupon code from metadata
	proof, err := x402.ParsePaymentProof(paymentHeader)
	if err == nil && proof.Metadata != nil {
		// Support both snake_case (coupon_code) and camelCase (couponCode)
		couponCode = proof.Metadata["coupon_code"]
		if couponCode == "" {
			couponCode = proof.Metadata["couponCode"]
		}
	}

	// Use existing Authorize logic
	authResult, err := h.paywall.Authorize(r.Context(), resourceID, "", paymentHeader, couponCode)
	if err != nil {
		if errors.Is(err, paywall.ErrStripeSessionPending) {
			paymentRequiredResponse(w, "Stripe payment is still being confirmed, please retry shortly.", resourceID, "regular")
			return
		}
		log.Error().
			Err(err).
			Str("resource_id", resourceID).
			Msg("paywall.verify.authorization_failed")
		// Check if it's a VerificationError with specific error code
		if vErr, ok := err.(x402.VerificationError); ok {
			apierrors.WriteErrorWithDetail(w, vErr.Code, vErr.Message, "resourceId", resourceID)
			return
		}
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeInternalError, err.Error(), "resourceId", resourceID)
		return
	}

	if !authResult.Granted {
		log.Warn().
			Str("resource_id", resourceID).
			Str("method", authResult.Method).
			Msg("paywall.verify.payment_failed")
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeTransactionFailed, "Payment verification failed", "resourceId", resourceID)
		return
	}

	// Return success
	payload := map[string]any{
		"resource": resourceID,
		"granted":  true,
		"method":   authResult.Method,
	}
	if authResult.Wallet != "" {
		payload["wallet"] = authResult.Wallet
	}
	var signature string
	if authResult.Settlement != nil && authResult.Settlement.TxHash != nil {
		signature = *authResult.Settlement.TxHash
		payload["signature"] = signature
	}

	log.Info().
		Str("resource_id", resourceID).
		Str("method", authResult.Method).
		Str("wallet", logger.TruncateAddress(authResult.Wallet)).
		Str("signature", logger.TruncateAddress(signature)).
		Msg("paywall.verify.success")

	// Add X-PAYMENT-RESPONSE header per x402 spec
	addSettlementHeader(w, authResult.Settlement)

	responders.JSON(w, http.StatusOK, payload)
}
