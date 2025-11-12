package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/CedrosPay/server/internal/money"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBStore implements Store using MongoDB.
type MongoDBStore struct {
	client              *mongo.Client
	db                  *mongo.Database // Database reference for new collections
	cartQuotes          *mongo.Collection
	refundQuotes        *mongo.Collection
	paymentTransactions *mongo.Collection
	stopCleanup         chan struct{}
	cleanupDone         chan struct{}
}

// NewMongoDBStore creates a new MongoDB-backed store.
func NewMongoDBStore(connectionString, database string) (*MongoDBStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(connectionString))
	if err != nil {
		return nil, fmt.Errorf("connect to mongodb: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		// NOTE: client.Disconnect() error is intentionally ignored during initialization cleanup.
		// If connection fails, the Disconnect() error is not actionable and would only obscure
		// the original connection failure. The primary error is returned to the caller.
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}

	db := client.Database(database)

	store := &MongoDBStore{
		client:              client,
		db:                  db,
		cartQuotes:          db.Collection("cart_quotes"),
		refundQuotes:        db.Collection("refund_quotes"),
		paymentTransactions: db.Collection("payment_transactions"),
		stopCleanup:         make(chan struct{}),
		cleanupDone:         make(chan struct{}),
	}

	// Create indexes
	if err := store.createIndexes(ctx); err != nil {
		// Same rationale: Disconnect() error during initialization cleanup is not actionable
		_ = client.Disconnect(ctx)
		return nil, err
	}

	// Start background cleanup
	go store.cleanupExpired()

	return store, nil
}

