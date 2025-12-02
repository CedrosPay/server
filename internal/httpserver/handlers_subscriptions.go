package httpserver

import (
	"fmt"
	"net/http"
	"time"

	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	stripesvc "github.com/CedrosPay/server/internal/stripe"
	"github.com/CedrosPay/server/internal/subscriptions"
	"github.com/CedrosPay/server/pkg/responders"
)

// createStripeSubscriptionRequest matches BACKEND_SUBSCRIPTION_API.md spec.
type createStripeSubscriptionRequest struct {
	Resource      string            `json:"resource"`      // Plan/resource ID (maps to productId)
	Interval      string            `json:"interval"`      // "weekly" | "monthly" | "yearly" | "custom"
	IntervalDays  int               `json:"intervalDays"`  // Only used when interval is "custom"
	TrialDays     int               `json:"trialDays"`     // Override product trial days
	CustomerEmail string            `json:"customerEmail"` // Pre-fills Stripe checkout
	Metadata      map[string]string `json:"metadata"`      // Metadata for tracking
	CouponCode    string            `json:"couponCode"`    // Coupon code for discount
	SuccessURL    string            `json:"successUrl"`    // Redirect URL on success
	CancelURL     string            `json:"cancelUrl"`     // Redirect URL on cancel
}

// createStripeSubscriptionResponse is the response for subscription checkout creation.
type createStripeSubscriptionResponse struct {
	SessionID string `json:"sessionId"`
	URL       string `json:"url"`
}

// createStripeSubscription creates a Stripe subscription checkout session.
// Matches BACKEND_SUBSCRIPTION_API.md: POST /paywall/v1/subscription/stripe-session
func (h *handlers) createStripeSubscription(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	var req createStripeSubscriptionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().Err(err).Msg("subscription.stripe.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	if req.Resource == "" {
		log.Warn().Msg("subscription.stripe.missing_resource")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "resource is required")
		return
	}

	if req.Interval == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "interval is required")
		return
	}

	// Get product and verify it's a subscription product
	product, err := h.paywall.GetProduct(r.Context(), req.Resource)
	if err != nil {
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeResourceNotFound, err.Error(), "resource", req.Resource)
		return
	}

	if !product.IsSubscription() {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "resource is not configured for subscriptions")
		return
	}

	// Get the Stripe price ID for the subscription
	stripePriceID := product.Subscription.StripePriceID
	if stripePriceID == "" {
		stripePriceID = product.StripePriceID // Fall back to product's price ID
	}
	if stripePriceID == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "resource has no Stripe price ID configured")
		return
	}

	// Build metadata
	metadata := make(map[string]string)
	for k, v := range product.Metadata {
		metadata[k] = v
	}
	for k, v := range req.Metadata {
		metadata[k] = v
	}
	metadata["resource"] = req.Resource
	metadata["interval"] = req.Interval

	// Determine trial days (request overrides product config)
	trialDays := product.Subscription.TrialDays
	if req.TrialDays > 0 {
		trialDays = req.TrialDays
	}

	// Create subscription checkout
	session, err := h.stripe.CreateSubscriptionCheckout(r.Context(), stripesvc.CreateSubscriptionRequest{
		ProductID:     req.Resource,
		PriceID:       stripePriceID,
		CustomerEmail: req.CustomerEmail,
		Metadata:      metadata,
		SuccessURL:    req.SuccessURL,
		CancelURL:     req.CancelURL,
		TrialDays:     trialDays,
	})
	if err != nil {
		log.Error().Err(err).Str("resource", req.Resource).Msg("subscription.stripe.checkout_failed")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeStripeError, err.Error())
		return
	}

	log.Info().
		Str("session_id", session.ID).
		Str("resource", req.Resource).
		Str("interval", req.Interval).
		Msg("subscription.stripe.checkout_created")

	responders.JSON(w, http.StatusOK, createStripeSubscriptionResponse{
		SessionID: session.ID,
		URL:       session.URL,
	})
}

// subscriptionStatusResponse matches BACKEND_SUBSCRIPTION_API.md spec.
type subscriptionStatusResponse struct {
	Active            bool    `json:"active"`                      // Required: Whether subscription is currently active
	Status            string  `json:"status"`                      // Required: "active" | "trialing" | "past_due" | "canceled" | "unpaid" | "expired"
	ExpiresAt         *string `json:"expiresAt,omitempty"`         // When subscription expires (ISO 8601)
	CurrentPeriodEnd  *string `json:"currentPeriodEnd,omitempty"`  // Current billing period end (ISO 8601)
	Interval          string  `json:"interval,omitempty"`          // Billing interval
	CancelAtPeriodEnd bool    `json:"cancelAtPeriodEnd,omitempty"` // Whether subscription will cancel at period end
}

