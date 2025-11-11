package callbacks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/rs/zerolog"
)

func TestRetryableClient_SuccessFirstAttempt(t *testing.T) {
	// Server that succeeds immediately
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.CallbacksConfig{
		PaymentSuccessURL: server.URL,
		Timeout:           config.Duration{Duration: 3 * time.Second},
		Retry: config.RetryConfig{
			Enabled: true,
		},
	}

	dlqStore := NewMemoryDLQStore()
	client := NewRetryableClient(cfg,
		WithRetryLogger(zerolog.Nop()),
		WithDLQStore(dlqStore),
		WithRetryConfig(RetryConfig{
			MaxAttempts:     3,
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     100 * time.Millisecond,
			Multiplier:      2.0,
			Timeout:         1 * time.Second,
		}),
	)

	event := PaymentEvent{
		ResourceID:         "test-resource",
		Method:             "x402",
		CryptoAtomicAmount: 10000000, // 10.0 USDC (6 decimals)
		CryptoToken:        "USDC",
		Wallet:             "test-wallet",
	}

	// Send event (async)
	client.PaymentSucceeded(context.Background(), event)

	// Wait for webhook to complete
	time.Sleep(200 * time.Millisecond)

	// Should succeed on first attempt
	if count := requestCount.Load(); count != 1 {
		t.Errorf("Expected 1 request, got %d", count)
	}

	// DLQ should be empty
	dlqItems, _ := dlqStore.ListFailedWebhooks(context.Background(), 100)
	if len(dlqItems) != 0 {
		t.Errorf("Expected empty DLQ, got %d items", len(dlqItems))
	}
}

func TestRetryableClient_RetryAfterFailures(t *testing.T) {
	// Server that fails first 2 attempts, then succeeds
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.CallbacksConfig{
		PaymentSuccessURL: server.URL,
		Timeout:           config.Duration{Duration: 3 * time.Second},
		Retry: config.RetryConfig{
			Enabled: true,
		},
	}

	dlqStore := NewMemoryDLQStore()
	client := NewRetryableClient(cfg,
		WithRetryLogger(zerolog.Nop()),
		WithDLQStore(dlqStore),
		WithRetryConfig(RetryConfig{
			MaxAttempts:     5,
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     100 * time.Millisecond,
			Multiplier:      2.0,
			Timeout:         1 * time.Second,
		}),
	)

	event := PaymentEvent{
		ResourceID:         "test-resource",
		Method:             "x402",
		CryptoAtomicAmount: 10000000, // 10.0 USDC (6 decimals)
		CryptoToken:        "USDC",
	}

	client.PaymentSucceeded(context.Background(), event)

	// Wait for retries to complete
	time.Sleep(500 * time.Millisecond)

	// Should retry until success (3 attempts total)
	if count := requestCount.Load(); count != 3 {
		t.Errorf("Expected 3 requests, got %d", count)
	}

	// DLQ should be empty (eventually succeeded)
	dlqItems, _ := dlqStore.ListFailedWebhooks(context.Background(), 100)
	if len(dlqItems) != 0 {
		t.Errorf("Expected empty DLQ, got %d items", len(dlqItems))
	}
}

func TestRetryableClient_ExhaustsRetriesAndSavesToDLQ(t *testing.T) {
	// Server that always fails
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	cfg := config.CallbacksConfig{
		PaymentSuccessURL: server.URL,
		Timeout:           config.Duration{Duration: 3 * time.Second},
		Retry: config.RetryConfig{
			Enabled: true,
		},
	}

	dlqStore := NewMemoryDLQStore()
	client := NewRetryableClient(cfg,
		WithRetryLogger(zerolog.Nop()),
		WithDLQStore(dlqStore),
		WithRetryConfig(RetryConfig{
			MaxAttempts:     3,
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     100 * time.Millisecond,
			Multiplier:      2.0,
			Timeout:         1 * time.Second,
		}),
	)

	event := PaymentEvent{
		ResourceID:   "test-resource",
		Method:       "stripe",
		FiatAmountCents: 100,
		FiatCurrency: "usd",
	}

	client.PaymentSucceeded(context.Background(), event)

	// Wait for all retries to exhaust
	time.Sleep(500 * time.Millisecond)

	// Should attempt MaxAttempts times
	if count := requestCount.Load(); count != 3 {
		t.Errorf("Expected 3 requests, got %d", count)
	}

	// DLQ should have 1 item
	dlqItems, _ := dlqStore.ListFailedWebhooks(context.Background(), 100)
	if len(dlqItems) != 1 {
		t.Fatalf("Expected 1 DLQ item, got %d", len(dlqItems))
	}

	// Verify DLQ item
	dlqItem := dlqItems[0]
	if dlqItem.EventType != "payment" {
		t.Errorf("Expected eventType 'payment', got %q", dlqItem.EventType)
	}
	if dlqItem.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", dlqItem.Attempts)
	}
	if dlqItem.URL != server.URL {
		t.Errorf("Expected URL %q, got %q", server.URL, dlqItem.URL)
	}

	// Verify payload is valid JSON
	var savedEvent PaymentEvent
	if err := json.Unmarshal(dlqItem.Payload, &savedEvent); err != nil {
		t.Errorf("Failed to unmarshal DLQ payload: %v", err)
	}
	if savedEvent.ResourceID != "test-resource" {
		t.Errorf("Expected ResourceID 'test-resource', got %q", savedEvent.ResourceID)
	}
}

