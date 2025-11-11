package storage

import (
	"context"
	"time"

	"github.com/CedrosPay/server/internal/money"
)

// PaymentTransaction represents a verified payment transaction.
// Used for replay protection - ensures each transaction signature is only used ONCE globally.
type PaymentTransaction struct {
	Signature  string            // Transaction signature (unique ID, globally unique)
	ResourceID string            // Resource that was purchased
	Wallet     string            // Wallet that made the payment
	Amount     money.Money       // Amount paid
	CreatedAt  time.Time         // When transaction was verified
	Metadata   map[string]string // Additional metadata
}

// PaymentTransactionStore defines the interface for payment transaction persistence.
// CRITICAL: This is used for replay protection to ensure each signature is only used ONCE,
// regardless of which resource it's being used for. Once a signature is consumed for any
// resource, it CANNOT be reused for any other resource.
type PaymentTransactionStore interface {
	// RecordPayment saves a verified payment transaction.
	// The signature must be globally unique - attempting to record the same signature
	// twice (even for different resources) should fail or be silently ignored.
	RecordPayment(ctx context.Context, tx PaymentTransaction) error

	// HasPaymentBeenProcessed checks if a transaction signature has EVER been used.
	// Returns true if the signature exists for ANY resource (not just the specified one).
	// This prevents cross-resource replay attacks.
	HasPaymentBeenProcessed(ctx context.Context, signature string) (bool, error)

	// GetPayment retrieves a payment transaction by signature.
	// Returns the original payment record, which shows which resource it was used for.
	GetPayment(ctx context.Context, signature string) (PaymentTransaction, error)
}
