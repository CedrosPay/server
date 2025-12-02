package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	stripeapi "github.com/stripe/stripe-go/v72"
	portalsession "github.com/stripe/stripe-go/v72/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v72/checkout/session"
	stripesub "github.com/stripe/stripe-go/v72/sub"

	"github.com/CedrosPay/server/internal/subscriptions"
)

// SubscriptionRepository provides access to subscription storage.
type SubscriptionRepository interface {
	GetByStripeSubscriptionID(ctx context.Context, stripeSubID string) (subscriptions.Subscription, error)
	Create(ctx context.Context, sub subscriptions.Subscription) error
	Update(ctx context.Context, sub subscriptions.Subscription) error
	UpdateStatus(ctx context.Context, id string, status subscriptions.Status) error
	ExtendPeriod(ctx context.Context, id string, newStart, newEnd time.Time) error
}

// CreateSubscriptionRequest contains parameters for creating a subscription checkout.
type CreateSubscriptionRequest struct {
	ProductID     string
	PriceID       string            // Stripe recurring price ID
	CustomerEmail string
	Metadata      map[string]string
	SuccessURL    string
	CancelURL     string
	TrialDays     int
}

// CreateSubscriptionCheckout creates a Stripe Checkout session for a subscription.
func (c *Client) CreateSubscriptionCheckout(ctx context.Context, req CreateSubscriptionRequest) (*stripeapi.CheckoutSession, error) {
	if req.PriceID == "" {
		return nil, errors.New("stripe: subscription price_id is required")
	}

	metadata := convertMetadata(req.Metadata, req.ProductID)
	metadata["subscription"] = "true"

	params := &stripeapi.CheckoutSessionParams{
		Mode:               stripeapi.String(string(stripeapi.CheckoutSessionModeSubscription)),
		PaymentMethodTypes: stripeapi.StringSlice([]string{"card"}),
		SuccessURL:         stripeapi.String(firstNonEmpty(req.SuccessURL, c.cfg.SuccessURL)),
		CancelURL:          stripeapi.String(firstNonEmpty(req.CancelURL, c.cfg.CancelURL)),
		LineItems: []*stripeapi.CheckoutSessionLineItemParams{
			{
				Price:    stripeapi.String(req.PriceID),
				Quantity: stripeapi.Int64(1),
			},
		},
	}
	params.Metadata = metadata

	if req.CustomerEmail != "" {
		params.CustomerEmail = stripeapi.String(req.CustomerEmail)
	}

	// Add trial period if configured
	if req.TrialDays > 0 {
		params.SubscriptionData = &stripeapi.CheckoutSessionSubscriptionDataParams{
			TrialPeriodDays: stripeapi.Int64(int64(req.TrialDays)),
		}
	}

	s, err := checkoutsession.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe: create subscription checkout: %w", err)
	}

	return s, nil
}

// CancelSubscription cancels a Stripe subscription.
func (c *Client) CancelSubscription(ctx context.Context, stripeSubID string, atPeriodEnd bool) error {
	if atPeriodEnd {
		// Cancel at end of billing period
		params := &stripeapi.SubscriptionParams{
			CancelAtPeriodEnd: stripeapi.Bool(true),
		}
		_, err := stripesub.Update(stripeSubID, params)
		if err != nil {
			return fmt.Errorf("stripe: cancel subscription: %w", err)
		}
		return nil
	}

	// Cancel immediately
	_, err := stripesub.Cancel(stripeSubID, nil)
	if err != nil {
		return fmt.Errorf("stripe: cancel subscription: %w", err)
	}
	return nil
}

// GetSubscription retrieves a Stripe subscription.
func (c *Client) GetSubscription(ctx context.Context, stripeSubID string) (*stripeapi.Subscription, error) {
	sub, err := stripesub.Get(stripeSubID, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe: get subscription: %w", err)
	}
	return sub, nil
}

// UpdateSubscriptionRequest contains parameters for updating a Stripe subscription.
type UpdateSubscriptionRequest struct {
	SubscriptionID    string
	NewPriceID        string // New price ID for plan change
	ProrationBehavior string // "create_prorations", "none", "always_invoice"
	Metadata          map[string]string
}