func TestRetryableClient_RefundSucceeded(t *testing.T) {
	// Server that succeeds
	var requestCount atomic.Int32
	var receivedPayload []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		receivedPayload = buf
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.CallbacksConfig{
		PaymentSuccessURL: server.URL,
		Timeout:           config.Duration{Duration: 3 * time.Second},
		Retry: config.RetryConfig{
			Enabled: true,
		},
	}

	client := NewRetryableClient(cfg,
		WithRetryLogger(zerolog.Nop()),
		WithRetryConfig(RetryConfig{
			MaxAttempts:     3,
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     100 * time.Millisecond,
			Multiplier:      2.0,
			Timeout:         1 * time.Second,
		}),
	)

	event := RefundEvent{
		RefundID:           "refund_123",
		OriginalPurchaseID: "purchase_456",
		RecipientWallet:    "wallet_abc",
		AtomicAmount:       25500000, // 25.5 USDC (6 decimals)
		Token:              "USDC",
		ProcessedBy:        "server_wallet",
		Signature:          "tx_sig",
	}

	client.RefundSucceeded(context.Background(), event)

	// Wait for webhook
	time.Sleep(200 * time.Millisecond)

	// Should succeed on first attempt
	if count := requestCount.Load(); count != 1 {
		t.Errorf("Expected 1 request, got %d", count)
	}

	// Verify payload
	var receivedEvent RefundEvent
	if err := json.Unmarshal(receivedPayload, &receivedEvent); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}
	if receivedEvent.RefundID != "refund_123" {
		t.Errorf("Expected RefundID 'refund_123', got %q", receivedEvent.RefundID)
	}
}

func TestRetryableClient_NoopWhenURLEmpty(t *testing.T) {
	cfg := config.CallbacksConfig{
		PaymentSuccessURL: "", // Empty URL
		Timeout:           config.Duration{Duration: 3 * time.Second},
	}

	client := NewRetryableClient(cfg)

	// Should return NoopNotifier
	if _, ok := client.(NoopNotifier); !ok {
		t.Error("NewRetryableClient() with empty URL should return NoopNotifier")
	}
}

func TestRetryableClient_ExponentialBackoff(t *testing.T) {
	// Server that counts attempts and records timing
	var requestCount atomic.Int32
	var firstAttempt time.Time
	var lastAttempt time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 1 {
			firstAttempt = time.Now()
		}
		lastAttempt = time.Now()
		w.WriteHeader(http.StatusServiceUnavailable) // Always fail
	}))
	defer server.Close()

	cfg := config.CallbacksConfig{
		PaymentSuccessURL: server.URL,
		Timeout:           config.Duration{Duration: 3 * time.Second},
		Retry: config.RetryConfig{
			Enabled: true,
		},
	}

	client := NewRetryableClient(cfg,
		WithRetryLogger(zerolog.Nop()),
		WithDLQStore(NewMemoryDLQStore()),
		WithRetryConfig(RetryConfig{
			MaxAttempts:     3,
			InitialInterval: 50 * time.Millisecond,
			MaxInterval:     500 * time.Millisecond,
			Multiplier:      2.0,
			Timeout:         1 * time.Second,
		}),
	)

	event := PaymentEvent{
		ResourceID: "test-resource",
	}

	client.PaymentSucceeded(context.Background(), event)

	// Wait for all retries
	time.Sleep(1 * time.Second)

	// Should make 3 attempts
	if count := requestCount.Load(); count != 3 {
		t.Errorf("Expected 3 requests, got %d", count)
	}

	// Verify exponential backoff timing
	// With initial 50ms, multiplier 2.0:
	// Attempt 1: immediate
	// Attempt 2: after 50ms
	// Attempt 3: after 100ms (50ms * 2)
	// Total minimum duration: ~150ms
	duration := lastAttempt.Sub(firstAttempt)
	if duration < 150*time.Millisecond {
		t.Errorf("Expected minimum 150ms between first and last attempt, got %v", duration)
	}
}

