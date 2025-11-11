package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/money"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	db                      *sql.DB
	ownsDB                  bool   // Track if we created the DB connection (for Close())
	paymentTransactionsTableName string // Configurable table name (default: "payment_transactions")
	adminNoncesTableName     string // Configurable table name (default: "admin_nonces")
	cartQuotesTableName      string // Configurable table name (default: "cart_quotes")
	refundQuotesTableName    string // Configurable table name (default: "refund_quotes")
	webhookQueueTableName    string // Configurable table name (default: "webhook_queue")
}

// NewPostgresStore creates a new PostgreSQL-backed store.
func NewPostgresStore(connectionString string, poolConfig config.PostgresPoolConfig) (*PostgresStore, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	if err := db.Ping(); err != nil {
		// NOTE: db.Close() error is intentionally ignored during initialization cleanup.
		// If connection fails, the Close() error is not actionable and would only obscure
		// the original connection failure. The primary error is returned to the caller.
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	// Apply connection pool settings from config
	config.ApplyPostgresPoolSettings(db, poolConfig)

	store := &PostgresStore{
		db:                      db,
		ownsDB:                  true,
		paymentTransactionsTableName: "payment_transactions",
		adminNoncesTableName:     "admin_nonces",
		cartQuotesTableName:      "cart_quotes",
		refundQuotesTableName:    "refund_quotes",
		webhookQueueTableName:    "webhook_queue",
	}

	// Create tables if they don't exist (using default table names)
	if err := store.createPostgresTables(); err != nil {
		// Same rationale: Close() error during initialization cleanup is not actionable
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

// NewPostgresStoreWithDB creates a PostgreSQL-backed store using an existing connection pool.
// This allows sharing a single connection pool across multiple stores/repositories.
func NewPostgresStoreWithDB(db *sql.DB) (*PostgresStore, error) {
	store := &PostgresStore{
		db:                      db,
		ownsDB:                  false,
		paymentTransactionsTableName: "payment_transactions",
		adminNoncesTableName:     "admin_nonces",
		cartQuotesTableName:      "cart_quotes",
		refundQuotesTableName:    "refund_quotes",
		webhookQueueTableName:    "webhook_queue",
	}

	// Create tables if they don't exist (using default table names)
	if err := store.createPostgresTables(); err != nil {
		return nil, err
	}

	return store, nil
}

// WithTableNames sets custom table names (for schema_mapping support).
// After setting table names, it recreates tables with the new names.
func (s *PostgresStore) WithTableNames(paymentTransactions, adminNonces, cartQuotes, refundQuotes, webhookQueue string) *PostgresStore {
	if paymentTransactions != "" {
		s.paymentTransactionsTableName = paymentTransactions
	}
	if adminNonces != "" {
		s.adminNoncesTableName = adminNonces
	}
	if cartQuotes != "" {
		s.cartQuotesTableName = cartQuotes
	}
	if refundQuotes != "" {
		s.refundQuotesTableName = refundQuotes
	}
	if webhookQueue != "" {
		s.webhookQueueTableName = webhookQueue
	}

	// Recreate tables with new names (CREATE TABLE IF NOT EXISTS will only create missing tables)
	_ = s.createPostgresTables()

	return s
}

// createPostgresTables creates the necessary tables if they don't exist.
// NOTE: These are the NEW schema tables using BIGINT atomic units + asset columns + tenant_id.
// Tables created using configured table names from schema_mapping.
func (s *PostgresStore) createPostgresTables() error {
	schema := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			items JSONB NOT NULL,
			total_amount BIGINT NOT NULL,
			total_asset TEXT NOT NULL,
			metadata JSONB,
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			wallet_paid_by TEXT
		);

		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			original_purchase_id TEXT NOT NULL,
			recipient_wallet TEXT NOT NULL,
			amount BIGINT NOT NULL,
			amount_asset TEXT NOT NULL,
			reason TEXT,
			metadata JSONB,
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			processed_by TEXT,
			processed_at TIMESTAMP,
			signature TEXT
		);

		CREATE TABLE IF NOT EXISTS %s (
			signature TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			resource_id TEXT NOT NULL,
			wallet TEXT NOT NULL,
			amount BIGINT NOT NULL,
			asset TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			metadata JSONB
		);

		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			purpose TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			consumed_at TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			url TEXT NOT NULL,
			payload JSONB NOT NULL,
			headers JSONB,
			event_type TEXT NOT NULL,
			status TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 5,
			last_error TEXT,
			last_attempt_at TIMESTAMP,
			next_attempt_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_cart_quotes_tenant ON %s(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_cart_quotes_tenant_expires ON %s(tenant_id, expires_at);
		CREATE INDEX IF NOT EXISTS idx_refund_quotes_tenant ON %s(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_refund_quotes_tenant_expires ON %s(tenant_id, expires_at);
		CREATE INDEX IF NOT EXISTS idx_refund_quotes_tenant_purchase ON %s(tenant_id, original_purchase_id);
		CREATE INDEX IF NOT EXISTS idx_refund_quotes_tenant_processed ON %s(tenant_id, processed_at) WHERE processed_at IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_payment_transactions_tenant ON %s(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_payment_transactions_tenant_resource ON %s(tenant_id, resource_id);
		CREATE INDEX IF NOT EXISTS idx_payment_transactions_tenant_wallet ON %s(tenant_id, wallet);
		CREATE INDEX IF NOT EXISTS idx_payment_transactions_tenant_created ON %s(tenant_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_admin_nonces_tenant ON %s(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_admin_nonces_tenant_expires ON %s(tenant_id, expires_at);
		CREATE INDEX IF NOT EXISTS idx_webhook_queue_pending ON %s(status, next_attempt_at) WHERE status = 'pending';
		CREATE INDEX IF NOT EXISTS idx_webhook_queue_status ON %s(status);
		CREATE INDEX IF NOT EXISTS idx_webhook_queue_created ON %s(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_webhook_queue_completed ON %s(completed_at) WHERE completed_at IS NOT NULL;
	`,
		// Table names
		s.cartQuotesTableName,
		s.refundQuotesTableName,
		s.paymentTransactionsTableName,
		s.adminNoncesTableName,
		s.webhookQueueTableName,
		// Index table references (cart_quotes)
		s.cartQuotesTableName, s.cartQuotesTableName,
		// Index table references (refund_quotes)
		s.refundQuotesTableName, s.refundQuotesTableName, s.refundQuotesTableName, s.refundQuotesTableName,
		// Index table references (payment_transactions)
		s.paymentTransactionsTableName, s.paymentTransactionsTableName, s.paymentTransactionsTableName, s.paymentTransactionsTableName,
		// Index table references (admin_nonces)
		s.adminNoncesTableName, s.adminNoncesTableName,
		// Index table references (webhook_queue)
		s.webhookQueueTableName, s.webhookQueueTableName, s.webhookQueueTableName, s.webhookQueueTableName,
	)

	_, err := s.db.Exec(schema)
	return err
}

// SaveCartQuote persists or updates a cart quote.
func (s *PostgresStore) SaveCartQuote(ctx context.Context, quote CartQuote) error {
	if err := validateAndPrepareCartQuote(&quote, 0); err != nil {
		return err
	}

	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	itemsJSON, err := json.Marshal(quote.Items)
	if err != nil {
		return fmt.Errorf("marshal items: %w", err)
	}

	metadataJSON, err := json.Marshal(quote.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (id, items, total_amount, total_asset, metadata, created_at, expires_at, wallet_paid_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			wallet_paid_by = EXCLUDED.wallet_paid_by
	`, s.cartQuotesTableName)

	// Convert timestamps to UTC for consistent timezone handling
	_, err = s.db.ExecContext(ctx, query,
		quote.ID, itemsJSON, quote.Total.Atomic, quote.Total.Asset.Code,
		metadataJSON, quote.CreatedAt.UTC(), quote.ExpiresAt.UTC(), quote.WalletPaidBy)

	return err
}

// GetCartQuote retrieves a cart quote by ID.
func (s *PostgresStore) GetCartQuote(ctx context.Context, cartID string) (CartQuote, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT id, items, total_amount, total_asset, metadata, created_at, expires_at, wallet_paid_by
		FROM %s
		WHERE id = $1
	`, s.cartQuotesTableName)

	var quote CartQuote
	var itemsJSON, metadataJSON []byte
	var totalAtomic int64
	var totalAsset string

	err := s.db.QueryRowContext(ctx, query, cartID).Scan(
		&quote.ID, &itemsJSON, &totalAtomic, &totalAsset,
		&metadataJSON, &quote.CreatedAt, &quote.ExpiresAt, &quote.WalletPaidBy)

	if err == sql.ErrNoRows {
		return CartQuote{}, ErrNotFound
	}
	if err != nil {
		return CartQuote{}, err
	}

	// Reconstruct Money from database columns
	asset, err := money.GetAsset(totalAsset)
	if err != nil {
		return CartQuote{}, fmt.Errorf("get asset %s: %w", totalAsset, err)
	}
	quote.Total = money.New(asset, totalAtomic)

	if err := json.Unmarshal(itemsJSON, &quote.Items); err != nil {
		return CartQuote{}, fmt.Errorf("unmarshal items: %w", err)
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &quote.Metadata); err != nil {
			return CartQuote{}, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	now := time.Now()
	if quote.IsExpiredAt(now) {
		return CartQuote{}, ErrCartExpired
	}

	return quote, nil
}

// MarkCartPaid marks a cart as paid.
func (s *PostgresStore) MarkCartPaid(ctx context.Context, cartID, wallet string) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`UPDATE %s SET wallet_paid_by = $2 WHERE id = $1`, s.cartQuotesTableName)
	result, err := s.db.ExecContext(ctx, query, cartID, wallet)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// HasCartAccess checks if a cart is paid by the wallet.
func (s *PostgresStore) HasCartAccess(ctx context.Context, cartID, wallet string) bool {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`SELECT 1 FROM %s WHERE id = $1 AND wallet_paid_by = $2`, s.cartQuotesTableName)
	var exists int
	err := s.db.QueryRowContext(ctx, query, cartID, wallet).Scan(&exists)
	return err == nil
}

// SaveCartQuotes stores multiple cart quotes in a single batch operation.
// Uses multi-row INSERT for optimal performance (single database round-trip).
func (s *PostgresStore) SaveCartQuotes(ctx context.Context, quotes []CartQuote) error {
	if len(quotes) == 0 {
		return nil
	}

	// Validate all quotes first
	for i := range quotes {
		if err := validateAndPrepareCartQuote(&quotes[i], 0); err != nil {
			return fmt.Errorf("quote %d: %w", i, err)
		}
	}

	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	// Build multi-row INSERT query with all values in a single statement
	// This reduces N database round-trips to 1
	baseQuery := fmt.Sprintf(`
		INSERT INTO %s (id, items, total_amount, total_asset, metadata, created_at, expires_at, wallet_paid_by)
		VALUES `, s.cartQuotesTableName)
	const conflictClause = `
		ON CONFLICT (id) DO UPDATE SET
			items = EXCLUDED.items,
			total_amount = EXCLUDED.total_amount,
			total_asset = EXCLUDED.total_asset,
			metadata = EXCLUDED.metadata,
			created_at = EXCLUDED.created_at,
			expires_at = EXCLUDED.expires_at,
			wallet_paid_by = EXCLUDED.wallet_paid_by`

	// Build VALUES placeholders and collect args
	valuePlaceholders := make([]string, 0, len(quotes))
	args := make([]interface{}, 0, len(quotes)*8)

	for i, quote := range quotes {
		itemsJSON, err := json.Marshal(quote.Items)
		if err != nil {
			return fmt.Errorf("quote %d: marshal items: %w", i, err)
		}

		metadataJSON, err := json.Marshal(quote.Metadata)
		if err != nil {
			return fmt.Errorf("quote %d: marshal metadata: %w", i, err)
		}

		// Each quote needs 8 parameters
		offset := i * 8
		placeholder := fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7, offset+8)
		valuePlaceholders = append(valuePlaceholders, placeholder)

		args = append(args,
			quote.ID,
			itemsJSON,
			quote.Total.Atomic,
			quote.Total.Asset.Code, // FIX: Use .Code to get string, not struct
			metadataJSON,
			quote.CreatedAt,
			quote.ExpiresAt,
			quote.WalletPaidBy,
		)
	}

	query := baseQuery + strings.Join(valuePlaceholders, ", ") + conflictClause

	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("batch insert: %w", err)
	}

	return nil
}

// GetCartQuotes retrieves multiple cart quotes by IDs in a single query.
// Returns found quotes - missing or expired carts are skipped (partial results).
func (s *PostgresStore) GetCartQuotes(ctx context.Context, cartIDs []string) ([]CartQuote, error) {
	if len(cartIDs) == 0 {
		return []CartQuote{}, nil
	}

	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	// Use PostgreSQL ANY to query multiple IDs efficiently
	query := fmt.Sprintf(`
		SELECT id, items, total_amount, total_asset, metadata, created_at, expires_at, wallet_paid_by
		FROM %s
		WHERE id = ANY($1) AND expires_at > NOW()
	`, s.cartQuotesTableName)

	rows, err := s.db.QueryContext(ctx, query, pq.Array(cartIDs))
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	quotes := make([]CartQuote, 0, len(cartIDs))
	for rows.Next() {
		var quote CartQuote
		var itemsJSON, metadataJSON []byte

		err := rows.Scan(
			&quote.ID,
			&itemsJSON,
			&quote.Total.Atomic,
			&quote.Total.Asset,
			&metadataJSON,
			&quote.CreatedAt,
			&quote.ExpiresAt,
			&quote.WalletPaidBy,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		if err := json.Unmarshal(itemsJSON, &quote.Items); err != nil {
			return nil, fmt.Errorf("unmarshal items: %w", err)
		}

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &quote.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}

		quotes = append(quotes, quote)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return quotes, nil
}

// SaveRefundQuote persists or updates a refund quote.
func (s *PostgresStore) SaveRefundQuote(ctx context.Context, quote RefundQuote) error {
	if err := validateAndPrepareRefundQuote(&quote, 0); err != nil {
		return err
	}

	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	metadataJSON, err := json.Marshal(quote.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (id, original_purchase_id, recipient_wallet, amount, amount_asset, reason, metadata, created_at, expires_at, processed_by, processed_at, signature)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			expires_at = EXCLUDED.expires_at,
			processed_by = EXCLUDED.processed_by,
			processed_at = EXCLUDED.processed_at,
			signature = EXCLUDED.signature
	`, s.refundQuotesTableName)

	// Convert timestamps to UTC for consistent timezone handling
	var processedAt interface{}
	if quote.ProcessedAt != nil {
		utcTime := quote.ProcessedAt.UTC()
		processedAt = &utcTime
	}

	_, err = s.db.ExecContext(ctx, query,
		quote.ID, quote.OriginalPurchaseID, quote.RecipientWallet, quote.Amount.Atomic, quote.Amount.Asset.Code,
		quote.Reason, metadataJSON, quote.CreatedAt.UTC(),
		quote.ExpiresAt.UTC(), quote.ProcessedBy, processedAt, quote.Signature)

	return err
}

// GetRefundQuote retrieves a refund quote by ID.
func (s *PostgresStore) GetRefundQuote(ctx context.Context, refundID string) (RefundQuote, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT id, original_purchase_id, recipient_wallet, amount, amount_asset, reason, metadata, created_at, expires_at, processed_by, processed_at, signature
		FROM %s
		WHERE id = $1
	`, s.refundQuotesTableName)

	var quote RefundQuote
	var metadataJSON []byte
	var amountAtomic int64
	var amountAsset string

	err := s.db.QueryRowContext(ctx, query, refundID).Scan(
		&quote.ID, &quote.OriginalPurchaseID, &quote.RecipientWallet, &amountAtomic, &amountAsset,
		&quote.Reason, &metadataJSON, &quote.CreatedAt,
		&quote.ExpiresAt, &quote.ProcessedBy, &quote.ProcessedAt, &quote.Signature)

	if err == sql.ErrNoRows {
		return RefundQuote{}, ErrNotFound
	}
	if err != nil {
		return RefundQuote{}, err
	}

	// Reconstruct Money from database columns
	asset, err := money.GetAsset(amountAsset)
	if err != nil {
		return RefundQuote{}, fmt.Errorf("get asset %s: %w", amountAsset, err)
	}
	quote.Amount = money.New(asset, amountAtomic)

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &quote.Metadata); err != nil {
			return RefundQuote{}, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	// Refund requests never expire - they remain pending until approved or denied by admin
	return quote, nil
}

// GetRefundQuoteByOriginalPurchaseID retrieves a refund quote by original purchase ID (transaction signature).
// This enforces the one-refund-per-signature limit.
func (s *PostgresStore) GetRefundQuoteByOriginalPurchaseID(ctx context.Context, originalPurchaseID string) (RefundQuote, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT id, original_purchase_id, recipient_wallet, amount, amount_asset, reason, metadata, created_at, expires_at, processed_by, processed_at, signature
		FROM %s
		WHERE original_purchase_id = $1
		LIMIT 1
	`, s.refundQuotesTableName)

	var quote RefundQuote
	var metadataJSON []byte
	var amountAtomic int64
	var amountAsset string

	err := s.db.QueryRowContext(ctx, query, originalPurchaseID).Scan(
		&quote.ID, &quote.OriginalPurchaseID, &quote.RecipientWallet, &amountAtomic, &amountAsset,
		&quote.Reason, &metadataJSON, &quote.CreatedAt,
		&quote.ExpiresAt, &quote.ProcessedBy, &quote.ProcessedAt, &quote.Signature)

	if err == sql.ErrNoRows {
		return RefundQuote{}, ErrNotFound
	}
	if err != nil {
		return RefundQuote{}, err
	}

	// Reconstruct Money from database columns
	asset, err := money.GetAsset(amountAsset)
	if err != nil {
		return RefundQuote{}, fmt.Errorf("get asset %s: %w", amountAsset, err)
	}
	quote.Amount = money.New(asset, amountAtomic)

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &quote.Metadata); err != nil {
			return RefundQuote{}, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return quote, nil
}

// ListPendingRefunds returns all unprocessed refund quotes.
func (s *PostgresStore) ListPendingRefunds(ctx context.Context) ([]RefundQuote, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT id, original_purchase_id, recipient_wallet, amount, amount_asset, reason, metadata, created_at, expires_at, processed_by, processed_at, signature
		FROM %s
		WHERE processed_at IS NULL
		ORDER BY created_at ASC
	`, s.refundQuotesTableName)

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refunds []RefundQuote
	for rows.Next() {
		var quote RefundQuote
		var metadataJSON []byte
		var amountAtomic int64
		var amountAsset string

		err := rows.Scan(
			&quote.ID, &quote.OriginalPurchaseID, &quote.RecipientWallet, &amountAtomic, &amountAsset,
			&quote.Reason, &metadataJSON, &quote.CreatedAt,
			&quote.ExpiresAt, &quote.ProcessedBy, &quote.ProcessedAt, &quote.Signature)
		if err != nil {
			return nil, err
		}

		// Reconstruct Money from database columns
		asset, err := money.GetAsset(amountAsset)
		if err != nil {
			return nil, fmt.Errorf("get asset %s: %w", amountAsset, err)
		}
		quote.Amount = money.New(asset, amountAtomic)

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &quote.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}

		refunds = append(refunds, quote)
	}

	// Check for errors from iteration
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return refunds, nil
}

// DeleteRefundQuote removes a refund quote by ID.
func (s *PostgresStore) DeleteRefundQuote(ctx context.Context, refundID string) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.refundQuotesTableName)
	result, err := s.db.ExecContext(ctx, query, refundID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// SaveRefundQuotes stores multiple refund quotes in a single batch operation.
// Uses multi-row INSERT for optimal performance (single database round-trip).
func (s *PostgresStore) SaveRefundQuotes(ctx context.Context, quotes []RefundQuote) error {
	if len(quotes) == 0 {
		return nil
	}

	// Validate all quotes first
	for i := range quotes {
		if err := validateAndPrepareRefundQuote(&quotes[i], 0); err != nil {
			return fmt.Errorf("quote %d: %w", i, err)
		}
	}

	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	// Build multi-row INSERT query with all values in a single statement
	baseQuery := fmt.Sprintf(`
		INSERT INTO %s (id, original_purchase_id, recipient_wallet, amount, amount_asset, reason, metadata, created_at, expires_at, processed_by, processed_at, signature)
		VALUES `, s.refundQuotesTableName)
	const conflictClause = `
		ON CONFLICT (id) DO UPDATE SET
			original_purchase_id = EXCLUDED.original_purchase_id,
			recipient_wallet = EXCLUDED.recipient_wallet,
			amount = EXCLUDED.amount,
			amount_asset = EXCLUDED.amount_asset,
			reason = EXCLUDED.reason,
			metadata = EXCLUDED.metadata,
			created_at = EXCLUDED.created_at,
			expires_at = EXCLUDED.expires_at,
			processed_by = EXCLUDED.processed_by,
			processed_at = EXCLUDED.processed_at,
			signature = EXCLUDED.signature`

	// Build VALUES placeholders and collect args
	valuePlaceholders := make([]string, 0, len(quotes))
	args := make([]interface{}, 0, len(quotes)*12)

	for i, quote := range quotes {
		metadataJSON, err := json.Marshal(quote.Metadata)
		if err != nil {
			return fmt.Errorf("quote %d: marshal metadata: %w", i, err)
		}

		// Each refund quote needs 12 parameters
		offset := i * 12
		placeholder := fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6,
			offset+7, offset+8, offset+9, offset+10, offset+11, offset+12)
		valuePlaceholders = append(valuePlaceholders, placeholder)

		args = append(args,
			quote.ID,
			quote.OriginalPurchaseID,
			quote.RecipientWallet,
			quote.Amount.Atomic,
			quote.Amount.Asset.Code, // Use .Code to get string, not struct
			quote.Reason,
			metadataJSON,
			quote.CreatedAt,
			quote.ExpiresAt,
			quote.ProcessedBy,
			quote.ProcessedAt,
			quote.Signature,
		)
	}

	query := baseQuery + strings.Join(valuePlaceholders, ", ") + conflictClause

	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("batch insert: %w", err)
	}

	return nil
}

// MarkRefundProcessed marks a refund as completed.
func (s *PostgresStore) MarkRefundProcessed(ctx context.Context, refundID, processedBy, signature string) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`UPDATE %s SET processed_by = $2, processed_at = NOW(), signature = $3 WHERE id = $1`, s.refundQuotesTableName)
	result, err := s.db.ExecContext(ctx, query, refundID, processedBy, signature)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// RecordPayment saves a verified payment transaction for replay protection.
// CRITICAL: Signature is globally unique - once used, cannot be reused for any resource.
// Returns error if signature was already used (concurrent request won the race).
func (s *PostgresStore) RecordPayment(ctx context.Context, tx PaymentTransaction) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	metadataJSON, err := json.Marshal(tx.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (signature, resource_id, wallet, amount, asset, created_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (signature) DO UPDATE
		SET resource_id = EXCLUDED.resource_id,
		    wallet      = EXCLUDED.wallet,
		    amount      = EXCLUDED.amount,
		    asset       = EXCLUDED.asset,
		    created_at  = EXCLUDED.created_at,
		    metadata    = EXCLUDED.metadata
		WHERE %s.wallet = ''
		   OR %s.metadata->>'status' = 'verifying'
	`, s.paymentTransactionsTableName, s.paymentTransactionsTableName, s.paymentTransactionsTableName)

	// Convert timestamp to UTC for consistent timezone handling
	result, err := s.db.ExecContext(ctx, query,
		tx.Signature,
		tx.ResourceID,
		tx.Wallet,
		tx.Amount.Atomic,
		tx.Amount.Asset.Code,
		tx.CreatedAt.UTC(),
		metadataJSON,
	)
	if err != nil {
		return err
	}

	// Check if row was actually inserted (RowsAffected = 0 means conflict occurred)
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		// Signature already exists - concurrent request won the race
		return fmt.Errorf("signature already used: replay attack detected")
	}

	return nil
}

// RecordPayments saves multiple verified payment transactions in a single batch operation.
// CRITICAL: ALL signatures must be globally unique - batch fails if ANY signature already exists.
// Uses multi-row INSERT for optimal performance (single database round-trip).
func (s *PostgresStore) RecordPayments(ctx context.Context, txs []PaymentTransaction) error {
	if len(txs) == 0 {
		return nil
	}

	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	// Build multi-row INSERT query with all values in a single statement
	baseQuery := fmt.Sprintf(`
		INSERT INTO %s (signature, resource_id, wallet, amount, asset, created_at, metadata)
		VALUES `, s.paymentTransactionsTableName)
	const conflictClause = ` ON CONFLICT (signature) DO NOTHING`

	// Build VALUES placeholders and collect args
	valuePlaceholders := make([]string, 0, len(txs))
	args := make([]interface{}, 0, len(txs)*7)

	for i, tx := range txs {
		metadataJSON, err := json.Marshal(tx.Metadata)
		if err != nil {
			return fmt.Errorf("tx %d: marshal metadata: %w", i, err)
		}

		// Each payment transaction needs 7 parameters
		offset := i * 7
		placeholder := fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7)
		valuePlaceholders = append(valuePlaceholders, placeholder)

		args = append(args,
			tx.Signature,
			tx.ResourceID,
			tx.Wallet,
			tx.Amount.Atomic,
			tx.Amount.Asset.Code, // Use .Code to get string, not struct
			tx.CreatedAt,
			metadataJSON,
		)
	}

	query := baseQuery + strings.Join(valuePlaceholders, ", ") + conflictClause

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin record payments tx: %w", err)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("batch insert: %w", err)
	}

	// Check if all rows were inserted (RowsAffected < len(txs) means some conflicts occurred)
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rowsAffected < int64(len(txs)) {
		tx.Rollback()
		return fmt.Errorf("batch insert aborted: %d/%d signatures already used (replay attack detected)", len(txs)-int(rowsAffected), len(txs))
	}

	return tx.Commit()
}

