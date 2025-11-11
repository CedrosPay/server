package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/CedrosPay/server/internal/config"
)

// ErrNotFound is returned when a requested entity is missing from the store.
var ErrNotFound = errors.New("storage: not found")

// ErrRefundExpired is returned when a refund quote has passed its expiration time.
var ErrRefundExpired = errors.New("storage: refund expired")

// Store captures the persistence requirements for paywall state.
//
// # Batch Operations
//
// The Store interface provides batch operations for improved performance:
//
//   - SaveCartQuotes / GetCartQuotes: Bulk cart operations (100x faster for bulk imports)
//   - SaveRefundQuotes: Bulk refund creation (for batch refund processing)
//   - RecordPayments: Bulk payment recording (for settlement jobs)
//
// PostgreSQL implementation uses prepared statements and bulk queries.
// MongoDB/FileStore implementations use loop-based fallbacks (can be optimized later).
type Store interface {
	// Single-record cart operations
	SaveCartQuote(ctx context.Context, quote CartQuote) error
	GetCartQuote(ctx context.Context, cartID string) (CartQuote, error)
	MarkCartPaid(ctx context.Context, cartID, wallet string) error
	HasCartAccess(ctx context.Context, cartID, wallet string) bool

	// Batch cart operations (for bulk imports, analytics, admin dashboards)
	// SaveCartQuotes stores multiple quotes efficiently (atomic: all succeed or all fail).
	// GetCartQuotes retrieves multiple quotes in a single query (partial results: skips missing/expired).
	SaveCartQuotes(ctx context.Context, quotes []CartQuote) error
	GetCartQuotes(ctx context.Context, cartIDs []string) ([]CartQuote, error)

	// Single-record refund operations
	SaveRefundQuote(ctx context.Context, quote RefundQuote) error
	GetRefundQuote(ctx context.Context, refundID string) (RefundQuote, error)
	GetRefundQuoteByOriginalPurchaseID(ctx context.Context, originalPurchaseID string) (RefundQuote, error)
	ListPendingRefunds(ctx context.Context) ([]RefundQuote, error)
	MarkRefundProcessed(ctx context.Context, refundID, processedBy, signature string) error
	DeleteRefundQuote(ctx context.Context, refundID string) error

	// Batch refund operations (for bulk refund processing)
	SaveRefundQuotes(ctx context.Context, quotes []RefundQuote) error

	// Single-record payment transaction tracking for replay protection
	// CRITICAL: Signatures are globally unique - once used for any resource, they cannot be reused
	RecordPayment(ctx context.Context, tx PaymentTransaction) error
	HasPaymentBeenProcessed(ctx context.Context, signature string) (bool, error)
	GetPayment(ctx context.Context, signature string) (PaymentTransaction, error)

	// Batch payment operations (for bulk settlement jobs, batch imports)
	// CRITICAL: All signatures must be globally unique - batch fails if any signature exists
	RecordPayments(ctx context.Context, txs []PaymentTransaction) error

	// Payment archival for database cleanup
	// Archives old payment signatures beyond the retention period to prevent unbounded growth
	ArchiveOldPayments(ctx context.Context, olderThan time.Time) (int64, error) // Returns count of archived records

	// Nonce management for admin signature replay protection
	// CRITICAL: Each nonce can only be used once - prevents signature replay attacks
	CreateNonce(ctx context.Context, nonce AdminNonce) error
	ConsumeNonce(ctx context.Context, nonceID string) error // Returns error if already consumed or not found

	// Admin nonce cleanup for database maintenance
	CleanupExpiredNonces(ctx context.Context) (int64, error) // Returns count of deleted nonces

	// Webhook queue operations for persistent webhook delivery
	// EnqueueWebhook adds a webhook to the delivery queue (returns webhook ID)
	EnqueueWebhook(ctx context.Context, webhook PendingWebhook) (string, error)
	// DequeueWebhooks retrieves webhooks ready for delivery (up to limit, ordered by next attempt time)
	DequeueWebhooks(ctx context.Context, limit int) ([]PendingWebhook, error)
	// MarkWebhookProcessing updates webhook status to prevent duplicate processing
	MarkWebhookProcessing(ctx context.Context, webhookID string) error
	// MarkWebhookSuccess marks webhook as successfully delivered and removes from queue
	MarkWebhookSuccess(ctx context.Context, webhookID string) error
	// MarkWebhookFailed records failed attempt and schedules retry (or moves to DLQ if exhausted)
	MarkWebhookFailed(ctx context.Context, webhookID string, errorMsg string, nextAttemptAt time.Time) error
	// GetWebhook retrieves a webhook by ID (for admin UI)
	GetWebhook(ctx context.Context, webhookID string) (PendingWebhook, error)
	// ListWebhooks lists webhooks with optional status filter (for admin UI)
	ListWebhooks(ctx context.Context, status WebhookStatus, limit int) ([]PendingWebhook, error)
	// RetryWebhook resets webhook to pending state for manual retry (admin operation)
	RetryWebhook(ctx context.Context, webhookID string) error
	// DeleteWebhook removes webhook from queue (admin operation)
	DeleteWebhook(ctx context.Context, webhookID string) error

	Close() error
}

