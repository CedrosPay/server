package subscriptions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresRepository implements Repository using PostgreSQL.
type PostgresRepository struct {
	db        *sql.DB
	tableName string
	ownsDB    bool // Whether we created the DB connection (vs. shared)
}

// NewPostgresRepository creates a new PostgreSQL repository.
func NewPostgresRepository(connStr string) (*PostgresRepository, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	repo := &PostgresRepository{
		db:        db,
		tableName: "subscriptions",
		ownsDB:    true,
	}

	if err := repo.createTable(); err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	return repo, nil
}

// NewPostgresRepositoryWithDB creates a repository using a shared database connection.
func NewPostgresRepositoryWithDB(db *sql.DB) *PostgresRepository {
	repo := &PostgresRepository{
		db:        db,
		tableName: "subscriptions",
		ownsDB:    false,
	}
	// Attempt to create table, but don't fail if it already exists
	_ = repo.createTable()
	return repo
}

// WithTableName returns a copy of the repository with a custom table name.
func (r *PostgresRepository) WithTableName(name string) *PostgresRepository {
	return &PostgresRepository{
		db:        r.db,
		tableName: name,
		ownsDB:    r.ownsDB,
	}
}

func (r *PostgresRepository) createTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id                     TEXT PRIMARY KEY,
			product_id             TEXT NOT NULL,
			wallet                 TEXT,
			stripe_customer_id     TEXT,
			stripe_subscription_id TEXT UNIQUE,
			payment_method         TEXT NOT NULL,
			billing_period         TEXT NOT NULL,
			billing_interval       INTEGER NOT NULL DEFAULT 1,
			status                 TEXT NOT NULL DEFAULT 'active',
			current_period_start   TIMESTAMPTZ NOT NULL,
			current_period_end     TIMESTAMPTZ NOT NULL,
			trial_end              TIMESTAMPTZ,
			cancelled_at           TIMESTAMPTZ,
			cancel_at_period_end   BOOLEAN NOT NULL DEFAULT FALSE,
			metadata               JSONB,
			created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_%s_wallet_product
			ON %s(wallet, product_id) WHERE wallet IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_%s_stripe_customer
			ON %s(stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_%s_status
			ON %s(status);
		CREATE INDEX IF NOT EXISTS idx_%s_period_end
			ON %s(current_period_end);
	`, r.tableName,
		r.tableName, r.tableName,
		r.tableName, r.tableName,
		r.tableName, r.tableName,
		r.tableName, r.tableName)

	_, err := r.db.Exec(query)
	return err
}

// Create stores a new subscription.
func (r *PostgresRepository) Create(ctx context.Context, sub Subscription) error {
	if sub.ID == "" {
		return ErrInvalidSubscription
	}

	now := time.Now()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	sub.UpdatedAt = now

	metadata, err := json.Marshal(sub.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			id, product_id, wallet, stripe_customer_id, stripe_subscription_id,
			payment_method, billing_period, billing_interval, status,
			current_period_start, current_period_end, trial_end, cancelled_at,
			cancel_at_period_end, metadata, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, r.tableName)

	_, err = r.db.ExecContext(ctx, query,
		sub.ID, sub.ProductID, nullString(sub.Wallet), nullString(sub.StripeCustomerID),
		nullString(sub.StripeSubscriptionID), sub.PaymentMethod, sub.BillingPeriod,
		sub.BillingInterval, sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		nullTime(sub.TrialEnd), nullTime(sub.CancelledAt), sub.CancelAtPeriodEnd,
		metadata, sub.CreatedAt, sub.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("insert subscription: %w", err)
	}

	return nil
}

// Get retrieves a subscription by ID.
func (r *PostgresRepository) Get(ctx context.Context, id string) (Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, product_id, wallet, stripe_customer_id, stripe_subscription_id,
			payment_method, billing_period, billing_interval, status,
			current_period_start, current_period_end, trial_end, cancelled_at,
			cancel_at_period_end, metadata, created_at, updated_at
		FROM %s WHERE id = $1
	`, r.tableName)

	return r.scanOne(ctx, query, id)
}

