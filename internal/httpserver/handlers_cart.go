package httpserver

import (
	"net/http"
	"time"

	"github.com/CedrosPay/server/internal/coupons"
	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	stripesvc "github.com/CedrosPay/server/internal/stripe"
	"github.com/CedrosPay/server/pkg/responders"
)

// createCartCheckoutRequest captures the multi-item cart checkout request.
type createCartCheckoutRequest struct {
	Items         []cartItemRequest `json:"items"`
	CustomerEmail string            `json:"customerEmail,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	SuccessURL    string            `json:"successUrl,omitempty"`
	CancelURL     string            `json:"cancelUrl,omitempty"`
	CouponCode    string            `json:"couponCode,omitempty"` // NEW: Optional coupon code
}

// cartItemRequest represents a single item in the cart.
type cartItemRequest struct {
	PriceID     string            `json:"priceId"`
	Resource    string            `json:"resource,omitempty"` // Optional: backend resource ID for indexing
	Quantity    int64             `json:"quantity"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// createCartCheckoutResponse contains the Stripe checkout session details.
type createCartCheckoutResponse struct {
	SessionID  string `json:"sessionId"`
	URL        string `json:"url"`
	TotalItems int    `json:"totalItems"`
}

// createCartCheckout creates a Stripe checkout session for multiple items.
// This is a NEW endpoint that doesn't modify existing single-item checkout behavior.
func (h *handlers) createCartCheckout(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Track cart Stripe session creation timing
	sessionStart := time.Now()

	var req createCartCheckoutRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().
			Err(err).
			Msg("cart.checkout.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, err.Error())
		return
	}

	// Validate request
	if len(req.Items) == 0 {
		log.Warn().
			Msg("cart.checkout.empty_cart")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeEmptyCart, "at least one item required in cart")
		return
	}

	// Convert to service request format and look up priceIds from resources
	var cartItems []stripesvc.CartLineItem
	for i, item := range req.Items {
		priceID := item.PriceID
		resourceID := item.Resource

		// If no priceID provided but resource is, look it up from resource definition
		if priceID == "" && resourceID != "" {
			resource, err := h.paywall.ResourceDefinition(r.Context(), resourceID)
			if err != nil {
				log.Warn().
					Err(err).
					Str("resource_id", resourceID).
					Int("item_index", i).
					Msg("cart.checkout.resource_not_found")
				apierrors.WriteError(w, apierrors.ErrCodeResourceNotFound, "resource not found: "+resourceID, map[string]interface{}{
					"item":       i,
					"resourceId": resourceID,
				})
				return
			}
			priceID = resource.StripePriceID
		}

		// If priceID provided but no resource, reverse-lookup the resource from priceID
		// This is critical for coupon validation - we must know which product each line represents
		if priceID != "" && resourceID == "" {
			resolvedResource, err := h.paywall.ResourceDefinitionByStripePriceID(r.Context(), priceID)
			if err != nil {
				// PriceID doesn't map to any known resource - reject when coupons are involved
				// Allow it to pass through otherwise (user may be using ad-hoc Stripe prices)
				if req.CouponCode != "" {
					apierrors.WriteError(w, apierrors.ErrCodeInvalidCartItem, "cannot apply coupon: priceId does not map to a known product", map[string]interface{}{
						"item":    i,
						"priceId": priceID,
						"hint":    "use resource field instead of priceId when applying coupons",
					})
					return
				}
			} else {
				// Successfully resolved priceID to resource
				resourceID = resolvedResource.ResourceID
			}
		}

		// Validate that we have a priceID by now
		if priceID == "" {
			apierrors.WriteError(w, apierrors.ErrCodeInvalidCartItem, "priceId or resource required for all items", map[string]interface{}{
				"item": i,
			})
			return
		}

		cartItems = append(cartItems, stripesvc.CartLineItem{
			PriceID:     priceID,
			Resource:    resourceID, // Now always populated if coupon is used
			Quantity:    item.Quantity,
			Description: item.Description,
			Metadata:    item.Metadata,
		})
	}

	// Validate coupon if provided (for metadata tracking)
	var couponCode string
	var stripeCouponID string

	if req.CouponCode != "" {
		// Validate against our internal coupon repository
		coupon, err := h.couponRepo.GetCoupon(r.Context(), req.CouponCode)
		if err == nil && coupon.IsValid() == nil && coupon.AppliesToPaymentMethod(coupons.PaymentMethodStripe) {
			// Verify the coupon applies to ALL products in the cart
			// SECURITY: All items MUST have a resolved resource ID when coupons are involved
			for i, item := range cartItems {
				if item.Resource == "" {
					// This should never happen due to reverse-lookup above, but defense in depth
					apierrors.WriteError(w, apierrors.ErrCodeInvalidCartItem, "cannot apply coupon: item does not map to a known product", map[string]interface{}{
						"item":   i,
						"coupon": req.CouponCode,
					})
					return
				}

				if !coupon.AppliesToProduct(item.Resource) {
					apierrors.WriteError(w, apierrors.ErrCodeCouponNotApplicable, "coupon does not apply to all items in cart", map[string]interface{}{
						"coupon":      req.CouponCode,
						"invalidItem": item.Resource,
						"couponScope": string(coupon.Scope),
					})
					return
				}
			}

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

	// Create cart checkout session
	session, err := h.cartService.CreateCartCheckoutSession(r.Context(), stripesvc.CreateCartSessionRequest{
		Items:          cartItems,
		CustomerEmail:  req.CustomerEmail,
		Metadata:       req.Metadata,
		SuccessURL:     req.SuccessURL,
		CancelURL:      req.CancelURL,
		CouponCode:     couponCode,     // For our internal tracking
		StripeCouponID: stripeCouponID, // For Stripe's discount system
		OriginalAmount: 0,              // Stripe calculates from items
		DiscountAmount: 0,              // Stripe calculates from promotion code
	})
	if err != nil {
		// Record failed cart Stripe session creation
		if h.metrics != nil {
			h.metrics.ObserveCartCheckout("session_creation_failed", len(req.Items))
		}
		log.Error().
			Err(err).
			Int("item_count", len(req.Items)).
			Str("coupon_code", couponCode).
			Msg("cart.checkout.session_failed")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeStripeError, err.Error())
		return
	}

	// Record successful cart Stripe session creation timing
	sessionDuration := time.Since(sessionStart)
	if h.metrics != nil {
		// Track cart checkout session creation (payment happens later in webhook)
		h.metrics.ObserveCartCheckout("session_created", len(req.Items))
		h.metrics.ObservePayment("stripe_cart", session.ID, false, sessionDuration, 0, "")
	}

	log.Info().
		Str("session_id", session.ID).
		Int("item_count", len(req.Items)).
		Str("coupon_code", couponCode).
		Str("customer_email", logger.RedactEmail(req.CustomerEmail)).
		Msg("cart.checkout.session_created")

	responders.JSON(w, http.StatusOK, createCartCheckoutResponse{
		SessionID:  session.ID,
		URL:        session.URL,
		TotalItems: len(req.Items),
	})
}