// UpdateSubscriptionResult contains the result of a subscription update.
type UpdateSubscriptionResult struct {
	Subscription      *stripeapi.Subscription
	ProrationAmount   int64  // Proration amount in cents (positive = charge, negative = credit)
	EffectiveDate     int64  // Unix timestamp of when change takes effect
}

// UpdateSubscription updates a Stripe subscription (for plan changes/upgrades/downgrades).
func (c *Client) UpdateSubscription(ctx context.Context, req UpdateSubscriptionRequest) (*UpdateSubscriptionResult, error) {
	if req.SubscriptionID == "" {
		return nil, errors.New("stripe: subscription_id is required")
	}

	// First, get the current subscription to find the subscription item ID
	currentSub, err := stripesub.Get(req.SubscriptionID, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe: get current subscription: %w", err)
	}

	// Get the first item (assuming single-item subscriptions)
	if currentSub.Items == nil || len(currentSub.Items.Data) == 0 {
		return nil, errors.New("stripe: subscription has no items")
	}
	itemID := currentSub.Items.Data[0].ID

	// Build update params
	params := &stripeapi.SubscriptionParams{
		Items: []*stripeapi.SubscriptionItemsParams{
			{
				ID:    stripeapi.String(itemID),
				Price: stripeapi.String(req.NewPriceID),
			},
		},
	}

	// Set proration behavior
	switch req.ProrationBehavior {
	case "none":
		params.ProrationBehavior = stripeapi.String(string(stripeapi.SubscriptionProrationBehaviorNone))
	case "always_invoice":
		params.ProrationBehavior = stripeapi.String(string(stripeapi.SubscriptionProrationBehaviorAlwaysInvoice))
	default:
		// Default to create_prorations
		params.ProrationBehavior = stripeapi.String(string(stripeapi.SubscriptionProrationBehaviorCreateProrations))
	}

	// Add metadata if provided
	if req.Metadata != nil {
		params.Metadata = make(map[string]string)
		for k, v := range req.Metadata {
			params.Metadata[k] = v
		}
	}

	// Update the subscription
	updatedSub, err := stripesub.Update(req.SubscriptionID, params)
	if err != nil {
		return nil, fmt.Errorf("stripe: update subscription: %w", err)
	}

	result := &UpdateSubscriptionResult{
		Subscription:  updatedSub,
		EffectiveDate: updatedSub.CurrentPeriodStart,
	}

	return result, nil
}

// PreviewProration calculates the proration amount for a plan change without applying it.
func (c *Client) PreviewProration(ctx context.Context, subscriptionID, newPriceID string) (*ProrationPreview, error) {
	// Get current subscription
	currentSub, err := stripesub.Get(subscriptionID, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe: get subscription: %w", err)
	}

	if currentSub.Items == nil || len(currentSub.Items.Data) == 0 {
		return nil, errors.New("stripe: subscription has no items")
	}
	itemID := currentSub.Items.Data[0].ID

	// Create an invoice preview to see what the proration would be
	// Note: This uses Stripe's upcoming invoice API
	params := &stripeapi.InvoiceParams{
		Customer:     stripeapi.String(currentSub.Customer.ID),
		Subscription: stripeapi.String(subscriptionID),
		SubscriptionItems: []*stripeapi.SubscriptionItemsParams{
			{
				ID:    stripeapi.String(itemID),
				Price: stripeapi.String(newPriceID),
			},
		},
		SubscriptionProrationBehavior: stripeapi.String(string(stripeapi.SubscriptionProrationBehaviorCreateProrations)),
	}

	// Get upcoming invoice preview
	invoice, err := c.previewInvoice(params)
	if err != nil {
		return nil, fmt.Errorf("stripe: preview invoice: %w", err)
	}

	// Calculate proration from invoice lines
	var prorationAmount int64
	for _, line := range invoice.Lines.Data {
		if line.Proration {
			prorationAmount += line.Amount
		}
	}

	return &ProrationPreview{
		ProrationAmount: prorationAmount,
		Currency:        string(invoice.Currency),
		EffectiveDate:   time.Now(),
		InvoiceTotal:    invoice.Total,
	}, nil
}

