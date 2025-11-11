package config

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration to support string based YAML decoding.
type Duration struct {
	time.Duration
}

// UnmarshalYAML parses duration values expressed as Go-style strings or numbers interpreted as seconds.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		raw := strings.TrimSpace(value.Value)
		if raw == "" {
			d.Duration = 0
			return nil
		}
		parsed, err := time.ParseDuration(raw)
		if err == nil {
			d.Duration = parsed
			return nil
		}
		secs, convErr := time.ParseDuration(fmt.Sprintf("%ss", raw))
		if convErr == nil {
			d.Duration = secs
			return nil
		}
		return fmt.Errorf("invalid duration value %q: %w", raw, err)
	default:
		return fmt.Errorf("unsupported duration node kind: %v", value.Kind)
	}
}

// MarshalYAML renders the duration as a string to keep config edits human-friendly.
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.Duration.String(), nil
}

// Config holds application level configuration aggregated from file and environment variables.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Logging    LoggingConfig    `yaml:"logging"`
	Stripe     StripeConfig     `yaml:"stripe"`
	X402       X402Config       `yaml:"x402"`
	Paywall    PaywallConfig    `yaml:"paywall"`
	Storage    StorageConfig    `yaml:"storage"`
	Coupons    CouponConfig     `yaml:"coupons"`
	Callbacks      CallbacksConfig      `yaml:"callbacks"`
	Monitoring     MonitoringConfig     `yaml:"monitoring"`
	RateLimit      RateLimitConfig      `yaml:"rate_limit"`
	APIKey         APIKeyConfig         `yaml:"api_key"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Address              string   `yaml:"address"`
	ReadTimeout          Duration `yaml:"read_timeout"`
	WriteTimeout         Duration `yaml:"write_timeout"`
	IdleTimeout          Duration `yaml:"idle_timeout"`
	CORSAllowedOrigins   []string `yaml:"cors_allowed_origins"`
	RoutePrefix          string   `yaml:"route_prefix"`          // Optional prefix for all routes (e.g., "/api", "/cedros")
	AdminMetricsAPIKey   string   `yaml:"admin_metrics_api_key"` // Optional API key to protect /metrics endpoint (leave empty to disable protection)
}

// StripeConfig holds Stripe payment integration configuration.
type StripeConfig struct {
	SecretKey      string `yaml:"secret_key"`
	WebhookSecret  string `yaml:"webhook_secret"`
	PublishableKey string `yaml:"publishable_key"`
	SuccessURL     string `yaml:"success_url"`
	CancelURL      string `yaml:"cancel_url"`
	TaxRateID      string `yaml:"tax_rate_id"`
	Mode           string `yaml:"mode"` // live | test
}

// GetSecretKey returns the Stripe secret key.
func (s StripeConfig) GetSecretKey() string {
	return s.SecretKey
}

// GetSuccessURL returns the Stripe success redirect URL.
func (s StripeConfig) GetSuccessURL() string {
	return s.SuccessURL
}

// GetCancelURL returns the Stripe cancel redirect URL.
func (s StripeConfig) GetCancelURL() string {
	return s.CancelURL
}

// GetTaxRateID returns the Stripe tax rate ID.
func (s StripeConfig) GetTaxRateID() string {
	return s.TaxRateID
}

// X402Config holds x402 protocol and Solana configuration.
type X402Config struct {
	PaymentAddress                string   `yaml:"payment_address"`
	TokenMint                     string   `yaml:"token_mint"`
	Network                       string   `yaml:"network"`
	RPCURL                        string   `yaml:"rpc_url"`
	WSURL                         string   `yaml:"ws_url"`
	TokenDecimals                 uint8    `yaml:"token_decimals"`
	MemoPrefix                    string   `yaml:"memo_prefix"`
	AllowedTokens                 []string `yaml:"allowed_tokens"`
	SkipPreflight                 bool     `yaml:"skip_preflight"`
	Commitment                    string   `yaml:"commitment"`
	GaslessEnabled                bool     `yaml:"gasless_enabled"`                   // Pay network fees for users
	AutoCreateTokenAccount        bool     `yaml:"auto_create_token_account"`         // Auto-create missing token accounts
	ServerWalletKeys              []string `yaml:"-"`                                 // Loaded from env (X402_SERVER_WALLET_1, X402_SERVER_WALLET_2, ...), used for both gasless and token account creation
	TxQueueMinTimeBetween         Duration `yaml:"tx_queue_min_time_between"`         // Minimum time between transaction sends (e.g., "100ms", "1s") - set to 0 for unlimited RPC
	TxQueueMaxInFlight            int      `yaml:"tx_queue_max_in_flight"`            // Maximum concurrent in-flight transactions (sent but waiting for confirmation) - set to 0 for unlimited
	ComputeUnitLimit              uint32   `yaml:"compute_unit_limit"`                // Compute unit limit for transactions (default: 200000)
	ComputeUnitPriceMicroLamports uint64   `yaml:"compute_unit_price_micro_lamports"` // Priority fee in microlamports (default: 1)
}

// PaywallConfig holds paywall service configuration.
type PaywallConfig struct {
	QuoteTTL          Duration                   `yaml:"quote_ttl"`
	ProductSource     string                     `yaml:"product_source"`      // "yaml", "postgres", or "mongodb"
	ProductCacheTTL   Duration                   `yaml:"product_cache_ttl"`   // How long to cache product list (0 = no cache)
	PostgresURL       string                     `yaml:"postgres_url"`        // PostgreSQL connection string
	PostgresTableName string                     `yaml:"postgres_table_name"` // PostgreSQL table name (auto-populated from schema_mapping)
	MongoDBURL        string                     `yaml:"mongodb_url"`         // MongoDB connection string
	MongoDBDatabase   string                     `yaml:"mongodb_database"`    // MongoDB database name
	MongoDBCollection string                     `yaml:"mongodb_collection"`  // MongoDB collection name
	Resources         map[string]PaywallResource `yaml:"resources"`           // Only used when ProductSource = "yaml"
	PostgresPool      PostgresPoolConfig         `yaml:"postgres_pool"`       // PostgreSQL connection pool settings
}

// PaywallResource defines a single protected resource with pricing.
// All monetary amounts use atomic units (int64) for precision.
type PaywallResource struct {
	ResourceID         string            `yaml:"resource_id"`
	Description        string            `yaml:"description"`
	StripePriceID      string            `yaml:"stripe_price_id"`
	FiatAmountCents    int64             `yaml:"fiat_amount_cents"`
	FiatCurrency       string            `yaml:"fiat_currency"`
	CryptoAtomicAmount int64             `yaml:"crypto_atomic_amount"`
	CryptoToken        string            `yaml:"crypto_token"`
	CryptoAccount      string            `yaml:"crypto_account"`
	MemoTemplate       string            `yaml:"memo_template"`
	Metadata           map[string]string `yaml:"metadata"`
	Extras             map[string]any    `yaml:"extras"`
}

// CallbacksConfig holds webhook callback configuration.
type CallbacksConfig struct {
	PaymentSuccessURL string            `yaml:"payment_success_url"`
	Headers           map[string]string `yaml:"headers"`
	Body              string            `yaml:"body"`
	BodyTemplate      string            `yaml:"body_template"`
	Timeout           Duration          `yaml:"timeout"`
	Retry             RetryConfig       `yaml:"retry"` // Retry configuration with exponential backoff
	DLQEnabled        bool              `yaml:"dlq_enabled"` // Enable dead letter queue for failed webhooks
	DLQPath           string            `yaml:"dlq_path"`    // File path for DLQ storage (default: ./data/webhook-dlq.json)
}

// RetryConfig holds webhook retry configuration.
type RetryConfig struct {
	Enabled         bool     `yaml:"enabled"`          // Enable retry with exponential backoff (default: true)
	MaxAttempts     int      `yaml:"max_attempts"`     // Maximum retry attempts (default: 5)
	InitialInterval Duration `yaml:"initial_interval"` // Initial backoff interval (default: 1s)
	MaxInterval     Duration `yaml:"max_interval"`     // Maximum backoff interval (default: 5m)
	Multiplier      float64  `yaml:"multiplier"`       // Backoff multiplier (default: 2.0)
}

// MonitoringConfig holds balance monitoring configuration.
type MonitoringConfig struct {
	LowBalanceAlertURL  string            `yaml:"low_balance_alert_url"` // Webhook URL for low balance alerts (Discord, Slack, etc.)
	LowBalanceThreshold float64           `yaml:"low_balance_threshold"` // SOL threshold to trigger alert (default: 0.01)
	CheckInterval       Duration          `yaml:"check_interval"`        // How often to check balances (default: 15m)
	Headers             map[string]string `yaml:"headers"`               // Custom headers for webhook
	BodyTemplate        string            `yaml:"body_template"`         // Custom body template (Go template)
	Timeout             Duration          `yaml:"timeout"`               // Request timeout (default: 5s)
}

// PostgresPoolConfig holds PostgreSQL connection pool settings.
type PostgresPoolConfig struct {
	MaxOpenConns    int      `yaml:"max_open_conns"`    // Maximum number of open connections (default: 25)
	MaxIdleConns    int      `yaml:"max_idle_conns"`    // Maximum number of idle connections (default: 5)
	ConnMaxLifetime Duration `yaml:"conn_max_lifetime"` // Maximum lifetime of connections (default: 5m)
}

// StorageConfig holds storage backend configuration.
type StorageConfig struct {
	Backend         string               `yaml:"backend"`           // "memory", "postgres", "mongodb", or "file"
	PostgresURL     string               `yaml:"postgres_url"`      // PostgreSQL connection string
	MongoDBURL      string               `yaml:"mongodb_url"`       // MongoDB connection string
	MongoDBDatabase string               `yaml:"mongodb_database"`  // MongoDB database name
	FilePath        string               `yaml:"file_path"`         // Path to JSON file for file backend
	PostgresPool    PostgresPoolConfig   `yaml:"postgres_pool"`     // PostgreSQL connection pool settings
	Archival        ArchivalConfig       `yaml:"archival"`          // Automatic archival configuration
	CartQuoteTTL    Duration             `yaml:"cart_quote_ttl"`    // How long cart quotes remain valid (default: 15m)
	RefundQuoteTTL  Duration             `yaml:"refund_quote_ttl"`  // How long refund quotes remain valid (default: 15m)
	CleanupInterval Duration             `yaml:"cleanup_interval"`  // How often to clean up expired quotes (default: 5m)
	SchemaMapping   SchemaMappingConfig  `yaml:"schema_mapping"`    // Table/collection name mappings for all entities
}

// SchemaMappingConfig holds table/collection name mappings for custom schemas.
type SchemaMappingConfig struct {
	Payments     TableMappingConfig `yaml:"payments"`      // Payment transactions table/collection
	Sessions     TableMappingConfig `yaml:"sessions"`      // Stripe sessions table/collection
	Products     TableMappingConfig `yaml:"products"`      // Products table/collection
	Coupons      TableMappingConfig `yaml:"coupons"`       // Coupons table/collection
	CartQuotes   TableMappingConfig `yaml:"cart_quotes"`   // Cart quotes table/collection
	RefundQuotes TableMappingConfig `yaml:"refund_quotes"` // Refund quotes table/collection
	AdminNonces  TableMappingConfig `yaml:"admin_nonces"`  // Admin nonces table/collection
	WebhookQueue TableMappingConfig `yaml:"webhook_queue"` // Webhook queue table/collection
}

// TableMappingConfig defines a single table/collection mapping.
type TableMappingConfig struct {
	TableName string `yaml:"table_name"` // Custom table/collection name
}

// ArchivalConfig holds automatic payment signature archival configuration.
type ArchivalConfig struct {
	Enabled         bool     `yaml:"enabled"`          // Enable automatic archival (default: false)
	RetentionPeriod Duration `yaml:"retention_period"` // How long to keep payment signatures (default: 90 days)
	RunInterval     Duration `yaml:"run_interval"`     // How often to run archival (default: 24 hours)
}

// CouponConfig holds coupon system configuration.
type CouponConfig struct {
	CouponSource      string            `yaml:"coupon_source"`      // "yaml", "postgres", "mongodb", or "disabled"
	PostgresURL       string            `yaml:"postgres_url"`       // PostgreSQL connection string
	PostgresTableName string            `yaml:"postgres_table_name"` // PostgreSQL table name (auto-populated from schema_mapping)
	MongoDBURL        string            `yaml:"mongodb_url"`        // MongoDB connection string
	MongoDBDatabase   string            `yaml:"mongodb_database"`   // MongoDB database name
	MongoDBCollection string            `yaml:"mongodb_collection"` // MongoDB collection name (default: "coupons")
	CacheTTL          Duration          `yaml:"cache_ttl"`          // Cache TTL for coupon validation (short, e.g., 1m)
	Coupons           map[string]Coupon `yaml:"coupons"`            // Only used when CouponSource = "yaml"
	PostgresPool      PostgresPoolConfig `yaml:"postgres_pool"`     // PostgreSQL connection pool settings
}

// Coupon defines a discount code in YAML configuration.
type Coupon struct {
	Code          string            `yaml:"code"`
	DiscountType  string            `yaml:"discount_type"`  // "percentage" or "fixed"
	DiscountValue float64           `yaml:"discount_value"` // Percentage (0-100) or fixed amount
	Currency      string            `yaml:"currency"`       // For fixed discounts (usd, usdc, etc.)
	Scope         string            `yaml:"scope"`          // "all" or "specific"
	ProductIDs    []string          `yaml:"product_ids"`    // Applicable product IDs (for scope=specific)
	PaymentMethod string            `yaml:"payment_method"` // Restrict to payment method: "stripe", "x402", or "" for any
	AutoApply     bool              `yaml:"auto_apply"`     // If true, automatically apply to matching products
	AppliesAt     string            `yaml:"applies_at"`     // When to display: "catalog" (product page) or "checkout" (cart only)
	UsageLimit    *int              `yaml:"usage_limit"`    // nil = unlimited, N = max uses
	UsageCount    int               `yaml:"usage_count"`    // Current redemption count
	StartsAt      string            `yaml:"starts_at"`      // RFC3339 timestamp when coupon becomes valid
	ExpiresAt     string            `yaml:"expires_at"`     // RFC3339 timestamp when coupon expires
	Active        bool              `yaml:"active"`         // Enable/disable coupon
	Metadata      map[string]string `yaml:"metadata"`       // Custom key-value pairs
}

// LoggingConfig holds structured logging configuration.
type LoggingConfig struct {
	Level       string `yaml:"level"`       // debug, info, warn, error (default: info)
	Format      string `yaml:"format"`      // json, console (default: json)
	Environment string `yaml:"environment"` // production, staging, development
}

// RateLimitConfig holds rate limiting configuration.
// Provides multi-tier rate limiting to prevent spam while allowing legitimate use.
type RateLimitConfig struct {
	// Global rate limiting (across all users)
	GlobalEnabled bool     `yaml:"global_enabled"` // Enable global rate limiting
	GlobalLimit   int      `yaml:"global_limit"`   // Requests allowed per global window
	GlobalWindow  Duration `yaml:"global_window"`  // Time window for global limit

	// Per-wallet rate limiting (identified by X-Wallet header)
	PerWalletEnabled bool     `yaml:"per_wallet_enabled"` // Enable per-wallet rate limiting
	PerWalletLimit   int      `yaml:"per_wallet_limit"`   // Requests allowed per wallet per window
	PerWalletWindow  Duration `yaml:"per_wallet_window"`  // Time window for per-wallet limit

	// Per-IP rate limiting (fallback when wallet not identified)
	PerIPEnabled bool     `yaml:"per_ip_enabled"` // Enable per-IP rate limiting
	PerIPLimit   int      `yaml:"per_ip_limit"`   // Requests allowed per IP per window
	PerIPWindow  Duration `yaml:"per_ip_window"`  // Time window for per-IP limit
}

// APIKeyConfig holds API key authentication and tier configuration.
// Allows trusted partners to bypass rate limits via X-API-Key header.
type APIKeyConfig struct {
	Enabled bool              `yaml:"enabled"` // Enable API key authentication (default: false)
	Keys    map[string]string `yaml:"keys"`    // Map of API key -> tier (free, pro, enterprise, partner)
}

// CircuitBreakerConfig holds circuit breaker configuration for external services.
// Prevents cascading failures by failing fast when external services are degraded.
type CircuitBreakerConfig struct {
	Enabled   bool                      `yaml:"enabled"`    // Enable circuit breakers (default: true)
	SolanaRPC BreakerServiceConfig      `yaml:"solana_rpc"` // Solana RPC circuit breaker
	StripeAPI BreakerServiceConfig      `yaml:"stripe_api"` // Stripe API circuit breaker
	Webhook   BreakerServiceConfig      `yaml:"webhook"`    // Webhook delivery circuit breaker
}

// BreakerServiceConfig configures a circuit breaker for a specific external service.
type BreakerServiceConfig struct {
	MaxRequests         uint32   `yaml:"max_requests"`         // Max requests in half-open state (default: 3)
	Interval            Duration `yaml:"interval"`             // Stats reset interval in closed state (default: 60s)
	Timeout             Duration `yaml:"timeout"`              // Open state timeout before half-open (default: 30s)
	ConsecutiveFailures uint32   `yaml:"consecutive_failures"` // Consecutive failures to trip (default: 5)
	FailureRatio        float64  `yaml:"failure_ratio"`        // Failure ratio to trip 0.0-1.0 (default: 0.5)
	MinRequests         uint32   `yaml:"min_requests"`         // Minimum requests before checking ratio (default: 10)
}
