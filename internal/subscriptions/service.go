package subscriptions

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Service provides subscription management operations.
type Service struct {
	repo             Repository
	gracePeriodHours int
}

// NewService creates a new subscription service.
func NewService(repo Repository, gracePeriodHours int) *Service {
	return &Service{
		repo:             repo,
		gracePeriodHours: gracePeriodHours,
	}
}

// CreateStripeSubscription creates a new Stripe-backed subscription.
func (s *Service) CreateStripeSubscription(ctx context.Context, req CreateStripeSubscriptionRequest) (Subscription, error) {
	if req.ProductID == "" {
		return Subscription{}, fmt.Errorf("product_id is required")
	}
	if req.StripeCustomerID == "" {
		return Subscription{}, fmt.Errorf("stripe_customer_id is required")
	}
	if req.StripeSubscriptionID == "" {
		return Subscription{}, fmt.Errorf("stripe_subscription_id is required")
	}

	now := time.Now()
	periodEnd := req.CurrentPeriodEnd
	if periodEnd.IsZero() {
		periodEnd = CalculatePeriodEnd(now, req.BillingPeriod, req.BillingInterval)
	}

	sub := Subscription{
		ID:                   uuid.New().String(),
		ProductID:            req.ProductID,
		StripeCustomerID:     req.StripeCustomerID,
		StripeSubscriptionID: req.StripeSubscriptionID,
		PaymentMethod:        PaymentMethodStripe,
		BillingPeriod:        req.BillingPeriod,
		BillingInterval:      req.BillingInterval,
		Status:               StatusActive,
		CurrentPeriodStart:   now,
		CurrentPeriodEnd:     periodEnd,
		TrialEnd:             req.TrialEnd,
		Metadata:             req.Metadata,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if req.TrialEnd != nil && req.TrialEnd.After(now) {
		sub.Status = StatusTrialing
	}

	if err := s.repo.Create(ctx, sub); err != nil {
		return Subscription{}, fmt.Errorf("create subscription: %w", err)
	}

	return sub, nil
}

// CreateX402Subscription creates a new x402 wallet-backed subscription.
func (s *Service) CreateX402Subscription(ctx context.Context, req CreateX402SubscriptionRequest) (Subscription, error) {
	if req.ProductID == "" {
		return Subscription{}, fmt.Errorf("product_id is required")
	}
	if req.Wallet == "" {
		return Subscription{}, fmt.Errorf("wallet is required")
	}

	// Check if wallet already has an active subscription for this product
	existing, err := s.repo.GetByWallet(ctx, req.Wallet, req.ProductID)
	if err == nil && existing.IsActive() {
		// Extend the existing subscription instead of creating a new one
		return s.ExtendX402Subscription(ctx, existing.ID, req.BillingPeriod, req.BillingInterval)
	}

	now := time.Now()
	periodEnd := CalculatePeriodEnd(now, req.BillingPeriod, req.BillingInterval)

	sub := Subscription{
		ID:                 uuid.New().String(),
		ProductID:          req.ProductID,
		Wallet:             req.Wallet,
		PaymentMethod:      PaymentMethodX402,
		BillingPeriod:      req.BillingPeriod,
		BillingInterval:    req.BillingInterval,
		Status:             StatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		Metadata:           req.Metadata,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.repo.Create(ctx, sub); err != nil {
		return Subscription{}, fmt.Errorf("create subscription: %w", err)
	}

	return sub, nil
}

// ExtendX402Subscription extends an existing x402 subscription.
func (s *Service) ExtendX402Subscription(ctx context.Context, id string, period BillingPeriod, interval int) (Subscription, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return Subscription{}, fmt.Errorf("get subscription: %w", err)
	}

	// Calculate new period - start from current end if still active, otherwise from now
	var newStart time.Time
	if sub.IsActive() {
		newStart = sub.CurrentPeriodEnd
	} else {
		newStart = time.Now()
	}
	newEnd := CalculatePeriodEnd(newStart, period, interval)

	// Update the subscription
	sub.CurrentPeriodStart = newStart
	sub.CurrentPeriodEnd = newEnd
	sub.Status = StatusActive
	sub.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, sub); err != nil {
		return Subscription{}, fmt.Errorf("update subscription: %w", err)
	}

	return sub, nil
}

// HasAccess checks if a wallet has active subscription access to a product.
func (s *Service) HasAccess(ctx context.Context, wallet, productID string) (bool, *Subscription, error) {
	sub, err := s.repo.GetByWallet(ctx, wallet, productID)
	if err == ErrNotFound {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("get subscription: %w", err)
	}

	// Check if subscription is active
	if sub.IsActive() {
		return true, &sub, nil
	}

	// Check grace period for recently expired subscriptions
	if s.gracePeriodHours > 0 && sub.Status == StatusActive {
		gracePeriodEnd := sub.CurrentPeriodEnd.Add(time.Duration(s.gracePeriodHours) * time.Hour)
		if time.Now().Before(gracePeriodEnd) {
			return true, &sub, nil
		}
	}

	return false, &sub, nil
}

