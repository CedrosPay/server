package idempotency

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddleware_NoIdempotencyKey(t *testing.T) {
	store := NewMemoryStore()
	handler := Middleware(store, 1*time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("POST", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Idempotency-Replay") != "" {
		t.Error("expected no replay header")
	}
	if rec.Body.String() != "success" {
		t.Errorf("expected 'success', got %s", rec.Body.String())
	}
}

func TestMiddleware_FirstRequest(t *testing.T) {
	store := NewMemoryStore()
	callCount := 0
	handler := Middleware(store, 1*time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("first request"))
	}))

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "test-key-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Idempotency-Replay") != "" {
		t.Error("expected no replay header on first request")
	}
	if rec.Body.String() != "first request" {
		t.Errorf("expected 'first request', got %s", rec.Body.String())
	}
	if callCount != 1 {
		t.Errorf("expected handler to be called once, got %d", callCount)
	}
}

func TestMiddleware_CachedResponse(t *testing.T) {
	store := NewMemoryStore()
	callCount := 0
	handler := Middleware(store, 1*time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("original response"))
	}))

	// First request
	req1 := httptest.NewRequest("POST", "/test", nil)
	req1.Header.Set("Idempotency-Key", "test-key-2")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Second request with same key
	req2 := httptest.NewRequest("POST", "/test", nil)
	req2.Header.Set("Idempotency-Key", "test-key-2")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	// Verify second response is cached
	if rec2.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-Idempotency-Replay") != "true" {
		t.Error("expected replay header on cached response")
	}
	if rec2.Body.String() != "original response" {
		t.Errorf("expected 'original response', got %s", rec2.Body.String())
	}
	if callCount != 1 {
		t.Errorf("expected handler to be called once, got %d times", callCount)
	}
}

func TestMiddleware_DifferentKeys(t *testing.T) {
	store := NewMemoryStore()
	callCount := 0
	handler := Middleware(store, 1*time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))

	// First request
	req1 := httptest.NewRequest("POST", "/test", nil)
	req1.Header.Set("Idempotency-Key", "key-1")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Second request with different key
	req2 := httptest.NewRequest("POST", "/test", nil)
	req2.Header.Set("Idempotency-Key", "key-2")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	// Both should execute
	if callCount != 2 {
		t.Errorf("expected handler to be called twice, got %d times", callCount)
	}
	if rec1.Header().Get("X-Idempotency-Replay") != "" {
		t.Error("expected no replay header on first request")
	}
	if rec2.Header().Get("X-Idempotency-Replay") != "" {
		t.Error("expected no replay header on second request with different key")
	}
}

func TestMiddleware_OnlyCachesSuccessful(t *testing.T) {
	store := NewMemoryStore()
	callCount := 0
	handler := Middleware(store, 1*time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("error"))
	}))

	// First request (error)
	req1 := httptest.NewRequest("POST", "/test", nil)
	req1.Header.Set("Idempotency-Key", "test-key-3")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Second request with same key
	req2 := httptest.NewRequest("POST", "/test", nil)
	req2.Header.Set("Idempotency-Key", "test-key-3")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	// Both should execute (error responses not cached)
	if callCount != 2 {
		t.Errorf("expected handler to be called twice for error responses, got %d times", callCount)
	}
	if rec2.Header().Get("X-Idempotency-Replay") != "" {
		t.Error("expected no replay header for error response")
	}
}

func TestMiddleware_PreservesHeaders(t *testing.T) {
	store := NewMemoryStore()
	handler := Middleware(store, 1*time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom-Header", "custom-value")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	// First request
	req1 := httptest.NewRequest("POST", "/test", nil)
	req1.Header.Set("Idempotency-Key", "test-key-4")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Second request (cached)
	req2 := httptest.NewRequest("POST", "/test", nil)
	req2.Header.Set("Idempotency-Key", "test-key-4")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	// Verify headers preserved
	if rec2.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type header preserved, got %s", rec2.Header().Get("Content-Type"))
	}
	if rec2.Header().Get("X-Custom-Header") != "custom-value" {
		t.Errorf("expected custom header preserved, got %s", rec2.Header().Get("X-Custom-Header"))
	}
	if rec2.Header().Get("X-Idempotency-Replay") != "true" {
		t.Error("expected replay header on cached response")
	}
}

func TestMiddleware_CustomTTL(t *testing.T) {
	store := NewMemoryStore()
	handler := Middleware(store, 100*time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))

	// First request
	req1 := httptest.NewRequest("POST", "/test", nil)
	req1.Header.Set("Idempotency-Key", "test-key-5")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Second request after expiry
	req2 := httptest.NewRequest("POST", "/test", nil)
	req2.Header.Set("Idempotency-Key", "test-key-5")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	// Should not be cached (expired)
	if rec2.Header().Get("X-Idempotency-Replay") != "" {
		t.Error("expected no replay header after TTL expiry")
	}
}

func TestMiddleware_DefaultTTL(t *testing.T) {
	store := NewMemoryStore()
	// Pass 0 to use default TTL
	handler := Middleware(store, 0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "test-key-6")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Verify response works (default TTL applied)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestStore_GetSetDelete(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Test Get on non-existent key
	_, found := store.Get(ctx, "nonexistent")
	if found {
		t.Error("expected not found for nonexistent key")
	}

	// Test Set and Get
	resp := &Response{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte("test body"),
		CachedAt:   time.Now(),
	}
	err := store.Set(ctx, "test-key", resp, 1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retrieved, found := store.Get(ctx, "test-key")
	if !found {
		t.Fatal("expected to find cached response")
	}
	if retrieved.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", retrieved.StatusCode)
	}
	if !bytes.Equal(retrieved.Body, []byte("test body")) {
		t.Errorf("expected body 'test body', got %s", retrieved.Body)
	}

	// Test Delete
	err = store.Delete(ctx, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, found = store.Get(ctx, "test-key")
	if found {
		t.Error("expected not found after delete")
	}
}

func TestStore_Expiration(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	resp := &Response{
		StatusCode: 200,
		Body:       []byte("test"),
		CachedAt:   time.Now(),
	}

	// Set with short TTL
	err := store.Set(ctx, "expire-key", resp, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be found immediately
	_, found := store.Get(ctx, "expire-key")
	if !found {
		t.Error("expected to find key immediately after set")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should not be found after expiration
	_, found = store.Get(ctx, "expire-key")
	if found {
		t.Error("expected key to be expired")
	}
}
