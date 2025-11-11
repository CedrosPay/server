package httpserver

import (
	"encoding/json"
	"net/http"

	apierrors "github.com/CedrosPay/server/internal/errors"
)

// adminMetricsAuth is middleware that protects the /metrics endpoint with an API key.
// If no API key is configured, the endpoint is accessible without authentication.
// If an API key is configured, requests must include an "Authorization: Bearer {key}" header.
func adminMetricsAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If no API key is configured, allow access without authentication
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check Authorization header
			authHeader := r.Header.Get("Authorization")
			expectedHeader := "Bearer " + apiKey

			if authHeader != expectedHeader {
				// Return 401 Unauthorized with appropriate error code
				resp := apierrors.NewErrorResponse(apierrors.ErrCodeInvalidField, "Invalid or missing admin API key", nil)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(resp)
				return
			}

			// API key is valid - proceed
			next.ServeHTTP(w, r)
		})
	}
}
