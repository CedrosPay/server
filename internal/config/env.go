package config

import (
	"fmt"
	"net/textproto"
	"os"
	"strings"
	"time"
)

// applyEnvOverrides applies environment variable overrides to the config.
// Environment variables take precedence over YAML configuration.
// All env vars use CEDROS_ prefix for namespace isolation.
func (c *Config) applyEnvOverrides() {
	// Server config
	setIfEnv(&c.Server.Address, "CEDROS_SERVER_ADDRESS")
	setIfEnv(&c.Server.RoutePrefix, "CEDROS_ROUTE_PREFIX")
	setIfEnv(&c.Server.AdminMetricsAPIKey, "CEDROS_ADMIN_METRICS_API_KEY")

	// Normalize route prefix: ensure it starts with / and doesn't end with /
	if c.Server.RoutePrefix != "" {
		c.Server.RoutePrefix = normalizeRoutePrefix(c.Server.RoutePrefix)
	}

	// Stripe config
	setIfEnv(&c.Stripe.SecretKey, "CEDROS_STRIPE_SECRET_KEY")
	setIfEnv(&c.Stripe.WebhookSecret, "CEDROS_STRIPE_WEBHOOK_SECRET")
	setIfEnv(&c.Stripe.PublishableKey, "CEDROS_STRIPE_PUBLISHABLE_KEY")
	setIfEnv(&c.Stripe.SuccessURL, "CEDROS_STRIPE_SUCCESS_URL")
	setIfEnv(&c.Stripe.CancelURL, "CEDROS_STRIPE_CANCEL_URL")
	setIfEnv(&c.Stripe.TaxRateID, "CEDROS_STRIPE_TAX_RATE_ID")
	setIfEnv(&c.Stripe.Mode, "CEDROS_STRIPE_MODE")

	// x402 config
	setIfEnv(&c.X402.PaymentAddress, "CEDROS_X402_PAYMENT_ADDRESS")
	setIfEnv(&c.X402.TokenMint, "CEDROS_X402_TOKEN_MINT")
	setIfEnv(&c.X402.Network, "CEDROS_X402_NETWORK")
	setIfEnv(&c.X402.RPCURL, "CEDROS_X402_RPC_URL")
	setIfEnv(&c.X402.WSURL, "CEDROS_X402_WS_URL")
	setIfEnv(&c.X402.MemoPrefix, "CEDROS_X402_MEMO_PREFIX")
	setBoolIfEnv(&c.X402.SkipPreflight, "CEDROS_X402_SKIP_PREFLIGHT")
	setIfEnv(&c.X402.Commitment, "CEDROS_X402_COMMITMENT")
	setBoolIfEnv(&c.X402.GaslessEnabled, "CEDROS_X402_GASLESS_ENABLED")
	setBoolIfEnv(&c.X402.AutoCreateTokenAccount, "CEDROS_X402_AUTO_CREATE_TOKEN_ACCOUNT")

	// Load server wallet keys (X402_SERVER_WALLET_1, X402_SERVER_WALLET_2, ...)
	c.X402.ServerWalletKeys = loadServerWalletKeys()

	// Paywall config
	setIfEnv(&c.Paywall.ProductSource, "CEDROS_PAYWALL_PRODUCT_SOURCE")
	setIfEnv(&c.Paywall.PostgresURL, "CEDROS_PAYWALL_POSTGRES_URL")
	setIfEnv(&c.Paywall.MongoDBURL, "CEDROS_PAYWALL_MONGODB_URL")
	setIfEnv(&c.Paywall.MongoDBDatabase, "CEDROS_PAYWALL_MONGODB_DATABASE")
	setIfEnv(&c.Paywall.MongoDBCollection, "CEDROS_PAYWALL_MONGODB_COLLECTION")
	setDurationIfEnv(&c.Paywall.QuoteTTL, "CEDROS_PAYWALL_QUOTE_TTL")
	setDurationIfEnv(&c.Paywall.ProductCacheTTL, "CEDROS_PAYWALL_PRODUCT_CACHE_TTL")

	// Coupon config
	setIfEnv(&c.Coupons.CouponSource, "COUPON_SOURCE")
	setIfEnv(&c.Coupons.PostgresURL, "COUPON_POSTGRES_URL")
	setIfEnv(&c.Coupons.MongoDBURL, "COUPON_MONGODB_URL")
	setIfEnv(&c.Coupons.MongoDBDatabase, "COUPON_MONGODB_DATABASE")
	setIfEnv(&c.Coupons.MongoDBCollection, "COUPON_MONGODB_COLLECTION")
	if v := os.Getenv("COUPON_CACHE_TTL"); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			c.Coupons.CacheTTL = Duration{Duration: dur}
		}
	}

	// Callbacks config
	setIfEnv(&c.Callbacks.PaymentSuccessURL, "CALLBACK_PAYMENT_SUCCESS_URL")
	if v := os.Getenv("CALLBACK_TIMEOUT"); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			c.Callbacks.Timeout = Duration{Duration: dur}
		}
	}
	// Load callback headers (CALLBACK_HEADER_*)
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "CALLBACK_HEADER_") {
			continue
		}
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimPrefix(parts[0], "CALLBACK_HEADER_")
		if name == "" {
			continue
		}
		if c.Callbacks.Headers == nil {
			c.Callbacks.Headers = make(map[string]string)
		}
		headerName := textproto.CanonicalMIMEHeaderKey(strings.ReplaceAll(name, "_", "-"))
		c.Callbacks.Headers[headerName] = parts[1]
	}

	// Monitoring config
	setIfEnv(&c.Monitoring.LowBalanceAlertURL, "MONITORING_LOW_BALANCE_ALERT_URL")
	if v := os.Getenv("MONITORING_LOW_BALANCE_THRESHOLD"); v != "" {
		var threshold float64
		if _, err := fmt.Sscanf(v, "%f", &threshold); err == nil {
			c.Monitoring.LowBalanceThreshold = threshold
		}
	}
	if v := os.Getenv("MONITORING_CHECK_INTERVAL"); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			c.Monitoring.CheckInterval = Duration{Duration: dur}
		}
	}
	if v := os.Getenv("MONITORING_TIMEOUT"); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			c.Monitoring.Timeout = Duration{Duration: dur}
		}
	}
	// Load monitoring headers (MONITORING_HEADER_*)
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "MONITORING_HEADER_") {
			continue
		}
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimPrefix(parts[0], "MONITORING_HEADER_")
		if name == "" {
			continue
		}
		if c.Monitoring.Headers == nil {
			c.Monitoring.Headers = make(map[string]string)
		}
		headerName := textproto.CanonicalMIMEHeaderKey(strings.ReplaceAll(name, "_", "-"))
		c.Monitoring.Headers[headerName] = parts[1]
	}

	// API Key config
	setBoolIfEnv(&c.APIKey.Enabled, "CEDROS_API_KEY_ENABLED")
	// Load API keys (CEDROS_API_KEY_*)
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "CEDROS_API_KEY_") {
			continue
		}
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimPrefix(parts[0], "CEDROS_API_KEY_")
		if name == "" || name == "ENABLED" {
			continue
		}
		if c.APIKey.Keys == nil {
			c.APIKey.Keys = make(map[string]string)
		}
		// CEDROS_API_KEY_STRIPE_ABC123=partner -> key: "stripe_abc123", tier: "partner"
		key := strings.ToLower(name)
		tier := strings.TrimSpace(parts[1])
		c.APIKey.Keys[key] = tier
	}
}

