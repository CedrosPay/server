package httpserver

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/CedrosPay/server/internal/coupons"
	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	stripesvc "github.com/CedrosPay/server/internal/stripe"
	"github.com/CedrosPay/server/pkg/responders"
)

type createSessionRequest struct {
	Resource      string            `json:"resource"`
	CustomerEmail string            `json:"customerEmail"`
	Metadata      map[string]string `json:"metadata"`
	SuccessURL    string            `json:"successUrl"`
	CancelURL     string            `json:"cancelUrl"`
	CouponCode    string            `json:"couponCode"` // NEW: Optional coupon code
}

type createSessionResponse struct {
	SessionID string `json:"sessionId"`
	URL       string `json:"url"`
}

// createStripeSession creates a new Stripe checkout session for a resource.
func (h *handlers) createStripeSession(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Track Stripe session creation timing
	sessionStart := time.Now()

	var req createSessionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().
			Err(err).
			Msg("stripe.session.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}
	if req.Resource == "" {
		log.Warn().
			Msg("stripe.session.missing_resource")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "resource is required")
		return
	}

	resource, err := h.paywall.ResourceDefinition(r.Context(), req.Resource)
	if err != nil {
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeResourceNotFound, err.Error(), "resourceId", req.Resource)
		return
	}

	metadata := make(map[string]string)
	for k, v := range resource.Metadata {
		metadata[k] = v
	}
	for k, v := range req.Metadata {
		metadata[k] = v
	}

	// Validate coupon if provided (for metadata tracking)
	originalAmount := resource.FiatAmountCents
	var couponCode string
	var stripeCouponID string

	if req.CouponCode != "" {
		// Validate against our internal coupon repository
		coupon, err := h.couponRepo.GetCoupon(r.Context(), req.CouponCode)
		if err == nil && coupon.IsValid() == nil && coupon.AppliesToProduct(req.Resource) && coupon.AppliesToPaymentMethod(coupons.PaymentMethodStripe) {
			couponCode = req.CouponCode
			// For Stripe, we pass the same code as a promotion code ID
			// User must create matching promotion codes in Stripe Dashboard
			stripeCouponID = req.CouponCode
			// Note: Usage will be incremented in webhook handler upon successful payment
		} else if err == nil && !coupon.AppliesToPaymentMethod(coupons.PaymentMethodStripe) {
			// Coupon exists but doesn't apply to Stripe payments
			apierrors.WriteSimpleError(w, apierrors.ErrCodeCouponWrongPaymentMethod, "coupon not valid for card payments")
			return
		}
	}

	session, err := h.stripe.CreateCheckoutSession(r.Context(), stripesvc.CreateSessionRequest{
		ResourceID:     req.Resource,
		AmountCents:    resource.FiatAmountCents,
		Currency:       resource.FiatCurrency,
		PriceID:        resource.StripePriceID,
		CustomerEmail:  req.CustomerEmail,
		Metadata:       metadata,
		SuccessURL:     req.SuccessURL,
		CancelURL:      req.CancelURL,
		Description:    resource.Description,
		CouponCode:     couponCode,     // For our internal tracking
		StripeCouponID: stripeCouponID, // For Stripe's discount system
		OriginalAmount: originalAmount,
		DiscountAmount: 0, // Stripe calculates this
	})
	if err != nil {
		// Record failed Stripe session creation
		if h.metrics != nil {
			h.metrics.ObservePaymentFailure("stripe", req.Resource, "session_creation_failed")
		}
		apierrors.WriteSimpleError(w, apierrors.ErrCodeStripeError, err.Error())
		return
	}

	// Record successful Stripe session creation timing
	sessionDuration := time.Since(sessionStart)
	if h.metrics != nil {
		// Note: Actual payment happens later in webhook, this is just session creation
		h.metrics.ObservePayment("stripe", req.Resource, false, sessionDuration, resource.FiatAmountCents, resource.FiatCurrency)
	}

	responders.JSON(w, http.StatusOK, createSessionResponse{
		SessionID: session.ID,
		URL:       session.URL,
	})
}

// verifyStripeSession verifies that a Stripe checkout session was completed and paid.
// This endpoint prevents payment bypass attacks where users manually enter success URLs.
func (h *handlers) verifyStripeSession(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Extract session_id from query parameter
	sessionID := r.URL.Query().Get("session_id")

	if sessionID == "" {
		log.Warn().Msg("stripe.verify.missing_session_id")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "session_id is required")
		return
	}

	// Look up payment record using Stripe signature format
	// When webhook processes payment, it stores: signature = "stripe:{session_id}"
	signature := fmt.Sprintf("stripe:%s", sessionID)
	tx, err := h.paywall.GetPayment(r.Context(), signature)

	if err != nil {
		// Payment not found = either not completed yet, or fake session_id
		log.Warn().
			Str("session_id", sessionID).
			Err(err).
			Msg("stripe.verify.payment_not_found")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeSessionNotFound, "Payment not completed or session invalid")
		return
	}

	// Payment verified! Return success with resource info
	log.Info().
		Str("session_id", sessionID).
		Str("resource_id", tx.ResourceID).
		Str("customer", tx.Wallet).
		Msg("stripe.verify.success")

	responders.JSON(w, http.StatusOK, map[string]any{
		"verified":    true,
		"resource_id": tx.ResourceID,
		"paid_at":     tx.CreatedAt,
		"amount":      tx.Amount.String(),
		"customer":    tx.Wallet,
		"metadata":    tx.Metadata,
	})
}

