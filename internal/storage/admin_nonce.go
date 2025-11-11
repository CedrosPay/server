package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// AdminNonce represents a one-time-use nonce for admin signature replay protection.
// Each nonce can only be consumed once and expires after a TTL.
type AdminNonce struct {
	ID         string     // Unique nonce identifier (UUID)
	Purpose    string     // What action this nonce is for (e.g., "list-pending-refunds")
	CreatedAt  time.Time  // When nonce was created
	ExpiresAt  time.Time  // When nonce expires
	ConsumedAt *time.Time // When nonce was consumed (nil if not yet consumed)
}

// NonceTTL is the time-to-live for admin nonces (5 minutes).
const NonceTTL = 5 * time.Minute

// IsConsumed returns true if this nonce has been used.
func (n AdminNonce) IsConsumed() bool {
	return n.ConsumedAt != nil
}

// IsExpiredAt returns true if this nonce has passed its expiration time at the given moment.
func (n AdminNonce) IsExpiredAt(now time.Time) bool {
	return now.After(n.ExpiresAt)
}

// GenerateNonceID creates a new random nonce ID.
func GenerateNonceID() (string, error) {
	bytes := make([]byte, 16) // 128-bit random value
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random nonce: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
