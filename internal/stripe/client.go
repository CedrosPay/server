package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	stripeapi "github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/promotioncode"
	"github.com/stripe/stripe-go/v72/webhook"

	"github.com/CedrosPay/server/internal/callbacks"
	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/metrics"
	"github.com/CedrosPay/server/internal/money"
	"github.com/CedrosPay/server/internal/storage"
)

// Client wraps stripe-go operations used by the server.
type Client struct {
	cfg     config.StripeConfig
	store   storage.Store
	notify  callbacks.Notifier
	coupons CouponRepository
	metrics *metrics.Metrics
}

// CouponRepository defines the minimal interface needed for coupon tracking.
type CouponRepository interface {
	IncrementUsage(ctx context.Context, code string) error
}

// NewClient sets up stripe-go with the provided credentials.
func NewClient(cfg config.StripeConfig, store storage.Store, notifier callbacks.Notifier, coupons CouponRepository, metricsCollector *metrics.Metrics) *Client {
	stripeapi.Key = cfg.SecretKey
	if notifier == nil {
		notifier = callbacks.NoopNotifier{}
	}
	return &Client{
		cfg:     cfg,
		store:   store,
		notify:  notifier,
		coupons: coupons,
		metrics: metricsCollector,
	}
}

// CreateSessionRequest captures checkout metadata.
type CreateSessionRequest struct {
	ResourceID     string
	AmountCents    int64
	Currency       string
	PriceID        string
	CustomerEmail  string
	Metadata       map[string]string
	SuccessURL     string
	CancelURL      string
	Description    string
	CouponCode     string // NEW: Coupon code applied (for metadata tracking)
	OriginalAmount int64  // NEW: Original price before discount (for metadata tracking)
	DiscountAmount int64  // NEW: Discount amount applied (for metadata tracking)
	StripeCouponID string // NEW: Optional Stripe coupon ID (if synced to Stripe)
}

// CreateCheckoutSession builds a Stripe Checkout session and persists minimal metadata.
func (c *Client) CreateCheckoutSession(ctx context.Context, req CreateSessionRequest) (*stripeapi.CheckoutSession, error) {
	metadata := convertMetadata(req.Metadata, req.ResourceID)

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

	params := &stripeapi.CheckoutSessionParams{
		Mode:               stripeapi.String(string(stripeapi.CheckoutSessionModePayment)),
		PaymentMethodTypes: stripeapi.StringSlice([]string{"card"}),
		SuccessURL:         stripeapi.String(firstNonEmpty(req.SuccessURL, c.cfg.SuccessURL)),
		CancelURL:          stripeapi.String(firstNonEmpty(req.CancelURL, c.cfg.CancelURL)),
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
	if req.PriceID != "" {
		params.LineItems = []*stripeapi.CheckoutSessionLineItemParams{
			{
				Price:    stripeapi.String(req.PriceID),
				Quantity: stripeapi.Int64(1),
			},
		}
	} else {
		if req.AmountCents <= 0 {
			return nil, errors.New("stripe: amount required when price id missing")
		}
		lineItem := &stripeapi.CheckoutSessionLineItemParams{
			Quantity: stripeapi.Int64(1),
			PriceData: &stripeapi.CheckoutSessionLineItemPriceDataParams{
				Currency: stripeapi.String(req.Currency),
				ProductData: &stripeapi.CheckoutSessionLineItemPriceDataProductDataParams{
					Name: stripeapi.String(req.Description),
				},
				UnitAmount: stripeapi.Int64(req.AmountCents),
			},
		}
		if c.cfg.TaxRateID != "" {
			lineItem.TaxRates = []*string{stripeapi.String(c.cfg.TaxRateID)}
		}
		params.LineItems = []*stripeapi.CheckoutSessionLineItemParams{lineItem}
	}

	s, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe: create checkout session: %w", err)
	}

	// Note: Session storage removed - rely on webhook callbacks for access control
	return s, nil
}

// WebhookEvent wraps the subset of event types we care about.
type WebhookEvent struct {
	Type        string
	SessionID   string
	ResourceID  string
	Customer    string
	Metadata    map[string]string
	AmountTotal int64
	Currency    string
}

