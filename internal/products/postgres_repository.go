package products

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/metrics"
	"github.com/CedrosPay/server/internal/money"
	"github.com/lib/pq"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresRepository implements Repository using PostgreSQL.
type PostgresRepository struct {
	db         *sql.DB
	ownsDB     bool            // Track if we created the DB connection (for Close())
	metrics    *metrics.Metrics // Optional: Prometheus metrics collector
	tableName  string          // Configurable table name (default: "products")
}

// Query timeout constants
const (
	queryTimeoutGet  = 5 * time.Second  // Timeout for single-row queries
	queryTimeoutList = 10 * time.Second // Timeout for list queries
)

// Input validation constraints
const maxIDLength = 255

var validTableNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// validateProductID validates product ID input
func validateProductID(id string) error {
	if len(id) == 0 || len(id) > maxIDLength {
		return fmt.Errorf("invalid product ID length: must be between 1 and %d characters", maxIDLength)
	}
	return nil
}

// validateTableName ensures table name is safe from SQL injection
func validateTableName(name string) error {
	if !validTableNameRegex.MatchString(name) {
		return fmt.Errorf("invalid table name: %s (must be alphanumeric with underscores only)", name)
	}
	return nil
}

// withQueryTimeout adds a timeout to the context if not already set
func withQueryTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {} // Already has deadline
	}
	return context.WithTimeout(ctx, timeout)
}

// NewPostgresRepository creates a PostgreSQL-backed repository.
func NewPostgresRepository(connectionString string, poolConfig config.PostgresPoolConfig) (*PostgresRepository, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	// Apply connection pool settings from config
	config.ApplyPostgresPoolSettings(db, poolConfig)

	return &PostgresRepository{db: db, ownsDB: true, tableName: "products"}, nil
}

// NewPostgresRepositoryWithDB creates a PostgreSQL-backed repository using an existing connection pool.
// This allows sharing a single connection pool across multiple repositories.
func NewPostgresRepositoryWithDB(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db, ownsDB: false, tableName: "products"}
}

// WithTableName sets a custom table name (for schema_mapping support).
// Validates the table name to prevent SQL injection.
func (r *PostgresRepository) WithTableName(tableName string) *PostgresRepository {
	if tableName != "" {
		if err := validateTableName(tableName); err != nil {
			panic(fmt.Sprintf("invalid table name: %v", err))
		}
		r.tableName = tableName
	}
	return r
}

// WithMetrics adds metrics collection to the repository.
func (r *PostgresRepository) WithMetrics(m *metrics.Metrics) *PostgresRepository {
	r.metrics = m
	return r
}

// GetProduct retrieves a product by ID.
func (r *PostgresRepository) GetProduct(ctx context.Context, id string) (Product, error) {
	defer metrics.MeasureDBQuery(r.metrics, "get_product", "postgres")()

	// Validate input
	if err := validateProductID(id); err != nil {
		return Product{}, err
	}

	// Add query timeout
	ctx, cancel := withQueryTimeout(ctx, queryTimeoutGet)
	defer cancel()

	// Use pq.QuoteIdentifier to prevent SQL injection
	query := fmt.Sprintf(`
		SELECT id, description, fiat_amount, fiat_currency, stripe_price_id,
		       crypto_amount, crypto_token, crypto_account, memo_template,
		       metadata, active, created_at, updated_at
		FROM %s
		WHERE id = $1 AND active = true
	`, pq.QuoteIdentifier(r.tableName))

	var p Product
	var metadataJSON []byte
	var fiatAtomic *int64
	var fiatAsset *string
	var cryptoAtomic *int64
	var cryptoAsset *string

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID,
		&p.Description,
		&fiatAtomic,
		&fiatAsset,
		&p.StripePriceID,
		&cryptoAtomic,
		&cryptoAsset,
		&p.CryptoAccount,
		&p.MemoTemplate,
		&metadataJSON,
		&p.Active,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return Product{}, ErrProductNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("query product: %w", err)
	}

	// Reconstruct FiatPrice from database columns
	if fiatAtomic != nil && fiatAsset != nil {
		asset, err := money.GetAsset(*fiatAsset)
		if err != nil {
			return Product{}, fmt.Errorf("get fiat asset: %w", err)
		}
		price := money.New(asset, *fiatAtomic)
		p.FiatPrice = &price
	}

	// Reconstruct CryptoPrice from database columns
	if cryptoAtomic != nil && cryptoAsset != nil {
		asset, err := money.GetAsset(*cryptoAsset)
		if err != nil {
			return Product{}, fmt.Errorf("get crypto asset: %w", err)
		}
		price := money.New(asset, *cryptoAtomic)
		p.CryptoPrice = &price
	}

	// Parse metadata JSON
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
			return Product{}, fmt.Errorf("parse metadata: %w", err)
		}
	}

	return p, nil
}

