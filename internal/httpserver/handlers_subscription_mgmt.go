package httpserver

import (
	"net/http"
	"time"

	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	stripesvc "github.com/CedrosPay/server/internal/stripe"
	"github.com/CedrosPay/server/internal/subscriptions"
	"github.com/CedrosPay/server/pkg/responders"
)

// cancelSubscriptionRequest is the request body for cancelling a subscription.
type cancelSubscriptionRequest struct {
	SubscriptionID string `json:"subscriptionId"`
	AtPeriodEnd    bool   `json:"atPeriodEnd"` // If true, cancel at end of current period
}

// cancelSubscription cancels a subscription.
func (h *handlers) cancelSubscription(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	var req cancelSubscriptionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().Err(err).Msg("subscription.cancel.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	if req.SubscriptionID == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "subscriptionId is required")
		return
	}

	if h.subscriptions == nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "subscriptions not enabled")
		return
	}

	// Get subscription to determine type
	sub, err := h.subscriptions.Get(r.Context(), req.SubscriptionID)
	if err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeResourceNotFound, "subscription not found")
		return
	}

	// If Stripe subscription, cancel via Stripe API
	if sub.PaymentMethod == subscriptions.PaymentMethodStripe && sub.StripeSubscriptionID != "" {
		if err := h.stripe.CancelSubscription(r.Context(), sub.StripeSubscriptionID, req.AtPeriodEnd); err != nil {
			log.Error().Err(err).Msg("subscription.cancel.stripe_error")
			apierrors.WriteSimpleError(w, apierrors.ErrCodeStripeError, err.Error())
			return
		}
	}

	// Cancel in our database
	if err := h.subscriptions.Cancel(r.Context(), req.SubscriptionID, req.AtPeriodEnd); err != nil {
		log.Error().Err(err).Msg("subscription.cancel.error")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "failed to cancel subscription")
		return
	}

	log.Info().
		Str("subscription_id", req.SubscriptionID).
		Bool("at_period_end", req.AtPeriodEnd).
		Msg("subscription.cancelled")

	responders.JSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"atPeriodEnd": req.AtPeriodEnd,
	})
}

// getBillingPortalRequest is the request for getting a billing portal URL.
type getBillingPortalRequest struct {
	CustomerID string `json:"customerId"`
	ReturnURL  string `json:"returnUrl"`
}

// getBillingPortal returns a Stripe billing portal URL for self-service.
func (h *handlers) getBillingPortal(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	var req getBillingPortalRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().Err(err).Msg("subscription.portal.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	if req.CustomerID == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "customerId is required")
		return
	}
	if req.ReturnURL == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "returnUrl is required")
		return
	}

	session, err := h.stripe.CreateBillingPortalSession(r.Context(), req.CustomerID, req.ReturnURL)
	if err != nil {
		log.Error().Err(err).Msg("subscription.portal.error")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeStripeError, err.Error())
		return
	}

	responders.JSON(w, http.StatusOK, map[string]any{
		"url": session.URL,
	})
}

// changeSubscriptionRequest is the request body for upgrading/downgrading a subscription.
type changeSubscriptionRequest struct {
	SubscriptionID    string `json:"subscriptionId"`    // ID of existing subscription
	NewResource       string `json:"newResource"`       // New plan/resource ID
	ProrationBehavior string `json:"prorationBehavior"` // "create_prorations" (default), "none", "always_invoice"
}

// changeSubscriptionResponse is the response for plan changes.
type changeSubscriptionResponse struct {
	Success           bool   `json:"success"`
	SubscriptionID    string `json:"subscriptionId"`
	PreviousResource  string `json:"previousResource"`
	NewResource       string `json:"newResource"`
	Status            string `json:"status"`
	CurrentPeriodEnd  string `json:"currentPeriodEnd,omitempty"`
	ProrationBehavior string `json:"prorationBehavior"`
}

