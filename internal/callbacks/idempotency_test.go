package callbacks

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateEventID(t *testing.T) {
	// Generate multiple event IDs
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateEventID()

		// Check format: "evt_" + 24 hex chars
		if !strings.HasPrefix(id, "evt_") {
			t.Errorf("EventID missing 'evt_' prefix: %s", id)
		}

		hexPart := strings.TrimPrefix(id, "evt_")
		if len(hexPart) != 24 {
			t.Errorf("EventID hex part wrong length (expected 24, got %d): %s", len(hexPart), id)
		}

		// Check for hex characters only
		for _, c := range hexPart {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("EventID contains non-hex character '%c': %s", c, id)
			}
		}

		// Check uniqueness
		if ids[id] {
			t.Errorf("Duplicate EventID generated: %s", id)
		}
		ids[id] = true
	}

	// Verify we generated 1000 unique IDs
	if len(ids) != 1000 {
		t.Errorf("Expected 1000 unique IDs, got %d", len(ids))
	}
}

func TestPreparePaymentEvent(t *testing.T) {
	tests := []struct {
		name  string
		event PaymentEvent
		check func(t *testing.T, event PaymentEvent)
	}{
		{
			name:  "generates event ID when missing",
			event: PaymentEvent{ResourceID: "test-resource"},
			check: func(t *testing.T, event PaymentEvent) {
				if event.EventID == "" {
					t.Error("EventID not generated")
				}
				if !strings.HasPrefix(event.EventID, "evt_") {
					t.Errorf("EventID has wrong format: %s", event.EventID)
				}
			},
		},
		{
			name:  "preserves existing event ID",
			event: PaymentEvent{EventID: "evt_existing123", ResourceID: "test"},
			check: func(t *testing.T, event PaymentEvent) {
				if event.EventID != "evt_existing123" {
					t.Errorf("EventID changed from evt_existing123 to %s", event.EventID)
				}
			},
		},
		{
			name:  "sets event type to payment.succeeded",
			event: PaymentEvent{ResourceID: "test"},
			check: func(t *testing.T, event PaymentEvent) {
				if event.EventType != "payment.succeeded" {
					t.Errorf("EventType = %s, want payment.succeeded", event.EventType)
				}
			},
		},
		{
			name:  "preserves existing event type",
			event: PaymentEvent{EventType: "custom.event", ResourceID: "test"},
			check: func(t *testing.T, event PaymentEvent) {
				if event.EventType != "custom.event" {
					t.Errorf("EventType changed from custom.event to %s", event.EventType)
				}
			},
		},
		{
			name:  "sets event timestamp when missing",
			event: PaymentEvent{ResourceID: "test"},
			check: func(t *testing.T, event PaymentEvent) {
				if event.EventTimestamp.IsZero() {
					t.Error("EventTimestamp not set")
				}
				// Should be recent (within last second)
				if time.Since(event.EventTimestamp) > time.Second {
					t.Errorf("EventTimestamp too old: %v", event.EventTimestamp)
				}
			},
		},
		{
			name: "preserves existing event timestamp",
			event: PaymentEvent{
				EventTimestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				ResourceID:     "test",
			},
			check: func(t *testing.T, event PaymentEvent) {
				expected := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
				if !event.EventTimestamp.Equal(expected) {
					t.Errorf("EventTimestamp changed from %v to %v", expected, event.EventTimestamp)
				}
			},
		},
		{
			name:  "sets paid at when missing",
			event: PaymentEvent{ResourceID: "test"},
			check: func(t *testing.T, event PaymentEvent) {
				if event.PaidAt.IsZero() {
					t.Error("PaidAt not set")
				}
			},
		},
		{
			name: "preserves existing paid at",
			event: PaymentEvent{
				PaidAt:     time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
				ResourceID: "test",
			},
			check: func(t *testing.T, event PaymentEvent) {
				expected := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
				if !event.PaidAt.Equal(expected) {
					t.Errorf("PaidAt changed from %v to %v", expected, event.PaidAt)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			PreparePaymentEvent(&tt.event)
			tt.check(t, tt.event)
		})
	}
}

func TestPrepareRefundEvent(t *testing.T) {
	tests := []struct {
		name  string
		event RefundEvent
		check func(t *testing.T, event RefundEvent)
	}{
		{
			name:  "generates event ID when missing",
			event: RefundEvent{RefundID: "refund-123"},
			check: func(t *testing.T, event RefundEvent) {
				if event.EventID == "" {
					t.Error("EventID not generated")
				}
				if !strings.HasPrefix(event.EventID, "evt_") {
					t.Errorf("EventID has wrong format: %s", event.EventID)
				}
			},
		},
		{
			name:  "preserves existing event ID",
			event: RefundEvent{EventID: "evt_refund_abc", RefundID: "refund-123"},
			check: func(t *testing.T, event RefundEvent) {
				if event.EventID != "evt_refund_abc" {
					t.Errorf("EventID changed from evt_refund_abc to %s", event.EventID)
				}
			},
		},
		{
			name:  "sets event type to refund.succeeded",
			event: RefundEvent{RefundID: "refund-123"},
			check: func(t *testing.T, event RefundEvent) {
				if event.EventType != "refund.succeeded" {
					t.Errorf("EventType = %s, want refund.succeeded", event.EventType)
				}
			},
		},
		{
			name:  "sets event timestamp when missing",
			event: RefundEvent{RefundID: "refund-123"},
			check: func(t *testing.T, event RefundEvent) {
				if event.EventTimestamp.IsZero() {
					t.Error("EventTimestamp not set")
				}
			},
		},
		{
			name:  "sets refunded at when missing",
			event: RefundEvent{RefundID: "refund-123"},
			check: func(t *testing.T, event RefundEvent) {
				if event.RefundedAt.IsZero() {
					t.Error("RefundedAt not set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			PrepareRefundEvent(&tt.event)
			tt.check(t, tt.event)
		})
	}
}

func TestIdempotencyAcrossRetries(t *testing.T) {
	// Simulate the same event being prepared multiple times (as would happen in retries)
	event := PaymentEvent{
		ResourceID: "test-resource",
		Method:     "x402",
	}

	// First preparation (initial send)
	PreparePaymentEvent(&event)
	firstEventID := event.EventID
	firstTimestamp := event.EventTimestamp

	if firstEventID == "" {
		t.Fatal("First preparation did not generate EventID")
	}

	// Simulate retry - prepare the SAME event again
	PreparePaymentEvent(&event)
	secondEventID := event.EventID
	secondTimestamp := event.EventTimestamp

	// EventID MUST be preserved across retries
	if secondEventID != firstEventID {
		t.Errorf("EventID changed on retry: %s → %s (BREAKS IDEMPOTENCY!)", firstEventID, secondEventID)
	}

	// Timestamp MUST be preserved across retries
	if !secondTimestamp.Equal(firstTimestamp) {
		t.Errorf("EventTimestamp changed on retry: %v → %v", firstTimestamp, secondTimestamp)
	}
}

func TestMultipleEventsGetUniqueIDs(t *testing.T) {
	// Generate 100 different payment events
	eventIDs := make(map[string]bool)

	for i := 0; i < 100; i++ {
		event := PaymentEvent{
			ResourceID: "test-resource",
			Method:     "x402",
		}
		PreparePaymentEvent(&event)

		// Each event should get a unique ID
		if eventIDs[event.EventID] {
			t.Errorf("Duplicate EventID generated: %s", event.EventID)
		}
		eventIDs[event.EventID] = true
	}

	if len(eventIDs) != 100 {
		t.Errorf("Expected 100 unique event IDs, got %d", len(eventIDs))
	}
}

func BenchmarkGenerateEventID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateEventID()
	}
}

func BenchmarkPreparePaymentEvent(b *testing.B) {
	for i := 0; i < b.N; i++ {
		event := PaymentEvent{ResourceID: "test"}
		PreparePaymentEvent(&event)
	}
}