func TestMemoryDLQStore(t *testing.T) {
	store := NewMemoryDLQStore()
	ctx := context.Background()

	// Initially empty
	items, err := store.ListFailedWebhooks(ctx, 100)
	if err != nil {
		t.Fatalf("ListFailedWebhooks failed: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("Expected empty store, got %d items", len(items))
	}

	// Save webhook
	webhook := FailedWebhook{
		ID:          "webhook_1",
		URL:         "http://example.com/webhook",
		Payload:     json.RawMessage(`{"test":"data"}`),
		EventType:   "payment",
		Attempts:    5,
		LastError:   "connection refused",
		LastAttempt: time.Now(),
		CreatedAt:   time.Now(),
	}

	if err := store.SaveFailedWebhook(ctx, webhook); err != nil {
		t.Fatalf("SaveFailedWebhook failed: %v", err)
	}

	// List webhooks
	items, err = store.ListFailedWebhooks(ctx, 100)
	if err != nil {
		t.Fatalf("ListFailedWebhooks failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(items))
	}
	if items[0].ID != "webhook_1" {
		t.Errorf("Expected ID 'webhook_1', got %q", items[0].ID)
	}

	// Delete webhook
	if err := store.DeleteFailedWebhook(ctx, "webhook_1"); err != nil {
		t.Fatalf("DeleteFailedWebhook failed: %v", err)
	}

	// Should be empty again
	items, err = store.ListFailedWebhooks(ctx, 100)
	if err != nil {
		t.Fatalf("ListFailedWebhooks failed: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("Expected empty store after delete, got %d items", len(items))
	}
}

func TestFileDLQStore(t *testing.T) {
	// Use temp file
	tmpFile := t.TempDir() + "/test-dlq.json"

	store, err := NewFileDLQStore(tmpFile)
	if err != nil {
		t.Fatalf("NewFileDLQStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Save webhook
	webhook := FailedWebhook{
		ID:          "webhook_file_1",
		URL:         "http://example.com/webhook",
		Payload:     json.RawMessage(`{"test":"data"}`),
		EventType:   "refund",
		Attempts:    3,
		LastError:   "timeout",
		LastAttempt: time.Now(),
		CreatedAt:   time.Now(),
	}

	if err := store.SaveFailedWebhook(ctx, webhook); err != nil {
		t.Fatalf("SaveFailedWebhook failed: %v", err)
	}

	// Create new store instance (simulates server restart)
	store2, err := NewFileDLQStore(tmpFile)
	if err != nil {
		t.Fatalf("NewFileDLQStore (reload) failed: %v", err)
	}
	defer store2.Close()

	// Should load persisted data
	items, err := store2.ListFailedWebhooks(ctx, 100)
	if err != nil {
		t.Fatalf("ListFailedWebhooks failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Expected 1 persisted item, got %d", len(items))
	}
	if items[0].ID != "webhook_file_1" {
		t.Errorf("Expected ID 'webhook_file_1', got %q", items[0].ID)
	}
}

func TestNoopDLQStore(t *testing.T) {
	store := NoopDLQStore{}
	ctx := context.Background()

	// Should accept everything without error
	webhook := FailedWebhook{ID: "test"}
	if err := store.SaveFailedWebhook(ctx, webhook); err != nil {
		t.Errorf("NoopDLQStore.SaveFailedWebhook should not error, got %v", err)
	}

	// Should always return empty list
	items, err := store.ListFailedWebhooks(ctx, 100)
	if err != nil {
		t.Errorf("NoopDLQStore.ListFailedWebhooks should not error, got %v", err)
	}
	if len(items) != 0 {
		t.Errorf("NoopDLQStore.ListFailedWebhooks should return empty list, got %d items", len(items))
	}

	// Should accept deletes without error
	if err := store.DeleteFailedWebhook(ctx, "test"); err != nil {
		t.Errorf("NoopDLQStore.DeleteFailedWebhook should not error, got %v", err)
	}
}