// StoreConfig holds storage backend configuration.
type StoreConfig struct {
	Backend         string // "memory", "postgres", "mongodb", or "file"
	PostgresURL     string
	MongoDBURL      string
	MongoDBDatabase string
	FilePath        string
	PostgresPool    config.PostgresPoolConfig // PostgreSQL connection pool settings
	CartQuoteTTL    time.Duration             // How long cart quotes remain valid
	RefundQuoteTTL  time.Duration             // How long refund quotes remain valid
	CleanupInterval time.Duration             // How often to clean up expired quotes

	// Schema mapping (table names for Postgres, collection names for MongoDB)
	PaymentTransactionsTableName string // Default: "payment_transactions"
	AdminNoncesTableName         string // Default: "admin_nonces"
	CartQuotesTableName          string // Default: "cart_quotes"
	RefundQuotesTableName        string // Default: "refund_quotes"
	WebhookQueueTableName        string // Default: "webhook_queue"
}

// NewStore creates a Store instance based on the provided configuration.
func NewStore(cfg StoreConfig) (Store, error) {
	return NewStoreWithDB(cfg, nil)
}

// NewStoreWithDB creates a Store instance with an optional shared database pool.
// If sharedDB is provided (non-nil) for postgres backends, it will be used instead of creating a new connection.
// Pass nil to create a new connection pool.
func NewStoreWithDB(cfg StoreConfig, sharedDB *sql.DB) (Store, error) {
	switch cfg.Backend {
	case "memory":
		// SECURITY WARNING: Memory backend loses all payment replay protection on restart
		// Only use for development/testing - NEVER in production
		return NewMemoryStore(), nil
	case "":
		// Smart defaults: Auto-detect backend from provided configuration
		// Priority order: postgres > mongodb > file (fallback)
		if cfg.PostgresURL != "" {
			// PostgreSQL URL provided without explicit backend - use postgres
			cfg.Backend = "postgres"
			var store *PostgresStore
			var err error
			if sharedDB != nil {
				store, err = NewPostgresStoreWithDB(sharedDB)
			} else {
				store, err = NewPostgresStore(cfg.PostgresURL, cfg.PostgresPool)
			}
			if err != nil {
				return nil, err
			}
			// Apply schema_mapping table names
			return store.WithTableNames(
				cfg.PaymentTransactionsTableName,
				cfg.AdminNoncesTableName,
				cfg.CartQuotesTableName,
				cfg.RefundQuotesTableName,
				cfg.WebhookQueueTableName,
			), nil
		}
		if cfg.MongoDBURL != "" {
			// MongoDB URL provided without explicit backend - use mongodb
			cfg.Backend = "mongodb"
			if cfg.MongoDBDatabase == "" {
				cfg.MongoDBDatabase = "cedros_pay" // Default database name
			}
			return NewMongoDBStore(cfg.MongoDBURL, cfg.MongoDBDatabase)
		}

		// Fallback to file-based storage for local development
		// ⚠️ WARNING: This is NOT production-safe (see docs/PRODUCTION.md)
		if cfg.FilePath == "" {
			cfg.FilePath = "./data/cedros-pay.db" // Default path
		}
		return NewFileStore(cfg.FilePath)
	case "postgres":
		if cfg.PostgresURL == "" {
			return nil, fmt.Errorf("postgres backend requires postgres_url")
		}
		// Use shared DB if provided, otherwise create new connection
		var store *PostgresStore
		var err error
		if sharedDB != nil {
			store, err = NewPostgresStoreWithDB(sharedDB)
		} else {
			store, err = NewPostgresStore(cfg.PostgresURL, cfg.PostgresPool)
		}
		if err != nil {
			return nil, err
		}
		// Apply schema_mapping table names
		return store.WithTableNames(
			cfg.PaymentTransactionsTableName,
			cfg.AdminNoncesTableName,
			cfg.CartQuotesTableName,
			cfg.RefundQuotesTableName,
			cfg.WebhookQueueTableName,
		), nil
	case "mongodb":
		if cfg.MongoDBURL == "" {
			return nil, fmt.Errorf("mongodb backend requires mongodb_url")
		}
		if cfg.MongoDBDatabase == "" {
			return nil, fmt.Errorf("mongodb backend requires mongodb_database")
		}
		return NewMongoDBStore(cfg.MongoDBURL, cfg.MongoDBDatabase)
	case "file":
		if cfg.FilePath == "" {
			return nil, fmt.Errorf("file backend requires file_path")
		}
		return NewFileStore(cfg.FilePath)
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", cfg.Backend)
	}
}