// Update modifies an existing subscription.
func (r *PostgresRepository) Update(ctx context.Context, sub Subscription) error {
	sub.UpdatedAt = time.Now()

	metadata, err := json.Marshal(sub.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE %s SET
			product_id = $2, wallet = $3, stripe_customer_id = $4,
			stripe_subscription_id = $5, payment_method = $6, billing_period = $7,
			billing_interval = $8, status = $9, current_period_start = $10,
			current_period_end = $11, trial_end = $12, cancelled_at = $13,
			cancel_at_period_end = $14, metadata = $15, updated_at = $16
		WHERE id = $1
	`, r.tableName)

	result, err := r.db.ExecContext(ctx, query,
		sub.ID, sub.ProductID, nullString(sub.Wallet), nullString(sub.StripeCustomerID),
		nullString(sub.StripeSubscriptionID), sub.PaymentMethod, sub.BillingPeriod,
		sub.BillingInterval, sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		nullTime(sub.TrialEnd), nullTime(sub.CancelledAt), sub.CancelAtPeriodEnd,
		metadata, sub.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete removes a subscription.
func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, r.tableName)

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// GetByWallet finds an active subscription for a wallet and product.
func (r *PostgresRepository) GetByWallet(ctx context.Context, wallet, productID string) (Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, product_id, wallet, stripe_customer_id, stripe_subscription_id,
			payment_method, billing_period, billing_interval, status,
			current_period_start, current_period_end, trial_end, cancelled_at,
			cancel_at_period_end, metadata, created_at, updated_at
		FROM %s
		WHERE wallet = $1 AND product_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, r.tableName)

	return r.scanOne(ctx, query, wallet, productID)
}

// GetByStripeSubscriptionID finds a subscription by Stripe subscription ID.
func (r *PostgresRepository) GetByStripeSubscriptionID(ctx context.Context, stripeSubID string) (Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, product_id, wallet, stripe_customer_id, stripe_subscription_id,
			payment_method, billing_period, billing_interval, status,
			current_period_start, current_period_end, trial_end, cancelled_at,
			cancel_at_period_end, metadata, created_at, updated_at
		FROM %s WHERE stripe_subscription_id = $1
	`, r.tableName)

	return r.scanOne(ctx, query, stripeSubID)
}

// GetByStripeCustomerID finds all subscriptions for a Stripe customer.
func (r *PostgresRepository) GetByStripeCustomerID(ctx context.Context, customerID string) ([]Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, product_id, wallet, stripe_customer_id, stripe_subscription_id,
			payment_method, billing_period, billing_interval, status,
			current_period_start, current_period_end, trial_end, cancelled_at,
			cancel_at_period_end, metadata, created_at, updated_at
		FROM %s WHERE stripe_customer_id = $1
		ORDER BY created_at DESC
	`, r.tableName)

	return r.scanMany(ctx, query, customerID)
}

// ListByProduct returns all subscriptions for a product.
func (r *PostgresRepository) ListByProduct(ctx context.Context, productID string) ([]Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, product_id, wallet, stripe_customer_id, stripe_subscription_id,
			payment_method, billing_period, billing_interval, status,
			current_period_start, current_period_end, trial_end, cancelled_at,
			cancel_at_period_end, metadata, created_at, updated_at
		FROM %s WHERE product_id = $1
		ORDER BY created_at DESC
	`, r.tableName)

	return r.scanMany(ctx, query, productID)
}

// ListActive returns all active subscriptions.
func (r *PostgresRepository) ListActive(ctx context.Context, productID string) ([]Subscription, error) {
	var query string
	var args []any

	if productID != "" {
		query = fmt.Sprintf(`
			SELECT id, product_id, wallet, stripe_customer_id, stripe_subscription_id,
				payment_method, billing_period, billing_interval, status,
				current_period_start, current_period_end, trial_end, cancelled_at,
				cancel_at_period_end, metadata, created_at, updated_at
			FROM %s
			WHERE product_id = $1 AND status IN ('active', 'trialing', 'past_due')
				AND current_period_end > NOW()
			ORDER BY created_at DESC
		`, r.tableName)
		args = []any{productID}
	} else {
		query = fmt.Sprintf(`
			SELECT id, product_id, wallet, stripe_customer_id, stripe_subscription_id,
				payment_method, billing_period, billing_interval, status,
				current_period_start, current_period_end, trial_end, cancelled_at,
				cancel_at_period_end, metadata, created_at, updated_at
			FROM %s
			WHERE status IN ('active', 'trialing', 'past_due')
				AND current_period_end > NOW()
			ORDER BY created_at DESC
		`, r.tableName)
	}

	return r.scanMany(ctx, query, args...)
}