// previewInvoice creates an invoice preview (helper method).
// Note: This method is a placeholder - proration preview requires the
// invoice.upcoming API which needs specific SDK handling.
func (c *Client) previewInvoice(_ *stripeapi.InvoiceParams) (*stripeapi.Invoice, error) {
	// Note: For actual preview, we'd use stripe.Invoice.Upcoming
	// but the v72 SDK requires a different approach
	// This is a simplified version that creates a preview
	return nil, errors.New("stripe: invoice preview not implemented in this SDK version - use UpdateSubscription with create_prorations")
}

// ProrationPreview contains the preview of proration for a plan change.
type ProrationPreview struct {
	ProrationAmount int64     // Amount in smallest currency unit (cents for USD)
	Currency        string    // Currency code (e.g., "usd")
	EffectiveDate   time.Time // When the change would take effect
	InvoiceTotal    int64     // Total invoice amount after proration
}

// ReactivateSubscription reactivates a cancelled Stripe subscription (if still within period).
func (c *Client) ReactivateSubscription(ctx context.Context, stripeSubID string) (*stripeapi.Subscription, error) {
	params := &stripeapi.SubscriptionParams{
		CancelAtPeriodEnd: stripeapi.Bool(false),
	}

	sub, err := stripesub.Update(stripeSubID, params)
	if err != nil {
		return nil, fmt.Errorf("stripe: reactivate subscription: %w", err)
	}

	return sub, nil
}

// CreateBillingPortalSession creates a Stripe billing portal session for self-service.
func (c *Client) CreateBillingPortalSession(ctx context.Context, customerID, returnURL string) (*stripeapi.BillingPortalSession, error) {
	params := &stripeapi.BillingPortalSessionParams{
		Customer:  stripeapi.String(customerID),
		ReturnURL: stripeapi.String(returnURL),
	}

	s, err := portalsession.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe: create billing portal session: %w", err)
	}
	return s, nil
}

// SubscriptionWebhookEvent represents a parsed subscription webhook event.
type SubscriptionWebhookEvent struct {
	Type                 string
	StripeSubscriptionID string
	StripeCustomerID     string
	ProductID            string
	Status               string
	CurrentPeriodStart   time.Time
	CurrentPeriodEnd     time.Time
	CancelAtPeriodEnd    bool
	CancelledAt          *time.Time
	Metadata             map[string]string
	// Plan change fields
	PriceID         string // Current price ID
	BillingPeriod   string // e.g., "month", "year"
	BillingInterval int    // Interval count
}

