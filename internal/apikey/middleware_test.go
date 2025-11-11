package apikey

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware_Disabled(t *testing.T) {
	cfg := Config{
		Enabled: false,
		APIKeys: make(map[string]Tier),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tier := GetTier(r)
		if tier != TierFree {
			t.Errorf("Expected TierFree when disabled, got %s", tier)
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := Middleware(cfg)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
}

func TestMiddleware_NoAPIKey(t *testing.T) {
	cfg := Config{
		Enabled: true,
		APIKeys: map[string]Tier{
			"test_key": TierPro,
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tier := GetTier(r)
		if tier != TierFree {
			t.Errorf("Expected TierFree when no API key provided, got %s", tier)
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := Middleware(cfg)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
}

func TestMiddleware_ValidAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected Tier
	}{
		{"Pro tier", "pro_key_123", TierPro},
		{"Enterprise tier", "enterprise_abc", TierEnterprise},
		{"Partner tier", "partner_stripe", TierPartner},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Enabled: true,
				APIKeys: map[string]Tier{
					"pro_key_123":      TierPro,
					"enterprise_abc":   TierEnterprise,
					"partner_stripe":   TierPartner,
				},
			}

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tier := GetTier(r)
				if tier != tt.expected {
					t.Errorf("Expected %s, got %s", tt.expected, tier)
				}
				w.WriteHeader(http.StatusOK)
			})

			mw := Middleware(cfg)
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-API-Key", tt.apiKey)
			rec := httptest.NewRecorder()

			mw(handler).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("Expected 200, got %d", rec.Code)
			}
		})
	}
}

func TestMiddleware_InvalidAPIKey(t *testing.T) {
	cfg := Config{
		Enabled: true,
		APIKeys: map[string]Tier{
			"valid_key": TierPro,
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tier := GetTier(r)
		if tier != TierFree {
			t.Errorf("Expected TierFree for invalid API key, got %s", tier)
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := Middleware(cfg)
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "invalid_key")
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
}

func TestIsExemptFromRateLimits(t *testing.T) {
	tests := []struct {
		name     string
		tier     Tier
		expected bool
	}{
		{"Free tier not exempt", TierFree, false},
		{"Pro tier not exempt", TierPro, false},
		{"Enterprise tier exempt", TierEnterprise, true},
		{"Partner tier exempt", TierPartner, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Enabled: true,
				APIKeys: map[string]Tier{
					"test_key": tt.tier,
				},
			}

			var result bool
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				result = IsExemptFromRateLimits(r)
				w.WriteHeader(http.StatusOK)
			})

			mw := Middleware(cfg)
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-API-Key", "test_key")
			rec := httptest.NewRecorder()

			mw(handler).ServeHTTP(rec, req)

			if result != tt.expected {
				t.Errorf("Expected IsExemptFromRateLimits=%v for %s, got %v", tt.expected, tt.tier, result)
			}
		})
	}
}

func TestShouldBypassGlobalLimit(t *testing.T) {
	tests := []struct {
		name     string
		tier     Tier
		expected bool
	}{
		{"Free tier no bypass", TierFree, false},
		{"Pro tier no bypass", TierPro, false},
		{"Enterprise tier no bypass", TierEnterprise, false},
		{"Partner tier bypass", TierPartner, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Enabled: true,
				APIKeys: map[string]Tier{
					"test_key": tt.tier,
				},
			}

			var result bool
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				result = ShouldBypassGlobalLimit(r)
				w.WriteHeader(http.StatusOK)
			})

			mw := Middleware(cfg)
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-API-Key", "test_key")
			rec := httptest.NewRecorder()

			mw(handler).ServeHTTP(rec, req)

			if result != tt.expected {
				t.Errorf("Expected ShouldBypassGlobalLimit=%v for %s, got %v", tt.expected, tt.tier, result)
			}
		})
	}
}

func TestGetTier_NoContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	tier := GetTier(req)

	if tier != TierFree {
		t.Errorf("Expected TierFree when no context, got %s", tier)
	}
}