// GetProductByStripePriceID retrieves a product by its Stripe Price ID.
func (r *PostgresRepository) GetProductByStripePriceID(ctx context.Context, stripePriceID string) (Product, error) {
	defer metrics.MeasureDBQuery(r.metrics, "get_product_by_stripe_price_id", "postgres")()

	// Validate input
	if err := validateProductID(stripePriceID); err != nil {
		return Product{}, err
	}

	// Add query timeout
	ctx, cancel := withQueryTimeout(ctx, queryTimeoutGet)
	defer cancel()

	// Use pq.QuoteIdentifier to prevent SQL injection
	query := fmt.Sprintf(`
		SELECT id, description, fiat_amount, fiat_currency, stripe_price_id,
		       crypto_amount, crypto_token, crypto_account, memo_template,
		       metadata, active, created_at, updated_at
		FROM %s
		WHERE stripe_price_id = $1 AND active = true
		LIMIT 1
	`, pq.QuoteIdentifier(r.tableName))

	var p Product
	var metadataJSON []byte
	var fiatAtomic *int64
	var fiatAsset *string
	var cryptoAtomic *int64
	var cryptoAsset *string

	err := r.db.QueryRowContext(ctx, query, stripePriceID).Scan(
		&p.ID,
		&p.Description,
		&fiatAtomic,
		&fiatAsset,
		&p.StripePriceID,
		&cryptoAtomic,
		&cryptoAsset,
		&p.CryptoAccount,
		&p.MemoTemplate,
		&metadataJSON,
		&p.Active,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return Product{}, ErrProductNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("query product by stripe price id: %w", err)
	}

	// Reconstruct FiatPrice from database columns
	if fiatAtomic != nil && fiatAsset != nil {
		asset, err := money.GetAsset(*fiatAsset)
		if err != nil {
			return Product{}, fmt.Errorf("get fiat asset: %w", err)
		}
		price := money.New(asset, *fiatAtomic)
		p.FiatPrice = &price
	}

	// Reconstruct CryptoPrice from database columns
	if cryptoAtomic != nil && cryptoAsset != nil {
		asset, err := money.GetAsset(*cryptoAsset)
		if err != nil {
			return Product{}, fmt.Errorf("get crypto asset: %w", err)
		}
		price := money.New(asset, *cryptoAtomic)
		p.CryptoPrice = &price
	}

	// Parse metadata JSON
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
			return Product{}, fmt.Errorf("parse metadata: %w", err)
		}
	}

	return p, nil
}

// ListProducts returns all active products.
func (r *PostgresRepository) ListProducts(ctx context.Context) ([]Product, error) {
	defer metrics.MeasureDBQuery(r.metrics, "list_products", "postgres")()

	// Add query timeout
	ctx, cancel := withQueryTimeout(ctx, queryTimeoutList)
	defer cancel()

	// Use pq.QuoteIdentifier to prevent SQL injection
	query := fmt.Sprintf(`
		SELECT id, description, fiat_amount, fiat_currency, stripe_price_id,
		       crypto_amount, crypto_token, crypto_account, memo_template,
		       metadata, active, created_at, updated_at
		FROM %s
		WHERE active = true
		ORDER BY id ASC
	`, pq.QuoteIdentifier(r.tableName))

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		var metadataJSON []byte
		var fiatAtomic *int64
		var fiatAsset *string
		var cryptoAtomic *int64
		var cryptoAsset *string

		err := rows.Scan(
			&p.ID,
			&p.Description,
			&fiatAtomic,
			&fiatAsset,
			&p.StripePriceID,
			&cryptoAtomic,
			&cryptoAsset,
			&p.CryptoAccount,
			&p.MemoTemplate,
			&metadataJSON,
			&p.Active,
			&p.CreatedAt,
			&p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}

		// Reconstruct FiatPrice from database columns
		if fiatAtomic != nil && fiatAsset != nil {
			asset, err := money.GetAsset(*fiatAsset)
			if err != nil {
				return nil, fmt.Errorf("get fiat asset: %w", err)
			}
			price := money.New(asset, *fiatAtomic)
			p.FiatPrice = &price
		}

		// Reconstruct CryptoPrice from database columns
		if cryptoAtomic != nil && cryptoAsset != nil {
			asset, err := money.GetAsset(*cryptoAsset)
			if err != nil {
				return nil, fmt.Errorf("get crypto asset: %w", err)
			}
			price := money.New(asset, *cryptoAtomic)
			p.CryptoPrice = &price
		}

		// Parse metadata JSON
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
				return nil, fmt.Errorf("parse metadata: %w", err)
			}
		}

		products = append(products, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}

	return products, nil
}

