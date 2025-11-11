package coupons

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/CedrosPay/server/internal/config"
)

// ErrCouponNotFound is returned when a coupon doesn't exist.
var ErrCouponNotFound = errors.New("coupon not found")

// ErrCouponExpired is returned when a coupon has expired.
var ErrCouponExpired = errors.New("coupon expired")

// ErrCouponUsageLimitReached is returned when coupon has no remaining uses.
var ErrCouponUsageLimitReached = errors.New("coupon usage limit reached")

// ErrCouponNotStarted is returned when coupon hasn't started yet.
var ErrCouponNotStarted = errors.New("coupon not started yet")

// DiscountType represents how the discount is applied.
type DiscountType string

const (
	DiscountTypePercentage DiscountType = "percentage" // Percentage off (0-100)
	DiscountTypeFixed      DiscountType = "fixed"      // Fixed amount off
)

// Scope represents what products the coupon applies to.
type Scope string

const (
	ScopeAll      Scope = "all"      // All products
	ScopeSpecific Scope = "specific" // Specific product IDs
)

// PaymentMethod represents which payment method the coupon applies to.
type PaymentMethod string

const (
	PaymentMethodAny    PaymentMethod = ""       // Any payment method (default, empty string)
	PaymentMethodStripe PaymentMethod = "stripe" // Stripe (fiat) payments only
	PaymentMethodX402   PaymentMethod = "x402"   // x402 (crypto) payments only
)

// AppliesAt represents when the coupon should be displayed and applied.
type AppliesAt string

const (
	AppliesAtCatalog  AppliesAt = "catalog"  // Product-level: Show on product pages with discounted price
	AppliesAtCheckout AppliesAt = "checkout" // Site-wide: Show only at cart/checkout
)

// Coupon represents a discount code that users can apply.
type Coupon struct {
	Code          string            // Coupon code (e.g., "SUMMER2024")
	DiscountType  DiscountType      // "percentage" or "fixed"
	DiscountValue float64           // Percentage (0-100) or fixed amount
	Currency      string            // For fixed discounts (usd, usdc, etc.)
	Scope         Scope             // "all" or "specific"
	ProductIDs    []string          // Applicable product IDs (for scope=specific)
	PaymentMethod PaymentMethod     // Restrict to specific payment method ("stripe", "x402", or "" for any)
	AutoApply     bool              // If true, automatically apply to matching products
	AppliesAt     AppliesAt         // When to display: "catalog" (product page) or "checkout" (cart only)
	UsageLimit    *int              // nil = unlimited, N = max uses
	UsageCount    int               // Current usage count
	StartsAt      *time.Time        // When coupon becomes valid
	ExpiresAt     *time.Time        // When coupon expires
	Active        bool              // Enable/disable coupon
	Metadata      map[string]string // Custom key-value pairs
	CreatedAt     time.Time         // Creation timestamp
	UpdatedAt     time.Time         // Last update timestamp
}

// IsValid checks if the coupon is currently valid for use.
func (c Coupon) IsValid() error {
	if !c.Active {
		return errors.New("coupon is inactive")
	}

	now := time.Now()

	// Check start date
	if c.StartsAt != nil && now.Before(*c.StartsAt) {
		return ErrCouponNotStarted
	}

	// Check expiration
	if c.ExpiresAt != nil && now.After(*c.ExpiresAt) {
		return ErrCouponExpired
	}

	// Check usage limit
	if c.UsageLimit != nil && c.UsageCount >= *c.UsageLimit {
		return ErrCouponUsageLimitReached
	}

	return nil
}

// ValidateConfiguration checks if the coupon configuration is consistent.
// Returns error if AppliesAt constraints are violated.
func (c Coupon) ValidateConfiguration() error {
	// Catalog coupons must be product-specific
	if c.AppliesAt == AppliesAtCatalog {
		if c.Scope != ScopeSpecific {
			return errors.New("catalog coupons must have scope=specific")
		}
		if len(c.ProductIDs) == 0 {
			return errors.New("catalog coupons must specify product IDs")
		}
	}

	// Checkout coupons must be site-wide
	if c.AppliesAt == AppliesAtCheckout {
		if c.Scope != ScopeAll {
			return errors.New("checkout coupons must have scope=all")
		}
	}

	// AppliesAt must be set for auto-apply coupons
	if c.AutoApply && c.AppliesAt == "" {
		return errors.New("auto-apply coupons must specify applies_at (catalog or checkout)")
	}

	return nil
}

// AppliesToProduct checks if the coupon applies to a given product ID.
func (c Coupon) AppliesToProduct(productID string) bool {
	if c.Scope == ScopeAll {
		return true
	}

	for _, id := range c.ProductIDs {
		if id == productID {
			return true
		}
	}

	return false
}