// getSubscriptionStatus checks if a user has an active subscription.
// Matches BACKEND_SUBSCRIPTION_API.md: GET /paywall/v1/subscription/status
func (h *handlers) getSubscriptionStatus(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Frontend spec uses "resource" and "userId" query params
	resource := r.URL.Query().Get("resource")
	userID := r.URL.Query().Get("userId")

	if resource == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "resource is required")
		return
	}
	if userID == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "userId is required")
		return
	}

	// Check subscription repository
	if h.subscriptions == nil {
		// Subscriptions not enabled - return inactive
		responders.JSON(w, http.StatusOK, subscriptionStatusResponse{
			Active: false,
			Status: "expired",
		})
		return
	}

	hasAccess, sub, err := h.subscriptions.HasAccess(r.Context(), userID, resource)
	if err != nil {
		log.Error().Err(err).Msg("subscription.status.error")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "failed to check subscription status")
		return
	}

	if sub == nil {
		// No subscription found - return inactive per spec
		responders.JSON(w, http.StatusOK, subscriptionStatusResponse{
			Active: false,
			Status: "expired",
		})
		return
	}

	// Format times as ISO 8601
	var expiresAt, currentPeriodEnd *string
	if !sub.CurrentPeriodEnd.IsZero() {
		t := sub.CurrentPeriodEnd.UTC().Format(time.RFC3339)
		expiresAt = &t
		currentPeriodEnd = &t
	}

	// Map billing period to frontend interval
	interval := mapBillingPeriodToInterval(sub.BillingPeriod)

	responders.JSON(w, http.StatusOK, subscriptionStatusResponse{
		Active:            hasAccess,
		Status:            string(sub.Status),
		ExpiresAt:         expiresAt,
		CurrentPeriodEnd:  currentPeriodEnd,
		Interval:          interval,
		CancelAtPeriodEnd: sub.CancelAtPeriodEnd,
	})
}

// mapBillingPeriodToInterval converts our billing period to frontend interval string.
func mapBillingPeriodToInterval(period subscriptions.BillingPeriod) string {
	switch period {
	case subscriptions.PeriodDay:
		return "daily"
	case subscriptions.PeriodWeek:
		return "weekly"
	case subscriptions.PeriodMonth:
		return "monthly"
	case subscriptions.PeriodYear:
		return "yearly"
	default:
		return "custom"
	}
}

// subscriptionQuoteRequest matches BACKEND_SUBSCRIPTION_API.md spec.
type subscriptionQuoteRequest struct {
	Resource     string `json:"resource"`     // Plan/resource ID
	Interval     string `json:"interval"`     // "weekly" | "monthly" | "yearly" | "custom"
	CouponCode   string `json:"couponCode"`   // Coupon code for discount
	IntervalDays int    `json:"intervalDays"` // Only used when interval is "custom"
}

// subscriptionQuoteResponse matches BACKEND_SUBSCRIPTION_API.md spec (HTTP 402).
type subscriptionQuoteResponse struct {
	Requirement  interface{}                  `json:"requirement"`
	Subscription subscriptionQuotePeriodInfo `json:"subscription"`
}

type subscriptionQuotePeriodInfo struct {
	Interval        string `json:"interval"`
	IntervalDays    int    `json:"intervalDays,omitempty"`
	DurationSeconds int64  `json:"durationSeconds"`
	PeriodStart     string `json:"periodStart"`
	PeriodEnd       string `json:"periodEnd"`
}

// getSubscriptionQuote returns a payment quote for a crypto subscription.
// Matches BACKEND_SUBSCRIPTION_API.md: POST /paywall/v1/subscription/quote
func (h *handlers) getSubscriptionQuote(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	var req subscriptionQuoteRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().Err(err).Msg("subscription.quote.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	if req.Resource == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "resource is required")
		return
	}
	if req.Interval == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "interval is required")
		return
	}

	// Get the quote from paywall service
	quote, err := h.paywall.GenerateQuote(r.Context(), req.Resource, req.CouponCode)
	if err != nil {
		log.Error().Err(err).Str("resource", req.Resource).Msg("subscription.quote.failed")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeResourceNotFound, err.Error())
		return
	}

	// Calculate subscription period based on interval
	now := time.Now().UTC()
	var periodEnd time.Time
	var durationSeconds int64
	intervalDays := 0

	switch req.Interval {
	case "weekly":
		periodEnd = now.AddDate(0, 0, 7)
		durationSeconds = 7 * 24 * 60 * 60
	case "monthly":
		periodEnd = now.AddDate(0, 1, 0)
		durationSeconds = 30 * 24 * 60 * 60
	case "yearly":
		periodEnd = now.AddDate(1, 0, 0)
		durationSeconds = 365 * 24 * 60 * 60
	case "custom":
		if req.IntervalDays <= 0 {
			apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "intervalDays required for custom interval")
			return
		}
		periodEnd = now.AddDate(0, 0, req.IntervalDays)
		durationSeconds = int64(req.IntervalDays) * 24 * 60 * 60
		intervalDays = req.IntervalDays
	default:
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "invalid interval")
		return
	}

	// Build subscription quote response per spec (HTTP 402)
	response := subscriptionQuoteResponse{
		Requirement: quote.Crypto, // Use the x402 requirement from the quote
		Subscription: subscriptionQuotePeriodInfo{
			Interval:        req.Interval,
			IntervalDays:    intervalDays,
			DurationSeconds: durationSeconds,
			PeriodStart:     now.Format(time.RFC3339),
			PeriodEnd:       periodEnd.Format(time.RFC3339),
		},
	}

	log.Info().
		Str("resource", req.Resource).
		Str("interval", req.Interval).
		Msg("subscription.quote.generated")

	// Return 402 Payment Required with the quote (per x402 spec)
	responders.JSON(w, http.StatusPaymentRequired, response)
}

