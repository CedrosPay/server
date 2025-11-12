package tenant

import (
	"context"
	"net/http"
	"strings"
)

// DefaultTenantID is used for single-tenant deployments and backwards compatibility
const DefaultTenantID = "default"

// contextKey is a private type for context keys to avoid collisions
type contextKey string

const tenantContextKey contextKey = "tenant-id"

// FromContext retrieves the tenant ID from the request context
// Returns DefaultTenantID if no tenant is set (backwards compatible)
func FromContext(ctx context.Context) string {
	if tenantID, ok := ctx.Value(tenantContextKey).(string); ok && tenantID != "" {
		return tenantID
	}
	return DefaultTenantID
}

// WithTenant adds the tenant ID to the context
func WithTenant(ctx context.Context, tenantID string) context.Context {
	if tenantID == "" {
		tenantID = DefaultTenantID
	}
	return context.WithValue(ctx, tenantContextKey, tenantID)
}

// Extraction handles tenant ID extraction from HTTP requests
// Supports multiple extraction methods (in priority order):
//  1. X-Tenant-ID header (explicit tenant specification)
//  2. JWT claims (tenant_id field in auth token)
//  3. Subdomain (tenant1.api.cedrospay.com → tenant1)
//  4. Default tenant for backwards compatibility
//
// This middleware is OPTIONAL - single-tenant deployments don't need it.
// Multi-tenant deployments should enable it via config.
func Extraction(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := extractTenantID(r)

		// Add tenant ID to response headers for debugging
		w.Header().Set("X-Tenant-ID", tenantID)

		// Add context with tenant ID
		ctx := WithTenant(r.Context(), tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractTenantID extracts tenant ID from the request using various methods
func extractTenantID(r *http.Request) string {
	// Method 1: Explicit X-Tenant-ID header (highest priority)
	// Used by API clients that manage multiple tenants
	if tenantID := r.Header.Get("X-Tenant-ID"); tenantID != "" {
		return sanitizeTenantID(tenantID)
	}

	// Method 2: JWT claims (if auth middleware has extracted tenant)
	// The auth middleware should set this context value from the JWT
	if tenantID := FromContext(r.Context()); tenantID != DefaultTenantID {
		return tenantID
	}

	// Method 3: Subdomain extraction
	// Example: tenant1.api.cedrospay.com → tenant1
	if tenantID := extractFromSubdomain(r.Host); tenantID != "" {
		return tenantID
	}

	// Default: Use default tenant (backwards compatible)
	return DefaultTenantID
}

// extractFromSubdomain extracts tenant ID from subdomain
// Example: tenant1.api.example.com → tenant1
// Returns empty string if not a tenant subdomain
func extractFromSubdomain(host string) string {
	// Remove port if present
	host = strings.Split(host, ":")[0]

	// Split by dots
	parts := strings.Split(host, ".")

	// Need at least 3 parts for subdomain.api.domain.com
	if len(parts) < 3 {
		return ""
	}

	// First part is potential tenant ID
	subdomain := parts[0]

	// Ignore common non-tenant subdomains
	ignoreList := []string{"www", "api", "app", "admin", "dashboard"}
	for _, ignore := range ignoreList {
		if subdomain == ignore {
			return ""
		}
	}

	return sanitizeTenantID(subdomain)
}

// sanitizeTenantID ensures tenant ID is safe for database queries
// Allows only alphanumeric, hyphens, and underscores
func sanitizeTenantID(tenantID string) string {
	if tenantID == "" {
		return DefaultTenantID
	}

	// Convert to lowercase
	tenantID = strings.ToLower(strings.TrimSpace(tenantID))

	// Build sanitized string
	var sanitized strings.Builder
	for _, r := range tenantID {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sanitized.WriteRune(r)
		}
	}

	result := sanitized.String()
	if result == "" {
		return DefaultTenantID
	}

	// Limit length to 64 characters (reasonable for tenant IDs)
	if len(result) > 64 {
		result = result[:64]
	}

	return result
}

// Validator checks if a tenant ID is valid and active
// This is a placeholder interface for future tenant management
type Validator interface {
	// IsValidTenant checks if tenant exists and is active
	IsValidTenant(ctx context.Context, tenantID string) (bool, error)

	// GetTenantSettings retrieves tenant-specific settings
	GetTenantSettings(ctx context.Context, tenantID string) (TenantSettings, error)
}

// TenantSettings holds tenant-specific configuration
type TenantSettings struct {
	ID              string
	Name            string
	StripeAccountID string // Connected Stripe account
	SolanaWallet    string // Tenant's payment receiving wallet
	Active          bool
	RateLimits      RateLimitSettings
	Features        FeatureFlags
}

// RateLimitSettings holds tenant-specific rate limits
type RateLimitSettings struct {
	RequestsPerMinute int
	ConcurrentQuotes  int
	MaxCartSize       int
}

// FeatureFlags controls tenant-specific feature access
type FeatureFlags struct {
	GaslessTransactions bool
	RefundsEnabled      bool
	CouponsEnabled      bool
	WebhooksEnabled     bool
}

// NoopValidator always returns true (for single-tenant deployments)
type NoopValidator struct{}

func (NoopValidator) IsValidTenant(ctx context.Context, tenantID string) (bool, error) {
	return true, nil
}

func (NoopValidator) GetTenantSettings(ctx context.Context, tenantID string) (TenantSettings, error) {
	return TenantSettings{
		ID:     tenantID,
		Active: true,
	}, nil
}
