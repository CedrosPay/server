package idempotency

import (
	"bytes"
	"net/http"
	"time"
)

const (
	// HeaderKey is the standard idempotency key header
	HeaderKey = "Idempotency-Key"

	// DefaultTTL is the default cache duration for idempotent responses (24 hours)
	DefaultTTL = 24 * time.Hour
)

// responseWriter wraps http.ResponseWriter to capture response details
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
	headers    map[string]string
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           &bytes.Buffer{},
		headers:        make(map[string]string),
	}
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// captureHeaders captures all headers that were set before WriteHeader was called
func (rw *responseWriter) captureHeaders() {
	for key := range rw.ResponseWriter.Header() {
		rw.headers[key] = rw.ResponseWriter.Header().Get(key)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.body.Write(b) // Capture body for caching
	return rw.ResponseWriter.Write(b)
}

// Middleware creates idempotency middleware for payment endpoints
func Middleware(store Store, ttl time.Duration) func(http.Handler) http.Handler {
	if ttl == 0 {
		ttl = DefaultTTL
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract idempotency key from header
			rawKey := r.Header.Get(HeaderKey)

			// If no key provided, pass through normally
			if rawKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Scope the key by method and path to prevent cross-endpoint collisions
			// This ensures the same idempotency key cannot be reused across different endpoints
			key := r.Method + ":" + r.URL.Path + ":" + rawKey

			// Check if we have a cached response
			cached, found := store.Get(r.Context(), key)
			if found {
				// Return cached response
				for k, v := range cached.Headers {
					w.Header().Set(k, v)
				}
				w.Header().Set("X-Idempotency-Replay", "true")
				w.WriteHeader(cached.StatusCode)
				w.Write(cached.Body)
				return
			}

			// Wrap response writer to capture response
			rw := newResponseWriter(w)

			// Process request normally
			next.ServeHTTP(rw, r)

			// Cache successful responses (2xx status codes)
			if rw.statusCode >= 200 && rw.statusCode < 300 {
				// Capture headers after response is complete
				rw.captureHeaders()

				response := &Response{
					StatusCode: rw.statusCode,
					Headers:    rw.headers,
					Body:       rw.body.Bytes(),
					CachedAt:   time.Now(),
				}

				store.Set(r.Context(), key, response, ttl)
			}
		})
	}
}