// changeSubscription handles subscription upgrades and downgrades.
// POST /paywall/v1/subscription/change
func (h *handlers) changeSubscription(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	var req changeSubscriptionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().Err(err).Msg("subscription.change.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	if req.SubscriptionID == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "subscriptionId is required")
		return
	}
	if req.NewResource == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "newResource is required")
		return
	}

	if h.subscriptions == nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "subscriptions not enabled")
		return
	}

	// Get the existing subscription
	sub, err := h.subscriptions.Get(r.Context(), req.SubscriptionID)
	if err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeResourceNotFound, "subscription not found")
		return
	}

	// Get the new product to validate it exists and get its Stripe price ID
	newProduct, err := h.paywall.GetProduct(r.Context(), req.NewResource)
	if err != nil {
		apierrors.WriteErrorWithDetail(w, apierrors.ErrCodeResourceNotFound, "new resource not found", "newResource", req.NewResource)
		return
	}

	if !newProduct.IsSubscription() {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "new resource is not a subscription product")
		return
	}

	// Store previous product for response
	previousResource := sub.ProductID

	// Default proration behavior
	prorationBehavior := req.ProrationBehavior
	if prorationBehavior == "" {
		prorationBehavior = "create_prorations"
	}

	// Handle based on payment method
	if sub.PaymentMethod == subscriptions.PaymentMethodStripe && sub.StripeSubscriptionID != "" {
		// Get Stripe price ID for the new product
		newPriceID := newProduct.Subscription.StripePriceID
		if newPriceID == "" {
			newPriceID = newProduct.StripePriceID
		}
		if newPriceID == "" {
			apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "new resource has no Stripe price ID")
			return
		}

		// Update the Stripe subscription
		_, err := h.stripe.UpdateSubscription(r.Context(), stripesvc.UpdateSubscriptionRequest{
			SubscriptionID:    sub.StripeSubscriptionID,
			NewPriceID:        newPriceID,
			ProrationBehavior: prorationBehavior,
			Metadata: map[string]string{
				"previous_resource": previousResource,
				"new_resource":      req.NewResource,
			},
		})
		if err != nil {
			log.Error().Err(err).Str("subscription_id", req.SubscriptionID).Msg("subscription.change.stripe_error")
			apierrors.WriteSimpleError(w, apierrors.ErrCodeStripeError, err.Error())
			return
		}
	}

	// Update our local subscription record
	result, err := h.subscriptions.ChangeSubscription(r.Context(), subscriptions.ChangeSubscriptionRequest{
		SubscriptionID:     req.SubscriptionID,
		NewProductID:       req.NewResource,
		NewBillingPeriod:   subscriptions.BillingPeriod(newProduct.Subscription.BillingPeriod),
		NewBillingInterval: newProduct.Subscription.BillingInterval,
	})
	if err != nil {
		log.Error().Err(err).Str("subscription_id", req.SubscriptionID).Msg("subscription.change.error")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "failed to update subscription")
		return
	}

	log.Info().
		Str("subscription_id", req.SubscriptionID).
		Str("previous_resource", previousResource).
		Str("new_resource", req.NewResource).
		Str("proration_behavior", prorationBehavior).
		Msg("subscription.changed")

	// Format period end
	var currentPeriodEnd string
	if !result.Subscription.CurrentPeriodEnd.IsZero() {
		currentPeriodEnd = result.Subscription.CurrentPeriodEnd.UTC().Format(time.RFC3339)
	}

	responders.JSON(w, http.StatusOK, changeSubscriptionResponse{
		Success:           true,
		SubscriptionID:    result.Subscription.ID,
		PreviousResource:  previousResource,
		NewResource:       result.Subscription.ProductID,
		Status:            string(result.Subscription.Status),
		CurrentPeriodEnd:  currentPeriodEnd,
		ProrationBehavior: prorationBehavior,
	})
}

// reactivateSubscriptionRequest is the request body for reactivating a subscription.
type reactivateSubscriptionRequest struct {
	SubscriptionID string `json:"subscriptionId"`
}

// reactivateSubscription reactivates a subscription that was scheduled for cancellation.
// POST /paywall/v1/subscription/reactivate
func (h *handlers) reactivateSubscription(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	var req reactivateSubscriptionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().Err(err).Msg("subscription.reactivate.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	if req.SubscriptionID == "" {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeMissingField, "subscriptionId is required")
		return
	}

	if h.subscriptions == nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "subscriptions not enabled")
		return
	}

	// Get the subscription first
	sub, err := h.subscriptions.Get(r.Context(), req.SubscriptionID)
	if err != nil {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeResourceNotFound, "subscription not found")
		return
	}

	// Check if it's actually scheduled for cancellation
	if !sub.CancelAtPeriodEnd {
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "subscription is not scheduled for cancellation")
		return
	}

	// For Stripe subscriptions, reactivate via Stripe API first
	if sub.PaymentMethod == subscriptions.PaymentMethodStripe && sub.StripeSubscriptionID != "" {
		_, err := h.stripe.ReactivateSubscription(r.Context(), sub.StripeSubscriptionID)
		if err != nil {
			log.Error().Err(err).Str("subscription_id", req.SubscriptionID).Msg("subscription.reactivate.stripe_error")
			apierrors.WriteSimpleError(w, apierrors.ErrCodeStripeError, err.Error())
			return
		}
	}

	// Reactivate in our database
	reactivatedSub, err := h.subscriptions.ReactivateSubscription(r.Context(), req.SubscriptionID)
	if err != nil {
		log.Error().Err(err).Str("subscription_id", req.SubscriptionID).Msg("subscription.reactivate.error")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, err.Error())
		return
	}

	log.Info().
		Str("subscription_id", req.SubscriptionID).
		Msg("subscription.reactivated")

	// Format period end
	var currentPeriodEnd *string
	if !reactivatedSub.CurrentPeriodEnd.IsZero() {
		t := reactivatedSub.CurrentPeriodEnd.UTC().Format(time.RFC3339)
		currentPeriodEnd = &t
	}

	responders.JSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"subscriptionId":    reactivatedSub.ID,
		"status":            string(reactivatedSub.Status),
		"cancelAtPeriodEnd": reactivatedSub.CancelAtPeriodEnd,
		"currentPeriodEnd":  currentPeriodEnd,
	})
}
