package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.GlobalEnabled {
		t.Error("Expected global rate limiting to be enabled by default")
	}
	if cfg.GlobalLimit != 1000 {
		t.Errorf("Expected global limit 1000, got %d", cfg.GlobalLimit)
	}
	if !cfg.PerWalletEnabled {
		t.Error("Expected per-wallet rate limiting to be enabled by default")
	}
	if cfg.PerWalletLimit != 60 {
		t.Errorf("Expected per-wallet limit 60, got %d", cfg.PerWalletLimit)
	}
	if !cfg.PerIPEnabled {
		t.Error("Expected per-IP rate limiting to be enabled by default")
	}
}

func TestGlobalLimiter_Disabled(t *testing.T) {
	cfg := Config{GlobalEnabled: false}
	limiter := GlobalLimiter(cfg)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Should allow unlimited requests when disabled
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: expected 200, got %d", i, w.Code)
		}
	}
}

func TestGlobalLimiter_EnforcesLimit(t *testing.T) {
	cfg := Config{
		GlobalEnabled: true,
		GlobalLimit:   5,
		GlobalWindow:  1 * time.Second,
		GlobalBurst:   2,
	}
	limiter := GlobalLimiter(cfg)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 5 requests should succeed
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: expected 200, got %d", i, w.Code)
		}
	}

	// 6th request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429 after limit exceeded, got %d", w.Code)
	}

	// Check Retry-After header
	if w.Header().Get("Retry-After") == "" {
		t.Error("Expected Retry-After header to be set")
	}
}

func TestWalletLimiter_Disabled(t *testing.T) {
	cfg := Config{PerWalletEnabled: false}
	limiter := WalletLimiter(cfg)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Should allow unlimited requests when disabled
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Wallet", "TestWallet123")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: expected 200, got %d", i, w.Code)
		}
	}
}

func TestWalletLimiter_PerWalletLimit(t *testing.T) {
	cfg := Config{
		PerWalletEnabled: true,
		PerWalletLimit:   3,
		PerWalletWindow:  1 * time.Second,
		PerWalletBurst:   1,
	}
	limiter := WalletLimiter(cfg)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	wallet1 := "Wallet1ABC"
	wallet2 := "Wallet2XYZ"

	// Wallet1: First 3 requests should succeed
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Wallet", wallet1)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Wallet1 request %d: expected 200, got %d", i, w.Code)
		}
	}

	// Wallet1: 4th request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Wallet", wallet1)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Wallet1: Expected 429 after limit, got %d", w.Code)
	}

	// Wallet2: Should still be able to make requests (separate limit)
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Wallet", wallet2)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Wallet2: Expected 200, got %d", w.Code)
	}
}

func TestWalletLimiter_FallbackToIP(t *testing.T) {
	cfg := Config{
		PerWalletEnabled: true,
		PerWalletLimit:   3,
		PerWalletWindow:  1 * time.Second,
		PerWalletBurst:   1,
	}
	limiter := WalletLimiter(cfg)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request without wallet header should fall back to IP-based limiting
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: expected 200, got %d", i, w.Code)
		}
	}

	// 4th request from same IP should be limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429 after IP limit, got %d", w.Code)
	}
}

func TestExtractWalletFromRequest(t *testing.T) {
	tests := []struct {
		name           string
		setupRequest   func(*http.Request)
		expectedWallet string
	}{
		{
			name: "X-Wallet header",
			setupRequest: func(r *http.Request) {
				r.Header.Set("X-Wallet", "WalletFromHeader")
			},
			expectedWallet: "WalletFromHeader",
		},
		{
			name: "X-Signer header",
			setupRequest: func(r *http.Request) {
				r.Header.Set("X-Signer", "WalletFromSigner")
			},
			expectedWallet: "WalletFromSigner",
		},
		{
			name: "Query parameter",
			setupRequest: func(r *http.Request) {
				r.URL.RawQuery = "wallet=WalletFromQuery"
			},
			expectedWallet: "WalletFromQuery",
		},
		{
			name: "X-Wallet priority over X-Signer",
			setupRequest: func(r *http.Request) {
				r.Header.Set("X-Wallet", "PriorityWallet")
				r.Header.Set("X-Signer", "SecondaryWallet")
			},
			expectedWallet: "PriorityWallet",
		},
		{
			name:           "No wallet information",
			setupRequest:   func(r *http.Request) {},
			expectedWallet: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			tt.setupRequest(req)

			wallet := extractWalletFromRequest(req)
			if wallet != tt.expectedWallet {
				t.Errorf("Expected wallet %q, got %q", tt.expectedWallet, wallet)
			}
		})
	}
}

func TestIPLimiter_EnforcesLimit(t *testing.T) {
	cfg := Config{
		PerIPEnabled: true,
		PerIPLimit:   3,
		PerIPWindow:  1 * time.Second,
		PerIPBurst:   1,
	}
	limiter := IPLimiter(cfg)

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "192.168.1.100:54321"

	// First 3 requests should succeed
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: expected 200, got %d", i, w.Code)
		}
	}

	// 4th request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429 after IP limit, got %d", w.Code)
	}

	// Different IP should not be affected
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.101:54321"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Different IP: Expected 200, got %d", w.Code)
	}
}
