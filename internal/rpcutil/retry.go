package rpcutil

import (
	"context"
	"strings"
	"time"

	"github.com/CedrosPay/server/internal/logger"
)

// retryConfig defines retry behavior for RPC operations.
type retryConfig struct {
	maxRetries int
	baseDelay  time.Duration
}

// defaultRetryConfig returns sensible defaults for RPC retries.
func defaultRetryConfig() retryConfig {
	return retryConfig{
		maxRetries: 3,
		baseDelay:  100 * time.Millisecond,
	}
}

// WithRetry wraps an RPC operation with retry logic using exponential backoff.
// It retries on transient errors like network issues and rate limits.
func WithRetry[T any](ctx context.Context, operation func() (T, error)) (T, error) {
	return WithRetryCustom(ctx, defaultRetryConfig(), operation)
}

// WithRetryCustom allows custom retry configuration.
func WithRetryCustom[T any](ctx context.Context, cfg retryConfig, operation func() (T, error)) (T, error) {
	var result T
	var err error

	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		result, err = operation()
		if err == nil {
			return result, nil
		}

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return result, err
		}

		// Check if error is retryable
		if !isRetryableError(err) {
			return result, err
		}

		// Last attempt - don't sleep
		if attempt == cfg.maxRetries {
			break
		}

		// Exponential backoff: 100ms, 200ms, 400ms
		delay := cfg.baseDelay * time.Duration(1<<uint(attempt))
		log := logger.FromContext(ctx)
		log.Warn().
			Err(err).
			Int("attempt", attempt+1).
			Int("max_attempts", cfg.maxRetries+1).
			Dur("retry_delay", delay).
			Msg("rpc.operation_retry")

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, ctx.Err()
		case <-timer.C:
			// Continue to next attempt
		}
	}

	return result, err
}

// isRetryableError determines if an error is worth retrying.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	// Network errors
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporary failure") ||
		strings.Contains(msg, "network") {
		return true
	}

	// Rate limiting
	if strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "throttle") {
		return true
	}

	// Server errors (5xx)
	if strings.Contains(msg, "500") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "504") ||
		strings.Contains(msg, "internal server error") ||
		strings.Contains(msg, "bad gateway") ||
		strings.Contains(msg, "service unavailable") ||
		strings.Contains(msg, "gateway timeout") {
		return true
	}

	return false
}
