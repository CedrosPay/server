package stripe

import (
	"context"
	"errors"
	"fmt"

	stripeapi "github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/promotioncode"

	"github.com/CedrosPay/server/internal/callbacks"
	"github.com/CedrosPay/server/internal/metrics"
	"github.com/CedrosPay/server/internal/storage"
)

// CartLineItem represents a single item in the cart with quantity.
type CartLineItem struct {
	PriceID     string            `json:"priceId"`
	Resource    string            `json:"resource,omitempty"` // Optional: backend resource ID for indexing
	Quantity    int64             `json:"quantity"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// CreateCartSessionRequest captures multi-item checkout metadata.
type CreateCartSessionRequest struct {
	Items          []CartLineItem    `json:"items"`
	CustomerEmail  string            `json:"customerEmail,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	SuccessURL     string            `json:"successUrl,omitempty"`
	CancelURL      string            `json:"cancelUrl,omitempty"`
	CouponCode     string            `json:"couponCode,omitempty"`     // NEW: Coupon code applied
	StripeCouponID string            `json:"stripeCouponId,omitempty"` // NEW: Optional Stripe coupon ID
	OriginalAmount int64             `json:"originalAmount,omitempty"` // NEW: Original total before discount
	DiscountAmount int64             `json:"discountAmount,omitempty"` // NEW: Total discount applied
}

// CartService handles multi-item Stripe checkout sessions.
// This is a separate service that extends (not modifies) the base Stripe client.
type CartService struct {
	cfg     stripeConfig
	store   storage.Store
	notify  callbacks.Notifier
	coupons CouponRepository
	metrics *metrics.Metrics
}

// stripeConfig defines the subset of config needed for cart operations.
type stripeConfig interface {
	GetSecretKey() string
	GetSuccessURL() string
	GetCancelURL() string
	GetTaxRateID() string
}

// NewCartService creates a cart service for multi-item checkouts.
func NewCartService(cfg stripeConfig, store storage.Store, notifier callbacks.Notifier, coupons CouponRepository, metricsCollector *metrics.Metrics) *CartService {
	if notifier == nil {
		notifier = callbacks.NoopNotifier{}
	}
	return &CartService{
		cfg:     cfg,
		store:   store,
		notify:  notifier,
		coupons: coupons,
		metrics: metricsCollector,
	}
}

// CreateCartCheckoutSession creates a Stripe checkout session with multiple line items.
// This is separate from the existing CreateCheckoutSession to avoid breaking existing code.
func (c *CartService) CreateCartCheckoutSession(ctx context.Context, req CreateCartSessionRequest) (*stripeapi.CheckoutSession, error) {
	// Validate request
	if len(req.Items) == 0 {
		return nil, errors.New("stripe cart: at least one item required")
	}

	// Build line items from cart items
	var lineItems []*stripeapi.CheckoutSessionLineItemParams
	var totalItems int64

	for _, item := range req.Items {
		if item.PriceID == "" {
			return nil, errors.New("stripe cart: priceId required for all items")
		}
		if item.Quantity <= 0 {
			item.Quantity = 1 // Default to 1 if not specified
		}

		lineItem := &stripeapi.CheckoutSessionLineItemParams{
			Price:    stripeapi.String(item.PriceID),
			Quantity: stripeapi.Int64(item.Quantity),
		}

		// Add tax rate if configured
		if c.cfg.GetTaxRateID() != "" {
			lineItem.TaxRates = []*string{stripeapi.String(c.cfg.GetTaxRateID())}
		}

		lineItems = append(lineItems, lineItem)
		totalItems += item.Quantity
	}

	// Build session metadata
	metadata := make(map[string]string, len(req.Metadata)+2)
	for k, v := range req.Metadata {
		metadata[k] = v
	}
	metadata["cart_items"] = fmt.Sprintf("%d", len(req.Items))
	metadata["total_quantity"] = fmt.Sprintf("%d", totalItems)

	// Add coupon tracking to metadata
	if req.CouponCode != "" {
		metadata["coupon_code"] = req.CouponCode
	}
	if req.OriginalAmount > 0 {
		metadata["original_amount_cents"] = fmt.Sprintf("%d", req.OriginalAmount)
	}
	if req.DiscountAmount > 0 {
		metadata["discount_amount_cents"] = fmt.Sprintf("%d", req.DiscountAmount)
	}

	// Store item details in metadata for callback/webhook processing
	// This allows the callback to know exactly what was purchased
	// Format: item_0_price_id, item_0_resource, item_0_quantity, item_0_description, etc.
	for i, item := range req.Items {
		prefix := fmt.Sprintf("item_%d_", i)
		metadata[prefix+"price_id"] = item.PriceID
		if item.Resource != "" {
			metadata[prefix+"resource"] = item.Resource
		}
		metadata[prefix+"quantity"] = fmt.Sprintf("%d", item.Quantity)
		if item.Description != "" {
			metadata[prefix+"description"] = item.Description
		}
		// Store per-item metadata too
		for k, v := range item.Metadata {
			metadata[prefix+k] = v
		}
	}

	// Create checkout session parameters
	params := &stripeapi.CheckoutSessionParams{
		Mode:               stripeapi.String(string(stripeapi.CheckoutSessionModePayment)),
		PaymentMethodTypes: stripeapi.StringSlice([]string{"card"}),
		SuccessURL:         stripeapi.String(firstNonEmpty(req.SuccessURL, c.cfg.GetSuccessURL())),
		CancelURL:          stripeapi.String(firstNonEmpty(req.CancelURL, c.cfg.GetCancelURL())),
		LineItems:          lineItems,
	}
	params.Metadata = metadata

	// Apply Stripe promotion code if provided
	// This uses Stripe's native discount system with promotion codes created in Stripe Dashboard
	if req.StripeCouponID != "" {
		// Look up the promotion code ID by the code string (e.g., "SAVE20" -> "promo_123")
		promoCodeID, err := c.lookupPromotionCodeID(req.StripeCouponID)
		if err == nil && promoCodeID != "" {
			params.Discounts = []*stripeapi.CheckoutSessionDiscountParams{
				{
					PromotionCode: stripeapi.String(promoCodeID),
				},
			}
		}
		// If lookup fails, silently ignore (coupon won't be applied in Stripe)
	}

	if req.CustomerEmail != "" {
		params.CustomerEmail = stripeapi.String(req.CustomerEmail)
	}

	// Create the session with Stripe
	s, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe cart: create checkout session: %w", err)
	}

	// Note: Session storage removed - rely on webhook callbacks for access control

	return s, nil
}

// lookupPromotionCodeID retrieves the Stripe promotion code ID from a code string (e.g., "SAVE20" -> "promo_123")
func (c *CartService) lookupPromotionCodeID(code string) (string, error) {
	params := &stripeapi.PromotionCodeListParams{
		Code: stripeapi.String(code),
	}
	params.Filters.AddFilter("limit", "", "1")
	params.Filters.AddFilter("active", "", "true")

	iter := promotioncode.List(params)
	if iter.Next() {
		return iter.PromotionCode().ID, nil
	}
	if err := iter.Err(); err != nil {
		return "", err
	}
	return "", errors.New("promotion code not found")
}