// createIndexes creates necessary indexes for collections.
func (s *MongoDBStore) createIndexes(ctx context.Context) error {
	// Cart quotes indexes
	// Note: _id is automatically unique in MongoDB, no need to create it
	_, err := s.cartQuotes.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "expiresat", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("create cart quotes indexes: %w", err)
	}

	// Refund quotes indexes
	_, err = s.refundQuotes.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "expires_at", Value: 1}}},
		{Keys: bson.D{{Key: "processed_at", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("create refund quotes indexes: %w", err)
	}

	// Payment transactions indexes
	_, err = s.paymentTransactions.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "signature", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "resource_id", Value: 1}}},
		{Keys: bson.D{{Key: "wallet", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("create payment transactions indexes: %w", err)
	}

	return nil
}

// SaveCartQuote persists or updates a cart quote.
func (s *MongoDBStore) SaveCartQuote(ctx context.Context, quote CartQuote) error {
	if err := validateAndPrepareCartQuote(&quote, 0); err != nil {
		return err
	}

	filter := bson.M{"_id": quote.ID}
	update := bson.M{"$set": quote}
	opts := options.Update().SetUpsert(true)

	_, err := s.cartQuotes.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetCartQuote retrieves a cart quote by ID.
func (s *MongoDBStore) GetCartQuote(ctx context.Context, cartID string) (CartQuote, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	filter := bson.M{"_id": cartID}

	// Decode into intermediate struct first (MongoDB stores money.Money as nested documents)
	var mongoQuote mongoCartQuote
	err := s.cartQuotes.FindOne(ctx, filter).Decode(&mongoQuote)
	if err == mongo.ErrNoDocuments {
		return CartQuote{}, ErrNotFound
	}
	if err != nil {
		return CartQuote{}, err
	}

	// Convert MongoDB document to CartQuote
	quote, err := convertMongoCartQuote(mongoQuote, cartID)
	if err != nil {
		return CartQuote{}, fmt.Errorf("convert mongo cart quote: %w", err)
	}

	now := time.Now()
	if quote.IsExpiredAt(now) {
		return CartQuote{}, ErrCartExpired
	}

	return quote, nil
}

// MarkCartPaid marks a cart as paid.
func (s *MongoDBStore) MarkCartPaid(ctx context.Context, cartID, wallet string) error {
	filter := bson.M{"_id": cartID}
	update := bson.M{"$set": bson.M{"walletpaidby": wallet}}

	result, err := s.cartQuotes.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}

	return nil
}

// HasCartAccess checks if a cart is paid by the wallet.
func (s *MongoDBStore) HasCartAccess(ctx context.Context, cartID, wallet string) bool {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	filter := bson.M{
		"_id":          cartID,
		"walletpaidby": wallet,
	}

	count, err := s.cartQuotes.CountDocuments(ctx, filter)
	return err == nil && count > 0
}

// SaveRefundQuote persists or updates a refund quote.
func (s *MongoDBStore) SaveRefundQuote(ctx context.Context, quote RefundQuote) error {
	if err := validateAndPrepareRefundQuote(&quote, 0); err != nil {
		return err
	}

	filter := bson.M{"_id": quote.ID}
	// Store as-is with nested money.Money (maintains compatibility with existing data)
	update := bson.M{"$set": quote}
	opts := options.Update().SetUpsert(true)

	_, err := s.refundQuotes.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetRefundQuote retrieves a refund quote by ID.
func (s *MongoDBStore) GetRefundQuote(ctx context.Context, refundID string) (RefundQuote, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	filter := bson.M{"_id": refundID}

	// Decode into intermediate struct first (MongoDB stores money.Money as separate fields)
	var mongoQuote mongoRefundQuote
	err := s.refundQuotes.FindOne(ctx, filter).Decode(&mongoQuote)
	if err == mongo.ErrNoDocuments {
		return RefundQuote{}, ErrNotFound
	}
	if err != nil {
		return RefundQuote{}, err
	}

	// Convert MongoDB document to RefundQuote (extract money.Money from nested bson.M)
	quote, err := convertMongoRefundQuote(mongoQuote, refundID)
	if err != nil {
		return RefundQuote{}, fmt.Errorf("convert mongo refund quote: %w", err)
	}

	// Refund requests never expire - they remain pending until approved or denied by admin
	return quote, nil
}

// GetRefundQuoteByOriginalPurchaseID retrieves a refund quote by original purchase ID (transaction signature).
// This enforces the one-refund-per-signature limit.
func (s *MongoDBStore) GetRefundQuoteByOriginalPurchaseID(ctx context.Context, originalPurchaseID string) (RefundQuote, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	filter := bson.M{"originalpurchaseid": originalPurchaseID}

	// Decode into intermediate struct first (MongoDB stores money.Money as separate fields)
	var mongoQuote mongoRefundQuote
	err := s.refundQuotes.FindOne(ctx, filter).Decode(&mongoQuote)
	if err == mongo.ErrNoDocuments {
		return RefundQuote{}, ErrNotFound
	}
	if err != nil {
		return RefundQuote{}, err
	}

	// Convert MongoDB document to RefundQuote (extract money.Money from nested bson.M)
	quote, err := convertMongoRefundQuote(mongoQuote, originalPurchaseID)
	if err != nil {
		return RefundQuote{}, fmt.Errorf("convert mongo refund quote: %w", err)
	}

	return quote, nil
}

// DeleteRefundQuote removes a refund quote by ID.
func (s *MongoDBStore) DeleteRefundQuote(ctx context.Context, refundID string) error {
	filter := bson.M{"_id": refundID}

	result, err := s.refundQuotes.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}

	return nil
}

// ListPendingRefunds returns all unprocessed refund quotes.
func (s *MongoDBStore) ListPendingRefunds(ctx context.Context) ([]RefundQuote, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	// Query for refunds where processed_at is null/zero
	filter := bson.M{"$or": []bson.M{
		{"processedat": bson.M{"$exists": false}},
		{"processedat": nil},
	}}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})

	cursor, err := s.refundQuotes.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var refunds []RefundQuote
	for cursor.Next(ctx) {
		// Decode into intermediate struct first (MongoDB stores money.Money as separate fields)
		var mongoQuote mongoRefundQuote
		if err := cursor.Decode(&mongoQuote); err != nil {
			return nil, fmt.Errorf("decode refund quote: %w", err)
		}

		// Convert MongoDB document to RefundQuote (extract money.Money from nested bson.M)
		quote, err := convertMongoRefundQuote(mongoQuote, mongoQuote.ID)
		if err != nil {
			return nil, fmt.Errorf("convert refund quote %s: %w", mongoQuote.ID, err)
		}

		refunds = append(refunds, quote)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return refunds, nil
}

