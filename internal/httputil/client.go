package httputil

import (
	"net/http"
	"time"
)

// NewClient creates a new HTTP client with the given timeout and optimized transport settings.
// This provides consistent configuration across all HTTP clients in the application.
//
// Transport settings:
//   - MaxIdleConns: 100 (total idle connections across all hosts)
//   - MaxIdleConnsPerHost: 10 (idle connections per host)
//   - IdleConnTimeout: 90s (time to keep idle connections alive)
//
// These settings enable connection reuse and reduce latency for repeated requests
// to the same hosts (e.g., webhook notifications, RPC calls).
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}