// verifyX402Transaction verifies that an x402 transaction was completed and paid.
// This endpoint allows frontends to check if a transaction signature is valid for re-access scenarios.
func (h *handlers) verifyX402Transaction(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Extract signature from query parameter
	signature := r.URL.Query().Get("signature")

	if signature == "" {
		log.Warn().Msg("x402.verify.missing_signature")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "signature is required")
		return
	}

	// Look up payment record by transaction signature
	tx, err := h.paywall.GetPayment(r.Context(), signature)

	if err != nil {
		// Payment not found = either not verified yet, or invalid signature
		log.Warn().
			Str("signature", logger.TruncateAddress(signature)).
			Err(err).
			Msg("x402.verify.payment_not_found")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeTransactionNotFound, "Transaction not found or not verified")
		return
	}

	// Payment verified! Return success with resource info
	log.Info().
		Str("signature", logger.TruncateAddress(signature)).
		Str("resource_id", tx.ResourceID).
		Str("wallet", logger.TruncateAddress(tx.Wallet)).
		Msg("x402.verify.success")

	responders.JSON(w, http.StatusOK, map[string]any{
		"verified":    true,
		"resource_id": tx.ResourceID,
		"wallet":      tx.Wallet,
		"paid_at":     tx.CreatedAt,
		"amount":      tx.Amount.String(),
		"metadata":    tx.Metadata,
	})
}

// handleStripeWebhook processes incoming Stripe webhook events.
func (h *handlers) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Track webhook processing timing
	webhookStart := time.Now()

	signature := r.Header.Get("Stripe-Signature")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().
			Err(err).
			Msg("stripe.webhook.read_body_failed")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, fmt.Sprintf("read body: %v", err))
		return
	}

	event, err := h.stripe.ParseWebhook(r.Context(), body, signature)
	if err != nil {
		log.Warn().
			Err(err).
			Msg("stripe.webhook.invalid_signature")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidSignature, err.Error())
		return
	}

	log.Info().
		Str("event_type", event.Type).
		Msg("stripe.webhook.received")

	if event.Type == "checkout.session.completed" {
		if err := h.stripe.HandleCompletion(r.Context(), event); err != nil {
			// Record webhook processing failure
			webhookDuration := time.Since(webhookStart)
			if h.metrics != nil {
				h.metrics.ObserveWebhook("stripe", "failed", webhookDuration, 1, false)
			}
			apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, err.Error())
			return
		}

		// Record successful webhook processing
		webhookDuration := time.Since(webhookStart)
		if h.metrics != nil {
			h.metrics.ObserveWebhook("stripe", "success", webhookDuration, 1, false)
		}
	}

	responders.JSON(w, http.StatusOK, map[string]any{
		"received": true,
		"type":     event.Type,
	})
}

// stripeWebhookInfo provides information about the Stripe webhook endpoint.
func (h *handlers) stripeWebhookInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Stripe Webhook Endpoint</title>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 40rem; margin: 4rem auto; padding: 0 1.5rem; color: #1f2933; }
    h1 { color: #364fc7; }
    code { background: #f1f5f9; padding: 0.1rem 0.3rem; border-radius: 0.25rem; }
    ol { padding-left: 1.4rem; }
    li { margin-bottom: 0.5rem; }
  </style>
</head>
<body>
  <h1>Stripe Webhook Endpoint</h1>
  <p>This URL accepts <code>POST</code> requests from Stripe. For local testing:</p>
  <ol>
    <li>Set the webhook endpoint in the Stripe dashboard to <code>http://localhost:8080/webhook/stripe</code>.</li>
    <li>Install the Stripe CLI and run <code>stripe listen --forward-to localhost:8080/webhook/stripe</code> to relay events.</li>
    <li>Trigger a test event (e.g. <code>stripe trigger checkout.session.completed</code>) and check the Cedros logs for <code>checkout.session.completed</code>.</li>
  </ol>
  <p>If you see this page, the endpoint is reachable over HTTP. Only <code>POST</code> requests from Stripe will be processed.</p>
</body>
</html>`)
}

// stripeSuccess displays the payment completion page.
func (h *handlers) stripeSuccess(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if sessionID == "" {
		sessionID = "(missing)"
	}
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Payment Complete</title>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 36rem; margin: 4rem auto; padding: 0 1.5rem; color: #1f2933; }
    h1 { color: #0b7285; }
    code { background: #f1f5f9; padding: 0.1rem 0.3rem; border-radius: 0.25rem; }
    a { color: #0b7285; text-decoration: none; }
    a:hover { text-decoration: underline; }
  </style>
</head>
<body>
  <h1>Payment Complete</h1>
  <p>Thanks! Stripe confirmed your checkout session.</p>
  <p>Session ID: <code>%s</code></p>
  <p>You can safely close this tab and return to your app. If you meant to direct users elsewhere, update
  <code>stripe.success_url</code> in your Cedros config or pass <code>successUrl</code> when creating the session.</p>
</body>
</html>`, sessionID)
}

// stripeCancel displays the checkout cancellation page.
func (h *handlers) stripeCancel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Checkout Canceled</title>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 36rem; margin: 4rem auto; padding: 0 1.5rem; color: #1f2933; }
    h1 { color: #c92a2a; }
    a { color: #c92a2a; text-decoration: none; }
    a:hover { text-decoration: underline; }
  </style>
</head>
<body>
  <h1>Checkout Canceled</h1>
  <p>No payment was captured. Navigate back to your app and restart checkout whenever you're ready.</p>
  <p>Want a custom experience? Override <code>stripe.cancel_url</code> in your Cedros config or pass <code>cancelUrl</code> in the session request.</p>
</body>
</html>`)
}
