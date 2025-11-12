package observability

import (
	"context"
	"time"
)

// Hook is the base interface for all observability hooks.
// Implementations can emit events to DataDog, New Relic, OpenTelemetry, etc.
type Hook interface {
	// Name returns the hook's identifier for logging/debugging
	Name() string
}

// PaymentHook receives events during the payment lifecycle.
type PaymentHook interface {
	Hook

	// OnPaymentStarted is called when a payment request is received.
	OnPaymentStarted(ctx context.Context, event PaymentStartedEvent)

	// OnPaymentCompleted is called when a payment succeeds or fails.
	OnPaymentCompleted(ctx context.Context, event PaymentCompletedEvent)

	// OnPaymentSettled is called when payment is confirmed on-chain.
	OnPaymentSettled(ctx context.Context, event PaymentSettledEvent)
}

// WebhookHook receives events during webhook delivery.
type WebhookHook interface {
	Hook

	// OnWebhookQueued is called when a webhook is added to the delivery queue.
	OnWebhookQueued(ctx context.Context, event WebhookQueuedEvent)

	// OnWebhookDelivered is called when a webhook is successfully delivered.
	OnWebhookDelivered(ctx context.Context, event WebhookDeliveredEvent)

	// OnWebhookFailed is called when a webhook delivery fails.
	OnWebhookFailed(ctx context.Context, event WebhookFailedEvent)

	// OnWebhookRetried is called when a webhook is retried.
	OnWebhookRetried(ctx context.Context, event WebhookRetriedEvent)
}

// RefundHook receives events during the refund lifecycle.
type RefundHook interface {
	Hook

	// OnRefundRequested is called when a refund is requested.
	OnRefundRequested(ctx context.Context, event RefundRequestedEvent)

	// OnRefundProcessed is called when a refund is processed (success or failure).
	OnRefundProcessed(ctx context.Context, event RefundProcessedEvent)
}

// CartHook receives events during cart operations.
type CartHook interface {
	Hook

	// OnCartCreated is called when a cart quote is created.
	OnCartCreated(ctx context.Context, event CartCreatedEvent)

	// OnCartCheckout is called when a cart checkout is attempted.
	OnCartCheckout(ctx context.Context, event CartCheckoutEvent)
}

// RPCHook receives events from blockchain RPC calls.
type RPCHook interface {
	Hook

	// OnRPCCall is called before/after an RPC call.
	OnRPCCall(ctx context.Context, event RPCCallEvent)
}

// DatabaseHook receives events from database operations.
type DatabaseHook interface {
	Hook

	// OnDatabaseQuery is called for database queries.
	OnDatabaseQuery(ctx context.Context, event DatabaseQueryEvent)
}

// ===============================================
// Event Types
// ===============================================

// PaymentStartedEvent is emitted when a payment request is received.
type PaymentStartedEvent struct {
	Timestamp  time.Time
	PaymentID  string
	Method     string // "stripe" or "x402"
	ResourceID string
	Amount     int64  // Amount in atomic units (e.g., cents, lamports)
	Token      string // Currency/token (e.g., "USD", "USDC")
	Wallet     string // Payer wallet address (for x402)
	Metadata   map[string]string
}

// PaymentCompletedEvent is emitted when a payment completes.
type PaymentCompletedEvent struct {
	Timestamp     time.Time
	PaymentID     string
	Method        string
	ResourceID    string
	Success       bool
	ErrorReason   string // Set if Success=false
	Amount        int64
	Token         string
	Wallet        string
	Duration      time.Duration // Time from start to completion
	TransactionID string        // Blockchain tx signature or Stripe session ID
	Metadata      map[string]string
}

// PaymentSettledEvent is emitted when on-chain settlement is confirmed.
type PaymentSettledEvent struct {
	Timestamp          time.Time
	PaymentID          string
	Network            string // "solana-mainnet", "ethereum-mainnet", etc.
	TransactionID      string
	Confirmations      int
	SettlementDuration time.Duration // Time from payment to settlement
}

// WebhookQueuedEvent is emitted when a webhook is queued for delivery.
type WebhookQueuedEvent struct {
	Timestamp time.Time
	WebhookID string
	EventType string // "payment" or "refund"
	URL       string
	EventID   string // Idempotency key for the webhook event
	Metadata  map[string]string
}

// WebhookDeliveredEvent is emitted when a webhook is successfully delivered.
type WebhookDeliveredEvent struct {
	Timestamp  time.Time
	WebhookID  string
	EventType  string
	URL        string
	EventID    string
	Attempts   int
	Duration   time.Duration
	StatusCode int
}

// WebhookFailedEvent is emitted when a webhook delivery fails.
type WebhookFailedEvent struct {
	Timestamp    time.Time
	WebhookID    string
	EventType    string
	URL          string
	EventID      string
	Attempts     int
	Error        string
	FinalFailure bool // true if all retries exhausted
}

// WebhookRetriedEvent is emitted when a webhook is scheduled for retry.
type WebhookRetriedEvent struct {
	Timestamp      time.Time
	WebhookID      string
	EventType      string
	URL            string
	EventID        string
	CurrentAttempt int
	MaxAttempts    int
	NextRetryAt    time.Time
	BackoffSeconds float64
}

// RefundRequestedEvent is emitted when a refund is requested.
type RefundRequestedEvent struct {
	Timestamp          time.Time
	RefundID           string
	OriginalPurchaseID string
	RecipientWallet    string
	Amount             int64
	Token              string
	Reason             string
	Metadata           map[string]string
}

// RefundProcessedEvent is emitted when a refund is processed.
type RefundProcessedEvent struct {
	Timestamp          time.Time
	RefundID           string
	OriginalPurchaseID string
	Success            bool
	ErrorReason        string
	Amount             int64
	Token              string
	TransactionID      string // On-chain transaction signature
	Duration           time.Duration
	Metadata           map[string]string
}

// CartCreatedEvent is emitted when a cart quote is created.
type CartCreatedEvent struct {
	Timestamp   time.Time
	CartID      string
	ItemCount   int
	TotalAmount int64
	Token       string
	ExpiresAt   time.Time
	Metadata    map[string]string
}

// CartCheckoutEvent is emitted when a cart checkout is attempted.
type CartCheckoutEvent struct {
	Timestamp     time.Time
	CartID        string
	ItemCount     int
	TotalAmount   int64
	Token         string
	Status        string // "success", "failed", "pending"
	PaymentMethod string
	Wallet        string
	Metadata      map[string]string
}

// RPCCallEvent is emitted for blockchain RPC calls.
type RPCCallEvent struct {
	Timestamp time.Time
	Method    string // "getTransaction", "sendTransaction", etc.
	Network   string // "solana-mainnet", etc.
	Duration  time.Duration
	Success   bool
	ErrorType string // "timeout", "rate_limit", "connection", "not_found", "other"
	Metadata  map[string]string
}

// DatabaseQueryEvent is emitted for database operations.
type DatabaseQueryEvent struct {
	Timestamp time.Time
	Operation string // "get", "list", "save", "delete", etc.
	Backend   string // "postgres", "mongodb", "file", "memory"
	Duration  time.Duration
	Success   bool
	Error     string
	Metadata  map[string]string
}
