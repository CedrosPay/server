package ratelimit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/CedrosPay/server/internal/apikey"
	"github.com/CedrosPay/server/internal/metrics"
	"github.com/go-chi/httprate"
)

// Config holds rate limiting configuration.
type Config struct {
	// Global rate limiting (across all users)
	GlobalEnabled bool
	GlobalLimit   int           // requests per window
	GlobalWindow  time.Duration // time window
	GlobalBurst   int           // burst capacity

	// Per-wallet rate limiting (identified by wallet address)
	PerWalletEnabled bool
	PerWalletLimit   int
	PerWalletWindow  time.Duration
	PerWalletBurst   int

	// Per-IP rate limiting (fallback when wallet not identified)
	PerIPEnabled bool
	PerIPLimit   int
	PerIPWindow  time.Duration
	PerIPBurst   int

	// Metrics collector (optional)
	Metrics *metrics.Metrics
}

// rateLimitResponse represents the JSON error response for rate limit exceeded.
type rateLimitResponse struct {
	Error             string `json:"error"`
	Message           string `json:"message"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}

// DefaultConfig returns sensible default rate limits.
// These are generous limits designed to stop obvious spam while not restricting legitimate use.
func DefaultConfig() Config {
	return Config{
		// Global: 1000 req/min (16.6 req/sec) - prevents DoS
		GlobalEnabled: true,
		GlobalLimit:   1000,
		GlobalWindow:  1 * time.Minute,
		GlobalBurst:   100,

		// Per-wallet: 60 req/min (1 req/sec avg) - prevents wallet spam
		PerWalletEnabled: true,
		PerWalletLimit:   60,
		PerWalletWindow:  1 * time.Minute,
		PerWalletBurst:   10,

		// Per-IP: 120 req/min (2 req/sec avg) - fallback for non-wallet requests
		PerIPEnabled: true,
		PerIPLimit:   120,
		PerIPWindow:  1 * time.Minute,
		PerIPBurst:   20,
	}
}

// createRateLimitHandler creates a standardized rate limit handler function.
// This eliminates duplication across global, per-wallet, and per-IP limiters.
func createRateLimitHandler(
	limitType string,
	windowSeconds int,
	extractIdentifier func(*http.Request) string,
	metricsCollector *metrics.Metrics,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract identifier for metrics (optional)
		identifier := "all"
		if extractIdentifier != nil {
			if id := extractIdentifier(r); id != "" {
				identifier = id
			}
		}

		// Record rate limit hit in metrics
		if metricsCollector != nil {
			metricsCollector.ObserveRateLimit(limitType, identifier)
		}

		// Build response message based on limit type
		var message string
		switch limitType {
		case "global":
			message = "Global rate limit exceeded. Please try again later."
		case "per_wallet":
			if identifier != "" && identifier != "all" && identifier != "unknown" {
				message = fmt.Sprintf("Per-wallet rate limit exceeded for %s. Please try again later.", identifier)
			} else {
				message = "Rate limit exceeded. Please try again later."
			}
		case "per_ip":
			message = "IP rate limit exceeded. Please try again later."
		default:
			message = "Rate limit exceeded. Please try again later."
		}

		response := rateLimitResponse{
			Error:             "rate_limit_exceeded",
			Message:           message,
			RetryAfterSeconds: windowSeconds,
		}

		// Set headers and write response
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", fmt.Sprintf("%d", windowSeconds))
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(response)
	}
}

// GlobalLimiter creates a global rate limiter middleware.
func GlobalLimiter(cfg Config) func(http.Handler) http.Handler {
	if !cfg.GlobalEnabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	limiter := httprate.Limit(
		cfg.GlobalLimit,
		cfg.GlobalWindow,
		httprate.WithLimitHandler(
			createRateLimitHandler(
				"global",
				int(cfg.GlobalWindow.Seconds()),
				nil, // No identifier extraction for global limiter
				cfg.Metrics,
			),
		),
	)

	// Wrap limiter to check for API key exemptions
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Partner tier bypasses global limits
			if apikey.ShouldBypassGlobalLimit(r) {
				next.ServeHTTP(w, r)
				return
			}
			limiter(next).ServeHTTP(w, r)
		})
	}
}

// WalletLimiter creates a per-wallet rate limiter middleware.
// It extracts wallet address from various request sources (headers, body, query params).
func WalletLimiter(cfg Config) func(http.Handler) http.Handler {
	if !cfg.PerWalletEnabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	limiter := httprate.Limit(
		cfg.PerWalletLimit,
		cfg.PerWalletWindow,
		httprate.WithKeyFuncs(walletKeyExtractor),
		httprate.WithLimitHandler(
			createRateLimitHandler(
				"per_wallet",
				int(cfg.PerWalletWindow.Seconds()),
				extractWalletFromRequest,
				cfg.Metrics,
			),
		),
	)

	// Wrap limiter to check for API key exemptions
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Enterprise and Partner tiers bypass per-wallet limits
			if apikey.IsExemptFromRateLimits(r) {
				next.ServeHTTP(w, r)
				return
			}
			limiter(next).ServeHTTP(w, r)
		})
	}
}

// IPLimiter creates a per-IP rate limiter middleware (fallback).
func IPLimiter(cfg Config) func(http.Handler) http.Handler {
	if !cfg.PerIPEnabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	limiter := httprate.Limit(
		cfg.PerIPLimit,
		cfg.PerIPWindow,
		httprate.WithKeyByIP(),
		httprate.WithLimitHandler(
			createRateLimitHandler(
				"per_ip",
				int(cfg.PerIPWindow.Seconds()),
				func(r *http.Request) string { return r.RemoteAddr },
				cfg.Metrics,
			),
		),
	)

	// Wrap limiter to check for API key exemptions
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Enterprise and Partner tiers bypass per-IP limits
			if apikey.IsExemptFromRateLimits(r) {
				next.ServeHTTP(w, r)
				return
			}
			limiter(next).ServeHTTP(w, r)
		})
	}
}

// walletKeyExtractor is a httprate.KeyFunc that extracts wallet address from request.
func walletKeyExtractor(r *http.Request) (string, error) {
	wallet := extractWalletFromRequest(r)
	if wallet == "" {
		// Fall back to IP-based limiting
		return httprate.KeyByIP(r)
	}
	return "wallet:" + wallet, nil
}

// extractWalletFromRequest attempts to extract wallet address from various sources.
// Prioritizes explicit wallet headers/params over derived addresses from payment proofs.
func extractWalletFromRequest(r *http.Request) string {
	// 1. Check X-Wallet header (explicit wallet identification)
	if wallet := r.Header.Get("X-Wallet"); wallet != "" {
		return wallet
	}

	// 2. Check X-Signer header (used in refund deny endpoint)
	if signer := r.Header.Get("X-Signer"); signer != "" {
		return signer
	}

	// 3. Check query parameter (used in some endpoints)
	if wallet := r.URL.Query().Get("wallet"); wallet != "" {
		return wallet
	}

	// 4. For payment verification, we could parse X-PAYMENT header
	// but that requires JSON parsing which is expensive for rate limiting
	// Better to rely on IP-based limiting for anonymous requests

	return ""
}
