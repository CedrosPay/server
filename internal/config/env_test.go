package config

import (
	"os"
	"testing"
	"time"
)

func TestEnvOverrides_ServerConfig(t *testing.T) {
	// Save original env
	defer os.Clearenv()

	tests := []struct {
		name      string
		envVars   map[string]string
		checkFunc func(*testing.T, *Config)
	}{
		{
			name: "CEDROS_SERVER_ADDRESS overrides default",
			envVars: map[string]string{
				"CEDROS_SERVER_ADDRESS": ":3000",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Server.Address != ":3000" {
					t.Errorf("Expected :3000, got %s", cfg.Server.Address)
				}
			},
		},
		{
			name: "CEDROS_ROUTE_PREFIX override",
			envVars: map[string]string{
				"CEDROS_ROUTE_PREFIX": "/api",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Server.RoutePrefix != "/api" {
					t.Errorf("Expected /api, got %s", cfg.Server.RoutePrefix)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			cfg := defaultConfig()
			cfg.applyEnvOverrides()
			tt.checkFunc(t, cfg)
		})
	}
}

func TestEnvOverrides_X402Config(t *testing.T) {
	defer os.Clearenv()

	tests := []struct {
		name      string
		envVars   map[string]string
		checkFunc func(*testing.T, *Config)
	}{
		{
			name: "CEDROS_X402_RPC_URL override",
			envVars: map[string]string{
				"CEDROS_X402_RPC_URL": "https://custom-rpc.solana.com",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.X402.RPCURL != "https://custom-rpc.solana.com" {
					t.Errorf("Expected custom RPC URL, got %s", cfg.X402.RPCURL)
				}
			},
		},
		{
			name: "CEDROS_X402_PAYMENT_ADDRESS override",
			envVars: map[string]string{
				"CEDROS_X402_PAYMENT_ADDRESS": "test-wallet-address",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.X402.PaymentAddress != "test-wallet-address" {
					t.Errorf("Expected test-wallet-address, got %s", cfg.X402.PaymentAddress)
				}
			},
		},
		{
			name: "CEDROS_X402_GASLESS_ENABLED boolean (true)",
			envVars: map[string]string{
				"CEDROS_X402_GASLESS_ENABLED": "true",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if !cfg.X402.GaslessEnabled {
					t.Error("Expected GaslessEnabled to be true")
				}
			},
		},
		{
			name: "CEDROS_X402_GASLESS_ENABLED boolean (1)",
			envVars: map[string]string{
				"CEDROS_X402_GASLESS_ENABLED": "1",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if !cfg.X402.GaslessEnabled {
					t.Error("Expected GaslessEnabled to be true with '1'")
				}
			},
		},
		{
			name: "CEDROS_X402_GASLESS_ENABLED boolean (false)",
			envVars: map[string]string{
				"CEDROS_X402_GASLESS_ENABLED": "false",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.X402.GaslessEnabled {
					t.Error("Expected GaslessEnabled to be false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			cfg := defaultConfig()
			cfg.applyEnvOverrides()
			tt.checkFunc(t, cfg)
		})
	}
}

func TestEnvOverrides_PaywallConfig(t *testing.T) {
	defer os.Clearenv()

	tests := []struct {
		name      string
		envVars   map[string]string
		checkFunc func(*testing.T, *Config)
	}{
		{
			name: "CEDROS_PAYWALL_QUOTE_TTL duration override (120s)",
			envVars: map[string]string{
				"CEDROS_PAYWALL_QUOTE_TTL": "120s",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				expected := 120 * time.Second
				if cfg.Paywall.QuoteTTL.Duration != expected {
					t.Errorf("Expected %v, got %v", expected, cfg.Paywall.QuoteTTL.Duration)
				}
			},
		},
		{
			name: "CEDROS_PAYWALL_PRODUCT_SOURCE override",
			envVars: map[string]string{
				"CEDROS_PAYWALL_PRODUCT_SOURCE": "postgres",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Paywall.ProductSource != "postgres" {
					t.Errorf("Expected postgres, got %s", cfg.Paywall.ProductSource)
				}
			},
		},
		{
			name: "CEDROS_PAYWALL_POSTGRES_URL override",
			envVars: map[string]string{
				"CEDROS_PAYWALL_POSTGRES_URL": "postgresql://user:pass@db:5432/products",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				expected := "postgresql://user:pass@db:5432/products"
				if cfg.Paywall.PostgresURL != expected {
					t.Errorf("Expected %s, got %s", expected, cfg.Paywall.PostgresURL)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			cfg := defaultConfig()
			cfg.applyEnvOverrides()
			tt.checkFunc(t, cfg)
		})
	}
}

func TestEnvOverrides_StripeConfig(t *testing.T) {
	defer os.Clearenv()

	tests := []struct {
		name      string
		envVars   map[string]string
		checkFunc func(*testing.T, *Config)
	}{
		{
			name: "CEDROS_STRIPE_SECRET_KEY override",
			envVars: map[string]string{
				"CEDROS_STRIPE_SECRET_KEY": "sk_live_test",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Stripe.SecretKey != "sk_live_test" {
					t.Errorf("Expected sk_live_test, got %s", cfg.Stripe.SecretKey)
				}
			},
		},
		{
			name: "CEDROS_STRIPE_MODE override to live",
			envVars: map[string]string{
				"CEDROS_STRIPE_MODE": "live",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Stripe.Mode != "live" {
					t.Errorf("Expected live, got %s", cfg.Stripe.Mode)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			cfg := defaultConfig()
			cfg.applyEnvOverrides()
			tt.checkFunc(t, cfg)
		})
	}
}

func TestEnvOverrides_CallbackHeaders(t *testing.T) {
	defer os.Clearenv()

	os.Setenv("CALLBACK_HEADER_AUTHORIZATION", "Bearer token123")
	os.Setenv("CALLBACK_HEADER_X_API_KEY", "api-key-456")

	cfg := defaultConfig()
	cfg.applyEnvOverrides()

	if cfg.Callbacks.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("Expected Authorization header to be set, got %v", cfg.Callbacks.Headers)
	}

	if cfg.Callbacks.Headers["X-Api-Key"] != "api-key-456" {
		t.Errorf("Expected X-Api-Key header to be set, got %v", cfg.Callbacks.Headers)
	}
}

func TestEnvOverrides_APIKeyConfig(t *testing.T) {
	defer os.Clearenv()

	tests := []struct {
		name      string
		envVars   map[string]string
		checkFunc func(*testing.T, *Config)
	}{
		{
			name: "CEDROS_API_KEY_ENABLED boolean (true)",
			envVars: map[string]string{
				"CEDROS_API_KEY_ENABLED": "true",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if !cfg.APIKey.Enabled {
					t.Error("Expected APIKey.Enabled to be true")
				}
			},
		},
		{
			name: "CEDROS_API_KEY_ENABLED boolean (false)",
			envVars: map[string]string{
				"CEDROS_API_KEY_ENABLED": "false",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.APIKey.Enabled {
					t.Error("Expected APIKey.Enabled to be false")
				}
			},
		},
		{
			name: "CEDROS_API_KEY_* env vars create key-tier mappings",
			envVars: map[string]string{
				"CEDROS_API_KEY_ENABLED":        "true",
				"CEDROS_API_KEY_STRIPE_ABC123":  "partner",
				"CEDROS_API_KEY_ENTERPRISE_XYZ": "enterprise",
				"CEDROS_API_KEY_PRO_TEST":       "pro",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if !cfg.APIKey.Enabled {
					t.Error("Expected APIKey.Enabled to be true")
				}
				if len(cfg.APIKey.Keys) != 3 {
					t.Errorf("Expected 3 API keys, got %d", len(cfg.APIKey.Keys))
				}
				if cfg.APIKey.Keys["stripe_abc123"] != "partner" {
					t.Errorf("Expected stripe_abc123=partner, got %s", cfg.APIKey.Keys["stripe_abc123"])
				}
				if cfg.APIKey.Keys["enterprise_xyz"] != "enterprise" {
					t.Errorf("Expected enterprise_xyz=enterprise, got %s", cfg.APIKey.Keys["enterprise_xyz"])
				}
				if cfg.APIKey.Keys["pro_test"] != "pro" {
					t.Errorf("Expected pro_test=pro, got %s", cfg.APIKey.Keys["pro_test"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			cfg := defaultConfig()
			cfg.applyEnvOverrides()
			tt.checkFunc(t, cfg)
		})
	}
}

// TestLoadServerWalletKeys and TestNormalizeRoutePrefix already exist in config_test.go
