package coupons

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/CedrosPay/server/internal/config"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresRepository implements Repository using PostgreSQL.
type PostgresRepository struct {
	db        *sql.DB
	ownsDB    bool   // Track if we created the DB connection (for Close())
	tableName string // Configurable table name (default: "coupons")
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

	return &PostgresRepository{db: db, ownsDB: true, tableName: "coupons"}, nil
}

// NewPostgresRepositoryWithDB creates a PostgreSQL-backed repository using an existing connection pool.
// This allows sharing a single connection pool across multiple repositories.
func NewPostgresRepositoryWithDB(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db, ownsDB: false, tableName: "coupons"}
}

// WithTableName sets a custom table name (for schema_mapping support).
func (r *PostgresRepository) WithTableName(tableName string) *PostgresRepository {
	if tableName != "" {
		r.tableName = tableName
	}
	return r
}

// GetCoupon retrieves a coupon by code.
func (r *PostgresRepository) GetCoupon(ctx context.Context, code string) (Coupon, error) {
	query := fmt.Sprintf(`
		SELECT code, discount_type, discount_value, currency, scope, product_ids,
		       payment_method, auto_apply, applies_at, usage_limit, usage_count, starts_at, expires_at,
		       active, metadata, created_at, updated_at
		FROM %s
		WHERE code = $1 AND active = true
	`, r.tableName)

	var c Coupon
	var discountType, scope, paymentMethod, appliesAt string
	var productIDsJSON []byte
	var metadataJSON []byte

	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&c.Code,
		&discountType,
		&c.DiscountValue,
		&c.Currency,
		&scope,
		&productIDsJSON,
		&paymentMethod,
		&c.AutoApply,
		&appliesAt,
		&c.UsageLimit,
		&c.UsageCount,
		&c.StartsAt,
		&c.ExpiresAt,
		&c.Active,
		&metadataJSON,
		&c.CreatedAt,
		&c.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return Coupon{}, ErrCouponNotFound
	}
	if err != nil {
		return Coupon{}, fmt.Errorf("query coupon: %w", err)
	}

	// Parse enums
	c.DiscountType = DiscountType(discountType)
	c.Scope = Scope(scope)
	c.PaymentMethod = PaymentMethod(paymentMethod)
	c.AppliesAt = AppliesAt(appliesAt)

	// Parse product IDs JSON array
	if len(productIDsJSON) > 0 {
		if err := json.Unmarshal(productIDsJSON, &c.ProductIDs); err != nil {
			return Coupon{}, fmt.Errorf("parse product_ids: %w", err)
		}
	}

	// Parse metadata JSON
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &c.Metadata); err != nil {
			return Coupon{}, fmt.Errorf("parse metadata: %w", err)
		}
	}

	return c, nil
}

// ListCoupons returns all active coupons.
func (r *PostgresRepository) ListCoupons(ctx context.Context) ([]Coupon, error) {
	query := fmt.Sprintf(`
		SELECT code, discount_type, discount_value, currency, scope, product_ids,
		       payment_method, auto_apply, applies_at, usage_limit, usage_count, starts_at, expires_at,
		       active, metadata, created_at, updated_at
		FROM %s
		WHERE active = true
		ORDER BY code ASC
	`, r.tableName)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query coupons: %w", err)
	}
	defer rows.Close()

	var coupons []Coupon
	for rows.Next() {
		var c Coupon
		var discountType, scope, paymentMethod, appliesAt string
		var productIDsJSON []byte
		var metadataJSON []byte

		err := rows.Scan(
			&c.Code,
			&discountType,
			&c.DiscountValue,
			&c.Currency,
			&scope,
			&productIDsJSON,
			&paymentMethod,
			&c.AutoApply,
			&appliesAt,
			&c.UsageLimit,
			&c.UsageCount,
			&c.StartsAt,
			&c.ExpiresAt,
			&c.Active,
			&metadataJSON,
			&c.CreatedAt,
			&c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan coupon: %w", err)
		}

		// Parse enums
		c.DiscountType = DiscountType(discountType)
		c.Scope = Scope(scope)
		c.PaymentMethod = PaymentMethod(paymentMethod)
		c.AppliesAt = AppliesAt(appliesAt)

		// Parse product IDs JSON array
		if len(productIDsJSON) > 0 {
			if err := json.Unmarshal(productIDsJSON, &c.ProductIDs); err != nil {
				return nil, fmt.Errorf("parse product_ids: %w", err)
			}
		}

		// Parse metadata JSON
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &c.Metadata); err != nil {
				return nil, fmt.Errorf("parse metadata: %w", err)
			}
		}

		coupons = append(coupons, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate coupons: %w", err)
	}

	return coupons, nil
}

