package observability

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// Mock hook implementations for testing

type mockPaymentHook struct {
	mu              sync.Mutex
	startedEvents   []PaymentStartedEvent
	completedEvents []PaymentCompletedEvent
	settledEvents   []PaymentSettledEvent
	shouldPanic     bool
}

func (h *mockPaymentHook) Name() string { return "mock_payment" }

func (h *mockPaymentHook) OnPaymentStarted(ctx context.Context, event PaymentStartedEvent) {
	if h.shouldPanic {
		panic("intentional panic for testing")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.startedEvents = append(h.startedEvents, event)
}

func (h *mockPaymentHook) OnPaymentCompleted(ctx context.Context, event PaymentCompletedEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.completedEvents = append(h.completedEvents, event)
}

func (h *mockPaymentHook) OnPaymentSettled(ctx context.Context, event PaymentSettledEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.settledEvents = append(h.settledEvents, event)
}

func (h *mockPaymentHook) getStartedCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.startedEvents)
}

func (h *mockPaymentHook) getCompletedCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.completedEvents)
}

type mockWebhookHook struct {
	mu              sync.Mutex
	queuedEvents    []WebhookQueuedEvent
	deliveredEvents []WebhookDeliveredEvent
	failedEvents    []WebhookFailedEvent
	retriedEvents   []WebhookRetriedEvent
}

func (h *mockWebhookHook) Name() string { return "mock_webhook" }

func (h *mockWebhookHook) OnWebhookQueued(ctx context.Context, event WebhookQueuedEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.queuedEvents = append(h.queuedEvents, event)
}

func (h *mockWebhookHook) OnWebhookDelivered(ctx context.Context, event WebhookDeliveredEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deliveredEvents = append(h.deliveredEvents, event)
}

func (h *mockWebhookHook) OnWebhookFailed(ctx context.Context, event WebhookFailedEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failedEvents = append(h.failedEvents, event)
}

func (h *mockWebhookHook) OnWebhookRetried(ctx context.Context, event WebhookRetriedEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.retriedEvents = append(h.retriedEvents, event)
}

func (h *mockWebhookHook) getDeliveredCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.deliveredEvents)
}

// Tests

func TestRegistry_RegisterAndEmitPayment(t *testing.T) {
	logger := zerolog.Nop()
	registry := NewRegistry(logger)

	hook := &mockPaymentHook{}
	registry.RegisterPaymentHook(hook)

	ctx := context.Background()

	// Test OnPaymentStarted
	startedEvent := PaymentStartedEvent{
		Timestamp:  time.Now(),
		PaymentID:  "pay_123",
		Method:     "x402",
		ResourceID: "resource_1",
		Amount:     1000,
		Token:      "USDC",
	}
	registry.EmitPaymentStarted(ctx, startedEvent)

	if hook.getStartedCount() != 1 {
		t.Errorf("Expected 1 started event, got %d", hook.getStartedCount())
	}

	// Test OnPaymentCompleted
	completedEvent := PaymentCompletedEvent{
		Timestamp:  time.Now(),
		PaymentID:  "pay_123",
		Method:     "x402",
		ResourceID: "resource_1",
		Success:    true,
		Amount:     1000,
		Token:      "USDC",
		Duration:   100 * time.Millisecond,
	}
	registry.EmitPaymentCompleted(ctx, completedEvent)

	if hook.getCompletedCount() != 1 {
		t.Errorf("Expected 1 completed event, got %d", hook.getCompletedCount())
	}
}

func TestRegistry_MultipleHooks(t *testing.T) {
	logger := zerolog.Nop()
	registry := NewRegistry(logger)

	hook1 := &mockPaymentHook{}
	hook2 := &mockPaymentHook{}

	registry.RegisterPaymentHook(hook1)
	registry.RegisterPaymentHook(hook2)

	ctx := context.Background()
	event := PaymentStartedEvent{
		Timestamp: time.Now(),
		PaymentID: "pay_456",
		Method:    "stripe",
	}

	registry.EmitPaymentStarted(ctx, event)

	// Both hooks should receive the event
	if hook1.getStartedCount() != 1 {
		t.Errorf("Hook1: Expected 1 started event, got %d", hook1.getStartedCount())
	}
	if hook2.getStartedCount() != 1 {
		t.Errorf("Hook2: Expected 1 started event, got %d", hook2.getStartedCount())
	}
}

func TestRegistry_PanicRecovery(t *testing.T) {
	logger := zerolog.Nop()
	registry := NewRegistry(logger)

	// Hook that panics
	panicHook := &mockPaymentHook{shouldPanic: true}
	normalHook := &mockPaymentHook{}

	registry.RegisterPaymentHook(panicHook)
	registry.RegisterPaymentHook(normalHook)

	ctx := context.Background()
	event := PaymentStartedEvent{
		Timestamp: time.Now(),
		PaymentID: "pay_789",
	}

	// Should not panic - panic should be recovered
	registry.EmitPaymentStarted(ctx, event)

	// Normal hook should still receive event
	if normalHook.getStartedCount() != 1 {
		t.Errorf("Normal hook should still receive event after panic, got %d events", normalHook.getStartedCount())
	}
}

func TestRegistry_WebhookHooks(t *testing.T) {
	logger := zerolog.Nop()
	registry := NewRegistry(logger)

	hook := &mockWebhookHook{}
	registry.RegisterWebhookHook(hook)

	ctx := context.Background()

	// Test webhook delivered
	deliveredEvent := WebhookDeliveredEvent{
		Timestamp: time.Now(),
		WebhookID: "wh_123",
		EventType: "payment",
		URL:       "https://example.com/webhook",
		Attempts:  2,
		Duration:  50 * time.Millisecond,
	}
	registry.EmitWebhookDelivered(ctx, deliveredEvent)

	if hook.getDeliveredCount() != 1 {
		t.Errorf("Expected 1 delivered event, got %d", hook.getDeliveredCount())
	}
}

func TestRegistry_ConcurrentEmissions(t *testing.T) {
	logger := zerolog.Nop()
	registry := NewRegistry(logger)

	hook := &mockPaymentHook{}
	registry.RegisterPaymentHook(hook)

	ctx := context.Background()

	// Emit events concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			event := PaymentStartedEvent{
				Timestamp: time.Now(),
				PaymentID: "pay_" + string(rune('0'+id)),
			}
			registry.EmitPaymentStarted(ctx, event)
		}(i)
	}

	wg.Wait()

	if hook.getStartedCount() != 100 {
		t.Errorf("Expected 100 started events, got %d", hook.getStartedCount())
	}
}
