package versioning

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
		expected Version
	}{
		{
			name:     "returns default when no version in context",
			ctx:      context.Background(),
			expected: DefaultVersion,
		},
		{
			name:     "returns v1 when set in context",
			ctx:      WithVersion(context.Background(), V1),
			expected: V1,
		},
		{
			name:     "returns v2 when set in context",
			ctx:      WithVersion(context.Background(), V2),
			expected: V2,
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

func TestVersionString(t *testing.T) {
	tests := []struct {
		version  Version
		expected string
	}{
		{V1, "v1"},
		{V2, "v2"},
		{Version(0), "v1"},  // Invalid version defaults to v1
		{Version(-1), "v1"}, // Negative version defaults to v1
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.version.String()
			if result != tt.expected {
				t.Errorf("Version.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNegotiateVersion(t *testing.T) {
	tests := []struct {
		name            string
		headers         map[string]string
		expectedVersion Version
	}{
		{
			name:            "defaults to v1 when no version specified",
			headers:         map[string]string{},
			expectedVersion: V1,
		},
		{
			name: "X-API-Version header takes priority",
			headers: map[string]string{
				"X-API-Version": "v2",
				"Accept":        "application/vnd.cedros.v1+json",
			},
			expectedVersion: V2,
		},
		{
			name: "X-API-Version without v prefix",
			headers: map[string]string{
				"X-API-Version": "2",
			},
			expectedVersion: V2,
		},
		{
			name: "vendor-specific media type in Accept header",
			headers: map[string]string{
				"Accept": "application/vnd.cedros.v2+json",
			},
			expectedVersion: V2,
		},
		{
			name: "version parameter in Accept header",
			headers: map[string]string{
				"Accept": "application/json; version=2",
			},
			expectedVersion: V2,
		},
		{
			name: "version parameter with spaces",
			headers: map[string]string{
				"Accept": "application/json; version= 2 ",
			},
			expectedVersion: V2,
		},
		{
			name: "invalid version falls back to v1",
			headers: map[string]string{
				"X-API-Version": "v99",
			},
			expectedVersion: V1,
		},
		{
			name: "case insensitive version parsing",
			headers: map[string]string{
				"X-API-Version": "V2",
			},
			expectedVersion: V2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result := negotiateVersion(req)
			if result != tt.expectedVersion {
				t.Errorf("negotiateVersion() = %v, want %v", result, tt.expectedVersion)
			}
		})
	}
}

func TestNegotiationMiddleware(t *testing.T) {
	tests := []struct {
		name                   string
		requestHeaders         map[string]string
		expectedVersion        Version
		expectedResponseHeader string
	}{
		{
			name:                   "adds version to context and response headers",
			requestHeaders:         map[string]string{"X-API-Version": "v2"},
			expectedVersion:        V2,
			expectedResponseHeader: "v2",
		},
		{
			name:                   "defaults to v1",
			requestHeaders:         map[string]string{},
			expectedVersion:        V1,
			expectedResponseHeader: "v1",
		},
		{
			name:                   "sets Vary header",
			requestHeaders:         map[string]string{},
			expectedVersion:        V1,
			expectedResponseHeader: "v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler that checks context
			var capturedVersion Version
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedVersion = FromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			// Wrap with versioning middleware
			handler := Negotiation(testHandler)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			for k, v := range tt.requestHeaders {
				req.Header.Set(k, v)
			}

			// Execute request
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			// Check version was added to context
			if capturedVersion != tt.expectedVersion {
				t.Errorf("Context version = %v, want %v", capturedVersion, tt.expectedVersion)
			}

			// Check response headers
			if versionHeader := w.Header().Get("X-API-Version"); versionHeader != tt.expectedResponseHeader {
				t.Errorf("X-API-Version header = %v, want %v", versionHeader, tt.expectedResponseHeader)
			}

			if varyHeader := w.Header().Get("Vary"); varyHeader != "Accept, X-API-Version" {
				t.Errorf("Vary header = %v, want 'Accept, X-API-Version'", varyHeader)
			}
		})
	}
}

func TestDeprecationWarning(t *testing.T) {
	tests := []struct {
		name                    string
		deprecatedVersion       Version
		requestVersion          Version
		sunsetDate              string
		message                 string
		expectDeprecationHeader bool
		expectSunsetHeader      bool
		expectWarningHeader     bool
	}{
		{
			name:                    "adds deprecation headers for deprecated version",
			deprecatedVersion:       V1,
			requestVersion:          V1,
			sunsetDate:              "2025-12-31T23:59:59Z",
			message:                 "Please upgrade to v2",
			expectDeprecationHeader: true,
			expectSunsetHeader:      true,
			expectWarningHeader:     true,
		},
		{
			name:                    "no headers for non-deprecated version",
			deprecatedVersion:       V1,
			requestVersion:          V2,
			sunsetDate:              "2025-12-31T23:59:59Z",
			message:                 "Please upgrade to v2",
			expectDeprecationHeader: false,
			expectSunsetHeader:      false,
			expectWarningHeader:     false,
		},
		{
			name:                    "deprecation without sunset date",
			deprecatedVersion:       V1,
			requestVersion:          V1,
			sunsetDate:              "",
			message:                 "This version is deprecated",
			expectDeprecationHeader: true,
			expectSunsetHeader:      false,
			expectWarningHeader:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Create deprecation warning middleware
			deprecation := NewDeprecationWarning(tt.deprecatedVersion, tt.sunsetDate, tt.message)
			handler := deprecation.Middleware(testHandler)

			// Wrap with versioning middleware to set version in context
			handler = Negotiation(handler)

			// Create request with version
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("X-API-Version", tt.requestVersion.String())

			// Execute request
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			// Check deprecation header
			deprecationHeader := w.Header().Get("Deprecation")
			if tt.expectDeprecationHeader && deprecationHeader != "true" {
				t.Errorf("Expected Deprecation header = 'true', got %v", deprecationHeader)
			}
			if !tt.expectDeprecationHeader && deprecationHeader != "" {
				t.Errorf("Expected no Deprecation header, got %v", deprecationHeader)
			}

			// Check sunset header
			sunsetHeader := w.Header().Get("Sunset")
			if tt.expectSunsetHeader && sunsetHeader != tt.sunsetDate {
				t.Errorf("Expected Sunset header = %v, got %v", tt.sunsetDate, sunsetHeader)
			}
			if !tt.expectSunsetHeader && sunsetHeader != "" {
				t.Errorf("Expected no Sunset header, got %v", sunsetHeader)
			}

			// Check warning header
			warningHeader := w.Header().Get("Warning")
			if tt.expectWarningHeader && warningHeader == "" {
				t.Errorf("Expected Warning header, got none")
			}
			if !tt.expectWarningHeader && warningHeader != "" {
				t.Errorf("Expected no Warning header, got %v", warningHeader)
			}
		})
	}
}

func TestParseVersionString(t *testing.T) {
	tests := []struct {
		input    string
		expected Version
	}{
		{"v1", V1},
		{"V1", V1},
		{"1", V1},
		{"v2", V2},
		{"V2", V2},
		{"2", V2},
		{" v2 ", V2}, // Test trimming
		{"v99", 0},   // Invalid version
		{"invalid", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseVersionString(tt.input)
			if result != tt.expected {
				t.Errorf("parseVersionString(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