// GetAutoApplyCouponsForPayment returns auto-apply coupons filtered by payment method.
func (r *PostgresRepository) GetAutoApplyCouponsForPayment(ctx context.Context, productID string, paymentMethod PaymentMethod) ([]Coupon, error) {
	now := time.Now()

	// Query filters at database level for efficiency:
	// - auto_apply = true
	// - active = true (part of IsValid)
	// - starts_at is null OR starts_at <= now (part of IsValid)
	// - expires_at is null OR expires_at > now (part of IsValid)
	// - usage_limit is null OR usage_count < usage_limit (part of IsValid)
	// - scope = 'all' OR productID in product_ids array
	// - payment_method = '' (any) OR payment_method = specified method
	query := fmt.Sprintf(`
		SELECT code, discount_type, discount_value, currency, scope, product_ids,
		       payment_method, auto_apply, applies_at, usage_limit, usage_count, starts_at, expires_at,
		       active, metadata, created_at, updated_at
		FROM %s
		WHERE auto_apply = true
		  AND active = true
		  AND (starts_at IS NULL OR starts_at <= $1)
		  AND (expires_at IS NULL OR expires_at > $1)
		  AND (usage_limit IS NULL OR usage_count < usage_limit)
		  AND (scope = 'all' OR product_ids::jsonb ? $2)
		  AND (payment_method = '' OR payment_method = $3)
		ORDER BY code ASC
	`, r.tableName)

	rows, err := r.db.QueryContext(ctx, query, now, productID, string(paymentMethod))
	if err != nil {
		return nil, fmt.Errorf("query auto-apply coupons: %w", err)
	}
	defer rows.Close()

	var coupons []Coupon
	for rows.Next() {
		var c Coupon
		var discountType, scope, paymentMethodStr, appliesAt string
		var productIDsJSON []byte
		var metadataJSON []byte

		err := rows.Scan(
			&c.Code,
			&discountType,
			&c.DiscountValue,
			&c.Currency,
			&scope,
			&productIDsJSON,
			&paymentMethodStr,
			&c.AutoApply,
			&appliesAt,
			&c.UsageLimit,
			&c.UsageCount,
			&c.StartsAt,
			&c.ExpiresAt,
			&c.Active,
			&metadataJSON,
			&c.CreatedAt,
			&c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan coupon: %w", err)
		}

		// Parse enums
		c.DiscountType = DiscountType(discountType)
		c.Scope = Scope(scope)
		c.PaymentMethod = PaymentMethod(paymentMethodStr)
		c.AppliesAt = AppliesAt(appliesAt)

		// Parse product IDs JSON array
		if len(productIDsJSON) > 0 {
			if err := json.Unmarshal(productIDsJSON, &c.ProductIDs); err != nil {
				return nil, fmt.Errorf("parse product_ids: %w", err)
			}
		}

		// Parse metadata JSON
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &c.Metadata); err != nil {
				return nil, fmt.Errorf("parse metadata: %w", err)
			}
		}

		coupons = append(coupons, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate coupons: %w", err)
	}

	return coupons, nil
}