// ParseWebhook validates event signatures and normalises the payload.
func (c *Client) ParseWebhook(ctx context.Context, payload []byte, signature string) (WebhookEvent, error) {
	if c.cfg.WebhookSecret == "" {
		return WebhookEvent{}, errors.New("stripe: webhook secret not configured")
	}
	event, err := webhook.ConstructEvent(payload, signature, c.cfg.WebhookSecret)
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("stripe: construct event: %w", err)
	}
	switch event.Type {
	case "checkout.session.completed":
		var checkout stripeapi.CheckoutSession
		if err := jsonExtract(event.Data.Raw, &checkout); err != nil {
			return WebhookEvent{}, err
		}

		// Extract resource ID with nil-safe metadata access
		resourceID := ""
		if checkout.Metadata != nil {
			resourceID = checkout.Metadata["resource_id"]
			if resourceID == "" {
				resourceID = checkout.Metadata["resourceId"]
			}
		}
		if resourceID == "" {
			return WebhookEvent{}, errors.New("stripe: webhook missing resource_id in metadata")
		}

		return WebhookEvent{
			Type:        event.Type,
			SessionID:   checkout.ID,
			ResourceID:  resourceID,
			Customer:    checkout.CustomerEmail,
			Metadata:    checkout.Metadata,
			AmountTotal: checkout.AmountTotal,
			Currency:    string(checkout.Currency),
		}, nil
	default:
		return WebhookEvent{
			Type: event.Type,
		}, nil
	}
}

// HandleCompletion handles webhook completion and triggers payment succeeded callback.
func (c *Client) HandleCompletion(ctx context.Context, event WebhookEvent) error {
	if event.SessionID == "" {
		return errors.New("stripe: completion missing session id")
	}
	now := time.Now()

	// Record Stripe payment in storage for access control
	asset, err := money.GetAsset(strings.ToUpper(event.Currency))
	if err != nil {
		return fmt.Errorf("stripe: unsupported currency %s: %w", event.Currency, err)
	}
	tx := storage.PaymentTransaction{
		Signature:  fmt.Sprintf("stripe:%s", event.SessionID),
		ResourceID: event.ResourceID,
		Wallet:     event.Customer,
		Amount:     money.New(asset, event.AmountTotal),
		CreatedAt:  now,
		Metadata: map[string]string{
			"status":     "stripe",
			"session_id": event.SessionID,
		},
	}
	if err := c.store.RecordPayment(ctx, tx); err != nil {
		if !strings.Contains(err.Error(), "signature already used") {
			return fmt.Errorf("stripe: record payment: %w", err)
		}
		return nil // duplicate webhook – already processed
	}

	// Increment coupon usage if a coupon was applied
	if couponCode := event.Metadata["coupon_code"]; couponCode != "" && c.coupons != nil {
		if err := c.coupons.IncrementUsage(ctx, couponCode); err != nil {
			// Log error but don't fail the webhook - payment was successful
			// Note: YAML coupons will return "read-only" error, which is expected
		}
	}

	c.notify.PaymentSucceeded(ctx, callbacks.PaymentEvent{
		ResourceID:      event.ResourceID,
		Method:          "stripe",
		StripeSessionID: event.SessionID,
		StripeCustomer:  event.Customer,
		FiatAmountCents: event.AmountTotal,
		FiatCurrency:    event.Currency,
		Metadata:        event.Metadata,
		PaidAt:          now.UTC(),
	})
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func convertMetadata(metadata map[string]string, resourceID string) map[string]string {
	out := make(map[string]string, len(metadata)+1)
	for k, v := range metadata {
		out[k] = v
	}
	if out["resource_id"] == "" {
		out["resource_id"] = resourceID
	}
	return out
}

func jsonExtract(data []byte, v any) error {
	if len(data) == 0 {
		return errors.New("stripe: webhook payload empty")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("stripe: decode webhook payload: %w", err)
	}
	return nil
}

// lookupPromotionCodeID retrieves the Stripe promotion code ID from a code string (e.g., "SAVE20" -> "promo_123")
func (c *Client) lookupPromotionCodeID(code string) (string, error) {
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
