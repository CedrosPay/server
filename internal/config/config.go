package config

import (
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Load reads configuration from a YAML file and applies environment overrides.
func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	if path != "" {
		if err := cfg.parseFile(path); err != nil {
			return nil, err
		}
	}

	cfg.applyEnvOverrides()

	if err := cfg.finalize(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// defaultConfig returns a Config with sensible defaults.
func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:      ":8080",
			ReadTimeout:  Duration{Duration: 15 * time.Second},
			WriteTimeout: Duration{Duration: 15 * time.Second},
			IdleTimeout:  Duration{Duration: 60 * time.Second},
		},
		Stripe: StripeConfig{
			Mode:           "test",
			SuccessURL:     "http://localhost:8080/stripe/success?session_id={CHECKOUT_SESSION_ID}",
			CancelURL:      "http://localhost:8080/stripe/cancel",
			PublishableKey: "",
			TaxRateID:      "",
		},
		X402: X402Config{
			Network:                       "mainnet-beta",
			RPCURL:                        "https://api.mainnet-beta.solana.com",
			WSURL:                         "wss://api.mainnet-beta.solana.com",
			TokenDecimals:                 6,
			MemoPrefix:                    "cedros",
			AllowedTokens:                 []string{"USDC"},
			ComputeUnitLimit:              200000,
			ComputeUnitPriceMicroLamports: 1,
		},
		Paywall: PaywallConfig{
			QuoteTTL:  Duration{Duration: 5 * time.Minute},
			Resources: map[string]PaywallResource{}, // Empty by default - user must define products in config
		},
		Callbacks: CallbacksConfig{
			Headers: make(map[string]string),
			Timeout: Duration{Duration: 3 * time.Second},
			Retry: RetryConfig{
				Enabled:         true,
				MaxAttempts:     5,
				InitialInterval: Duration{Duration: 1 * time.Second},
				MaxInterval:     Duration{Duration: 5 * time.Minute},
				Multiplier:      2.0,
			},
			DLQEnabled: false,
			DLQPath:    "./data/webhook-dlq.json",
		},
		Monitoring: MonitoringConfig{
			LowBalanceThreshold: 0.01,
			CheckInterval:       Duration{Duration: 15 * time.Minute},
			Headers:             make(map[string]string),
			Timeout:             Duration{Duration: 5 * time.Second},
		},
		RateLimit: RateLimitConfig{
			// Generous limits - designed to prevent spam, not restrict legitimate use
			GlobalEnabled:      true,
			GlobalLimit:        1000,
			GlobalWindow:       Duration{Duration: 1 * time.Minute},
			PerWalletEnabled:   true,
			PerWalletLimit:     60,
			PerWalletWindow:    Duration{Duration: 1 * time.Minute},
			PerIPEnabled:       true,
			PerIPLimit:         120,
			PerIPWindow:        Duration{Duration: 1 * time.Minute},
		},
		APIKey: APIKeyConfig{
			Enabled: false,
			Keys:    make(map[string]string),
		},
		Storage: StorageConfig{
			CartQuoteTTL:    Duration{Duration: 15 * time.Minute},
			RefundQuoteTTL:  Duration{Duration: 15 * time.Minute},
			CleanupInterval: Duration{Duration: 5 * time.Minute},
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled: true,
			SolanaRPC: BreakerServiceConfig{
				MaxRequests:         3,
				Interval:            Duration{Duration: 60 * time.Second},
				Timeout:             Duration{Duration: 30 * time.Second},
				ConsecutiveFailures: 5,
				FailureRatio:        0.5,
				MinRequests:         10,
			},
			StripeAPI: BreakerServiceConfig{
				MaxRequests:         3,
				Interval:            Duration{Duration: 60 * time.Second},
				Timeout:             Duration{Duration: 30 * time.Second},
				ConsecutiveFailures: 5,
				FailureRatio:        0.5,
				MinRequests:         10,
			},
			Webhook: BreakerServiceConfig{
				MaxRequests:         5,
				Interval:            Duration{Duration: 60 * time.Second},
				Timeout:             Duration{Duration: 60 * time.Second}, // Longer timeout for webhooks
				ConsecutiveFailures: 10,                                   // More tolerant for webhooks
				FailureRatio:        0.7,
				MinRequests:         20,
			},
		},
	}
}

// parseFile reads and unmarshals a YAML configuration file.
func (c *Config) parseFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("parse config yaml: %w", err)
	}
	return nil
}
