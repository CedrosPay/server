package httpserver

import "net/http"

// securityHeadersMiddleware adds security headers to all responses.
// These headers help protect against common web vulnerabilities.
//
// Applied headers:
// - X-Content-Type-Options: Prevents MIME-type sniffing
// - X-Frame-Options: Prevents clickjacking attacks
// - X-XSS-Protection: Enables browser XSS filtering
// - Referrer-Policy: Controls referrer information leakage
// - Strict-Transport-Security: Enforces HTTPS (only added if request uses TLS)
//
// Note: While this is primarily an API server (not serving HTML),
// these headers provide defense-in-depth security.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME-type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking by disallowing framing
		w.Header().Set("X-Frame-Options", "DENY")

		// Enable browser XSS protection
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Control referrer information
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Add HSTS only if using HTTPS
		// HSTS tells browsers to only access this domain via HTTPS
		if r.TLS != nil {
			// max-age=31536000 = 1 year
			// includeSubDomains applies to all subdomains
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}
