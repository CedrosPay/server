package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Test loading with empty path uses defaults
	cfg, err := Load("")
	if err == nil {
		t.Fatal("expected error when required fields are missing, got nil")
	}
	if cfg != nil {
		t.Fatal("expected nil config when validation fails")
	}
}

func TestLoadConfig_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr string
	}{
		{
			name: "missing payment address",
			envVars: map[string]string{
				"X402_TOKEN_MINT": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
				"SOLANA_RPC_URL":  "https://api.mainnet-beta.solana.com",
			},
			wantErr: "x402.payment_address is required",
		},
		{
			name: "missing token mint",
			envVars: map[string]string{
				"X402_PAYMENT_ADDRESS": "11111111111111111111111111111111",
				"SOLANA_RPC_URL":       "https://api.mainnet-beta.solana.com",
			},
			wantErr: "x402.token_mint is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			clearEnv()
			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}
			defer clearEnv()

			_, err := Load("")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantErr != "" && err.Error() != tt.wantErr {
				if !contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
			}
		})
	}
}

func TestLoadConfig_ValidMinimal(t *testing.T) {
	clearEnv()
	os.Setenv("CEDROS_X402_PAYMENT_ADDRESS", "11111111111111111111111111111111")
	os.Setenv("CEDROS_X402_TOKEN_MINT", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	os.Setenv("CEDROS_X402_RPC_URL", "https://api.mainnet-beta.solana.com")
	os.Setenv("CEDROS_PAYWALL_PRODUCT_SOURCE", "postgres")                          // Use postgres source so YAML resources aren't required
	os.Setenv("CEDROS_PAYWALL_POSTGRES_URL", "postgres://user:pass@localhost/test") // Dummy URL for validation
	defer clearEnv()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error with valid config, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}

	// Check defaults were applied
	if cfg.Server.Address != ":8080" {
		t.Errorf("expected default address :8080, got %s", cfg.Server.Address)
	}
	if cfg.Stripe.Mode != "test" {
		t.Errorf("expected default stripe mode 'test', got %s", cfg.Stripe.Mode)
	}
	if cfg.Paywall.QuoteTTL.Duration != 5*time.Minute {
		t.Errorf("expected default quote TTL 5m, got %v", cfg.Paywall.QuoteTTL.Duration)
	}

	// Check WebSocket URL was auto-derived
	if cfg.X402.WSURL == "" {
		t.Error("expected WebSocket URL to be auto-derived")
	}
	if cfg.X402.WSURL != "wss://api.mainnet-beta.solana.com" {
		t.Errorf("expected wss URL, got %s", cfg.X402.WSURL)
	}
}

