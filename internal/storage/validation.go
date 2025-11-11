package storage

import (
	"fmt"
	"time"
)

// validateAndPrepareCartQuote validates required fields and sets default timestamps.
func validateAndPrepareCartQuote(quote *CartQuote, ttl time.Duration) error {
	if quote.ID == "" {
		return fmt.Errorf("cart quote requires id")
	}
	if quote.CreatedAt.IsZero() {
		quote.CreatedAt = time.Now()
	}
	if quote.ExpiresAt.IsZero() {
		// Use provided TTL, fallback to 15 minutes if zero
		if ttl == 0 {
			ttl = 15 * time.Minute
		}
		quote.ExpiresAt = quote.CreatedAt.Add(ttl)
	}
	return nil
}

// validateAndPrepareRefundQuote validates required fields and sets default timestamps.
func validateAndPrepareRefundQuote(quote *RefundQuote, ttl time.Duration) error {
	if quote.ID == "" {
		return fmt.Errorf("refund quote requires id")
	}
	if quote.CreatedAt.IsZero() {
		quote.CreatedAt = time.Now()
	}
	if quote.ExpiresAt.IsZero() {
		// Use provided TTL, fallback to 15 minutes if zero
		if ttl == 0 {
			ttl = 15 * time.Minute
		}
		quote.ExpiresAt = quote.CreatedAt.Add(ttl)
	}
	return nil
}
