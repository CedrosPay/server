package idempotency

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// Response represents a cached idempotent response
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	CachedAt   time.Time
}

// Store manages idempotency keys and cached responses
type Store interface {
	// Get retrieves a cached response for the given key
	Get(ctx context.Context, key string) (*Response, bool)

	// Set stores a response for the given key with TTL
	Set(ctx context.Context, key string, response *Response, ttl time.Duration) error

	// Delete removes a cached response
	Delete(ctx context.Context, key string) error
}

// MemoryStore is an in-memory implementation of Store with LRU eviction
type MemoryStore struct {
	mu          sync.RWMutex
	cache       map[string]*cacheEntry
	expires     map[string]time.Time
	lru         *list.List
	maxSize     int
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

type cacheEntry struct {
	key      string
	response *Response
	element  *list.Element
}

// NewMemoryStore creates a new in-memory idempotency store with a maximum of 10,000 entries
func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithSize(10000)
}

// NewMemoryStoreWithSize creates a new in-memory idempotency store with custom max size
func NewMemoryStoreWithSize(maxSize int) *MemoryStore {
	s := &MemoryStore{
		cache:       make(map[string]*cacheEntry),
		expires:     make(map[string]time.Time),
		lru:         list.New(),
		maxSize:     maxSize,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go s.cleanup()

	return s
}

// Get retrieves a cached response for the given key
func (s *MemoryStore) Get(ctx context.Context, key string) (*Response, bool) {
	// Cache time.Now() to avoid syscall under lock
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if key exists and hasn't expired
	expiry, exists := s.expires[key]
	if !exists || now.After(expiry) {
		return nil, false
	}

	entry, found := s.cache[key]
	if !found {
		return nil, false
	}

	// Move to front of LRU list (most recently used)
	s.lru.MoveToFront(entry.element)

	return entry.response, true
}

// Set stores a response for the given key with TTL
func (s *MemoryStore) Set(ctx context.Context, key string, response *Response, ttl time.Duration) error {
	// Cache time.Now() to avoid multiple syscalls
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	// If key already exists, update it and move to front
	if entry, exists := s.cache[key]; exists {
		entry.response = response
		s.expires[key] = now.Add(ttl)
		s.lru.MoveToFront(entry.element)
		return nil
	}

	// Evict before adding to ensure we never exceed maxSize
	// This prevents race conditions where multiple goroutines
	// could each see Len() < maxSize and all add entries
	if len(s.cache) >= s.maxSize {
		s.evictLRU()
	}

	// Add new entry
	entry := &cacheEntry{
		key:      key,
		response: response,
	}
	entry.element = s.lru.PushFront(entry)
	s.cache[key] = entry
	s.expires[key] = now.Add(ttl)

	return nil
}

// evictLRU removes the least recently used entry (caller must hold lock)
func (s *MemoryStore) evictLRU() {
	element := s.lru.Back()
	if element == nil {
		return
	}

	entry := element.Value.(*cacheEntry)
	s.lru.Remove(element)
	delete(s.cache, entry.key)
	delete(s.expires, entry.key)
}

// Delete removes a cached response
func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.cache[key]; exists {
		s.lru.Remove(entry.element)
		delete(s.cache, key)
		delete(s.expires, key)
	}

	return nil
}

// cleanup periodically removes expired entries
func (s *MemoryStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	defer close(s.cleanupDone)

	for {
		select {
		case <-s.stopCleanup:
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()

			// Collect keys to delete first to avoid concurrent map iteration and write
			var keysToDelete []string
			for key, expiry := range s.expires {
				if now.After(expiry) {
					keysToDelete = append(keysToDelete, key)
				}
			}

			// Delete expired entries
			for _, key := range keysToDelete {
				if entry, exists := s.cache[key]; exists {
					s.lru.Remove(entry.element)
					delete(s.cache, key)
					delete(s.expires, key)
				}
			}

			s.mu.Unlock()
		}
	}
}

// Stop gracefully shuts down the cleanup goroutine
func (s *MemoryStore) Stop() {
	close(s.stopCleanup)
	<-s.cleanupDone
}
