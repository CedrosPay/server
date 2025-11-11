package config

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/CedrosPay/server/internal/money"
	"github.com/gagliardetto/solana-go/rpc"
)

// finalize applies defaults and validates the configuration.
func (c *Config) finalize() error {
	// Apply defaults
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	if c.Logging.Environment == "" {
		c.Logging.Environment = "production"
	}
	if c.Stripe.Mode == "" {
		c.Stripe.Mode = "test"
	}
	if c.Server.Address == "" {
		c.Server.Address = ":8080"
	}

	// IMPORTANT: Auto-configure product_source and coupon_source from storage.backend
	// This simplifies configuration - users only need to set storage.backend once
	// If explicitly set, respect user's choice (allow override)
	if c.Paywall.ProductSource == "" {
		// Map storage backend to product source
		switch c.Storage.Backend {
		case "postgres":
			c.Paywall.ProductSource = "postgres"
		case "mongodb":
			c.Paywall.ProductSource = "mongodb"
		default:
			c.Paywall.ProductSource = "yaml" // file/memory/empty default to yaml
		}
	}

	if c.Coupons.CouponSource == "" {
		// Map storage backend to coupon source
		switch c.Storage.Backend {
		case "postgres":
			c.Coupons.CouponSource = "postgres"
		case "mongodb":
			c.Coupons.CouponSource = "mongodb"
		default:
			c.Coupons.CouponSource = "yaml" // file/memory/empty default to yaml
		}
	}

	// Auto-copy database connection URLs from storage config to paywall/coupons
	// This completes the unified storage configuration - users only set URLs once
	if c.Paywall.ProductSource == "postgres" {
		if c.Paywall.PostgresURL == "" {
			c.Paywall.PostgresURL = c.Storage.PostgresURL
		}
		// Auto-copy table name from schema_mapping if set
		if c.Paywall.PostgresTableName == "" && c.Storage.SchemaMapping.Products.TableName != "" {
			c.Paywall.PostgresTableName = c.Storage.SchemaMapping.Products.TableName
		}
	}
	if c.Paywall.ProductSource == "mongodb" {
		if c.Paywall.MongoDBURL == "" {
			c.Paywall.MongoDBURL = c.Storage.MongoDBURL
		}
		if c.Paywall.MongoDBDatabase == "" {
			c.Paywall.MongoDBDatabase = c.Storage.MongoDBDatabase
		}
		// Auto-copy collection name from schema_mapping if set
		if c.Paywall.MongoDBCollection == "" && c.Storage.SchemaMapping.Products.TableName != "" {
			c.Paywall.MongoDBCollection = c.Storage.SchemaMapping.Products.TableName
		}
	}

	if c.Coupons.CouponSource == "postgres" {
		if c.Coupons.PostgresURL == "" {
			c.Coupons.PostgresURL = c.Storage.PostgresURL
		}
		// Auto-copy table name from schema_mapping if set
		if c.Coupons.PostgresTableName == "" && c.Storage.SchemaMapping.Coupons.TableName != "" {
			c.Coupons.PostgresTableName = c.Storage.SchemaMapping.Coupons.TableName
		}
	}
	if c.Coupons.CouponSource == "mongodb" {
		if c.Coupons.MongoDBURL == "" {
			c.Coupons.MongoDBURL = c.Storage.MongoDBURL
		}
		if c.Coupons.MongoDBDatabase == "" {
			c.Coupons.MongoDBDatabase = c.Storage.MongoDBDatabase
		}
		// Auto-copy collection name from schema_mapping if set
		if c.Coupons.MongoDBCollection == "" && c.Storage.SchemaMapping.Coupons.TableName != "" {
			c.Coupons.MongoDBCollection = c.Storage.SchemaMapping.Coupons.TableName
		}
	}

	if c.Paywall.QuoteTTL.Duration == 0 {
		c.Paywall.QuoteTTL = Duration{Duration: 5 * time.Minute}
	}
	if c.Callbacks.Timeout.Duration == 0 {
		c.Callbacks.Timeout = Duration{Duration: 3 * time.Second}
	}
	if c.Callbacks.Headers == nil {
		c.Callbacks.Headers = make(map[string]string)
	}
	if c.Monitoring.LowBalanceThreshold <= 0 {
		c.Monitoring.LowBalanceThreshold = 0.01
	}
	if c.Monitoring.CheckInterval.Duration <= 0 {
		c.Monitoring.CheckInterval = Duration{Duration: 15 * time.Minute}
	}
	if c.Monitoring.Timeout.Duration <= 0 {
		c.Monitoring.Timeout = Duration{Duration: 5 * time.Second}
	}
	if c.Monitoring.Headers == nil {
		c.Monitoring.Headers = make(map[string]string)
	}
	if c.X402.Commitment == "" {
		c.X402.Commitment = string(rpc.CommitmentConfirmed)
	}
	switch strings.ToLower(c.X402.Commitment) {
	case "processed", "confirmed", "finalized", "finalised":
	default:
		c.X402.Commitment = string(rpc.CommitmentConfirmed)
	}

	// IMPORTANT: Clear YAML resources when using database sources
	// This prevents confusion where users have both YAML and database configured
	// and expect database to be used but YAML silently takes precedence
	if c.Paywall.ProductSource == "postgres" || c.Paywall.ProductSource == "mongodb" {
		if len(c.Paywall.Resources) > 0 {
			fmt.Printf("⚠️  WARNING: paywall.resources (YAML) is defined but product_source='%s'\n", c.Paywall.ProductSource)
			fmt.Printf("   Ignoring YAML resources and using %s database instead.\n", c.Paywall.ProductSource)
			fmt.Printf("   Remove paywall.resources from config to suppress this warning.\n")
			// Clear YAML resources to prevent confusion
			c.Paywall.Resources = nil
		}
	}

	// IMPORTANT: Clear YAML coupons when using database sources
	if c.Coupons.CouponSource == "postgres" || c.Coupons.CouponSource == "mongodb" {
		if len(c.Coupons.Coupons) > 0 {
			fmt.Printf("⚠️  WARNING: coupons.coupons (YAML) is defined but coupon_source='%s'\n", c.Coupons.CouponSource)
			fmt.Printf("   Ignoring YAML coupons and using %s database instead.\n", c.Coupons.CouponSource)
			fmt.Printf("   Remove coupons.coupons from config to suppress this warning.\n")
			// Clear YAML coupons to prevent confusion
			c.Coupons.Coupons = nil
		}
	}

	// Normalize resource fields (only for YAML source)
	for key, resource := range c.Paywall.Resources {
		if resource.ResourceID == "" {
			resource.ResourceID = key
		}
		if resource.FiatCurrency == "" {
			resource.FiatCurrency = "usd"
		}
		if resource.CryptoToken == "" {
			resource.CryptoToken = c.X402.TokenMint
		}
		c.Paywall.Resources[key] = resource
	}

	return c.validate()
}