// MemoryStore is an in-memory Store implementation suitable for tests and single-instance deployments.
type MemoryStore struct {
	mu                       sync.RWMutex
	cartQuotes               map[string]CartQuote          // cartID -> quote
	refundQuotes             map[string]RefundQuote        // refundID -> quote
	refundQuotesByPurchaseID map[string]string             // originalPurchaseID -> refundID (secondary index for O(1) lookups)
	paymentTransactions      map[string]PaymentTransaction // signature -> transaction (globally unique)
	adminNonces              map[string]AdminNonce         // nonceID -> nonce (one-time use)
	webhookQueue             map[string]PendingWebhook     // webhookID -> webhook (persistent delivery queue)
	stopCleanup              chan struct{}
	cleanupDone              chan struct{}
}

// NewMemoryStore constructs a MemoryStore and starts background cleanup.
func NewMemoryStore() *MemoryStore {
	m := &MemoryStore{
		cartQuotes:               make(map[string]CartQuote),
		refundQuotes:             make(map[string]RefundQuote),
		refundQuotesByPurchaseID: make(map[string]string),
		paymentTransactions:      make(map[string]PaymentTransaction),
		adminNonces:              make(map[string]AdminNonce),
		webhookQueue:             make(map[string]PendingWebhook),
		stopCleanup:              make(chan struct{}),
		cleanupDone:              make(chan struct{}),
	}
	go m.cleanupExpiredAccess()
	return m
}

// cleanupExpiredAccess runs periodically and removes expired cart quotes, refund quotes, and admin nonces.
func (m *MemoryStore) cleanupExpiredAccess() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()
	defer close(m.cleanupDone)

	for {
		select {
		case <-m.stopCleanup:
			return
		case <-ticker.C:
			m.removeExpiredCarts()
			m.removeExpiredRefunds()
			m.removeExpiredNonces()
		}
	}
}

// Stop gracefully stops the cleanup goroutine.
func (m *MemoryStore) Stop() {
	close(m.stopCleanup)
	<-m.cleanupDone
}

// Close implements the Store interface by calling Stop.
func (m *MemoryStore) Close() error {
	m.Stop()
	return nil
}

