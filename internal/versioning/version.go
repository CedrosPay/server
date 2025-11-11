package versioning

import (
	"context"
	"net/http"
	"strings"
)

// Version represents an API version
type Version int

const (
	// V1 is the initial API version (current default)
	V1 Version = 1
	// V2 is reserved for future breaking changes
	V2 Version = 2

	// LatestVersion points to the most recent stable API version
	LatestVersion = V1

	// DefaultVersion is used when client doesn't specify a version
	DefaultVersion = V1
)

// String returns the version as a string (e.g., "v1", "v2")
func (v Version) String() string {
	if v <= 0 {
		return "v1"
	}
	return "v" + string(rune('0'+v))
}

// contextKey is a private type for context keys to avoid collisions
type contextKey string

const versionContextKey contextKey = "api-version"

// FromContext retrieves the API version from the request context
func FromContext(ctx context.Context) Version {
	if v, ok := ctx.Value(versionContextKey).(Version); ok {
		return v
	}
	return DefaultVersion
}

// WithVersion adds the API version to the context
func WithVersion(ctx context.Context, version Version) context.Context {
	return context.WithValue(ctx, versionContextKey, version)
}

// Negotiation handles API version negotiation via Accept header
// Supports:
//   - Accept: application/vnd.cedros.v2+json  (vendor-specific version)
//   - Accept: application/json; version=2      (version parameter)
//   - X-API-Version: 2                         (explicit header)
//   - Default: v1 if not specified
func Negotiation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version := negotiateVersion(r)

		// Add version to response headers for client awareness
		w.Header().Set("X-API-Version", version.String())
		w.Header().Set("Vary", "Accept, X-API-Version")

		// Add context with version
		ctx := WithVersion(r.Context(), version)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// negotiateVersion extracts the requested API version from the request
func negotiateVersion(r *http.Request) Version {
	// Method 1: Explicit X-API-Version header (highest priority)
	if versionHeader := r.Header.Get("X-API-Version"); versionHeader != "" {
		if v := parseVersionString(versionHeader); v > 0 {
			return v
		}
	}

	// Method 2: Accept header with vendor-specific media type
	// Example: Accept: application/vnd.cedros.v2+json
	acceptHeader := r.Header.Get("Accept")
	if strings.Contains(acceptHeader, "application/vnd.cedros.") {
		parts := strings.Split(acceptHeader, ".")
		for _, part := range parts {
			// Extract version from "v2+json" -> "v2"
			versionPart := strings.Split(part, "+")[0]
			if strings.HasPrefix(versionPart, "v") || strings.HasPrefix(versionPart, "V") {
				if v := parseVersionString(versionPart); v > 0 {
					return v
				}
			}
		}
	}

	// Method 3: Accept header with version parameter
	// Example: Accept: application/json; version=2
	if strings.Contains(acceptHeader, "version=") {
		parts := strings.Split(acceptHeader, "version=")
		if len(parts) > 1 {
			versionStr := strings.TrimSpace(strings.Split(parts[1], ";")[0])
			if v := parseVersionString(versionStr); v > 0 {
				return v
			}
		}
	}

	// Default: Use v1
	return DefaultVersion
}

// parseVersionString converts version strings like "v2", "2", "V2" to Version
func parseVersionString(s string) Version {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "v")

	switch s {
	case "1":
		return V1
	case "2":
		return V2
	default:
		return 0 // Invalid version
	}
}

// DeprecationWarning adds deprecation headers to responses for old API versions
// This gives clients advance notice before breaking changes
type DeprecationWarning struct {
	deprecatedVersion Version
	sunsetDate        string // RFC 3339 date when version will be removed
	message           string
}

// NewDeprecationWarning creates a deprecation warning for a specific API version
func NewDeprecationWarning(version Version, sunsetDate, message string) *DeprecationWarning {
	return &DeprecationWarning{
		deprecatedVersion: version,
		sunsetDate:        sunsetDate,
		message:           message,
	}
}

// Middleware returns a middleware that adds deprecation warnings to responses
func (d *DeprecationWarning) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version := FromContext(r.Context())

		// If client is using deprecated version, add warning headers
		if version == d.deprecatedVersion {
			// Standard deprecation headers (RFC 8594)
			w.Header().Set("Deprecation", "true")
			if d.sunsetDate != "" {
				w.Header().Set("Sunset", d.sunsetDate)
			}
			if d.message != "" {
				w.Header().Set("Warning", `299 - "Deprecated API Version: `+d.message+`"`)
			}
		}

		next.ServeHTTP(w, r)
	})
}