// HasStripeAccess checks if a Stripe customer has active subscription access.
func (s *Service) HasStripeAccess(ctx context.Context, stripeSubID string) (bool, *Subscription, error) {
	sub, err := s.repo.GetByStripeSubscriptionID(ctx, stripeSubID)
	if err == ErrNotFound {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("get subscription: %w", err)
	}

	return sub.IsActive(), &sub, nil
}

// Cancel cancels a subscription.
func (s *Service) Cancel(ctx context.Context, id string, atPeriodEnd bool) error {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get subscription: %w", err)
	}

	if atPeriodEnd {
		// Mark to cancel at end of period
		sub.CancelAtPeriodEnd = true
		sub.UpdatedAt = time.Now()
		return s.repo.Update(ctx, sub)
	}

	// Cancel immediately
	return s.repo.UpdateStatus(ctx, id, StatusCancelled)
}

// HandleStripeRenewal processes a successful Stripe subscription renewal.
func (s *Service) HandleStripeRenewal(ctx context.Context, stripeSubID string, periodStart, periodEnd time.Time) error {
	sub, err := s.repo.GetByStripeSubscriptionID(ctx, stripeSubID)
	if err != nil {
		return fmt.Errorf("get subscription: %w", err)
	}

	// Update period and ensure status is active
	sub.CurrentPeriodStart = periodStart
	sub.CurrentPeriodEnd = periodEnd
	sub.Status = StatusActive
	sub.UpdatedAt = time.Now()

	return s.repo.Update(ctx, sub)
}

// HandleStripePaymentFailed marks a subscription as past due.
func (s *Service) HandleStripePaymentFailed(ctx context.Context, stripeSubID string) error {
	return s.repo.UpdateStatus(ctx, stripeSubID, StatusPastDue)
}

// HandleStripeCancelled marks a subscription as cancelled.
func (s *Service) HandleStripeCancelled(ctx context.Context, stripeSubID string) error {
	sub, err := s.repo.GetByStripeSubscriptionID(ctx, stripeSubID)
	if err != nil {
		return fmt.Errorf("get subscription: %w", err)
	}

	return s.repo.UpdateStatus(ctx, sub.ID, StatusCancelled)
}

// Get retrieves a subscription by ID.
func (s *Service) Get(ctx context.Context, id string) (Subscription, error) {
	return s.repo.Get(ctx, id)
}

// GetByWallet retrieves a subscription by wallet and product.
func (s *Service) GetByWallet(ctx context.Context, wallet, productID string) (Subscription, error) {
	return s.repo.GetByWallet(ctx, wallet, productID)
}

// GetByStripeSubscriptionID retrieves a subscription by Stripe subscription ID.
func (s *Service) GetByStripeSubscriptionID(ctx context.Context, stripeSubID string) (Subscription, error) {
	return s.repo.GetByStripeSubscriptionID(ctx, stripeSubID)
}

// ListExpiring returns subscriptions expiring within the given duration.
func (s *Service) ListExpiring(ctx context.Context, within time.Duration) ([]Subscription, error) {
	return s.repo.ListExpiring(ctx, time.Now().Add(within))
}

// ExpireOverdue marks overdue subscriptions as expired.
func (s *Service) ExpireOverdue(ctx context.Context) (int, error) {
	// Find all x402 subscriptions past their period end
	expiring, err := s.repo.ListExpiring(ctx, time.Now())
	if err != nil {
		return 0, fmt.Errorf("list expiring: %w", err)
	}

	count := 0
	for _, sub := range expiring {
		// Only auto-expire x402 subscriptions
		// Stripe subscriptions are managed by Stripe webhooks
		if sub.PaymentMethod == PaymentMethodX402 {
			if err := s.repo.UpdateStatus(ctx, sub.ID, StatusExpired); err != nil {
				continue // Log error but continue processing
			}
			count++
		}
	}

	return count, nil
}

// ChangeSubscription changes a subscription to a different plan (upgrade/downgrade).
// For Stripe subscriptions, this should be called after the Stripe API update.
// For x402 subscriptions, this handles the local database update.
func (s *Service) ChangeSubscription(ctx context.Context, req ChangeSubscriptionRequest) (*ChangeSubscriptionResult, error) {
	if req.SubscriptionID == "" {
		return nil, fmt.Errorf("subscription_id is required")
	}
	if req.NewProductID == "" {
		return nil, fmt.Errorf("new_product_id is required")
	}

	sub, err := s.repo.Get(ctx, req.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	// Store previous product for result
	previousProduct := sub.ProductID

	// Update subscription fields
	sub.ProductID = req.NewProductID
	if req.NewBillingPeriod != "" {
		sub.BillingPeriod = req.NewBillingPeriod
	}
	if req.NewBillingInterval > 0 {
		sub.BillingInterval = req.NewBillingInterval
	}

	// Merge metadata
	if sub.Metadata == nil {
		sub.Metadata = make(map[string]string)
	}
	for k, v := range req.Metadata {
		sub.Metadata[k] = v
	}
	sub.Metadata["previous_product"] = previousProduct
	sub.Metadata["changed_at"] = time.Now().UTC().Format(time.RFC3339)

	sub.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, sub); err != nil {
		return nil, fmt.Errorf("update subscription: %w", err)
	}

	return &ChangeSubscriptionResult{
		Subscription:    sub,
		PreviousProduct: previousProduct,
		NewProduct:      req.NewProductID,
		EffectiveDate:   time.Now(),
	}, nil
}