// MarkRefundProcessed marks a refund as completed.
func (s *MongoDBStore) MarkRefundProcessed(ctx context.Context, refundID, processedBy, signature string) error {
	filter := bson.M{"_id": refundID}
	now := time.Now()
	update := bson.M{"$set": bson.M{
		"processedby": processedBy,
		"processedat": now,
		"signature":   signature,
	}}

	result, err := s.refundQuotes.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}

	return nil
}

// cleanupExpired removes expired records periodically.
func (s *MongoDBStore) cleanupExpired() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()
	defer close(s.cleanupDone)

	for {
		select {
		case <-s.stopCleanup:
			return
		case <-ticker.C:
			ctx := context.Background()
			now := time.Now()

			// Remove expired cart quotes
			s.cartQuotes.DeleteMany(ctx, bson.M{"expiresat": bson.M{"$lt": now}})

			// NOTE: Refund requests are NOT auto-deleted when expired
			// They must be explicitly denied by admin via DELETE /refund/:id
			// ExpiresAt is only used to prevent stale transaction execution
		}
	}
}

// RecordPayment saves a verified payment transaction for replay protection.
// CRITICAL: Signature is globally unique - once used, cannot be reused for any resource.
// Returns error if signature already exists (concurrent replay attack).
func (s *MongoDBStore) RecordPayment(ctx context.Context, tx PaymentTransaction) error {
	// Check if placeholder record exists that needs updating
	filter := bson.M{"signature": tx.Signature}

	// Try to update placeholder records (wallet='' or status='verifying')
	placeholderFilter := bson.M{
		"signature": tx.Signature,
		"$or": []bson.M{
			{"wallet": ""},
			{"metadata.status": "verifying"},
		},
	}

	updateFields := bson.M{
		"resource_id": tx.ResourceID,
		"wallet":      tx.Wallet,
		"amount":      tx.Amount.Atomic,
		"asset":       tx.Amount.Asset.Code,
		"created_at":  tx.CreatedAt,
		"metadata":    tx.Metadata,
	}

	// First try updating placeholder
	result, err := s.paymentTransactions.UpdateOne(ctx, placeholderFilter, bson.M{"$set": updateFields})
	if err != nil {
		return err
	}

	if result.MatchedCount > 0 {
		// Successfully updated placeholder
		return nil
	}

	// No placeholder found, try insert with setOnInsert
	update := bson.M{"$setOnInsert": updateFields}
	opts := options.Update().SetUpsert(true)

	result, err = s.paymentTransactions.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return err
	}

	// Check if document was actually inserted (UpsertedCount = 0 means signature already existed)
	if result.UpsertedCount == 0 {
		// Signature already exists as verified record - replay attack
		return fmt.Errorf("signature already used: replay attack detected")
	}

	return nil
}

// HasPaymentBeenProcessed checks if a transaction signature has EVER been used.
// Returns true if signature exists for ANY resource (prevents cross-resource replay).
func (s *MongoDBStore) HasPaymentBeenProcessed(ctx context.Context, signature string) (bool, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	filter := bson.M{"signature": signature}
	count, err := s.paymentTransactions.CountDocuments(ctx, filter)
	if err != nil {
		return false, fmt.Errorf("check payment processed: %w", err)
	}
	return count > 0, nil
}

