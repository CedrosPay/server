package apikey

import (
	"context"
	"net/http"
	"strings"
)

// Tier represents the API key tier level.
type Tier string

const (
	TierFree       Tier = "free"       // Default tier with standard rate limits
	TierPro        Tier = "pro"        // Pro tier with higher limits
	TierEnterprise Tier = "enterprise" // Enterprise tier with no limits
	TierPartner    Tier = "partner"    // Trusted partner (Stripe, etc.) with no limits
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const (
	// contextKeyTier stores the API key tier in request context.
	contextKeyTier contextKey = "api_key_tier"
)

// Config holds API key configuration.
type Config struct {
	// APIKeys maps API key to tier level.
	// Example: {"pro_abc123": TierPro, "enterprise_xyz789": TierEnterprise}
	APIKeys map[string]Tier

	// Enabled controls whether API key authentication is active.
	Enabled bool
}

// Middleware validates API keys and stores tier information in request context.
// If no API key is provided or key is invalid, request proceeds with TierFree (default rate limits apply).
// If valid API key is provided, tier is stored in context for rate limit exemptions.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	if !cfg.Enabled || len(cfg.APIKeys) == 0 {
		// API key system disabled - all requests are free tier
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(r.Context(), contextKeyTier, TierFree)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tier := TierFree // Default tier

			// Extract API key from X-API-Key header
			apiKey := r.Header.Get("X-API-Key")
			if apiKey != "" {
				apiKey = strings.TrimSpace(apiKey)

				// Lookup tier for this API key
				if keyTier, ok := cfg.APIKeys[apiKey]; ok {
					tier = keyTier
				}
				// Invalid API keys are treated as free tier (no error returned)
			}

			// Store tier in context for downstream middleware
			ctx := context.WithValue(r.Context(), contextKeyTier, tier)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetTier extracts the API key tier from request context.
// Returns TierFree if no tier is set (default).
func GetTier(r *http.Request) Tier {
	if tier, ok := r.Context().Value(contextKeyTier).(Tier); ok {
		return tier
	}
	return TierFree
}

// IsExemptFromRateLimits returns true if the request's API key tier is exempt from rate limits.
// Enterprise and Partner tiers are exempt from wallet/IP rate limits.
func IsExemptFromRateLimits(r *http.Request) bool {
	tier := GetTier(r)
	return tier == TierEnterprise || tier == TierPartner
}

// ShouldBypassGlobalLimit returns true if the request should bypass global rate limits.
// Only Partner tier bypasses global limits (to prevent bulk import issues).
func ShouldBypassGlobalLimit(r *http.Request) bool {
	return GetTier(r) == TierPartner
}