// ParseSubscriptionWebhook parses subscription-related webhook events.
func (c *Client) ParseSubscriptionWebhook(payload []byte, eventType string) (*SubscriptionWebhookEvent, error) {
	switch eventType {
	case "customer.subscription.created",
		"customer.subscription.updated",
		"customer.subscription.deleted":
		var sub stripeapi.Subscription
		if err := json.Unmarshal(payload, &sub); err != nil {
			return nil, fmt.Errorf("stripe: parse subscription: %w", err)
		}

		// Extract product/resource ID from metadata
		productID := ""
		if sub.Metadata != nil {
			productID = sub.Metadata["resource_id"]
			if productID == "" {
				productID = sub.Metadata["product_id"]
			}
			// Check for new resource from plan change
			if sub.Metadata["new_resource"] != "" {
				productID = sub.Metadata["new_resource"]
			}
		}

		// Extract cancelled time
		var cancelledAt *time.Time
		if sub.CanceledAt > 0 {
			t := time.Unix(sub.CanceledAt, 0)
			cancelledAt = &t
		}

		// Extract price and billing info from subscription items
		var priceID, billingPeriod string
		var billingInterval int
		if sub.Items != nil && len(sub.Items.Data) > 0 {
			item := sub.Items.Data[0]
			if item.Price != nil {
				priceID = item.Price.ID
				if item.Price.Recurring != nil {
					billingPeriod = string(item.Price.Recurring.Interval)
					billingInterval = int(item.Price.Recurring.IntervalCount)
				}
			}
		}

		return &SubscriptionWebhookEvent{
			Type:                 eventType,
			StripeSubscriptionID: sub.ID,
			StripeCustomerID:     sub.Customer.ID,
			ProductID:            productID,
			Status:               string(sub.Status),
			CurrentPeriodStart:   time.Unix(sub.CurrentPeriodStart, 0),
			CurrentPeriodEnd:     time.Unix(sub.CurrentPeriodEnd, 0),
			CancelAtPeriodEnd:    sub.CancelAtPeriodEnd,
			CancelledAt:          cancelledAt,
			Metadata:             sub.Metadata,
			PriceID:              priceID,
			BillingPeriod:        billingPeriod,
			BillingInterval:      billingInterval,
		}, nil

	case "invoice.paid", "invoice.payment_failed":
		var invoice stripeapi.Invoice
		if err := json.Unmarshal(payload, &invoice); err != nil {
			return nil, fmt.Errorf("stripe: parse invoice: %w", err)
		}

		// Get subscription ID from invoice
		stripeSubID := ""
		if invoice.Subscription != nil {
			stripeSubID = invoice.Subscription.ID
		}

		return &SubscriptionWebhookEvent{
			Type:                 eventType,
			StripeSubscriptionID: stripeSubID,
			StripeCustomerID:     invoice.Customer.ID,
		}, nil

	case "checkout.session.completed":
		var checkout stripeapi.CheckoutSession
		if err := json.Unmarshal(payload, &checkout); err != nil {
			return nil, fmt.Errorf("stripe: parse checkout session: %w", err)
		}

		// Only process subscription checkouts
		if checkout.Mode != stripeapi.CheckoutSessionModeSubscription {
			return nil, nil
		}

		// Get subscription from checkout
		stripeSubID := ""
		if checkout.Subscription != nil {
			stripeSubID = checkout.Subscription.ID
		}

		// Extract product/resource ID from metadata
		productID := ""
		if checkout.Metadata != nil {
			productID = checkout.Metadata["resource_id"]
			if productID == "" {
				productID = checkout.Metadata["product_id"]
			}
		}

		return &SubscriptionWebhookEvent{
			Type:                 eventType,
			StripeSubscriptionID: stripeSubID,
			StripeCustomerID:     checkout.Customer.ID,
			ProductID:            productID,
			Metadata:             checkout.Metadata,
		}, nil

	default:
		return nil, nil
	}
}