// mongoPaymentTransaction is the MongoDB document structure for payment transactions.
// MongoDB stores amount and asset as separate fields, not as embedded money.Money struct.
type mongoPaymentTransaction struct {
	Signature  string            `bson:"signature"`
	ResourceID string            `bson:"resource_id"`
	Wallet     string            `bson:"wallet"`
	Amount     int64             `bson:"amount"` // Atomic amount
	Asset      string            `bson:"asset"`  // Asset code (e.g., "USDC")
	CreatedAt  time.Time         `bson:"created_at"`
	Metadata   map[string]string `bson:"metadata"`
}

// mongoRefundQuote is an intermediate struct for MongoDB decoding.
// RefundQuote stores money.Money as nested document: {asset: {code: "USDC"}, atomic: 123}
type mongoRefundQuote struct {
	ID                 string            `bson:"_id"`
	OriginalPurchaseID string            `bson:"originalpurchaseid"`
	RecipientWallet    string            `bson:"recipientwallet"`
	Amount             bson.M            `bson:"amount"` // Nested: {asset: {code: "USDC", ...}, atomic: 640000}
	Reason             string            `bson:"reason"`
	Metadata           map[string]string `bson:"metadata"`
	CreatedAt          time.Time         `bson:"createdat"`
	ExpiresAt          time.Time         `bson:"expiresat"`
	ProcessedBy        string            `bson:"processedby"`
	ProcessedAt        *time.Time        `bson:"processedat"`
}

// mongoCartItem is an intermediate struct for MongoDB decoding.
// CartItem has no BSON tags, so MongoDB uses lowercase field names.
// money.Money is stored as nested document: {asset: {...}, atomic: 123}
type mongoCartItem struct {
	ResourceID string            `bson:"resourceid"`
	Quantity   int64             `bson:"quantity"`
	Price      bson.M            `bson:"price"` // Nested document: {asset: {code: "USDC", ...}, atomic: 123}
	Metadata   map[string]string `bson:"metadata"`
}

// mongoCartQuote is an intermediate struct for MongoDB decoding.
// CartQuote has no BSON tags, so MongoDB uses lowercase field names.
// money.Money fields stored as nested documents.
type mongoCartQuote struct {
	ID           string            `bson:"_id"`
	Items        []mongoCartItem   `bson:"items"`
	Total        bson.M            `bson:"total"` // Nested document: {asset: {code: "USDC", ...}, atomic: 123}
	Metadata     map[string]string `bson:"metadata"`
	CreatedAt    time.Time         `bson:"createdat"`
	ExpiresAt    time.Time         `bson:"expiresat"`
	WalletPaidBy string            `bson:"walletpaidby"`
}

// convertMongoRefundQuote converts a MongoDB refund quote document to RefundQuote struct.
// Extracts money.Money from nested BSON document: {asset: {code: "USDC", ...}, atomic: 123}
func convertMongoRefundQuote(mongoQuote mongoRefundQuote, refundID string) (RefundQuote, error) {
	// Extract amount from nested document (lowercase fields)
	atomic, ok := mongoQuote.Amount["atomic"].(int64)
	if !ok {
		return RefundQuote{}, fmt.Errorf("refund %s: invalid amount.atomic type", refundID)
	}

	assetDoc, ok := mongoQuote.Amount["asset"].(bson.M)
	if !ok {
		return RefundQuote{}, fmt.Errorf("refund %s: invalid amount.asset type", refundID)
	}

	assetCode, ok := assetDoc["code"].(string)
	if !ok {
		return RefundQuote{}, fmt.Errorf("refund %s: invalid amount.asset.code type", refundID)
	}

	asset, err := money.GetAsset(assetCode)
	if err != nil {
		return RefundQuote{}, fmt.Errorf("refund %s: invalid asset %q: %w", refundID, assetCode, err)
	}

	return RefundQuote{
		ID:                 mongoQuote.ID,
		OriginalPurchaseID: mongoQuote.OriginalPurchaseID,
		RecipientWallet:    mongoQuote.RecipientWallet,
		Amount:             money.Money{Asset: asset, Atomic: atomic},
		Reason:             mongoQuote.Reason,
		Metadata:           mongoQuote.Metadata,
		CreatedAt:          mongoQuote.CreatedAt,
		ExpiresAt:          mongoQuote.ExpiresAt,
		ProcessedBy:        mongoQuote.ProcessedBy,
		ProcessedAt:        mongoQuote.ProcessedAt,
	}, nil
}

