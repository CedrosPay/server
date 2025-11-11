package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileStore implements Store using JSON file storage.
type FileStore struct {
	filePath            string
	mu                  sync.RWMutex
	data                fileData // In-memory copy of file data
	cartQuotes          map[string]CartQuote
	refundQuotes        map[string]RefundQuote
	paymentTransactions map[string]PaymentTransaction
	adminNonces         map[string]AdminNonce
	stopCleanup         chan struct{}
	cleanupDone         chan struct{}
	dirty               bool
	flushTicker         *time.Ticker
	stopFlush           chan struct{}
	flushDone           chan struct{}
}

// fileData represents the JSON structure stored in the file.
type fileData struct {
	CartQuotes          map[string]CartQuote          `json:"cart_quotes"`
	RefundQuotes        map[string]RefundQuote        `json:"refund_quotes"`
	PaymentTransactions map[string]PaymentTransaction `json:"payment_transactions"`
	AdminNonces         map[string]AdminNonce         `json:"admin_nonces"`
	WebhookQueue        map[string]PendingWebhook     `json:"webhook_queue"`
}

// NewFileStore creates a new file-backed store.
//
// ⚠️ PRODUCTION WARNING: FileStore is NOT safe for production use!
// This storage backend should ONLY be used for local development and testing.
// For production deployments, use PostgreSQL or MongoDB instead.
//
// Reasons FileStore is unsuitable for production:
//   1. No horizontal scaling support (multiple instances corrupt data)
//   2. Race conditions at high concurrency (>100 req/sec)
//   3. 5-second flush interval creates data loss risk
//   4. No ACID guarantees (partial writes corrupt database)
//   5. Single point of failure (file corruption = total loss)
//
// See docs/PRODUCTION.md for production deployment guide.
func NewFileStore(filePath string) (*FileStore, error) {
	// Log production warning if environment suggests production use
	if env := os.Getenv("ENVIRONMENT"); env == "production" || env == "prod" {
		fmt.Fprintf(os.Stderr, "\n"+
			"╔══════════════════════════════════════════════════════════════╗\n"+
			"║ ⚠️  CRITICAL WARNING: FileStore in Production Detected!     ║\n"+
			"╠══════════════════════════════════════════════════════════════╣\n"+
			"║ FileStore is NOT safe for production deployments.           ║\n"+
			"║                                                              ║\n"+
			"║ Issues:                                                      ║\n"+
			"║   • No horizontal scaling (data corruption)                 ║\n"+
			"║   • Race conditions at >100 req/sec                          ║\n"+
			"║   • 5-second flush = potential data loss                     ║\n"+
			"║   • No ACID guarantees                                       ║\n"+
			"║                                                              ║\n"+
			"║ REQUIRED: Use PostgreSQL or MongoDB for production          ║\n"+
			"║ See: docs/PRODUCTION.md                                     ║\n"+
			"╚══════════════════════════════════════════════════════════════╝\n\n")
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	store := &FileStore{
		filePath:            filePath,
		cartQuotes:          make(map[string]CartQuote),
		refundQuotes:        make(map[string]RefundQuote),
		paymentTransactions: make(map[string]PaymentTransaction),
		adminNonces:         make(map[string]AdminNonce),
		stopCleanup:         make(chan struct{}),
		cleanupDone:         make(chan struct{}),
		flushTicker:         time.NewTicker(5 * time.Second),
		stopFlush:           make(chan struct{}),
		flushDone:           make(chan struct{}),
	}

	// Load existing data
	if err := store.load(); err != nil {
		return nil, err
	}

	// Start background cleanup
	go store.cleanupExpired()

	// Start background flush
	go store.periodicFlush()

	return store, nil
}

// load reads data from the file.
func (s *FileStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If file doesn't exist, start with empty maps
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	var fileData fileData
	if err := json.Unmarshal(data, &fileData); err != nil {
		return fmt.Errorf("unmarshal data: %w", err)
	}

	if fileData.CartQuotes != nil {
		s.cartQuotes = fileData.CartQuotes
	}
	if fileData.RefundQuotes != nil {
		s.refundQuotes = fileData.RefundQuotes
	}
	if fileData.PaymentTransactions != nil {
		s.paymentTransactions = fileData.PaymentTransactions
	}
	if fileData.AdminNonces != nil {
		s.adminNonces = fileData.AdminNonces
	}

	// Load webhook queue into data struct for webhook queue methods
	s.data = fileData
	if s.data.WebhookQueue == nil {
		s.data.WebhookQueue = make(map[string]PendingWebhook)
	}

	return nil
}

// save writes data to the file.
func (s *FileStore) save() error {
	data := fileData{
		CartQuotes:          s.cartQuotes,
		RefundQuotes:        s.refundQuotes,
		PaymentTransactions: s.paymentTransactions,
		AdminNonces:         s.adminNonces,
		WebhookQueue:        s.data.WebhookQueue,
	}
	return s.saveData(data)
}

// saveData writes the given data to disk.
func (s *FileStore) saveData(data fileData) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	// Write to temporary file first
	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename file: %w", err)
	}
	_ = os.Chmod(s.filePath, 0600)

	return nil
}

