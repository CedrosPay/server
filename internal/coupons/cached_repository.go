package coupons

import (
	"context"
	"sync"
	"time"

	"github.com/CedrosPay/server/internal/cacheutil"
)

// CachedRepository wraps any Repository with a TTL-based cache.
type CachedRepository struct {
	underlying      Repository
	cacheTTL        time.Duration
	mu              sync.RWMutex
	cachedCoupon    map[string]cacheutil.CachedValue[Coupon]
	cachedList      cacheutil.CachedValue[[]Coupon]
	cachedAutoApply map[PaymentMethod]cacheutil.CachedValue[map[string][]Coupon]
}

// NewCachedRepository wraps a repository with caching.
func NewCachedRepository(underlying Repository, cacheTTL time.Duration) *CachedRepository {
	return &CachedRepository{
		underlying:      underlying,
		cacheTTL:        cacheTTL,
		cachedCoupon:    make(map[string]cacheutil.CachedValue[Coupon]),
		cachedAutoApply: make(map[PaymentMethod]cacheutil.CachedValue[map[string][]Coupon]),
	}
}

// GetCoupon retrieves a coupon with caching.
func (r *CachedRepository) GetCoupon(ctx context.Context, code string) (Coupon, error) {
	if r.cacheTTL == 0 {
		return r.underlying.GetCoupon(ctx, code)
	}

	// Use ReadThrough helper to handle caching with race condition protection
	return cacheutil.ReadThrough(
		&r.mu,
		func(now time.Time) (Coupon, bool) {
			// Check if cached entry exists and is still valid
			if entry, ok := r.cachedCoupon[code]; ok && now.Sub(entry.FetchedAt) < r.cacheTTL {
				return entry.Value, true
			}
			return Coupon{}, false
		},
		func(now time.Time) (Coupon, error) {
			// Fetch from underlying repository
			coupon, err := r.underlying.GetCoupon(ctx, code)
			if err != nil {
				return Coupon{}, err
			}
			// Update cache with consistent timestamp
			r.cachedCoupon[code] = cacheutil.CachedValue[Coupon]{
				Value:     coupon,
				FetchedAt: now,
			}
			return coupon, nil
		},
	)
}

// ListCoupons returns all coupons with caching.
func (r *CachedRepository) ListCoupons(ctx context.Context) ([]Coupon, error) {
	if r.cacheTTL == 0 {
		return r.underlying.ListCoupons(ctx)
	}

	// Use ReadThrough helper to handle caching with race condition protection
	return cacheutil.ReadThrough(
		&r.mu,
		func(now time.Time) ([]Coupon, bool) {
			// Check if cache is still valid (cachedList.Value != nil means it's populated)
			if r.cachedList.Value != nil && now.Sub(r.cachedList.FetchedAt) < r.cacheTTL {
				return r.cachedList.Value, true
			}
			return nil, false
		},
		func(now time.Time) ([]Coupon, error) {
			// Fetch from underlying repository
			coupons, err := r.underlying.ListCoupons(ctx)
			if err != nil {
				return nil, err
			}
			// Update cache with consistent timestamp
			r.cachedList = cacheutil.CachedValue[[]Coupon]{
				Value:     coupons,
				FetchedAt: now,
			}
			return coupons, nil
		},
	)
}

// GetAutoApplyCouponsForPayment delegates to the underlying repository (no caching).
func (r *CachedRepository) GetAutoApplyCouponsForPayment(ctx context.Context, productID string, paymentMethod PaymentMethod) ([]Coupon, error) {
	// Note: Auto-apply coupons are not cached separately as they are dynamic
	// based on productID and payment method. Delegate to underlying repository.
	return r.underlying.GetAutoApplyCouponsForPayment(ctx, productID, paymentMethod)
}

// GetAllAutoApplyCouponsForPayment returns auto-apply coupons for all products with caching.
func (r *CachedRepository) GetAllAutoApplyCouponsForPayment(ctx context.Context, paymentMethod PaymentMethod) (map[string][]Coupon, error) {
	if r.cacheTTL == 0 {
		return r.underlying.GetAllAutoApplyCouponsForPayment(ctx, paymentMethod)
	}

	// Use ReadThrough helper to handle caching with race condition protection
	return cacheutil.ReadThrough(
		&r.mu,
		func(now time.Time) (map[string][]Coupon, bool) {
			// Check if cached entry exists and is still valid
			if entry, ok := r.cachedAutoApply[paymentMethod]; ok && now.Sub(entry.FetchedAt) < r.cacheTTL {
				return entry.Value, true
			}
			return nil, false
		},
		func(now time.Time) (map[string][]Coupon, error) {
			// Fetch from underlying repository
			coupons, err := r.underlying.GetAllAutoApplyCouponsForPayment(ctx, paymentMethod)
			if err != nil {
				return nil, err
			}
			// Update cache with consistent timestamp
			r.cachedAutoApply[paymentMethod] = cacheutil.CachedValue[map[string][]Coupon]{
				Value:     coupons,
				FetchedAt: now,
			}
			return coupons, nil
		},
	)
}

// CreateCoupon creates a coupon and invalidates cache.
func (r *CachedRepository) CreateCoupon(ctx context.Context, coupon Coupon) error {
	return cacheutil.WriteThrough(r.InvalidateCache, func() error {
		return r.underlying.CreateCoupon(ctx, coupon)
	})
}

// UpdateCoupon updates a coupon and invalidates cache.
func (r *CachedRepository) UpdateCoupon(ctx context.Context, coupon Coupon) error {
	return cacheutil.WriteThrough(r.InvalidateCache, func() error {
		return r.underlying.UpdateCoupon(ctx, coupon)
	})
}

// IncrementUsage increments usage and invalidates cache.
func (r *CachedRepository) IncrementUsage(ctx context.Context, code string) error {
	err := r.underlying.IncrementUsage(ctx, code)
	if err != nil {
		return err
	}

	// Invalidate only the specific coupon cache
	r.mu.Lock()
	delete(r.cachedCoupon, code)
	r.mu.Unlock()

	return nil
}

// DeleteCoupon deletes a coupon and invalidates cache.
func (r *CachedRepository) DeleteCoupon(ctx context.Context, code string) error {
	return cacheutil.WriteThrough(r.InvalidateCache, func() error {
		return r.underlying.DeleteCoupon(ctx, code)
	})
}

// Close closes the underlying repository.
func (r *CachedRepository) Close() error {
	return r.underlying.Close()
}

// InvalidateCache forces the next operations to fetch fresh data.
func (r *CachedRepository) InvalidateCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cachedCoupon = make(map[string]cacheutil.CachedValue[Coupon])
	r.cachedList = cacheutil.CachedValue[[]Coupon]{} // Reset to zero value
	r.cachedAutoApply = make(map[PaymentMethod]cacheutil.CachedValue[map[string][]Coupon])
}