// convertMongoCartQuote converts a MongoDB cart quote document to CartQuote struct.
// Extracts money.Money from nested BSON documents: {asset: {code: "USDC", ...}, atomic: 123}
func convertMongoCartQuote(mongoQuote mongoCartQuote, cartID string) (CartQuote, error) {
	// Extract total money from nested document (lowercase fields)
	totalAtomic, ok := mongoQuote.Total["atomic"].(int64)
	if !ok {
		return CartQuote{}, fmt.Errorf("cart %s: invalid total.atomic type", cartID)
	}

	totalAssetDoc, ok := mongoQuote.Total["asset"].(bson.M)
	if !ok {
		return CartQuote{}, fmt.Errorf("cart %s: invalid total.asset type", cartID)
	}

	totalAssetCode, ok := totalAssetDoc["code"].(string)
	if !ok {
		return CartQuote{}, fmt.Errorf("cart %s: invalid total.asset.code type", cartID)
	}

	totalAsset, err := money.GetAsset(totalAssetCode)
	if err != nil {
		return CartQuote{}, fmt.Errorf("cart %s: invalid total asset %q: %w", cartID, totalAssetCode, err)
	}

	// Convert cart items
	items := make([]CartItem, len(mongoQuote.Items))
	for i, mongoItem := range mongoQuote.Items {
		// Extract price money from nested document (lowercase fields)
		priceAtomic, ok := mongoItem.Price["atomic"].(int64)
		if !ok {
			return CartQuote{}, fmt.Errorf("cart %s item %d: invalid price.atomic type", cartID, i)
		}

		priceAssetDoc, ok := mongoItem.Price["asset"].(bson.M)
		if !ok {
			return CartQuote{}, fmt.Errorf("cart %s item %d: invalid price.asset type", cartID, i)
		}

		priceAssetCode, ok := priceAssetDoc["code"].(string)
		if !ok {
			return CartQuote{}, fmt.Errorf("cart %s item %d: invalid price.asset.code type", cartID, i)
		}

		priceAsset, err := money.GetAsset(priceAssetCode)
		if err != nil {
			return CartQuote{}, fmt.Errorf("cart %s item %d: invalid price asset %q: %w", cartID, i, priceAssetCode, err)
		}

		items[i] = CartItem{
			ResourceID: mongoItem.ResourceID,
			Quantity:   mongoItem.Quantity,
			Price:      money.Money{Asset: priceAsset, Atomic: priceAtomic},
			Metadata:   mongoItem.Metadata,
		}
	}

	return CartQuote{
		ID:           mongoQuote.ID,
		Items:        items,
		Total:        money.Money{Asset: totalAsset, Atomic: totalAtomic},
		Metadata:     mongoQuote.Metadata,
		CreatedAt:    mongoQuote.CreatedAt,
		ExpiresAt:    mongoQuote.ExpiresAt,
		WalletPaidBy: mongoQuote.WalletPaidBy,
	}, nil
}

// GetPayment retrieves a payment transaction by signature.
// Returns the original payment record showing which resource it was used for.
func (s *MongoDBStore) GetPayment(ctx context.Context, signature string) (PaymentTransaction, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	filter := bson.M{"signature": signature}

	var mongoTx mongoPaymentTransaction
	err := s.paymentTransactions.FindOne(ctx, filter).Decode(&mongoTx)
	if err == mongo.ErrNoDocuments {
		return PaymentTransaction{}, ErrNotFound
	}
	if err != nil {
		return PaymentTransaction{}, fmt.Errorf("query payment: %w", err)
	}

	// Convert MongoDB document to PaymentTransaction
	asset, err := money.GetAsset(mongoTx.Asset)
	if err != nil {
		return PaymentTransaction{}, fmt.Errorf("invalid asset %q: %w", mongoTx.Asset, err)
	}

	tx := PaymentTransaction{
		Signature:  mongoTx.Signature,
		ResourceID: mongoTx.ResourceID,
		Wallet:     mongoTx.Wallet,
		Amount:     money.Money{Asset: asset, Atomic: mongoTx.Amount},
		CreatedAt:  mongoTx.CreatedAt,
		Metadata:   mongoTx.Metadata,
	}

	return tx, nil
}

