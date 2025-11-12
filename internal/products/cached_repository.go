package products

import (
	"context"
	"sync"
	"time"

	"github.com/CedrosPay/server/internal/cacheutil"
)

// CachedRepository wraps a Repository with caching for ListProducts and lookups.
type CachedRepository struct {
	underlying Repository
	cacheTTL   time.Duration

	mu                   sync.RWMutex
	cachedList           cacheutil.CachedValue[[]Product]
	stripePriceIDToID    map[string]string  // Reverse index: stripePriceID → productID
	productCache         map[string]Product // Product cache by ID
	stripePriceIDToIDTTL time.Time          // TTL for reverse index
}

// NewCachedRepository wraps a repository with a caching layer.
// cacheTTL determines how long the product list cache is valid.
// Set to 0 to disable caching (pass-through mode).
func NewCachedRepository(underlying Repository, cacheTTL time.Duration) *CachedRepository {
	return &CachedRepository{
		underlying: underlying,
		cacheTTL:   cacheTTL,
	}
}

// GetProduct retrieves a product by ID with caching.
func (r *CachedRepository) GetProduct(ctx context.Context, id string) (Product, error) {
	if r.cacheTTL == 0 {
		return r.underlying.GetProduct(ctx, id)
	}

	r.mu.RLock()
	// Check product cache
	if product, found := r.productCache[id]; found {
		r.mu.RUnlock()
		return product, nil
	}
	r.mu.RUnlock()

	// Cache miss - fetch from underlying repository
	product, err := r.underlying.GetProduct(ctx, id)
	if err != nil {
		return Product{}, err
	}

	// Update cache
	r.mu.Lock()
	if r.productCache == nil {
		r.productCache = make(map[string]Product)
	}
	r.productCache[id] = product
	r.mu.Unlock()

	return product, nil
}

// GetProductByStripePriceID retrieves a product by its Stripe Price ID with caching.
func (r *CachedRepository) GetProductByStripePriceID(ctx context.Context, stripePriceID string) (Product, error) {
	if r.cacheTTL == 0 {
		return r.underlying.GetProductByStripePriceID(ctx, stripePriceID)
	}

	// Build reverse index if needed
	r.ensureReverseIndex(ctx)

	r.mu.RLock()
	productID, found := r.stripePriceIDToID[stripePriceID]
	r.mu.RUnlock()

	if !found {
		// Not in index - fetch directly and update cache
		product, err := r.underlying.GetProductByStripePriceID(ctx, stripePriceID)
		if err != nil {
			return Product{}, err
		}

		// Update both caches
		r.mu.Lock()
		if r.stripePriceIDToID == nil {
			r.stripePriceIDToID = make(map[string]string)
		}
		r.stripePriceIDToID[stripePriceID] = product.ID
		if r.productCache == nil {
			r.productCache = make(map[string]Product)
		}
		r.productCache[product.ID] = product
		r.mu.Unlock()

		return product, nil
	}

	// Use the product ID to get from cache
	return r.GetProduct(ctx, productID)
}

// ensureReverseIndex builds the stripePriceID → productID index if expired.
func (r *CachedRepository) ensureReverseIndex(ctx context.Context) {
	r.mu.RLock()
	indexValid := time.Now().Before(r.stripePriceIDToIDTTL)
	r.mu.RUnlock()

	if indexValid {
		return
	}

	// Index expired - rebuild it
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if time.Now().Before(r.stripePriceIDToIDTTL) {
		return
	}

	// Fetch all products to build index
	products, err := r.underlying.ListProducts(ctx)
	if err != nil {
		// Failed to build index - log and continue with pass-through
		return
	}

	// Build reverse index
	newIndex := make(map[string]string, len(products))
	newProductCache := make(map[string]Product, len(products))
	for _, p := range products {
		if p.StripePriceID != "" {
			newIndex[p.StripePriceID] = p.ID
		}
		newProductCache[p.ID] = p
	}

	r.stripePriceIDToID = newIndex
	r.productCache = newProductCache
	r.stripePriceIDToIDTTL = time.Now().Add(r.cacheTTL)
}

// ListProducts returns all active products with TTL-based caching.
func (r *CachedRepository) ListProducts(ctx context.Context) ([]Product, error) {
	// Check if caching is disabled
	if r.cacheTTL == 0 {
		return r.underlying.ListProducts(ctx)
	}

	// Use ReadThrough helper to handle caching with race condition protection
	return cacheutil.ReadThrough(
		&r.mu,
		func(now time.Time) ([]Product, bool) {
			// Check if cache is still valid (cachedList.Value != nil means it's populated)
			if r.cachedList.Value != nil && now.Sub(r.cachedList.FetchedAt) < r.cacheTTL {
				return r.cachedList.Value, true
			}
			return nil, false
		},
		func(now time.Time) ([]Product, error) {
			// Fetch from underlying repository
			products, err := r.underlying.ListProducts(ctx)
			if err != nil {
				return nil, err
			}
			// Update cache with consistent timestamp
			r.cachedList = cacheutil.CachedValue[[]Product]{
				Value:     products,
				FetchedAt: now,
			}
			return products, nil
		},
	)
}

// InvalidateCache forces the next ListProducts call to fetch fresh data and clears all caches.
func (r *CachedRepository) InvalidateCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cachedList = cacheutil.CachedValue[[]Product]{} // Reset to zero value
	r.stripePriceIDToID = nil
	r.productCache = nil
	r.stripePriceIDToIDTTL = time.Time{} // Zero time
}

// CreateProduct creates a new product and invalidates the cache.
func (r *CachedRepository) CreateProduct(ctx context.Context, product Product) error {
	return cacheutil.WriteThrough(r.InvalidateCache, func() error {
		return r.underlying.CreateProduct(ctx, product)
	})
}

// UpdateProduct updates an existing product and invalidates the cache.
func (r *CachedRepository) UpdateProduct(ctx context.Context, product Product) error {
	return cacheutil.WriteThrough(r.InvalidateCache, func() error {
		return r.underlying.UpdateProduct(ctx, product)
	})
}

// DeleteProduct soft-deletes a product and invalidates the cache.
func (r *CachedRepository) DeleteProduct(ctx context.Context, id string) error {
	return cacheutil.WriteThrough(r.InvalidateCache, func() error {
		return r.underlying.DeleteProduct(ctx, id)
	})
}

// Close closes the underlying repository.
func (r *CachedRepository) Close() error {
	return r.underlying.Close()
}