func TestLoadConfig_GaslessRequiresWallets(t *testing.T) {
	clearEnv()
	os.Setenv("CEDROS_X402_PAYMENT_ADDRESS", "11111111111111111111111111111111")
	os.Setenv("CEDROS_X402_TOKEN_MINT", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	os.Setenv("CEDROS_X402_RPC_URL", "https://api.mainnet-beta.solana.com")
	os.Setenv("CEDROS_X402_GASLESS_ENABLED", "true")
	os.Setenv("CEDROS_PAYWALL_PRODUCT_SOURCE", "postgres")
	os.Setenv("CEDROS_PAYWALL_POSTGRES_URL", "postgres://user:pass@localhost/test")
	defer clearEnv()

	_, err := Load("")
	if err == nil {
		t.Fatal("expected error when gasless enabled without server wallets")
	}
	if !contains(err.Error(), "server_wallet_keys") {
		t.Errorf("expected error about server_wallet_keys, got: %v", err)
	}
}

func TestLoadConfig_AutoCreateRequiresWallets(t *testing.T) {
	clearEnv()
	os.Setenv("CEDROS_X402_PAYMENT_ADDRESS", "11111111111111111111111111111111")
	os.Setenv("CEDROS_X402_TOKEN_MINT", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	os.Setenv("CEDROS_X402_RPC_URL", "https://api.mainnet-beta.solana.com")
	os.Setenv("CEDROS_X402_AUTO_CREATE_TOKEN_ACCOUNT", "true")
	os.Setenv("CEDROS_PAYWALL_PRODUCT_SOURCE", "postgres")
	os.Setenv("CEDROS_PAYWALL_POSTGRES_URL", "postgres://user:pass@localhost/test")
	defer clearEnv()

	_, err := Load("")
	if err == nil {
		t.Fatal("expected error when auto-create enabled without server wallets")
	}
	if !contains(err.Error(), "server_wallet_keys") {
		t.Errorf("expected error about server_wallet_keys, got: %v", err)
	}
}

func TestLoadServerWalletKeys(t *testing.T) {
	clearEnv()
	os.Setenv("X402_SERVER_WALLET_1", "wallet1")
	os.Setenv("X402_SERVER_WALLET_2", "wallet2")
	os.Setenv("X402_SERVER_WALLET_3", "wallet3")
	// Gap - X402_SERVER_WALLET_4 missing
	os.Setenv("X402_SERVER_WALLET_5", "wallet5")
	defer clearEnv()

	keys := loadServerWalletKeys()
	if len(keys) != 3 {
		t.Errorf("expected 3 wallets (stops at gap), got %d", len(keys))
	}
	if keys[0] != "wallet1" || keys[1] != "wallet2" || keys[2] != "wallet3" {
		t.Errorf("unexpected wallet keys: %v", keys)
	}
}

func TestNormalizeRoutePrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"api", "/api"},
		{"/api", "/api"},
		{"/api/", "/api"},
		{"  /api/  ", "/api"},
		{"cedros-pay", "/cedros-pay"},
		{"/v1/cedros", "/v1/cedros"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeRoutePrefix(tt.input)
			if got != tt.want {
				t.Errorf("normalizeRoutePrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMonitoringThresholdValidation(t *testing.T) {
	clearEnv()
	os.Setenv("CEDROS_X402_PAYMENT_ADDRESS", "11111111111111111111111111111111")
	os.Setenv("CEDROS_X402_TOKEN_MINT", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	os.Setenv("CEDROS_X402_RPC_URL", "https://api.mainnet-beta.solana.com")
	os.Setenv("CEDROS_X402_GASLESS_ENABLED", "true")
	os.Setenv("X402_SERVER_WALLET_1", "test_wallet")
	os.Setenv("MONITORING_LOW_BALANCE_ALERT_URL", "https://example.com/webhook")
	os.Setenv("MONITORING_LOW_BALANCE_THRESHOLD", "0.0005") // Below critical threshold
	os.Setenv("CEDROS_PAYWALL_PRODUCT_SOURCE", "postgres")
	os.Setenv("CEDROS_PAYWALL_POSTGRES_URL", "postgres://user:pass@localhost/test")
	defer clearEnv()

	_, err := Load("")
	if err == nil {
		t.Fatal("expected error when threshold is below critical balance")
	}
	if !contains(err.Error(), "below critical threshold") {
		t.Errorf("expected error about threshold, got: %v", err)
	}
}

// Test helpers

func clearEnv() {
	// Clear all relevant env vars
	envVars := []string{
		"SERVER_ADDRESS", "ROUTE_PREFIX",
		"STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET", "STRIPE_PUBLISHABLE_KEY",
		"STRIPE_SUCCESS_URL", "STRIPE_CANCEL_URL", "STRIPE_TAX_RATE_ID", "STRIPE_MODE",
		"X402_PAYMENT_ADDRESS", "X402_TOKEN_MINT", "X402_NETWORK",
		"SOLANA_RPC_URL", "SOLANA_WS_URL", "X402_MEMO_PREFIX",
		"X402_SKIP_PREFLIGHT", "X402_COMMITMENT",
		"X402_GASLESS_ENABLED", "X402_AUTO_CREATE_TOKEN_ACCOUNT",
		"X402_SERVER_WALLET_1", "X402_SERVER_WALLET_2", "X402_SERVER_WALLET_3",
		"PAYWALL_QUOTE_TTL",
		"CALLBACK_PAYMENT_SUCCESS_URL", "CALLBACK_TIMEOUT",
		"MONITORING_LOW_BALANCE_ALERT_URL", "MONITORING_LOW_BALANCE_THRESHOLD",
		"MONITORING_CHECK_INTERVAL", "MONITORING_TIMEOUT",
	}
	for _, key := range envVars {
		os.Unsetenv(key)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAny(s, substr))
}

func containsAny(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
