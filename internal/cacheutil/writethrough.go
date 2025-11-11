package cacheutil

import (
	"sync"
	"time"
)

// WriteThrough executes a write operation and invalidates cache on success.
// This is a common pattern for cached repositories to maintain cache consistency.
//
// Usage:
//
//	func (r *CachedRepo) UpdateItem(ctx context.Context, item Item) error {
//	    return cacheutil.WriteThrough(r.InvalidateCache, func() error {
//	        return r.underlying.UpdateItem(ctx, item)
//	    })
//	}
func WriteThrough(invalidate func(), operation func() error) error {
	if err := operation(); err != nil {
		return err
	}
	invalidate()
	return nil
}

// CachedValue represents a cached value with expiration timestamp.
type CachedValue[T any] struct {
	Value     T
	FetchedAt time.Time
}

// ReadThrough implements a thread-safe read-through cache pattern with race condition protection.
// It uses double-checked locking with proper re-validation to prevent duplicate fetches.
//
// Parameters:
//   - mu: RWMutex for protecting cache access
//   - checkCache: Function to check if cached value is valid (called under RLock)
//   - fetchAndCache: Function to fetch and cache new value (called under Lock)
//
// This helper solves three common cache problems:
//  1. Race condition: Re-checks cache after acquiring write lock
//  2. Performance: Reuses single time.Now() call across check and update
//  3. Code duplication: Consolidates ~20 lines of boilerplate per cache method
//
// Usage:
//
//	func (r *CachedRepo) GetItem(ctx context.Context, id string) (Item, error) {
//	    return cacheutil.ReadThrough(
//	        &r.mu,
//	        func(now time.Time) (Item, bool) {
//	            if entry, ok := r.cache[id]; ok && now.Sub(entry.FetchedAt) < r.ttl {
//	                return entry.Value, true
//	            }
//	            return Item{}, false
//	        },
//	        func(now time.Time) (Item, error) {
//	            item, err := r.underlying.GetItem(ctx, id)
//	            if err != nil {
//	                return Item{}, err
//	            }
//	            r.cache[id] = CachedValue[Item]{Value: item, FetchedAt: now}
//	            return item, nil
//	        },
//	    )
//	}
func ReadThrough[T any](
	mu *sync.RWMutex,
	checkCache func(now time.Time) (T, bool),
	fetchAndCache func(now time.Time) (T, error),
) (T, error) {
	// Fast path: check cache under read lock
	now := time.Now()
	mu.RLock()
	if value, ok := checkCache(now); ok {
		mu.RUnlock()
		return value, nil
	}
	mu.RUnlock()

	// Cache miss: acquire write lock
	mu.Lock()
	defer mu.Unlock()

	// CRITICAL: Re-check cache after acquiring write lock with fresh timestamp
	// Another goroutine may have populated the cache between RUnlock and Lock
	// Use fresh time to avoid treating newly-cached data as expired
	nowAfterLock := time.Now()
	if value, ok := checkCache(nowAfterLock); ok {
		return value, nil
	}

	// Cache still invalid: fetch and populate
	return fetchAndCache(nowAfterLock)
}
