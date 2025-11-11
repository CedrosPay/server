package products

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/money"
)

// ErrProductNotFound is returned when a product doesn't exist.
var ErrProductNotFound = errors.New("product not found")

// Product represents a product/resource with pricing information.
type Product struct {
	ID            string            // Resource ID (e.g., "demo-content")
	Description   string            // Human-readable description
	FiatPrice     *money.Money      // Stripe price (optional, nil if not available)
	StripePriceID string            // Stripe Price ID (optional)
	CryptoPrice   *money.Money      // Crypto price (optional, nil if not available)
	CryptoAccount string            // Override token account (optional)
	MemoTemplate  string            // Transaction memo template
	Metadata      map[string]string // Custom key-value pairs
	Active        bool              // Enable/disable product

	CreatedAt time.Time // Creation timestamp
	UpdatedAt time.Time // Last update timestamp
}

// Repository defines the interface for product storage.
type Repository interface {
	// GetProduct retrieves a product by ID.
	GetProduct(ctx context.Context, id string) (Product, error)

	// GetProductByStripePriceID retrieves a product by its Stripe Price ID.
	// Returns ErrProductNotFound if no product matches the given price ID.
	GetProductByStripePriceID(ctx context.Context, stripePriceID string) (Product, error)

	// ListProducts returns all active products.
	ListProducts(ctx context.Context) ([]Product, error)

	// CreateProduct creates a new product.
	CreateProduct(ctx context.Context, product Product) error

	// UpdateProduct updates an existing product.
	UpdateProduct(ctx context.Context, product Product) error

	// DeleteProduct soft-deletes a product (sets active = false).
	DeleteProduct(ctx context.Context, id string) error

	// Close closes any open connections.
	Close() error
}

// NewRepository creates a product repository based on config with optional caching.
func NewRepository(cfg config.PaywallConfig) (Repository, error) {
	return NewRepositoryWithDB(cfg, nil)
}

// NewRepositoryWithDB creates a product repository with an optional shared database pool.
// If sharedDB is provided (non-nil) for postgres sources, it will be used instead of creating a new connection.
// Pass nil to create a new connection pool.
func NewRepositoryWithDB(cfg config.PaywallConfig, sharedDB *sql.DB) (Repository, error) {
	// Default to yaml if not specified
	source := cfg.ProductSource
	if source == "" {
		source = "yaml"
	}

	var underlying Repository
	var err error

	switch source {
	case "yaml":
		underlying = NewYAMLRepository(cfg.Resources)
	case "postgres":
		if cfg.PostgresURL == "" {
			return nil, errors.New("postgres_url required when product_source is 'postgres'")
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
		// Apply schema_mapping table name if configured (from storage.schema_mapping.products.table_name)
		// This is auto-populated by config.finalize() from storage.schema_mapping
		if cfg.PostgresTableName != "" {
			pgRepo = pgRepo.WithTableName(cfg.PostgresTableName)
		}
		underlying = pgRepo
	case "mongodb":
		if cfg.MongoDBURL == "" {
			return nil, errors.New("mongodb_url required when product_source is 'mongodb'")
		}
		if cfg.MongoDBDatabase == "" {
			return nil, errors.New("mongodb_database required when product_source is 'mongodb'")
		}
		collection := cfg.MongoDBCollection
		if collection == "" {
			collection = "products" // Default collection name
		}
		underlying, err = NewMongoDBRepository(cfg.MongoDBURL, cfg.MongoDBDatabase, collection)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("invalid product_source: must be 'yaml', 'postgres', or 'mongodb'")
	}

	// Wrap with caching layer if TTL is configured
	cacheTTL := cfg.ProductCacheTTL.Duration
	if cacheTTL > 0 {
		return NewCachedRepository(underlying, cacheTTL), nil
	}

	return underlying, nil
}

// ToPaywallResource converts a Product to a PaywallResource.
func (p Product) ToPaywallResource() config.PaywallResource {
	resource := config.PaywallResource{
		ResourceID:    p.ID,
		Description:   p.Description,
		StripePriceID: p.StripePriceID,
		CryptoAccount: p.CryptoAccount,
		MemoTemplate:  p.MemoTemplate,
		Metadata:      p.Metadata,
	}

	// Extract fiat pricing if available
	if p.FiatPrice != nil {
		resource.FiatAmountCents = p.FiatPrice.Atomic
		resource.FiatCurrency = p.FiatPrice.Asset.Code
	}

	// Extract crypto pricing if available
	if p.CryptoPrice != nil {
		resource.CryptoAtomicAmount = p.CryptoPrice.Atomic
		resource.CryptoToken = p.CryptoPrice.Asset.Code
	}

	return resource
}