// persist is a helper for webhook queue methods to save data immediately.
// Unlike save(), this method is called from within lock-holding webhook methods.
func (s *FileStore) persist() error {
	// Note: Caller already holds the lock
	return s.save()
}

// markDirty marks the store as modified, requiring a flush.
func (s *FileStore) markDirty() {
	s.dirty = true
}

// periodicFlush flushes dirty data to disk every 5 seconds.
func (s *FileStore) periodicFlush() {
	defer close(s.flushDone)

	for {
		select {
		case <-s.stopFlush:
			return
		case <-s.flushTicker.C:
			// Snapshot maps under minimal lock, then copy outside lock
			s.mu.Lock()
			if !s.dirty {
				s.mu.Unlock()
				continue
			}

			// Snapshot map references (no deep copy yet) - fast operation
			snapshotQuotes := s.cartQuotes
			snapshotRefunds := s.refundQuotes
			snapshotPayments := s.paymentTransactions
			snapshotNonces := s.adminNonces
			s.dirty = false
			s.mu.Unlock()

			// Deep copy maps outside of lock to avoid blocking reads
			data := fileData{
				CartQuotes:          copyMap(snapshotQuotes),
				RefundQuotes:        copyMap(snapshotRefunds),
				PaymentTransactions: copyMap(snapshotPayments),
				AdminNonces:         copyMap(snapshotNonces),
			}

			// Perform I/O outside of lock
			s.saveData(data)
		}
	}
}

// copyMap creates a shallow copy of a map using generics.
// This helper function replaces the previous type-specific copy functions.
func copyMap[K comparable, V any](m map[K]V) map[K]V {
	copy := make(map[K]V, len(m))
	for k, v := range m {
		copy[k] = v
	}
	return copy
}

// SaveCartQuote persists or updates a cart quote.
func (s *FileStore) SaveCartQuote(ctx context.Context, quote CartQuote) error {
	if err := validateAndPrepareCartQuote(&quote, 0); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cartQuotes[quote.ID] = quote
	s.markDirty()
	return nil
}

// GetCartQuote retrieves a cart quote by ID.
func (s *FileStore) GetCartQuote(ctx context.Context, cartID string) (CartQuote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	quote, exists := s.cartQuotes[cartID]
	if !exists {
		return CartQuote{}, ErrNotFound
	}

	now := time.Now()
	if quote.IsExpiredAt(now) {
		return CartQuote{}, ErrCartExpired
	}

	return quote, nil
}

// MarkCartPaid marks a cart as paid.
func (s *FileStore) MarkCartPaid(ctx context.Context, cartID, wallet string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	quote, exists := s.cartQuotes[cartID]
	if !exists {
		return ErrNotFound
	}

	quote.WalletPaidBy = wallet
	s.cartQuotes[cartID] = quote

	s.markDirty()
	return nil
}

// HasCartAccess checks if a cart is paid by the wallet.
func (s *FileStore) HasCartAccess(ctx context.Context, cartID, wallet string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	quote, exists := s.cartQuotes[cartID]
	if !exists {
		return false
	}

	return quote.WalletPaidBy == wallet
}

// SaveRefundQuote persists or updates a refund quote.
func (s *FileStore) SaveRefundQuote(ctx context.Context, quote RefundQuote) error {
	if err := validateAndPrepareRefundQuote(&quote, 0); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.refundQuotes[quote.ID] = quote
	s.markDirty()
	return nil
}

// GetRefundQuote retrieves a refund quote by ID.
func (s *FileStore) GetRefundQuote(ctx context.Context, refundID string) (RefundQuote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	quote, exists := s.refundQuotes[refundID]
	if !exists {
		return RefundQuote{}, ErrNotFound
	}

	// Refund requests never expire - they remain pending until approved or denied by admin
	return quote, nil
}

