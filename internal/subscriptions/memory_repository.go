package subscriptions

import (
	"context"
	"sync"
	"time"
)

// MemoryRepository is an in-memory implementation of Repository for testing.
type MemoryRepository struct {
	mu   sync.RWMutex
	subs map[string]Subscription

	// Secondary indexes for efficient lookups
	byWalletProduct map[string]string // "wallet:productID" -> subscription ID
	byStripeSubID   map[string]string // stripeSubscriptionID -> subscription ID
	byStripeCustomer map[string][]string // stripeCustomerID -> []subscription IDs
}

// NewMemoryRepository creates a new in-memory repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		subs:             make(map[string]Subscription),
		byWalletProduct:  make(map[string]string),
		byStripeSubID:    make(map[string]string),
		byStripeCustomer: make(map[string][]string),
	}
}

func walletProductKey(wallet, productID string) string {
	return wallet + ":" + productID
}

// Create stores a new subscription.
func (r *MemoryRepository) Create(_ context.Context, sub Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sub.ID == "" {
		return ErrInvalidSubscription
	}

	if _, exists := r.subs[sub.ID]; exists {
		return ErrAlreadyExists
	}

	// Set timestamps
	now := time.Now()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	sub.UpdatedAt = now

	r.subs[sub.ID] = sub

	// Update indexes
	if sub.Wallet != "" {
		r.byWalletProduct[walletProductKey(sub.Wallet, sub.ProductID)] = sub.ID
	}
	if sub.StripeSubscriptionID != "" {
		r.byStripeSubID[sub.StripeSubscriptionID] = sub.ID
	}
	if sub.StripeCustomerID != "" {
		r.byStripeCustomer[sub.StripeCustomerID] = append(
			r.byStripeCustomer[sub.StripeCustomerID], sub.ID,
		)
	}

	return nil
}

// Get retrieves a subscription by ID.
func (r *MemoryRepository) Get(_ context.Context, id string) (Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sub, ok := r.subs[id]
	if !ok {
		return Subscription{}, ErrNotFound
	}
	return sub, nil
}

// Update modifies an existing subscription.
func (r *MemoryRepository) Update(_ context.Context, sub Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.subs[sub.ID]
	if !ok {
		return ErrNotFound
	}

	sub.UpdatedAt = time.Now()

	// Update indexes if wallet changed
	if existing.Wallet != sub.Wallet {
		delete(r.byWalletProduct, walletProductKey(existing.Wallet, existing.ProductID))
		if sub.Wallet != "" {
			r.byWalletProduct[walletProductKey(sub.Wallet, sub.ProductID)] = sub.ID
		}
	}

	// Update stripe subscription index
	if existing.StripeSubscriptionID != sub.StripeSubscriptionID {
		delete(r.byStripeSubID, existing.StripeSubscriptionID)
		if sub.StripeSubscriptionID != "" {
			r.byStripeSubID[sub.StripeSubscriptionID] = sub.ID
		}
	}

	r.subs[sub.ID] = sub
	return nil
}

// Delete removes a subscription.
func (r *MemoryRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sub, ok := r.subs[id]
	if !ok {
		return ErrNotFound
	}

	// Remove from indexes
	if sub.Wallet != "" {
		delete(r.byWalletProduct, walletProductKey(sub.Wallet, sub.ProductID))
	}
	if sub.StripeSubscriptionID != "" {
		delete(r.byStripeSubID, sub.StripeSubscriptionID)
	}
	if sub.StripeCustomerID != "" {
		ids := r.byStripeCustomer[sub.StripeCustomerID]
		for i, sid := range ids {
			if sid == id {
				r.byStripeCustomer[sub.StripeCustomerID] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
	}

	delete(r.subs, id)
	return nil
}

// GetByWallet finds an active subscription for a wallet and product.
func (r *MemoryRepository) GetByWallet(_ context.Context, wallet, productID string) (Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byWalletProduct[walletProductKey(wallet, productID)]
	if !ok {
		return Subscription{}, ErrNotFound
	}

	sub, ok := r.subs[id]
	if !ok {
		return Subscription{}, ErrNotFound
	}

	return sub, nil
}

// GetByStripeSubscriptionID finds a subscription by Stripe subscription ID.
func (r *MemoryRepository) GetByStripeSubscriptionID(_ context.Context, stripeSubID string) (Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byStripeSubID[stripeSubID]
	if !ok {
		return Subscription{}, ErrNotFound
	}

	sub, ok := r.subs[id]
	if !ok {
		return Subscription{}, ErrNotFound
	}

	return sub, nil
}

// GetByStripeCustomerID finds all subscriptions for a Stripe customer.
func (r *MemoryRepository) GetByStripeCustomerID(_ context.Context, customerID string) ([]Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := r.byStripeCustomer[customerID]
	if len(ids) == 0 {
		return nil, nil
	}

	result := make([]Subscription, 0, len(ids))
	for _, id := range ids {
		if sub, ok := r.subs[id]; ok {
			result = append(result, sub)
		}
	}

	return result, nil
}

// ListByProduct returns all subscriptions for a product.
func (r *MemoryRepository) ListByProduct(_ context.Context, productID string) ([]Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Subscription
	for _, sub := range r.subs {
		if sub.ProductID == productID {
			result = append(result, sub)
		}
	}
	return result, nil
}

// ListActive returns all active subscriptions.
func (r *MemoryRepository) ListActive(_ context.Context, productID string) ([]Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Subscription
	for _, sub := range r.subs {
		if productID != "" && sub.ProductID != productID {
			continue
		}
		if sub.IsActive() {
			result = append(result, sub)
		}
	}
	return result, nil
}

// ListExpiring returns subscriptions expiring before the given time.
func (r *MemoryRepository) ListExpiring(_ context.Context, before time.Time) ([]Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Subscription
	for _, sub := range r.subs {
		// Only include active subscriptions that are expiring
		if sub.Status == StatusActive && sub.CurrentPeriodEnd.Before(before) {
			result = append(result, sub)
		}
	}
	return result, nil
}

// UpdateStatus changes a subscription's status.
func (r *MemoryRepository) UpdateStatus(_ context.Context, id string, status Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sub, ok := r.subs[id]
	if !ok {
		return ErrNotFound
	}

	sub.Status = status
	sub.UpdatedAt = time.Now()

	if status == StatusCancelled {
		now := time.Now()
		sub.CancelledAt = &now
	}

	r.subs[id] = sub
	return nil
}

// ExtendPeriod updates the current period for renewals.
func (r *MemoryRepository) ExtendPeriod(_ context.Context, id string, newStart, newEnd time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sub, ok := r.subs[id]
	if !ok {
		return ErrNotFound
	}

	sub.CurrentPeriodStart = newStart
	sub.CurrentPeriodEnd = newEnd
	sub.UpdatedAt = time.Now()

	r.subs[id] = sub
	return nil
}

// Close implements Repository.Close (no-op for memory).
func (r *MemoryRepository) Close() error {
	return nil
}