// HandleSubscriptionWebhook processes subscription webhook events and updates storage.
func (c *Client) HandleSubscriptionWebhook(ctx context.Context, event *SubscriptionWebhookEvent, subRepo SubscriptionRepository) error {
	if event == nil {
		return nil
	}

	switch event.Type {
	case "checkout.session.completed":
		// New subscription created via checkout
		if event.StripeSubscriptionID == "" {
			return nil // Not a subscription checkout
		}

		// Fetch full subscription details from Stripe
		stripeSub, err := c.GetSubscription(ctx, event.StripeSubscriptionID)
		if err != nil {
			return fmt.Errorf("stripe: get subscription details: %w", err)
		}

		// Create subscription record
		sub := subscriptions.Subscription{
			ID:                   fmt.Sprintf("sub_%s", event.StripeSubscriptionID),
			ProductID:            event.ProductID,
			StripeCustomerID:     event.StripeCustomerID,
			StripeSubscriptionID: event.StripeSubscriptionID,
			PaymentMethod:        subscriptions.PaymentMethodStripe,
			Status:               mapStripeStatus(string(stripeSub.Status)),
			CurrentPeriodStart:   time.Unix(stripeSub.CurrentPeriodStart, 0),
			CurrentPeriodEnd:     time.Unix(stripeSub.CurrentPeriodEnd, 0),
			CancelAtPeriodEnd:    stripeSub.CancelAtPeriodEnd,
			Metadata:             event.Metadata,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}

		// Determine billing period from Stripe price
		if stripeSub.Items != nil && len(stripeSub.Items.Data) > 0 {
			price := stripeSub.Items.Data[0].Price
			if price != nil && price.Recurring != nil {
				sub.BillingPeriod = mapStripeBillingPeriod(string(price.Recurring.Interval))
				sub.BillingInterval = int(price.Recurring.IntervalCount)
			}
		}

		// Check for trial
		if stripeSub.TrialEnd > 0 {
			trialEnd := time.Unix(stripeSub.TrialEnd, 0)
			sub.TrialEnd = &trialEnd
			if time.Now().Before(trialEnd) {
				sub.Status = subscriptions.StatusTrialing
			}
		}

		if err := subRepo.Create(ctx, sub); err != nil {
			// If already exists, update instead
			if strings.Contains(err.Error(), "already exists") {
				return subRepo.Update(ctx, sub)
			}
			return fmt.Errorf("stripe: create subscription record: %w", err)
		}
		return nil

	case "customer.subscription.updated":
		// Subscription updated (plan change, renewal, etc.)
		existing, err := subRepo.GetByStripeSubscriptionID(ctx, event.StripeSubscriptionID)
		if err != nil {
			return nil // Subscription not tracked by us
		}

		// Update status
		existing.Status = mapStripeStatus(event.Status)
		existing.CurrentPeriodStart = event.CurrentPeriodStart
		existing.CurrentPeriodEnd = event.CurrentPeriodEnd
		existing.CancelAtPeriodEnd = event.CancelAtPeriodEnd
		existing.CancelledAt = event.CancelledAt
		existing.UpdatedAt = time.Now()

		// Handle plan change (productID updated)
		if event.ProductID != "" && event.ProductID != existing.ProductID {
			if existing.Metadata == nil {
				existing.Metadata = make(map[string]string)
			}
			existing.Metadata["previous_product"] = existing.ProductID
			existing.Metadata["changed_at"] = time.Now().UTC().Format(time.RFC3339)
			existing.ProductID = event.ProductID
		}

		// Update billing period if changed
		if event.BillingPeriod != "" {
			existing.BillingPeriod = mapStripeBillingPeriod(event.BillingPeriod)
		}
		if event.BillingInterval > 0 {
			existing.BillingInterval = event.BillingInterval
		}

		return subRepo.Update(ctx, existing)

	case "customer.subscription.deleted":
		// Subscription cancelled or expired
		existing, err := subRepo.GetByStripeSubscriptionID(ctx, event.StripeSubscriptionID)
		if err != nil {
			return nil // Subscription not tracked by us
		}

		return subRepo.UpdateStatus(ctx, existing.ID, subscriptions.StatusCancelled)

	case "invoice.paid":
		// Successful renewal payment
		if event.StripeSubscriptionID == "" {
			return nil
		}

		// Fetch updated subscription from Stripe
		stripeSub, err := c.GetSubscription(ctx, event.StripeSubscriptionID)
		if err != nil {
			return fmt.Errorf("stripe: get subscription for renewal: %w", err)
		}

		existing, err := subRepo.GetByStripeSubscriptionID(ctx, event.StripeSubscriptionID)
		if err != nil {
			return nil // Subscription not tracked by us
		}

		// Extend the period
		return subRepo.ExtendPeriod(ctx, existing.ID,
			time.Unix(stripeSub.CurrentPeriodStart, 0),
			time.Unix(stripeSub.CurrentPeriodEnd, 0),
		)

	case "invoice.payment_failed":
		// Payment failed - mark as past due
		if event.StripeSubscriptionID == "" {
			return nil
		}

		existing, err := subRepo.GetByStripeSubscriptionID(ctx, event.StripeSubscriptionID)
		if err != nil {
			return nil // Subscription not tracked by us
		}

		return subRepo.UpdateStatus(ctx, existing.ID, subscriptions.StatusPastDue)
	}

	return nil
}

// mapStripeStatus converts Stripe subscription status to our status.
func mapStripeStatus(stripeStatus string) subscriptions.Status {
	switch stripeStatus {
	case "active":
		return subscriptions.StatusActive
	case "trialing":
		return subscriptions.StatusTrialing
	case "past_due":
		return subscriptions.StatusPastDue
	case "canceled", "cancelled":
		return subscriptions.StatusCancelled
	case "unpaid", "incomplete", "incomplete_expired":
		return subscriptions.StatusExpired
	default:
		return subscriptions.StatusActive
	}
}

// mapStripeBillingPeriod converts Stripe interval to our billing period.
func mapStripeBillingPeriod(interval string) subscriptions.BillingPeriod {
	switch interval {
	case "day":
		return subscriptions.PeriodDay
	case "week":
		return subscriptions.PeriodWeek
	case "month":
		return subscriptions.PeriodMonth
	case "year":
		return subscriptions.PeriodYear
	default:
		return subscriptions.PeriodMonth
	}
}