// GetAllAutoApplyCouponsForPayment returns all auto-apply coupons grouped by product ID.
func (r *PostgresRepository) GetAllAutoApplyCouponsForPayment(ctx context.Context, paymentMethod PaymentMethod) (map[string][]Coupon, error) {
	now := time.Now()

	// Query all auto-apply coupons for the payment method
	query := fmt.Sprintf(`
		SELECT code, discount_type, discount_value, currency, scope, product_ids,
		       payment_method, auto_apply, applies_at, usage_limit, usage_count, starts_at, expires_at,
		       active, metadata, created_at, updated_at
		FROM %s
		WHERE auto_apply = true
		  AND active = true
		  AND (starts_at IS NULL OR starts_at <= $1)
		  AND (expires_at IS NULL OR expires_at > $1)
		  AND (usage_limit IS NULL OR usage_count < usage_limit)
		  AND (payment_method = '' OR payment_method = $2)
		ORDER BY code ASC
	`, r.tableName)

	rows, err := r.db.QueryContext(ctx, query, now, string(paymentMethod))
	if err != nil {
		return nil, fmt.Errorf("query all auto-apply coupons: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]Coupon)

	for rows.Next() {
		var c Coupon
		var discountType, scope, paymentMethodStr, appliesAt string
		var productIDsJSON []byte
		var metadataJSON []byte

		err := rows.Scan(
			&c.Code,
			&discountType,
			&c.DiscountValue,
			&c.Currency,
			&scope,
			&productIDsJSON,
			&paymentMethodStr,
			&c.AutoApply,
			&appliesAt,
			&c.UsageLimit,
			&c.UsageCount,
			&c.StartsAt,
			&c.ExpiresAt,
			&c.Active,
			&metadataJSON,
			&c.CreatedAt,
			&c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan coupon: %w", err)
		}

		// Parse enums
		c.DiscountType = DiscountType(discountType)
		c.Scope = Scope(scope)
		c.PaymentMethod = PaymentMethod(paymentMethodStr)
		c.AppliesAt = AppliesAt(appliesAt)

		// Parse product IDs JSON array
		if len(productIDsJSON) > 0 {
			if err := json.Unmarshal(productIDsJSON, &c.ProductIDs); err != nil {
				return nil, fmt.Errorf("parse product_ids: %w", err)
			}
		}

		// Parse metadata JSON
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &c.Metadata); err != nil {
				return nil, fmt.Errorf("parse metadata: %w", err)
			}
		}

		// Group by product IDs
		if c.Scope == ScopeAll {
			// For "all" scope coupons, store under special key
			result["*"] = append(result["*"], c)
		} else {
			// For specific products, add to each product ID
			for _, productID := range c.ProductIDs {
				result[productID] = append(result[productID], c)
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate coupons: %w", err)
	}

	return result, nil
}

// CreateCoupon creates a new coupon.
func (r *PostgresRepository) CreateCoupon(ctx context.Context, c Coupon) error {
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()

	// Marshal product IDs
	productIDsJSON, err := json.Marshal(c.ProductIDs)
	if err != nil {
		return fmt.Errorf("marshal product_ids: %w", err)
	}

	// Marshal metadata
	metadataJSON, err := json.Marshal(c.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (code, discount_type, discount_value, currency, scope, product_ids,
		                     payment_method, auto_apply, applies_at, usage_limit, usage_count, starts_at,
		                     expires_at, active, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, r.tableName)

	_, err = r.db.ExecContext(ctx, query,
		c.Code,
		string(c.DiscountType),
		c.DiscountValue,
		c.Currency,
		string(c.Scope),
		productIDsJSON,
		string(c.PaymentMethod),
		c.AutoApply,
		string(c.AppliesAt),
		c.UsageLimit,
		c.UsageCount,
		c.StartsAt,
		c.ExpiresAt,
		c.Active,
		metadataJSON,
		c.CreatedAt,
		c.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("insert coupon: %w", err)
	}

	return nil
}

// UpdateCoupon updates an existing coupon.
func (r *PostgresRepository) UpdateCoupon(ctx context.Context, c Coupon) error {
	c.UpdatedAt = time.Now()

	// Marshal product IDs
	productIDsJSON, err := json.Marshal(c.ProductIDs)
	if err != nil {
		return fmt.Errorf("marshal product_ids: %w", err)
	}

	// Marshal metadata
	metadataJSON, err := json.Marshal(c.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE %s
		SET discount_type = $2, discount_value = $3, currency = $4, scope = $5, product_ids = $6,
		    payment_method = $7, auto_apply = $8, applies_at = $9, usage_limit = $10, usage_count = $11,
		    starts_at = $12, expires_at = $13, active = $14, metadata = $15, updated_at = $16
		WHERE code = $1
	`, r.tableName)

	result, err := r.db.ExecContext(ctx, query,
		c.Code,
		string(c.DiscountType),
		c.DiscountValue,
		c.Currency,
		string(c.Scope),
		productIDsJSON,
		string(c.PaymentMethod),
		c.AutoApply,
		string(c.AppliesAt),
		c.UsageLimit,
		c.UsageCount,
		c.StartsAt,
		c.ExpiresAt,
		c.Active,
		metadataJSON,
		c.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("update coupon: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rows == 0 {
		return ErrCouponNotFound
	}

	return nil
}

// IncrementUsage atomically increments the usage count.
func (r *PostgresRepository) IncrementUsage(ctx context.Context, code string) error {
	query := fmt.Sprintf(`
		UPDATE %s
		SET usage_count = usage_count + 1, updated_at = $2
		WHERE code = $1 AND active = true
	`, r.tableName)

	result, err := r.db.ExecContext(ctx, query, code, time.Now())
	if err != nil {
		return fmt.Errorf("increment usage: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rows == 0 {
		return ErrCouponNotFound
	}

	return nil
}

// DeleteCoupon soft-deletes a coupon (sets active = false).
func (r *PostgresRepository) DeleteCoupon(ctx context.Context, code string) error {
	query := fmt.Sprintf(`UPDATE %s SET active = false, updated_at = $2 WHERE code = $1`, r.tableName)

	result, err := r.db.ExecContext(ctx, query, code, time.Now())
	if err != nil {
		return fmt.Errorf("delete coupon: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rows == 0 {
		return ErrCouponNotFound
	}

	return nil
}

// Close closes the database connection.
func (r *PostgresRepository) Close() error {
	if r.ownsDB {
		return r.db.Close()
	}
	return nil
}