// validate checks that required configuration fields are set correctly.
func (c *Config) validate() error {
	var errs []string

	// Stripe validation
	if c.Stripe.SecretKey == "" && c.Stripe.PublishableKey != "" {
		errs = append(errs, "stripe.secret_key is required when publishable key is set")
	}

	// Paywall validation
	// Only require resources when using YAML product source (default)
	productSource := c.Paywall.ProductSource
	if productSource == "" {
		productSource = "yaml" // default
	}
	if productSource == "yaml" && len(c.Paywall.Resources) == 0 {
		errs = append(errs, "paywall.resources must define at least one resource when product_source is 'yaml'")
	}
	for name, resource := range c.Paywall.Resources {
		if resource.FiatAmountCents <= 0 && resource.CryptoAtomicAmount <= 0 && resource.StripePriceID == "" {
			errs = append(errs, fmt.Sprintf("paywall.resource %q must define fiat_amount_cents, crypto_atomic_amount, or stripe_price_id", name))
		}
	}

	// x402 validation
	if c.X402.PaymentAddress == "" {
		errs = append(errs, "x402.payment_address is required")
	}
	if c.X402.TokenMint == "" {
		errs = append(errs, "x402.token_mint is required")
	} else {
		// CRITICAL: Validate token mint is a known stablecoin
		// This prevents catastrophic misconfigurations where payments go to wrong token
		if err := validateStablecoinMint(c.X402.TokenMint); err != nil {
			errs = append(errs, fmt.Sprintf("x402.token_mint validation failed: %v", err))
		}
	}
	if c.X402.RPCURL == "" {
		errs = append(errs, "x402.rpc_url is required")
	}
	if (c.X402.GaslessEnabled || c.X402.AutoCreateTokenAccount) && len(c.X402.ServerWalletKeys) == 0 {
		errs = append(errs, "x402.server_wallet_keys (X402_SERVER_WALLET_1, X402_SERVER_WALLET_2, ...) is required when gasless_enabled or auto_create_token_account is enabled")
	}

	// Auto-derive WebSocket URL if not set
	if c.X402.WSURL == "" && c.X402.RPCURL != "" {
		wsURL, err := deriveWebsocketURL(c.X402.RPCURL)
		if err != nil {
			errs = append(errs, fmt.Sprintf("derive websocket url: %v", err))
		} else {
			c.X402.WSURL = wsURL
		}
	}

	// Validate monitoring threshold against wallet health thresholds when gasless is enabled
	if c.X402.GaslessEnabled && c.Monitoring.LowBalanceAlertURL != "" {
		const minHealthyBalance = 0.005 // Must match pkg/x402/solana/health.go MinHealthyBalance
		const criticalBalance = 0.001   // Must match pkg/x402/solana/health.go CriticalBalance

		if c.Monitoring.LowBalanceThreshold < criticalBalance {
			errs = append(errs, fmt.Sprintf(
				"monitoring.low_balance_threshold (%.6f SOL) is below critical threshold (%.6f SOL). "+
					"This will cause alerts AFTER wallets are already disabled. "+
					"Recommended: set to %.6f SOL or higher to get early warning before wallet health checker disables wallets.",
				c.Monitoring.LowBalanceThreshold, criticalBalance, minHealthyBalance,
			))
		} else if c.Monitoring.LowBalanceThreshold < minHealthyBalance {
			// Warning but not error - this is suboptimal but not breaking
			// We'll log this at startup instead
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// deriveWebsocketURL converts an HTTP(S) RPC URL to WS(S) format.
func deriveWebsocketURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("rpc url empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "ws", "wss":
		return raw, nil
	case "":
		return "", errors.New("rpc url missing scheme")
	default:
		return "", fmt.Errorf("unsupported rpc url scheme %q", u.Scheme)
	}
	return u.String(), nil
}

// ApplyPostgresPoolSettings applies connection pool settings to a database connection.
// If pool config is not specified, applies sensible defaults.
func ApplyPostgresPoolSettings(db *sql.DB, pool PostgresPoolConfig) {
	maxOpen := pool.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25 // default
	}

	maxIdle := pool.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5 // default
	}

	// Validate: maxIdle cannot exceed maxOpen
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}

	maxLifetime := pool.ConnMaxLifetime.Duration
	if maxLifetime <= 0 {
		maxLifetime = 5 * time.Minute // default
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLifetime)
}