// removeExpiredRefunds deletes all expired refund quotes.
func (m *MemoryStore) removeExpiredRefunds() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for refundID, quote := range m.refundQuotes {
		if quote.IsExpiredAt(now) {
			delete(m.refundQuotes, refundID)
			// Also remove from secondary index
			delete(m.refundQuotesByPurchaseID, quote.OriginalPurchaseID)
		}
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

// SaveRefundQuote persists or updates a refund quote.
func (m *MemoryStore) SaveRefundQuote(_ context.Context, quote RefundQuote) error {
	if err := validateAndPrepareRefundQuote(&quote, 0); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.refundQuotes[quote.ID] = quote
	// Maintain secondary index for O(1) lookups by original purchase ID
	m.refundQuotesByPurchaseID[quote.OriginalPurchaseID] = quote.ID
	return nil
}

// GetRefundQuote retrieves a refund quote by ID.
// NOTE: Refund requests never expire - they remain pending until approved or denied by admin.
func (m *MemoryStore) GetRefundQuote(_ context.Context, refundID string) (RefundQuote, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	quote, ok := m.refundQuotes[refundID]
	if !ok {
		return RefundQuote{}, ErrNotFound
	}
	// Refunds do not expire - they can always be retrieved and re-quoted
	return quote, nil
}

// GetRefundQuoteByOriginalPurchaseID retrieves a refund quote by original purchase ID (transaction signature).
// This enforces the one-refund-per-signature limit.
func (m *MemoryStore) GetRefundQuoteByOriginalPurchaseID(_ context.Context, originalPurchaseID string) (RefundQuote, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// O(1) lookup using secondary index
	refundID, exists := m.refundQuotesByPurchaseID[originalPurchaseID]
	if !exists {
		return RefundQuote{}, ErrNotFound
	}

	quote, ok := m.refundQuotes[refundID]
	if !ok {
		// Index is out of sync (should never happen, but handle gracefully)
		return RefundQuote{}, ErrNotFound
	}

	return quote, nil
}

// ListPendingRefunds returns all unprocessed refund quotes.
func (m *MemoryStore) ListPendingRefunds(_ context.Context) ([]RefundQuote, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var pending []RefundQuote
	for _, quote := range m.refundQuotes {
		if !quote.IsProcessed() {
			pending = append(pending, quote)
		}
	}
	return pending, nil
}

// DeleteRefundQuote removes a refund quote by ID.
func (m *MemoryStore) DeleteRefundQuote(_ context.Context, refundID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	quote, ok := m.refundQuotes[refundID]
	if !ok {
		return ErrNotFound
	}
	delete(m.refundQuotes, refundID)
	// Also remove from secondary index
	delete(m.refundQuotesByPurchaseID, quote.OriginalPurchaseID)
	return nil
}

// SaveRefundQuotes stores multiple refund quotes in a single operation.
// All quotes are validated before any are stored (atomic batch).
func (m *MemoryStore) SaveRefundQuotes(_ context.Context, quotes []RefundQuote) error {
	if len(quotes) == 0 {
		return nil // No-op for empty batch
	}

	// Validate all quotes first (fail fast before modifying state)
	for i := range quotes {
		if err := validateAndPrepareRefundQuote(&quotes[i], 0); err != nil {
			return fmt.Errorf("quote %d: %w", i, err)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Store all quotes (already validated)
	for _, quote := range quotes {
		m.refundQuotes[quote.ID] = quote
		// Maintain secondary index for O(1) lookups by original purchase ID
		m.refundQuotesByPurchaseID[quote.OriginalPurchaseID] = quote.ID
	}

	return nil
}

// MarkRefundProcessed marks a refund as completed with the transaction signature.
func (m *MemoryStore) MarkRefundProcessed(_ context.Context, refundID, processedBy, signature string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	quote, ok := m.refundQuotes[refundID]
	if !ok {
		return ErrNotFound
	}
	now := time.Now()
	quote.ProcessedBy = processedBy
	quote.ProcessedAt = &now
	quote.Signature = signature
	m.refundQuotes[refundID] = quote
	return nil
}

// RecordPayment saves a verified payment transaction for replay protection.
// CRITICAL: Signature is the sole key - prevents cross-resource replay attacks.
// Returns error if signature already exists (concurrent replay attack).
func (m *MemoryStore) RecordPayment(_ context.Context, tx PaymentTransaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if signature already exists
	existingTx, exists := m.paymentTransactions[tx.Signature]
	if exists {
		// Allow updating placeholder records (wallet='' or status='verifying')
		isPlaceholder := existingTx.Wallet == "" ||
			(existingTx.Metadata != nil && existingTx.Metadata["status"] == "verifying")

		if !isPlaceholder {
			// Real transaction already exists - this is a replay attack
			return fmt.Errorf("signature already used: replay attack detected")
		}

		// Update placeholder with verified data
		m.paymentTransactions[tx.Signature] = tx
		return nil
	}

	// First time seeing this signature - insert as new
	m.paymentTransactions[tx.Signature] = tx
	return nil
}

// RecordPayments saves multiple verified payment transactions in a single operation.
// CRITICAL: ALL signatures must be globally unique - batch fails if ANY signature exists.
// This is atomic - either all succeed or none are stored (fail fast on first duplicate).
func (m *MemoryStore) RecordPayments(_ context.Context, txs []PaymentTransaction) error {
	if len(txs) == 0 {
		return nil // No-op for empty batch
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// First pass: check for duplicate signatures (fail fast)
	for i, tx := range txs {
		if _, exists := m.paymentTransactions[tx.Signature]; exists {
			return fmt.Errorf("transaction %d: signature already used: %s", i, tx.Signature)
		}
	}

	// Second pass: check for duplicates within the batch itself
	seen := make(map[string]bool, len(txs))
	for i, tx := range txs {
		if seen[tx.Signature] {
			return fmt.Errorf("transaction %d: duplicate signature in batch: %s", i, tx.Signature)
		}
		seen[tx.Signature] = true
	}

	// All signatures are unique - store all transactions
	for _, tx := range txs {
		m.paymentTransactions[tx.Signature] = tx
	}

	return nil
}

// HasPaymentBeenProcessed checks if a transaction signature has EVER been used.
// Returns true if signature exists for ANY resource (prevents cross-resource replay).
func (m *MemoryStore) HasPaymentBeenProcessed(_ context.Context, signature string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.paymentTransactions[signature]
	return exists, nil
}

// GetPayment retrieves a payment transaction by signature.
// Returns the original payment showing which resource it was used for.
func (m *MemoryStore) GetPayment(_ context.Context, signature string) (PaymentTransaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tx, ok := m.paymentTransactions[signature]
	if !ok {
		return PaymentTransaction{}, ErrNotFound
	}
	return tx, nil
}

// CreateNonce stores a new admin nonce for replay protection.
// Nonce must be unique and not already exist.
func (m *MemoryStore) CreateNonce(_ context.Context, nonce AdminNonce) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if nonce already exists
	if _, exists := m.adminNonces[nonce.ID]; exists {
		return fmt.Errorf("nonce already exists: %s", nonce.ID)
	}

	m.adminNonces[nonce.ID] = nonce
	return nil
}

// ConsumeNonce marks a nonce as consumed (one-time use).
// Returns error if nonce doesn't exist, is already consumed, or has expired.
func (m *MemoryStore) ConsumeNonce(_ context.Context, nonceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nonce, ok := m.adminNonces[nonceID]
	if !ok {
		return fmt.Errorf("nonce not found: %s", nonceID)
	}

	if nonce.IsConsumed() {
		return fmt.Errorf("nonce already consumed: %s", nonceID)
	}

	// Mark as consumed
	now := time.Now()
	if nonce.IsExpiredAt(now) {
		return fmt.Errorf("nonce expired: %s", nonceID)
	}

	nonce.ConsumedAt = &now
	m.adminNonces[nonceID] = nonce
	return nil
}

// removeExpiredNonces deletes all expired admin nonces.
func (m *MemoryStore) removeExpiredNonces() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for nonceID, nonce := range m.adminNonces {
		if nonce.IsExpiredAt(now) {
			delete(m.adminNonces, nonceID)
		}
	}
}

// ArchiveOldPayments deletes payment transactions older than the specified time.
// For MemoryStore, this is primarily for testing - memory stores are ephemeral.
func (m *MemoryStore) ArchiveOldPayments(_ context.Context, olderThan time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := int64(0)
	for sig, tx := range m.paymentTransactions {
		if tx.CreatedAt.Before(olderThan) {
			delete(m.paymentTransactions, sig)
			count++
		}
	}

	return count, nil
}

// CleanupExpiredNonces deletes expired admin nonces.
// For MemoryStore, this returns the count of deleted nonces.
func (m *MemoryStore) CleanupExpiredNonces(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	count := int64(0)

	for nonceID, nonce := range m.adminNonces {
		if nonce.IsExpiredAt(now) {
			delete(m.adminNonces, nonceID)
			count++
		}
	}

	return count, nil
}