// setIfEnv sets a string pointer to the environment variable value if it exists.
func setIfEnv(target *string, key string) {
	if val := os.Getenv(key); val != "" {
		*target = val
	}
}

// setBoolIfEnv sets a boolean pointer from an environment variable.
// Accepts "1", "true", "TRUE", "True" as true values.
func setBoolIfEnv(target *bool, key string) {
	if v := os.Getenv(key); v != "" {
		*target = v == "1" || strings.EqualFold(v, "true")
	}
}

// setDurationIfEnv sets a Duration pointer from an environment variable.
// Uses time.ParseDuration to parse values like "5m", "120s", "1h30m".
func setDurationIfEnv(target *Duration, key string) {
	if v := os.Getenv(key); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			*target = Duration{Duration: dur}
		}
	}
}

// loadServerWalletKeys loads server wallet keys from environment variables.
// Looks for X402_SERVER_WALLET_1, X402_SERVER_WALLET_2, X402_SERVER_WALLET_3, etc.
// Stops when it finds a gap in the numbering.
func loadServerWalletKeys() []string {
	var keys []string
	for i := 1; i <= 100; i++ { // Reasonable upper limit
		key := fmt.Sprintf("X402_SERVER_WALLET_%d", i)
		val := os.Getenv(key)
		if val == "" {
			// Stop at first missing key
			break
		}
		keys = append(keys, val)
	}
	return keys
}

// normalizeRoutePrefix ensures the prefix starts with / and doesn't end with /.
// Examples: "api" -> "/api", "/api/" -> "/api", "cedros-pay" -> "/cedros-pay"
func normalizeRoutePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	// Ensure it starts with /
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	// Ensure it doesn't end with /
	prefix = strings.TrimSuffix(prefix, "/")
	return prefix
}