// validateStablecoinMint validates that the token mint address is a known stablecoin.
// Returns an error with helpful message if the mint is not recognized.
//
// Why this is critical:
//   - Typo in token mint = payments go to wrong token = permanent loss
//   - Non-stablecoins have unpredictable values (1 SOL ≠ $1, 1 BONK ≠ $1)
//   - System rounds to 2 decimal places assuming $1 peg
//   - Using non-stablecoins will cause incorrect pricing and precision issues
func validateStablecoinMint(mintAddress string) error {
	symbol, err := money.ValidateStablecoinMint(mintAddress)
	if err != nil {
		// Not a known stablecoin - return detailed error
		return fmt.Errorf(`%w

⚠️  WARNING: Only stablecoins are supported for payments!

The system rounds all amounts to 2 decimal places (cents) assuming a $1 peg.
Using non-stablecoin tokens (SOL, BONK, etc.) will cause:
  - Incorrect pricing (1 SOL ≠ $1, 1 BONK ≠ $1)
  - Precision loss from improper rounding
  - Potential payment failures

Supported stablecoins:
  - USDC: EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v
  - USDT: Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB
  - PYUSD: 2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo
  - CASH: CASHx9KJUStyftLFWGvEVf59SGeG9sh5FfcnZMVPCASH

Your configured mint: %s`, err, mintAddress)
	}

	// Valid stablecoin - log success (this will show in config loading)
	fmt.Printf("✓ Token mint validated: %s (%s)\n", symbol, mintAddress)
	return nil
}
