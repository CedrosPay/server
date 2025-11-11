package idempotency

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMemoryStore_BasicOperations(t *testing.T) {
	store := NewMemoryStoreWithSize(10)
	defer store.Stop()

	ctx := context.Background()

	// Test Set and Get
	response := &Response{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(`{"status":"ok"}`),
		CachedAt:   time.Now(),
	}

	err := store.Set(ctx, "key1", response, 5*time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, found := store.Get(ctx, "key1")
	if !found {
		t.Fatal("Expected to find key1")
	}
	if retrieved.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", retrieved.StatusCode)
	}
}

func TestMemoryStore_Expiration(t *testing.T) {
	store := NewMemoryStoreWithSize(10)
	defer store.Stop()

	ctx := context.Background()
	response := &Response{
		StatusCode: 200,
		Body:       []byte(`{"status":"ok"}`),
		CachedAt:   time.Now(),
	}

	// Set with very short TTL
	err := store.Set(ctx, "expiring-key", response, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Should exist immediately
	_, found := store.Get(ctx, "expiring-key")
	if !found {
		t.Fatal("Expected to find key immediately after setting")
	}

	// Wait for expiration
	time.Sleep(50 * time.Millisecond)

	// Should not exist after expiration
	_, found = store.Get(ctx, "expiring-key")
	if found {
		t.Fatal("Expected key to be expired")
	}
}

func TestMemoryStore_LRUEviction(t *testing.T) {
	// Create store with small max size
	store := NewMemoryStoreWithSize(3)
	defer store.Stop()

	ctx := context.Background()
	response := &Response{
		StatusCode: 200,
		Body:       []byte(`{"status":"ok"}`),
		CachedAt:   time.Now(),
	}

	// Add 3 items (fills the cache)
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("key%d", i)
		err := store.Set(ctx, key, response, 5*time.Minute)
		if err != nil {
			t.Fatalf("Set failed for %s: %v", key, err)
		}
	}

	// Verify all 3 exist
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("key%d", i)
		_, found := store.Get(ctx, key)
		if !found {
			t.Fatalf("Expected to find %s", key)
		}
	}

	// Add 4th item (should evict least recently used - key1)
	err := store.Set(ctx, "key4", response, 5*time.Minute)
	if err != nil {
		t.Fatalf("Set failed for key4: %v", err)
	}

	// key1 should be evicted
	_, found := store.Get(ctx, "key1")
	if found {
		t.Error("Expected key1 to be evicted")
	}

	// key2, key3, key4 should still exist
	for _, key := range []string{"key2", "key3", "key4"} {
		_, found := store.Get(ctx, key)
		if !found {
			t.Errorf("Expected to find %s", key)
		}
	}
}