// HasPaymentBeenProcessed checks if a transaction signature has EVER been used.
// Returns true if signature exists for ANY resource (prevents cross-resource replay).
func (s *PostgresStore) HasPaymentBeenProcessed(ctx context.Context, signature string) (bool, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE signature = $1)`, s.paymentTransactionsTableName)

	var exists bool
	err := s.db.QueryRowContext(ctx, query, signature).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check payment processed: %w", err)
	}

	return exists, nil
}

// GetPayment retrieves a payment transaction by signature.
// Returns the original payment record showing which resource it was used for.
func (s *PostgresStore) GetPayment(ctx context.Context, signature string) (PaymentTransaction, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT signature, resource_id, wallet, amount, asset, created_at, metadata
		FROM %s
		WHERE signature = $1
	`, s.paymentTransactionsTableName)

	var tx PaymentTransaction
	var metadataJSON []byte
	var amountAtomic int64
	var assetCode string

	err := s.db.QueryRowContext(ctx, query, signature).Scan(
		&tx.Signature,
		&tx.ResourceID,
		&tx.Wallet,
		&amountAtomic,
		&assetCode,
		&tx.CreatedAt,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return PaymentTransaction{}, ErrNotFound
	}
	if err != nil {
		return PaymentTransaction{}, fmt.Errorf("query payment: %w", err)
	}

	// Reconstruct Money from database columns
	asset, err := money.GetAsset(assetCode)
	if err != nil {
		return PaymentTransaction{}, fmt.Errorf("get asset %s: %w", assetCode, err)
	}
	tx.Amount = money.New(asset, amountAtomic)

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &tx.Metadata); err != nil {
			return PaymentTransaction{}, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return tx, nil
}