// CreateProduct creates a new product.
func (r *PostgresRepository) CreateProduct(ctx context.Context, p Product) error {
	defer metrics.MeasureDBQuery(r.metrics, "create_product", "postgres")()

	// Validate input
	if err := validateProductID(p.ID); err != nil {
		return err
	}

	// Add query timeout
	ctx, cancel := withQueryTimeout(ctx, queryTimeoutGet)
	defer cancel()

	// Set defaults
	if p.MemoTemplate == "" {
		p.MemoTemplate = "{{resource}}:{{nonce}}"
	}
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()

	// Marshal metadata
	metadataJSON, err := json.Marshal(p.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	// Extract fiat pricing (nullable)
	var fiatAtomic *int64
	var fiatAsset *string
	if p.FiatPrice != nil {
		atomic := p.FiatPrice.Atomic
		asset := p.FiatPrice.Asset.Code
		fiatAtomic = &atomic
		fiatAsset = &asset
	}

	// Extract crypto pricing (nullable)
	var cryptoAtomic *int64
	var cryptoAsset *string
	if p.CryptoPrice != nil {
		atomic := p.CryptoPrice.Atomic
		asset := p.CryptoPrice.Asset.Code
		cryptoAtomic = &atomic
		cryptoAsset = &asset
	}

	// Use pq.QuoteIdentifier to prevent SQL injection
	query := fmt.Sprintf(`
		INSERT INTO %s (id, description, fiat_amount, fiat_currency, stripe_price_id,
		                     crypto_amount, crypto_token, crypto_account, memo_template,
		                     metadata, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, pq.QuoteIdentifier(r.tableName))

	_, err = r.db.ExecContext(ctx, query,
		p.ID,
		p.Description,
		fiatAtomic,
		fiatAsset,
		p.StripePriceID,
		cryptoAtomic,
		cryptoAsset,
		p.CryptoAccount,
		p.MemoTemplate,
		metadataJSON,
		p.Active,
		p.CreatedAt,
		p.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("insert product: %w", err)
	}

	return nil
}

// UpdateProduct updates an existing product.
func (r *PostgresRepository) UpdateProduct(ctx context.Context, p Product) error {
	defer metrics.MeasureDBQuery(r.metrics, "update_product", "postgres")()

	// Validate input
	if err := validateProductID(p.ID); err != nil {
		return err
	}

	// Add query timeout
	ctx, cancel := withQueryTimeout(ctx, queryTimeoutGet)
	defer cancel()

	p.UpdatedAt = time.Now()

	// Marshal metadata
	metadataJSON, err := json.Marshal(p.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	// Extract fiat pricing (nullable)
	var fiatAtomic *int64
	var fiatAsset *string
	if p.FiatPrice != nil {
		atomic := p.FiatPrice.Atomic
		asset := p.FiatPrice.Asset.Code
		fiatAtomic = &atomic
		fiatAsset = &asset
	}

	// Extract crypto pricing (nullable)
	var cryptoAtomic *int64
	var cryptoAsset *string
	if p.CryptoPrice != nil {
		atomic := p.CryptoPrice.Atomic
		asset := p.CryptoPrice.Asset.Code
		cryptoAtomic = &atomic
		cryptoAsset = &asset
	}

	// Use pq.QuoteIdentifier to prevent SQL injection
	query := fmt.Sprintf(`
		UPDATE %s
		SET description = $2, fiat_amount = $3, fiat_currency = $4, stripe_price_id = $5,
		    crypto_amount = $6, crypto_token = $7, crypto_account = $8, memo_template = $9,
		    metadata = $10, active = $11, updated_at = $12
		WHERE id = $1
	`, pq.QuoteIdentifier(r.tableName))

	result, err := r.db.ExecContext(ctx, query,
		p.ID,
		p.Description,
		fiatAtomic,
		fiatAsset,
		p.StripePriceID,
		cryptoAtomic,
		cryptoAsset,
		p.CryptoAccount,
		p.MemoTemplate,
		metadataJSON,
		p.Active,
		p.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("update product: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rows == 0 {
		return ErrProductNotFound
	}

	return nil
}

// DeleteProduct soft-deletes a product (sets active = false).
func (r *PostgresRepository) DeleteProduct(ctx context.Context, id string) error {
	defer metrics.MeasureDBQuery(r.metrics, "delete_product", "postgres")()

	// Validate input
	if err := validateProductID(id); err != nil {
		return err
	}

	// Add query timeout
	ctx, cancel := withQueryTimeout(ctx, queryTimeoutGet)
	defer cancel()

	// Use pq.QuoteIdentifier to prevent SQL injection
	query := fmt.Sprintf(`UPDATE %s SET active = false, updated_at = $2 WHERE id = $1`, pq.QuoteIdentifier(r.tableName))

	result, err := r.db.ExecContext(ctx, query, id, time.Now())
	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rows == 0 {
		return ErrProductNotFound
	}

	return nil
}

// Close closes the database connection only if this repository owns it.
func (r *PostgresRepository) Close() error {
	if r.ownsDB {
		return r.db.Close()
	}
	return nil
}
