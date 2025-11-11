package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestRoutePatternUniqueness verifies that cart and refund routes have unique paths
// that don't conflict with the generic paywall route pattern.
// This is a regression test for the routing conflict where /paywall/{cartId} would
// intercept ALL /paywall/{id} requests.
func TestRoutePatternUniqueness(t *testing.T) {
	// Simulate the actual route structure
	router := chi.NewRouter()

	cartHit := false
	refundHit := false
	genericHit := false

	// Register routes in the same order as server.go
	router.Get("/paywall/v1/cart/{cartId}", func(w http.ResponseWriter, r *http.Request) {
		cartHit = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("cart"))
	})

	router.Get("/paywall/v1/refunds/{refundId}", func(w http.ResponseWriter, r *http.Request) {
		refundHit = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("refund"))
	})

	// Generic paywall route registered last (as in configurePaywallRoutes)
	router.Route("/paywall", func(r chi.Router) {
		r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
			genericHit = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("generic"))
		})
	})

	tests := []struct {
		name            string
		path            string
		expectedHandler string
	}{
		{
			name:            "cart_path_hits_cart_handler",
			path:            "/paywall/v1/cart/abc123",
			expectedHandler: "cart",
		},
		{
			name:            "refund_path_hits_refund_handler",
			path:            "/paywall/v1/refunds/xyz789",
			expectedHandler: "refund",
		},
		{
			name:            "generic_path_hits_generic_handler",
			path:            "/paywall/test-resource",
			expectedHandler: "generic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			cartHit = false
			refundHit = false
			genericHit = false

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			body := rec.Body.String()
			if body != tt.expectedHandler {
				t.Errorf("Path %s hit wrong handler: expected %q, got %q",
					tt.path, tt.expectedHandler, body)
			}

			// Verify only one handler was hit
			hitCount := 0
			if cartHit {
				hitCount++
			}
			if refundHit {
				hitCount++
			}
			if genericHit {
				hitCount++
			}

			if hitCount != 1 {
				t.Errorf("Expected exactly 1 handler to be hit, got %d (cart:%v refund:%v generic:%v)",
					hitCount, cartHit, refundHit, genericHit)
			}
		})
	}
}