// ReactivateSubscription reactivates a cancelled subscription (if still within period).
func (s *Service) ReactivateSubscription(ctx context.Context, id string) (Subscription, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return Subscription{}, fmt.Errorf("get subscription: %w", err)
	}

	// Can only reactivate if scheduled to cancel at period end and still within period
	if !sub.CancelAtPeriodEnd {
		return Subscription{}, fmt.Errorf("subscription is not scheduled for cancellation")
	}

	if time.Now().After(sub.CurrentPeriodEnd) {
		return Subscription{}, fmt.Errorf("subscription period has already ended")
	}

	sub.CancelAtPeriodEnd = false
	sub.CancelledAt = nil
	sub.Status = StatusActive
	sub.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, sub); err != nil {
		return Subscription{}, fmt.Errorf("update subscription: %w", err)
	}

	return sub, nil
}

// HandleStripeSubscriptionUpdated handles subscription update events from Stripe.
// This is more comprehensive than HandleStripeRenewal - it handles plan changes too.
func (s *Service) HandleStripeSubscriptionUpdated(ctx context.Context, stripeSubID string, update StripeSubscriptionUpdate) error {
	sub, err := s.repo.GetByStripeSubscriptionID(ctx, stripeSubID)
	if err != nil {
		return fmt.Errorf("get subscription: %w", err)
	}

	// Update status
	if update.Status != "" {
		sub.Status = update.Status
	}

	// Update period dates
	if !update.CurrentPeriodStart.IsZero() {
		sub.CurrentPeriodStart = update.CurrentPeriodStart
	}
	if !update.CurrentPeriodEnd.IsZero() {
		sub.CurrentPeriodEnd = update.CurrentPeriodEnd
	}

	// Update cancel state
	sub.CancelAtPeriodEnd = update.CancelAtPeriodEnd
	if update.CancelledAt != nil {
		sub.CancelledAt = update.CancelledAt
	}

	// Update product if changed (plan change)
	if update.NewProductID != "" && update.NewProductID != sub.ProductID {
		if sub.Metadata == nil {
			sub.Metadata = make(map[string]string)
		}
		sub.Metadata["previous_product"] = sub.ProductID
		sub.Metadata["changed_at"] = time.Now().UTC().Format(time.RFC3339)
		sub.ProductID = update.NewProductID
	}

	// Update billing period if changed
	if update.BillingPeriod != "" {
		sub.BillingPeriod = update.BillingPeriod
	}
	if update.BillingInterval > 0 {
		sub.BillingInterval = update.BillingInterval
	}

	sub.UpdatedAt = time.Now()

	return s.repo.Update(ctx, sub)
}

// StripeSubscriptionUpdate contains fields that can be updated from Stripe webhooks.
type StripeSubscriptionUpdate struct {
	Status             Status
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CancelAtPeriodEnd  bool
	CancelledAt        *time.Time
	NewProductID       string        // Set if plan changed
	BillingPeriod      BillingPeriod // Set if billing interval changed
	BillingInterval    int
}

// CreateStripeSubscriptionRequest contains parameters for creating a Stripe subscription.
type CreateStripeSubscriptionRequest struct {
	ProductID            string
	StripeCustomerID     string
	StripeSubscriptionID string
	BillingPeriod        BillingPeriod
	BillingInterval      int
	CurrentPeriodEnd     time.Time
	TrialEnd             *time.Time
	Metadata             map[string]string
}

// CreateX402SubscriptionRequest contains parameters for creating an x402 subscription.
type CreateX402SubscriptionRequest struct {
	ProductID       string
	Wallet          string
	BillingPeriod   BillingPeriod
	BillingInterval int
	Metadata        map[string]string
}

// ChangeSubscriptionRequest contains parameters for upgrading or downgrading a subscription.
type ChangeSubscriptionRequest struct {
	SubscriptionID  string            // ID of existing subscription
	NewProductID    string            // New product/plan to switch to
	NewPriceID      string            // New Stripe price ID (for Stripe subscriptions)
	NewBillingPeriod   BillingPeriod  // New billing period
	NewBillingInterval int            // New billing interval
	ProrationBehavior  string         // "create_prorations", "none", "always_invoice"
	Metadata        map[string]string // Updated metadata
}

// ChangeSubscriptionResult contains the result of a plan change.
type ChangeSubscriptionResult struct {
	Subscription     Subscription
	PreviousProduct  string
	NewProduct       string
	ProrationAmount  int64  // Amount in cents (positive = charge, negative = credit)
	EffectiveDate    time.Time
}