// ListExpiring returns subscriptions expiring before the given time.
func (r *PostgresRepository) ListExpiring(ctx context.Context, before time.Time) ([]Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, product_id, wallet, stripe_customer_id, stripe_subscription_id,
			payment_method, billing_period, billing_interval, status,
			current_period_start, current_period_end, trial_end, cancelled_at,
			cancel_at_period_end, metadata, created_at, updated_at
		FROM %s
		WHERE status = 'active' AND current_period_end < $1
		ORDER BY current_period_end ASC
	`, r.tableName)

	return r.scanMany(ctx, query, before)
}

// UpdateStatus changes a subscription's status.
func (r *PostgresRepository) UpdateStatus(ctx context.Context, id string, status Status) error {
	query := fmt.Sprintf(`
		UPDATE %s SET status = $2, updated_at = NOW(),
			cancelled_at = CASE WHEN $2 = 'cancelled' THEN NOW() ELSE cancelled_at END
		WHERE id = $1
	`, r.tableName)

	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// ExtendPeriod updates the current period for renewals.
func (r *PostgresRepository) ExtendPeriod(ctx context.Context, id string, newStart, newEnd time.Time) error {
	query := fmt.Sprintf(`
		UPDATE %s SET
			current_period_start = $2,
			current_period_end = $3,
			updated_at = NOW()
		WHERE id = $1
	`, r.tableName)

	result, err := r.db.ExecContext(ctx, query, id, newStart, newEnd)
	if err != nil {
		return fmt.Errorf("extend period: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// Close closes the database connection if owned.
func (r *PostgresRepository) Close() error {
	if r.ownsDB && r.db != nil {
		return r.db.Close()
	}
	return nil
}

// scanOne scans a single subscription from a query.
func (r *PostgresRepository) scanOne(ctx context.Context, query string, args ...any) (Subscription, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	return r.scanRow(row)
}

// scanMany scans multiple subscriptions from a query.
func (r *PostgresRepository) scanMany(ctx context.Context, query string, args ...any) ([]Subscription, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var result []Subscription
	for rows.Next() {
		sub, err := r.scanRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sub)
	}

	return result, rows.Err()
}

func (r *PostgresRepository) scanRow(row *sql.Row) (Subscription, error) {
	var sub Subscription
	var wallet, stripeCustomerID, stripeSubID sql.NullString
	var trialEnd, cancelledAt sql.NullTime
	var metadata []byte

	err := row.Scan(
		&sub.ID, &sub.ProductID, &wallet, &stripeCustomerID, &stripeSubID,
		&sub.PaymentMethod, &sub.BillingPeriod, &sub.BillingInterval, &sub.Status,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &trialEnd, &cancelledAt,
		&sub.CancelAtPeriodEnd, &metadata, &sub.CreatedAt, &sub.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, fmt.Errorf("scan: %w", err)
	}

	sub.Wallet = wallet.String
	sub.StripeCustomerID = stripeCustomerID.String
	sub.StripeSubscriptionID = stripeSubID.String
	if trialEnd.Valid {
		sub.TrialEnd = &trialEnd.Time
	}
	if cancelledAt.Valid {
		sub.CancelledAt = &cancelledAt.Time
	}

	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &sub.Metadata)
	}

	return sub, nil
}

func (r *PostgresRepository) scanRows(rows *sql.Rows) (Subscription, error) {
	var sub Subscription
	var wallet, stripeCustomerID, stripeSubID sql.NullString
	var trialEnd, cancelledAt sql.NullTime
	var metadata []byte

	err := rows.Scan(
		&sub.ID, &sub.ProductID, &wallet, &stripeCustomerID, &stripeSubID,
		&sub.PaymentMethod, &sub.BillingPeriod, &sub.BillingInterval, &sub.Status,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &trialEnd, &cancelledAt,
		&sub.CancelAtPeriodEnd, &metadata, &sub.CreatedAt, &sub.UpdatedAt,
	)

	if err != nil {
		return Subscription{}, fmt.Errorf("scan: %w", err)
	}

	sub.Wallet = wallet.String
	sub.StripeCustomerID = stripeCustomerID.String
	sub.StripeSubscriptionID = stripeSubID.String
	if trialEnd.Valid {
		sub.TrialEnd = &trialEnd.Time
	}
	if cancelledAt.Valid {
		sub.CancelledAt = &cancelledAt.Time
	}

	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &sub.Metadata)
	}

	return sub, nil
}

// Helper functions for nullable types
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