// CreateNonce stores a new admin nonce for replay protection.
func (s *PostgresStore) CreateNonce(ctx context.Context, nonce AdminNonce) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		INSERT INTO %s (id, purpose, created_at, expires_at, consumed_at)
		VALUES ($1, $2, $3, $4, $5)
	`, s.adminNoncesTableName)

	// Convert times to UTC to ensure consistent timezone handling
	createdAt := nonce.CreatedAt.UTC()
	expiresAt := nonce.ExpiresAt.UTC()
	var consumedAt interface{}
	if nonce.ConsumedAt != nil {
		utcTime := nonce.ConsumedAt.UTC()
		consumedAt = &utcTime
	}

	_, err := s.db.ExecContext(ctx, query,
		nonce.ID, nonce.Purpose, createdAt, expiresAt, consumedAt)

	return err
}

// ConsumeNonce marks a nonce as consumed (one-time use).
func (s *PostgresStore) ConsumeNonce(ctx context.Context, nonceID string) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		UPDATE %s
		SET consumed_at = NOW()
		WHERE id = $1
			AND consumed_at IS NULL
			AND expires_at > NOW()
	`, s.adminNoncesTableName)

	result, err := s.db.ExecContext(ctx, query, nonceID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		// Check why the update failed
		var consumedAt *time.Time
		var expiresAt time.Time
		checkQuery := fmt.Sprintf(`SELECT consumed_at, expires_at FROM %s WHERE id = $1`, s.adminNoncesTableName)
		err := s.db.QueryRowContext(ctx, checkQuery, nonceID).Scan(&consumedAt, &expiresAt)

		if err == sql.ErrNoRows {
			return fmt.Errorf("nonce not found: %s", nonceID)
		}
		if err != nil {
			return fmt.Errorf("check nonce status: %w", err)
		}

		if consumedAt != nil {
			return fmt.Errorf("nonce already consumed: %s", nonceID)
		}
		if time.Now().After(expiresAt) {
			return fmt.Errorf("nonce expired: %s", nonceID)
		}

		return fmt.Errorf("failed to consume nonce: %s", nonceID)
	}

	return nil
}

// ArchiveOldPayments deletes payment transactions older than the specified time.
// This prevents unbounded growth of the payment_transactions table while maintaining
// replay protection for recent transactions (e.g., last 90 days).
//
// Returns the number of archived (deleted) records.
func (s *PostgresStore) ArchiveOldPayments(ctx context.Context, olderThan time.Time) (int64, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`DELETE FROM %s WHERE created_at < $1`, s.paymentTransactionsTableName)

	result, err := s.db.ExecContext(ctx, query, olderThan)
	if err != nil {
		return 0, fmt.Errorf("archive old payments: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}

	return count, nil
}

// CleanupExpiredNonces deletes expired admin nonces from the database.
// This prevents unbounded growth of the admin_nonces table.
//
// Returns the number of deleted nonces.
func (s *PostgresStore) CleanupExpiredNonces(ctx context.Context) (int64, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`DELETE FROM %s WHERE expires_at < NOW()`, s.adminNoncesTableName)

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired nonces: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}

	return count, nil
}

// Close closes the database connection.
func (s *PostgresStore) Close() error {
	if s.ownsDB {
		return s.db.Close()
	}
	return nil
}
