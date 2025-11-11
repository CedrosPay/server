package callbacks

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/httputil"
)

// Notifier delivers payment events to user-defined callbacks.
type Notifier interface {
	PaymentSucceeded(ctx context.Context, event PaymentEvent)
	RefundSucceeded(ctx context.Context, event RefundEvent)
}

// NoopNotifier ignores all events.
type NoopNotifier struct{}

func (NoopNotifier) PaymentSucceeded(context.Context, PaymentEvent) {}
func (NoopNotifier) RefundSucceeded(context.Context, RefundEvent)   {}

// PaymentEvent encapsulates the essential information about a completed payment.
// IMPORTANT: EventID is the idempotency key - webhook consumers MUST use this to prevent duplicate processing.
type PaymentEvent struct {
	// Idempotency and event metadata (ALWAYS present)
	EventID        string    `json:"eventId"`        // Unique event identifier for idempotency (e.g., "evt_abc123")
	EventType      string    `json:"eventType"`      // Always "payment.succeeded" for this event
	EventTimestamp time.Time `json:"eventTimestamp"` // ISO8601 timestamp when event was created (UTC)

	// Payment details
	ResourceID         string            `json:"resource"`
	Method             string            `json:"method"` // "stripe" or "x402"
	StripeSessionID    string            `json:"stripeSessionId,omitempty"`
	StripeCustomer     string            `json:"stripeCustomer,omitempty"`
	FiatAmountCents    int64             `json:"fiatAmountCents,omitempty"`
	FiatCurrency       string            `json:"fiatCurrency,omitempty"`
	CryptoAtomicAmount int64             `json:"cryptoAtomicAmount,omitempty"`
	CryptoToken        string            `json:"cryptoToken,omitempty"`
	Wallet             string            `json:"wallet,omitempty"`
	ProofSignature     string            `json:"proofSignature,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	PaidAt             time.Time         `json:"paidAt"`
}

// RefundEvent encapsulates the essential information about a completed refund.
// IMPORTANT: EventID is the idempotency key - webhook consumers MUST use this to prevent duplicate processing.
type RefundEvent struct {
	// Idempotency and event metadata (ALWAYS present)
	EventID        string    `json:"eventId"`        // Unique event identifier for idempotency (e.g., "evt_refund_xyz")
	EventType      string    `json:"eventType"`      // Always "refund.succeeded" for this event
	EventTimestamp time.Time `json:"eventTimestamp"` // ISO8601 timestamp when event was created (UTC)

	// Refund details
	RefundID           string            `json:"refundId"`
	OriginalPurchaseID string            `json:"originalPurchaseId"`
	RecipientWallet    string            `json:"recipientWallet"`
	AtomicAmount       int64             `json:"atomicAmount"` // Amount in atomic units (e.g., 10500000 for 10.5 USDC with 6 decimals)
	Token              string            `json:"token"`
	ProcessedBy        string            `json:"processedBy"` // Server wallet that executed refund
	Signature          string            `json:"signature"`
	Reason             string            `json:"reason,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	RefundedAt         time.Time         `json:"refundedAt"`
}

// ErrCallbackDisabled is returned when callbacks are not configured.
var ErrCallbackDisabled = errors.New("callbacks: disabled")

// generateEventID creates a unique event identifier for idempotency.
// Format: "evt_" + 24 hex characters (12 random bytes)
// Example: "evt_a1b2c3d4e5f67890abcdef12"
func generateEventID() string {
	randomBytes := make([]byte, 12)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails (extremely rare)
		return fmt.Sprintf("evt_%d", time.Now().UnixNano())
	}
	return "evt_" + hex.EncodeToString(randomBytes)
}

// prepareEventFields sets common idempotency fields for webhook events.
// Extracted from PreparePaymentEvent and PrepareRefundEvent to eliminate duplication.
func prepareEventFields(eventID *string, eventType *string, eventTimestamp *time.Time, defaultEventType string) {
	if *eventID == "" {
		*eventID = generateEventID()
	}
	if *eventType == "" {
		*eventType = defaultEventType
	}
	if eventTimestamp.IsZero() {
		*eventTimestamp = time.Now().UTC()
	}
}

// PreparePaymentEvent ensures PaymentEvent has required idempotency fields set.
// If EventID is already set, it's preserved (for retries). If not, a new one is generated.
func PreparePaymentEvent(event *PaymentEvent) {
	prepareEventFields(&event.EventID, &event.EventType, &event.EventTimestamp, "payment.succeeded")
	if event.PaidAt.IsZero() {
		event.PaidAt = time.Now().UTC()
	}
}

// PrepareRefundEvent ensures RefundEvent has required idempotency fields set.
// If EventID is already set, it's preserved (for retries). If not, a new one is generated.
func PrepareRefundEvent(event *RefundEvent) {
	prepareEventFields(&event.EventID, &event.EventType, &event.EventTimestamp, "refund.succeeded")
	if event.RefundedAt.IsZero() {
		event.RefundedAt = time.Now().UTC()
	}
}

// SendOnce sends a payment event webhook without retry logic (for testing/CLI tools).
func SendOnce(ctx context.Context, cfg config.CallbacksConfig, event PaymentEvent) error {
	if cfg.PaymentSuccessURL == "" {
		return ErrCallbackDisabled
	}

	// Ensure idempotency fields are set
	PreparePaymentEvent(&event)

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	timeout := cfg.Timeout.Duration
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	httpClient := httputil.NewClient(timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.PaymentSuccessURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	contentType := cfg.Headers["Content-Type"]
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)

	for k, v := range cfg.Headers {
		if k == "" || k == "Content-Type" {
			continue
		}
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("received status %d from %s", resp.StatusCode, cfg.PaymentSuccessURL)
	}

	return nil
}
