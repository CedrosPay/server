package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/CedrosPay/server/internal/money"
)

// RefundQuote represents a generated refund quote with payment details.
type RefundQuote struct {
	ID                 string
	OriginalPurchaseID string // Reference to original purchase (resource ID, cart ID, session ID)
	RecipientWallet    string // Wallet receiving the refund
	Amount             money.Money
	Reason             string
	Metadata           map[string]string
	CreatedAt          time.Time
	ExpiresAt          time.Time
	ProcessedBy        string // Wallet that executed the refund
	ProcessedAt        *time.Time
	Signature          string // Transaction signature
}

// IsExpiredAt returns true if the refund quote's transaction execution window has passed at the given moment.
// This means the blockhash is expired and the transaction cannot be executed.
// NOTE: This does NOT mean the refund request is deleted - it remains in storage
// and can be re-quoted or denied by an admin.
func (r *RefundQuote) IsExpiredAt(now time.Time) bool {
	return now.After(r.ExpiresAt)
}

// IsProcessed returns true if the refund has been completed.
func (r *RefundQuote) IsProcessed() bool {
	return r.ProcessedAt != nil && r.Signature != ""
}

// GenerateRefundID creates a cryptographically random refund identifier.
func GenerateRefundID() (string, error) {
	b := make([]byte, 16) // 128 bits of randomness
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refund id: %w", err)
	}
	return "refund_" + hex.EncodeToString(b), nil
}