// GetRefundQuoteByOriginalPurchaseID retrieves a refund quote by original purchase ID (transaction signature).
// This enforces the one-refund-per-signature limit.
func (s *FileStore) GetRefundQuoteByOriginalPurchaseID(ctx context.Context, originalPurchaseID string) (RefundQuote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Linear search through refunds to find match
	for _, quote := range s.refundQuotes {
		if quote.OriginalPurchaseID == originalPurchaseID {
			return quote, nil
		}
	}
	return RefundQuote{}, ErrNotFound
}

// ListPendingRefunds returns all unprocessed refund quotes.
func (s *FileStore) ListPendingRefunds(_ context.Context) ([]RefundQuote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var pending []RefundQuote
	for _, quote := range s.refundQuotes {
		if !quote.IsProcessed() {
			pending = append(pending, quote)
		}
	}
	return pending, nil
}

// DeleteRefundQuote removes a refund quote by ID.
func (s *FileStore) DeleteRefundQuote(ctx context.Context, refundID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.refundQuotes[refundID]; !exists {
		return ErrNotFound
	}

	delete(s.refundQuotes, refundID)
	s.markDirty()
	return nil
}

// MarkRefundProcessed marks a refund as completed.
func (s *FileStore) MarkRefundProcessed(ctx context.Context, refundID, processedBy, signature string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	quote, exists := s.refundQuotes[refundID]
	if !exists {
		return ErrNotFound
	}

	now := time.Now()
	quote.ProcessedBy = processedBy
	quote.ProcessedAt = &now
	quote.Signature = signature
	s.refundQuotes[refundID] = quote

	s.markDirty()
	return nil
}

// cleanupExpired removes expired records periodically.
func (s *FileStore) cleanupExpired() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()
	defer close(s.cleanupDone)

	for {
		select {
		case <-s.stopCleanup:
			return
		case <-ticker.C:
			s.performCleanup()
		}
	}
}

// performCleanup removes expired records.
func (s *FileStore) performCleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	modified := false

	// Remove expired cart quotes
	for key, quote := range s.cartQuotes {
		if now.After(quote.ExpiresAt) {
			delete(s.cartQuotes, key)
			modified = true
		}
	}

	// Remove expired admin nonces
	for key, nonce := range s.adminNonces {
		if now.After(nonce.ExpiresAt) {
			delete(s.adminNonces, key)
			modified = true
		}
	}

	// NOTE: Refund requests are NOT auto-deleted when expired
	// They must be explicitly denied by admin via DELETE /refund/:id
	// ExpiresAt is only used to prevent stale transaction execution

	// Only mark dirty if we actually removed something
	if modified {
		s.markDirty()
	}
}

// RecordPayment saves a verified payment transaction for replay protection.
// CRITICAL: Signature is globally unique - once used, cannot be reused for any resource.
// Returns error if signature already exists (concurrent replay attack).
func (s *FileStore) RecordPayment(_ context.Context, tx PaymentTransaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if signature already exists
	existingTx, exists := s.paymentTransactions[tx.Signature]
	if exists {
		// Allow updating placeholder records (wallet='' or status='verifying')
		isPlaceholder := existingTx.Wallet == "" ||
			(existingTx.Metadata != nil && existingTx.Metadata["status"] == "verifying")

		if !isPlaceholder {
			// Real transaction already exists - this is a replay attack
			return fmt.Errorf("signature already used: replay attack detected")
		}

		// Update placeholder with verified data
		s.paymentTransactions[tx.Signature] = tx
		s.markDirty()
		return nil
	}

	// First time seeing this signature - insert as new
	s.paymentTransactions[tx.Signature] = tx
	s.markDirty()
	return nil
}

// HasPaymentBeenProcessed checks if a transaction signature has EVER been used.
// Returns true if signature exists for ANY resource (prevents cross-resource replay).
func (s *FileStore) HasPaymentBeenProcessed(_ context.Context, signature string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.paymentTransactions[signature]
	return exists, nil
}

// GetPayment retrieves a payment transaction by signature.
// Returns the original payment record showing which resource it was used for.
func (s *FileStore) GetPayment(_ context.Context, signature string) (PaymentTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tx, ok := s.paymentTransactions[signature]
	if !ok {
		return PaymentTransaction{}, ErrNotFound
	}
	return tx, nil
}