// Close closes the database connection.
func (s *MongoDBStore) Close() error {
	close(s.stopCleanup)
	<-s.cleanupDone

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.client.Disconnect(ctx)
}

// CreateNonce stores a new admin nonce for replay protection.
func (s *MongoDBStore) CreateNonce(ctx context.Context, nonce AdminNonce) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	_, err := s.paymentTransactions.Database().Collection("admin_nonces").InsertOne(ctx, bson.M{
		"_id":         nonce.ID,
		"purpose":     nonce.Purpose,
		"created_at":  nonce.CreatedAt,
		"expires_at":  nonce.ExpiresAt,
		"consumed_at": nonce.ConsumedAt,
	})
	return err
}

// ConsumeNonce marks a nonce as consumed (one-time use).
func (s *MongoDBStore) ConsumeNonce(ctx context.Context, nonceID string) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	now := time.Now()
	filter := bson.M{
		"_id":         nonceID,
		"consumed_at": nil,
		"expires_at":  bson.M{"$gt": now},
	}
	update := bson.M{"$set": bson.M{"consumed_at": now}}

	result, err := s.paymentTransactions.Database().Collection("admin_nonces").UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		// Check why
		var doc bson.M
		err := s.paymentTransactions.Database().Collection("admin_nonces").FindOne(ctx, bson.M{"_id": nonceID}).Decode(&doc)
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("nonce not found: %s", nonceID)
		}
		if doc["consumed_at"] != nil {
			return fmt.Errorf("nonce already consumed: %s", nonceID)
		}
		if expiresAt, ok := doc["expires_at"].(time.Time); ok && now.After(expiresAt) {
			return fmt.Errorf("nonce expired: %s", nonceID)
		}
		return fmt.Errorf("failed to consume nonce: %s", nonceID)
	}

	return nil
}

// ArchiveOldPayments deletes payment transactions older than the specified time.
// This prevents unbounded growth of the payment_transactions collection while maintaining
// replay protection for recent transactions (e.g., last 90 days).
//
// Returns the number of archived (deleted) records.
func (s *MongoDBStore) ArchiveOldPayments(ctx context.Context, olderThan time.Time) (int64, error) {
	filter := bson.M{"created_at": bson.M{"$lt": olderThan}}

	result, err := s.paymentTransactions.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("archive old payments: %w", err)
	}

	return result.DeletedCount, nil
}

// CleanupExpiredNonces deletes expired admin nonces from the database.
// This prevents unbounded growth of the admin_nonces collection.
//
// Returns the number of deleted nonces.
func (s *MongoDBStore) CleanupExpiredNonces(ctx context.Context) (int64, error) {
	now := time.Now()
	filter := bson.M{"expires_at": bson.M{"$lt": now}}

	result, err := s.paymentTransactions.Database().Collection("admin_nonces").DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired nonces: %w", err)
	}

	return result.DeletedCount, nil
}

// Batch operations (optimized with MongoDB bulk operations)

