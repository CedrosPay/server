package storage

import (
	"context"
	"time"
)

const (
	// DefaultQueryTimeout is the maximum time allowed for database queries.
	// This prevents queries from hanging indefinitely and causing cascading failures.
	DefaultQueryTimeout = 5 * time.Second
)

// withQueryTimeout wraps the context with a query timeout if one isn't already set.
// This ensures all database operations have a reasonable deadline while respecting
// any existing timeout that the caller may have set.
func withQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	// Check if context already has a deadline
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		// Context already has timeout, don't override it
		return ctx, func() {}
	}
	// Add default query timeout
	return context.WithTimeout(ctx, DefaultQueryTimeout)
}