func TestMemoryStore_Update(t *testing.T) {
	store := NewMemoryStoreWithSize(10)
	defer store.Stop()

	ctx := context.Background()

	// Set initial value
	response1 := &Response{
		StatusCode: 200,
		Body:       []byte(`{"version":1}`),
		CachedAt:   time.Now(),
	}
	err := store.Set(ctx, "update-key", response1, 5*time.Minute)
	if err != nil {
		t.Fatalf("Initial Set failed: %v", err)
	}

	// Update with new value
	response2 := &Response{
		StatusCode: 201,
		Body:       []byte(`{"version":2}`),
		CachedAt:   time.Now(),
	}
	err = store.Set(ctx, "update-key", response2, 5*time.Minute)
	if err != nil {
		t.Fatalf("Update Set failed: %v", err)
	}

	// Verify updated value
	retrieved, found := store.Get(ctx, "update-key")
	if !found {
		t.Fatal("Expected to find updated key")
	}
	if retrieved.StatusCode != 201 {
		t.Errorf("StatusCode = %d, want 201", retrieved.StatusCode)
	}
	if string(retrieved.Body) != `{"version":2}` {
		t.Errorf("Body = %s, want {\"version\":2}", string(retrieved.Body))
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStoreWithSize(10)
	defer store.Stop()

	ctx := context.Background()
	response := &Response{
		StatusCode: 200,
		Body:       []byte(`{"status":"ok"}`),
		CachedAt:   time.Now(),
	}

	// Set and verify
	err := store.Set(ctx, "delete-key", response, 5*time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	_, found := store.Get(ctx, "delete-key")
	if !found {
		t.Fatal("Expected to find key before deletion")
	}

	// Delete
	err = store.Delete(ctx, "delete-key")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, found = store.Get(ctx, "delete-key")
	if found {
		t.Error("Expected key to be deleted")
	}
}

// TestMemoryStore_ConcurrentAccess tests the race condition fix
// Previously, concurrent Set() calls could exceed maxSize
func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	const maxSize = 100
	const numGoroutines = 20
	const opsPerGoroutine = 50

	store := NewMemoryStoreWithSize(maxSize)
	defer store.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup

	// Launch multiple goroutines performing concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < opsPerGoroutine; j++ {
				key := fmt.Sprintf("worker%d-key%d", workerID, j)
				response := &Response{
					StatusCode: 200,
					Body:       []byte(fmt.Sprintf(`{"worker":%d,"op":%d}`, workerID, j)),
					CachedAt:   time.Now(),
				}

				// Set
				err := store.Set(ctx, key, response, 5*time.Minute)
				if err != nil {
					t.Errorf("Set failed: %v", err)
					return
				}

				// Get (to exercise LRU updating)
				_, found := store.Get(ctx, key)
				if !found {
					// It's possible the key was evicted due to LRU, which is fine
					// We just want to ensure no panics or corruption
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify cache size doesn't exceed maxSize
	store.mu.Lock()
	cacheSize := len(store.cache)
	lruSize := store.lru.Len()
	store.mu.Unlock()

	if cacheSize > maxSize {
		t.Errorf("Cache size %d exceeds maxSize %d (race condition not fixed)", cacheSize, maxSize)
	}

	if cacheSize != lruSize {
		t.Errorf("Cache size %d doesn't match LRU size %d (data structure corruption)", cacheSize, lruSize)
	}

	t.Logf("Final cache size: %d/%d (within limit)", cacheSize, maxSize)
}

// TestMemoryStore_ConcurrentReadWrite tests concurrent reads and writes
func TestMemoryStore_ConcurrentReadWrite(t *testing.T) {
	store := NewMemoryStoreWithSize(50)
	defer store.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup

	// Writer goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("shared-key-%d", j%10)
				response := &Response{
					StatusCode: 200,
					Body:       []byte(fmt.Sprintf(`{"writer":%d,"iteration":%d}`, writerID, j)),
					CachedAt:   time.Now(),
				}
				_ = store.Set(ctx, key, response, 1*time.Minute)
			}
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("shared-key-%d", j%10)
				_, _ = store.Get(ctx, key)
			}
		}(i)
	}

	wg.Wait()

	// If we got here without panicking, the concurrent access is safe
	t.Log("Concurrent read/write test passed")
}

// TestMemoryStore_LRUBehavior verifies LRU ordering is maintained
func TestMemoryStore_LRUBehavior(t *testing.T) {
	store := NewMemoryStoreWithSize(3)
	defer store.Stop()

	ctx := context.Background()
	response := &Response{
		StatusCode: 200,
		Body:       []byte(`{"status":"ok"}`),
		CachedAt:   time.Now(),
	}

	// Add key1, key2, key3 (fills cache)
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("key%d", i)
		_ = store.Set(ctx, key, response, 5*time.Minute)
	}

	// Access key1 (makes it most recently used)
	_, _ = store.Get(ctx, "key1")

	// Add key4 (should evict key2, which is now least recently used)
	_ = store.Set(ctx, "key4", response, 5*time.Minute)

	// Verify key2 was evicted
	_, found := store.Get(ctx, "key2")
	if found {
		t.Error("Expected key2 to be evicted (it was least recently used)")
	}

	// Verify key1, key3, key4 still exist
	for _, key := range []string{"key1", "key3", "key4"} {
		_, found := store.Get(ctx, key)
		if !found {
			t.Errorf("Expected to find %s", key)
		}
	}
}