// SaveCartQuotes stores multiple cart quotes using MongoDB bulk operations.
func (s *MongoDBStore) SaveCartQuotes(ctx context.Context, quotes []CartQuote) error {
	if len(quotes) == 0 {
		return nil
	}

	// Validate all quotes first
	for i := range quotes {
		if err := validateAndPrepareCartQuote(&quotes[i], 0); err != nil {
			return fmt.Errorf("quote %d: %w", i, err)
		}
	}

	// Use bulk write for better performance (single round-trip to MongoDB)
	var operations []mongo.WriteModel
	for _, quote := range quotes {
		// Upsert operation (uses struct with bson tags for proper field mapping)
		operations = append(operations, mongo.NewReplaceOneModel().
			SetFilter(bson.M{"_id": quote.ID}).
			SetReplacement(quote).
			SetUpsert(true))
	}

	_, err := s.cartQuotes.BulkWrite(ctx, operations, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return fmt.Errorf("mongodb: bulk save cart quotes: %w", err)
	}

	return nil
}

// GetCartQuotes retrieves multiple cart quotes using MongoDB $in operator (single query).
func (s *MongoDBStore) GetCartQuotes(ctx context.Context, cartIDs []string) ([]CartQuote, error) {
	if len(cartIDs) == 0 {
		return []CartQuote{}, nil
	}

	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	now := time.Now()

	// Single query with $in operator for all cart IDs
	filter := bson.M{
		"_id":        bson.M{"$in": cartIDs},
		"expires_at": bson.M{"$gt": now}, // Exclude expired quotes
	}

	cursor, err := s.cartQuotes.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("mongodb: bulk get cart quotes: %w", err)
	}
	defer cursor.Close(ctx)

	var quotes []CartQuote
	for cursor.Next(ctx) {
		// Decode into intermediate struct first (MongoDB stores money.Money as nested documents)
		var mongoQuote mongoCartQuote
		if err := cursor.Decode(&mongoQuote); err != nil {
			return nil, fmt.Errorf("mongodb: decode cart quote: %w", err)
		}

		// Convert MongoDB document to CartQuote
		quote, err := convertMongoCartQuote(mongoQuote, mongoQuote.ID)
		if err != nil {
			return nil, fmt.Errorf("mongodb: convert cart quote %s: %w", mongoQuote.ID, err)
		}

		quotes = append(quotes, quote)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("mongodb: cursor error: %w", err)
	}

	return quotes, nil
}

// SaveRefundQuotes stores multiple refund quotes using MongoDB bulk operations.
func (s *MongoDBStore) SaveRefundQuotes(ctx context.Context, quotes []RefundQuote) error {
	if len(quotes) == 0 {
		return nil
	}

	// Validate all quotes first
	for i := range quotes {
		if err := validateAndPrepareRefundQuote(&quotes[i], 0); err != nil {
			return fmt.Errorf("quote %d: %w", i, err)
		}
	}

	// Use bulk write for better performance
	var operations []mongo.WriteModel
	for _, quote := range quotes {
		// Store as-is with nested money.Money (maintains compatibility with existing data)
		operations = append(operations, mongo.NewReplaceOneModel().
			SetFilter(bson.M{"_id": quote.ID}).
			SetReplacement(quote).
			SetUpsert(true))
	}

	_, err := s.refundQuotes.BulkWrite(ctx, operations, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return fmt.Errorf("mongodb: bulk save refund quotes: %w", err)
	}

	return nil
}

// RecordPayments saves multiple payment transactions using MongoDB bulk operations.
// Note: Uses ordered=true to maintain atomic failure on duplicate signatures.
func (s *MongoDBStore) RecordPayments(ctx context.Context, txs []PaymentTransaction) error {
	if len(txs) == 0 {
		return nil
	}

	// Use bulk write with ordered=true for atomic failure on duplicates
	var operations []mongo.WriteModel
	for _, tx := range txs {
		// Insert operation (uses struct with bson tags, fails on duplicate signature)
		operations = append(operations, mongo.NewInsertOneModel().SetDocument(tx))
	}

	_, err := s.paymentTransactions.BulkWrite(ctx, operations, options.BulkWrite().SetOrdered(true))
	if err != nil {
		// Check for duplicate key error (signature already used)
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("storage: signature already used")
		}
		return fmt.Errorf("mongodb: bulk record payments: %w", err)
	}

	return nil
}