// Close closes the file store.
func (s *FileStore) Close() error {
	// Signal goroutines to stop first (without holding lock)
	close(s.stopCleanup)
	close(s.stopFlush)
	s.flushTicker.Stop()

	// Wait for goroutines to finish before final flush
	<-s.cleanupDone
	<-s.flushDone

	// Now acquire lock and perform final flush
	// All goroutines are stopped, so no contention
	s.mu.Lock()
	defer s.mu.Unlock()

	finalFlushErr := error(nil)
	if s.dirty {
		finalFlushErr = s.save()
	}

	return finalFlushErr
}

// CreateNonce stores a new admin nonce for replay protection.
func (s *FileStore) CreateNonce(_ context.Context, nonce AdminNonce) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if nonce already exists
	if _, exists := s.adminNonces[nonce.ID]; exists {
		return fmt.Errorf("nonce already exists: %s", nonce.ID)
	}

	s.adminNonces[nonce.ID] = nonce
	s.markDirty()
	return nil
}

// ConsumeNonce marks a nonce as consumed (one-time use).
func (s *FileStore) ConsumeNonce(_ context.Context, nonceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	nonce, ok := s.adminNonces[nonceID]
	if !ok {
		return fmt.Errorf("nonce not found: %s", nonceID)
	}

	if nonce.IsConsumed() {
		return fmt.Errorf("nonce already consumed: %s", nonceID)
	}

	now := time.Now()
	if nonce.IsExpiredAt(now) {
		return fmt.Errorf("nonce expired: %s", nonceID)
	}

	nonce.ConsumedAt = &now
	s.adminNonces[nonceID] = nonce

	s.markDirty()
	return nil
}

// ArchiveOldPayments deletes payment transactions older than the specified time.
// This prevents unbounded growth of the file store while maintaining
// replay protection for recent transactions (e.g., last 90 days).
//
// Returns the number of archived (deleted) records.
func (s *FileStore) ArchiveOldPayments(_ context.Context, olderThan time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := int64(0)
	for sig, tx := range s.paymentTransactions {
		if tx.CreatedAt.Before(olderThan) {
			delete(s.paymentTransactions, sig)
			count++
		}
	}

	if count > 0 {
		s.markDirty()
	}

	return count, nil
}

// CleanupExpiredNonces deletes expired admin nonces from the file store.
// This prevents unbounded growth of the file store.
//
// Returns the number of deleted nonces.
func (s *FileStore) CleanupExpiredNonces(_ context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := int64(0)

	for nonceID, nonce := range s.adminNonces {
		if nonce.IsExpiredAt(now) {
			delete(s.adminNonces, nonceID)
			count++
		}
	}

	if count > 0 {
		s.markDirty()
	}

	return count, nil
}

// Batch operations (simple loop-based implementations for FileStore)

// SaveCartQuotes stores multiple cart quotes using individual operations.
func (s *FileStore) SaveCartQuotes(ctx context.Context, quotes []CartQuote) error {
	for i, quote := range quotes {
		if err := s.SaveCartQuote(ctx, quote); err != nil {
			return fmt.Errorf("quote %d: %w", i, err)
		}
	}
	return nil
}

// GetCartQuotes retrieves multiple cart quotes using individual queries.
func (s *FileStore) GetCartQuotes(ctx context.Context, cartIDs []string) ([]CartQuote, error) {
	quotes := make([]CartQuote, 0, len(cartIDs))
	for _, cartID := range cartIDs {
		quote, err := s.GetCartQuote(ctx, cartID)
		if err == ErrNotFound || err == ErrCartExpired {
			continue
		}
		if err != nil {
			return nil, err
		}
		quotes = append(quotes, quote)
	}
	return quotes, nil
}

// SaveRefundQuotes stores multiple refund quotes using individual operations.
func (s *FileStore) SaveRefundQuotes(ctx context.Context, quotes []RefundQuote) error {
	for i, quote := range quotes {
		if err := s.SaveRefundQuote(ctx, quote); err != nil {
			return fmt.Errorf("quote %d: %w", i, err)
		}
	}
	return nil
}

// RecordPayments saves multiple payment transactions using individual operations.
func (s *FileStore) RecordPayments(ctx context.Context, txs []PaymentTransaction) error {
	for i, tx := range txs {
		if err := s.RecordPayment(ctx, tx); err != nil {
			return fmt.Errorf("tx %d: %w", i, err)
		}
	}
	return nil
}