// AppliesToPaymentMethod checks if the coupon applies to a given payment method.
// Empty string (PaymentMethodAny) matches all payment methods.
func (c Coupon) AppliesToPaymentMethod(method PaymentMethod) bool {
	// Empty payment method (default) applies to all payment methods
	if c.PaymentMethod == PaymentMethodAny {
		return true
	}

	// Otherwise, must match exactly
	return c.PaymentMethod == method
}

// ApplyDiscount calculates the price after applying this coupon.
func (c Coupon) ApplyDiscount(originalPrice float64) float64 {
	if c.DiscountType == DiscountTypePercentage {
		return originalPrice * (1 - c.DiscountValue/100)
	}
	// Fixed amount discount
	discounted := originalPrice - c.DiscountValue
	if discounted < 0 {
		return 0
	}
	return discounted
}

// Repository defines the interface for coupon storage.
type Repository interface {
	// GetCoupon retrieves a coupon by code.
	GetCoupon(ctx context.Context, code string) (Coupon, error)

	// ListCoupons returns all active coupons.
	ListCoupons(ctx context.Context) ([]Coupon, error)

	// GetAutoApplyCouponsForPayment returns auto-apply coupons that match the product ID and payment method.
	// Returns coupons where AutoApply=true, product matches, and payment method matches (or is empty).
	// paymentMethod should be "stripe", "x402", or "" for any.
	GetAutoApplyCouponsForPayment(ctx context.Context, productID string, paymentMethod PaymentMethod) ([]Coupon, error)

	// GetAllAutoApplyCouponsForPayment returns all auto-apply coupons for a specific payment method.
	// Returns a map of productID -> []Coupon for efficient batch lookup.
	GetAllAutoApplyCouponsForPayment(ctx context.Context, paymentMethod PaymentMethod) (map[string][]Coupon, error)

	// CreateCoupon creates a new coupon.
	CreateCoupon(ctx context.Context, coupon Coupon) error

	// UpdateCoupon updates an existing coupon.
	UpdateCoupon(ctx context.Context, coupon Coupon) error

	// IncrementUsage atomically increments the usage count.
	IncrementUsage(ctx context.Context, code string) error

	// DeleteCoupon soft-deletes a coupon (sets active = false).
	DeleteCoupon(ctx context.Context, code string) error

	// Close closes any open connections.
	Close() error
}

// NewRepository creates a coupon repository based on config.
func NewRepository(cfg config.CouponConfig) (Repository, error) {
	return NewRepositoryWithDB(cfg, nil)
}

// NewRepositoryWithDB creates a coupon repository with an optional shared database pool.
// If sharedDB is provided (non-nil) for postgres sources, it will be used instead of creating a new connection.
// Pass nil to create a new connection pool.
func NewRepositoryWithDB(cfg config.CouponConfig, sharedDB *sql.DB) (Repository, error) {
	// Default to disabled if not specified
	source := cfg.CouponSource
	if source == "" || source == "disabled" {
		return NewDisabledRepository(), nil
	}

	var underlying Repository
	var err error

	switch source {
	case "yaml":
		return NewYAMLRepository(cfg.Coupons), nil
	case "postgres":
		if cfg.PostgresURL == "" {
			return nil, errors.New("postgres_url required when coupon_source is 'postgres'")
		}
		// Use shared DB if provided, otherwise create new connection
		var pgRepo *PostgresRepository
		if sharedDB != nil {
			pgRepo = NewPostgresRepositoryWithDB(sharedDB)
		} else {
			pgRepo, err = NewPostgresRepository(cfg.PostgresURL, cfg.PostgresPool)
			if err != nil {
				return nil, err
			}
		}
		// Apply schema_mapping table name if configured
		if cfg.PostgresTableName != "" {
			pgRepo = pgRepo.WithTableName(cfg.PostgresTableName)
		}
		underlying = pgRepo
	case "mongodb":
		if cfg.MongoDBURL == "" {
			return nil, errors.New("mongodb_url required when coupon_source is 'mongodb'")
		}
		if cfg.MongoDBDatabase == "" {
			return nil, errors.New("mongodb_database required when coupon_source is 'mongodb'")
		}
		collection := cfg.MongoDBCollection
		if collection == "" {
			collection = "coupons" // Default collection name
		}
		underlying, err = NewMongoDBRepository(cfg.MongoDBURL, cfg.MongoDBDatabase, collection)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("invalid coupon_source: must be 'yaml', 'postgres', 'mongodb', or 'disabled'")
	}

	// Wrap with caching layer if TTL is configured (short cache for coupons)
	cacheTTL := cfg.CacheTTL.Duration
	if cacheTTL > 0 {
		return NewCachedRepository(underlying, cacheTTL), nil
	}

	return underlying, nil
}
