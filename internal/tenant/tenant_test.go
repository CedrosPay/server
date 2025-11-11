package tenant

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFromContext(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name:     "returns default when no tenant in context",
			ctx:      context.Background(),
			expected: DefaultTenantID,
		},
		{
			name:     "returns tenant when set in context",
			ctx:      WithTenant(context.Background(), "tenant-123"),
			expected: "tenant-123",
		},
		{
			name:     "returns default when empty tenant set",
			ctx:      WithTenant(context.Background(), ""),
			expected: DefaultTenantID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromContext(tt.ctx)
			if result != tt.expected {
				t.Errorf("FromContext() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWithTenant(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
		expected string
	}{
		{
			name:     "sets tenant in context",
			tenantID: "tenant-123",
			expected: "tenant-123",
		},
		{
			name:     "defaults empty tenant to default",
			tenantID: "",
			expected: DefaultTenantID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithTenant(context.Background(), tt.tenantID)
			result := FromContext(ctx)
			if result != tt.expected {
				t.Errorf("WithTenant() context value = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractTenantID(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		host           string
		expectedTenant string
	}{
		{
			name:           "extracts from X-Tenant-ID header",
			headers:        map[string]string{"X-Tenant-ID": "tenant-123"},
			expectedTenant: "tenant-123",
		},
		{
			name:           "sanitizes tenant ID from header",
			headers:        map[string]string{"X-Tenant-ID": "Tenant@123!"},
			expectedTenant: "tenant123",
		},
		{
			name:           "extracts from subdomain",
			host:           "acme-corp.api.cedrospay.com",
			expectedTenant: "acme-corp",
		},
		{
			name:           "ignores www subdomain",
			host:           "www.cedrospay.com",
			expectedTenant: DefaultTenantID,
		},
		{
			name:           "ignores api subdomain",
			host:           "api.cedrospay.com",
			expectedTenant: DefaultTenantID,
		},
		{
			name:           "defaults when no tenant info",
			expectedTenant: DefaultTenantID,
		},
		{
			name:           "header takes priority over subdomain",
			headers:        map[string]string{"X-Tenant-ID": "tenant-from-header"},
			host:           "tenant-from-subdomain.api.cedrospay.com",
			expectedTenant: "tenant-from-header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://"+tt.host+"/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result := extractTenantID(req)
			if result != tt.expectedTenant {
				t.Errorf("extractTenantID() = %v, want %v", result, tt.expectedTenant)
			}
		})
	}
}

func TestExtractFromSubdomain(t *testing.T) {
	tests := []struct {
		host           string
		expectedTenant string
	}{
		{"tenant1.api.example.com", "tenant1"},
		{"acme-corp.api.example.com", "acme-corp"},
		{"acme_corp.api.example.com", "acme_corp"},
		{"www.example.com", ""},
		{"api.example.com", ""},
		{"app.example.com", ""},
		{"admin.example.com", ""},
		{"dashboard.example.com", ""},
		{"example.com", ""},
		{"localhost:8080", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			result := extractFromSubdomain(tt.host)
			if result != tt.expectedTenant {
				t.Errorf("extractFromSubdomain(%q) = %v, want %v", tt.host, result, tt.expectedTenant)
			}
		})
	}
}

func TestSanitizeTenantID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"tenant-123", "tenant-123"},
		{"tenant_123", "tenant_123"},
		{"Tenant123", "tenant123"},
		{"tenant@123", "tenant123"},
		{"tenant!@#$%123", "tenant123"},
		{"tenant 123", "tenant123"},
		{"  tenant-123  ", "tenant-123"},
		{"", DefaultTenantID},
		{"@@@", DefaultTenantID},
		// Test length limit (64 chars)
		{string(make([]byte, 100)), DefaultTenantID}, // All invalid chars
		{"a" + string(make([]byte, 100)), "a"},       // One valid + invalid = just "a"
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeTenantID(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeTenantID(%q) = %v, want %v", tt.input, result, tt.expected)
			}

			// Verify sanitized output is safe (alphanumeric, hyphen, underscore only)
			for _, r := range result {
				if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
					t.Errorf("sanitizeTenantID(%q) produced unsafe character: %c", tt.input, r)
				}
			}

			// Verify length limit
			if len(result) > 64 {
				t.Errorf("sanitizeTenantID(%q) exceeded 64 character limit: %d", tt.input, len(result))
			}
		})
	}
}

func TestExtractionMiddleware(t *testing.T) {
	tests := []struct {
		name                  string
		requestHeaders        map[string]string
		host                  string
		expectedTenant        string
		expectedResponseHeader string
	}{
		{
			name:                  "adds tenant to context and response headers",
			requestHeaders:        map[string]string{"X-Tenant-ID": "tenant-123"},
			expectedTenant:        "tenant-123",
			expectedResponseHeader: "tenant-123",
		},
		{
			name:                  "defaults to default tenant",
			requestHeaders:        map[string]string{},
			expectedTenant:        DefaultTenantID,
			expectedResponseHeader: DefaultTenantID,
		},
		{
			name:                  "extracts from subdomain",
			host:                  "acme.api.example.com",
			expectedTenant:        "acme",
			expectedResponseHeader: "acme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler that checks context
			var capturedTenant string
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedTenant = FromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			// Wrap with tenant extraction middleware
			handler := Extraction(testHandler)

			// Create request
			host := tt.host
			if host == "" {
				host = "localhost"
			}
			req := httptest.NewRequest(http.MethodGet, "http://"+host+"/test", nil)
			for k, v := range tt.requestHeaders {
				req.Header.Set(k, v)
			}

			// Execute request
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			// Check tenant was added to context
			if capturedTenant != tt.expectedTenant {
				t.Errorf("Context tenant = %v, want %v", capturedTenant, tt.expectedTenant)
			}

			// Check response header
			if tenantHeader := w.Header().Get("X-Tenant-ID"); tenantHeader != tt.expectedResponseHeader {
				t.Errorf("X-Tenant-ID header = %v, want %v", tenantHeader, tt.expectedResponseHeader)
			}
		})
	}
}

func TestNoopValidator(t *testing.T) {
	validator := NoopValidator{}
	ctx := context.Background()

	// Test IsValidTenant
	valid, err := validator.IsValidTenant(ctx, "any-tenant")
	if err != nil {
		t.Errorf("IsValidTenant() error = %v, want nil", err)
	}
	if !valid {
		t.Errorf("IsValidTenant() = false, want true")
	}

	// Test GetTenantSettings
	settings, err := validator.GetTenantSettings(ctx, "test-tenant")
	if err != nil {
		t.Errorf("GetTenantSettings() error = %v, want nil", err)
	}
	if settings.ID != "test-tenant" {
		t.Errorf("GetTenantSettings().ID = %v, want test-tenant", settings.ID)
	}
	if !settings.Active {
		t.Errorf("GetTenantSettings().Active = false, want true")
	}
}
