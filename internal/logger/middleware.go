package logger

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/rs/zerolog"
)

// Middleware creates HTTP middleware that injects request logger into context.
// It generates a unique request ID and adds it to both context and response headers.
func Middleware(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate or extract request ID
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = generateRequestID()
			}

			// Add request ID to response header for client correlation
			w.Header().Set("X-Request-ID", requestID)

			// Create request-scoped logger with context fields
			reqLogger := logger.With().
				Str("request_id", requestID).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remote_addr", getRemoteAddr(r)).
				Logger()

			// Add logger and request ID to context
			ctx := WithContext(r.Context(), reqLogger)
			ctx = WithRequestID(ctx, requestID)

			// Log incoming request
			reqLogger.Info().
				Str("user_agent", r.UserAgent()).
				Msg("request.started")

			// Call next handler with enriched context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// generateRequestID creates a cryptographically random request identifier.
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID (should never happen)
		return "req_fallback"
	}
	return "req_" + hex.EncodeToString(b)
}

// getRemoteAddr extracts client IP, respecting X-Forwarded-For header.
func getRemoteAddr(r *http.Request) string {
	// Check X-Forwarded-For first (behind proxy/load balancer)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take first IP in chain (original client)
		return forwarded
	}

	// Check X-Real-IP (some proxies use this)
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fallback to RemoteAddr
	return r.RemoteAddr
}