// createX402SubscriptionRequest is the request body for creating an x402 subscription.
type createX402SubscriptionRequest struct {
	ProductID        string            `json:"productId"`
	Wallet           string            `json:"wallet"`
	PaymentSignature string            `json:"paymentSignature"` // x402 payment signature (optional - if provided, verifies payment)
	Metadata         map[string]string `json:"metadata"`
}

// createX402SubscriptionResponse is the response for x402 subscription creation.
type createX402SubscriptionResponse struct {
	SubscriptionID     string    `json:"subscriptionId"`
	ProductID          string    `json:"productId"`
	Wallet             string    `json:"wallet"`
	Status             string    `json:"status"`
	CurrentPeriodStart time.Time `json:"currentPeriodStart"`
	CurrentPeriodEnd   time.Time `json:"currentPeriodEnd"`
	BillingPeriod      string    `json:"billingPeriod"`
	BillingInterval    int       `json:"billingInterval"`

	// Payment quote info - use existing payment flow
	Quote interface{} `json:"quote,omitempty"`
}

// createX402Subscription creates or renews an x402 subscription after payment verification.
func (h *handlers) createX402Subscription(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	var req createX402SubscriptionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().Err(err).Msg("subscription.x402.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	if req.ProductID == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "productId is required")
		return
	}
	if req.Wallet == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "wallet is required")
		return
	}

	// Check if subscriptions are enabled
	if h.subscriptions == nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "subscriptions not enabled")
		return
	}

	// Get product and verify it's a subscription product
	product, err := h.paywall.GetProduct(r.Context(), req.ProductID)
	if err != nil {
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeResourceNotFound, err.Error(), "productId", req.ProductID)
		return
	}

	if !product.IsSubscription() {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "product is not configured for subscriptions")
		return
	}

	if !product.Subscription.AllowX402 {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "product does not allow x402 subscriptions")
		return
	}

	// If payment signature provided, verify it was a successful payment for this product
	if req.PaymentSignature != "" {
		payment, err := h.paywall.GetPayment(r.Context(), req.PaymentSignature)
		if err != nil {
			log.Warn().
				Err(err).
				Str("signature", req.PaymentSignature).
				Msg("subscription.x402.payment_not_found")
			apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "payment signature not found or not yet processed")
			return
		}

		// Verify payment was for this product
		if payment.ResourceID != req.ProductID {
			log.Warn().
				Str("payment_resource", payment.ResourceID).
				Str("requested_product", req.ProductID).
				Msg("subscription.x402.payment_product_mismatch")
			apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "payment was for a different product")
			return
		}

		// Verify wallet matches (if wallet info is available in payment)
		if payment.Wallet != "" && payment.Wallet != req.Wallet {
			log.Warn().
				Str("payment_wallet", payment.Wallet).
				Str("requested_wallet", req.Wallet).
				Msg("subscription.x402.payment_wallet_mismatch")
			apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "payment was made from a different wallet")
			return
		}
	}

	// Create or extend subscription
	sub, err := h.subscriptions.CreateX402Subscription(r.Context(), subscriptions.CreateX402SubscriptionRequest{
		ProductID:       req.ProductID,
		Wallet:          req.Wallet,
		BillingPeriod:   subscriptions.BillingPeriod(product.Subscription.BillingPeriod),
		BillingInterval: product.Subscription.BillingInterval,
		Metadata:        req.Metadata,
	})
	if err != nil {
		log.Error().Err(err).Str("product_id", req.ProductID).Msg("subscription.x402.create_failed")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, fmt.Sprintf("failed to create subscription: %v", err))
		return
	}

	log.Info().
		Str("subscription_id", sub.ID).
		Str("product_id", req.ProductID).
		Str("wallet", logger.TruncateAddress(req.Wallet)).
		Msg("subscription.x402.created")

	responders.JSON(w, http.StatusOK, createX402SubscriptionResponse{
		SubscriptionID:     sub.ID,
		ProductID:          sub.ProductID,
		Wallet:             sub.Wallet,
		Status:             string(sub.Status),
		CurrentPeriodStart: sub.CurrentPeriodStart,
		CurrentPeriodEnd:   sub.CurrentPeriodEnd,
		BillingPeriod:      string(sub.BillingPeriod),
		BillingInterval:    sub.BillingInterval,
	})
}

// Note: Cancel, portal, change, and reactivate handlers are in handlers_subscription_mgmt.go
