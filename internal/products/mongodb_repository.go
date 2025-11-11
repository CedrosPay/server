package products

import (
	"context"
	"fmt"
	"time"

	"github.com/CedrosPay/server/internal/money"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBRepository implements Repository using MongoDB.
type MongoDBRepository struct {
	client     *mongo.Client
	collection *mongo.Collection
}

// mongoProduct represents the MongoDB document structure.
type mongoProduct struct {
	ID            string            `bson:"_id"`
	Description   string            `bson:"description"`
	FiatAtomic    *int64            `bson:"fiatAtomic,omitempty"`
	FiatAsset     *string           `bson:"fiatAsset,omitempty"`
	StripePriceID string            `bson:"stripePriceId"`
	CryptoAtomic  *int64            `bson:"cryptoAtomic,omitempty"`
	CryptoAsset   *string           `bson:"cryptoAsset,omitempty"`
	CryptoAccount string            `bson:"cryptoAccount"`
	MemoTemplate  string            `bson:"memoTemplate"`
	Metadata      map[string]string `bson:"metadata"`
	Active        bool              `bson:"active"`
	CreatedAt     time.Time         `bson:"createdAt"`
	UpdatedAt     time.Time         `bson:"updatedAt"`
}

// NewMongoDBRepository creates a MongoDB-backed repository.
func NewMongoDBRepository(connectionString, database, collection string) (*MongoDBRepository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(connectionString))
	if err != nil {
		return nil, fmt.Errorf("connect to mongodb: %w", err)
	}

	// Test connection
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(ctx)
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}

	// Get collection
	coll := client.Database(database).Collection(collection)

	// Create indexes
	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "active", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "stripePriceId", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "createdAt", Value: -1}},
		},
	}

	if _, err := coll.Indexes().CreateMany(ctx, indexModels); err != nil {
		client.Disconnect(ctx)
		return nil, fmt.Errorf("create indexes: %w", err)
	}

	return &MongoDBRepository{
		client:     client,
		collection: coll,
	}, nil
}

// GetProduct retrieves a product by ID.
func (r *MongoDBRepository) GetProduct(ctx context.Context, id string) (Product, error) {
	filter := bson.M{"_id": id, "active": true}

	var mp mongoProduct
	err := r.collection.FindOne(ctx, filter).Decode(&mp)
	if err == mongo.ErrNoDocuments {
		return Product{}, ErrProductNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("find product: %w", err)
	}

	return mongoToProduct(mp), nil
}

// GetProductByStripePriceID retrieves a product by its Stripe Price ID.
func (r *MongoDBRepository) GetProductByStripePriceID(ctx context.Context, stripePriceID string) (Product, error) {
	filter := bson.M{"stripePriceId": stripePriceID, "active": true}

	var mp mongoProduct
	err := r.collection.FindOne(ctx, filter).Decode(&mp)
	if err == mongo.ErrNoDocuments {
		return Product{}, ErrProductNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("find product by stripe price id: %w", err)
	}

	return mongoToProduct(mp), nil
}

// ListProducts returns all active products.
func (r *MongoDBRepository) ListProducts(ctx context.Context) ([]Product, error) {
	filter := bson.M{"active": true}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find products: %w", err)
	}
	defer cursor.Close(ctx)

	var products []Product
	for cursor.Next(ctx) {
		var mp mongoProduct
		if err := cursor.Decode(&mp); err != nil {
			return nil, fmt.Errorf("decode product: %w", err)
		}
		products = append(products, mongoToProduct(mp))
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}

	return products, nil
}

// CreateProduct creates a new product.
func (r *MongoDBRepository) CreateProduct(ctx context.Context, p Product) error {
	// Set defaults
	if p.MemoTemplate == "" {
		p.MemoTemplate = "{{resource}}:{{nonce}}"
	}
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()

	mp := productToMongo(p)
	_, err := r.collection.InsertOne(ctx, mp)
	if err != nil {
		// Check if this is a duplicate key error
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("product already exists: %s", p.ID)
		}
		return fmt.Errorf("insert product: %w", err)
	}

	return nil
}

// UpdateProduct updates an existing product.
func (r *MongoDBRepository) UpdateProduct(ctx context.Context, p Product) error {
	p.UpdatedAt = time.Now()

	// Extract pricing fields
	var fiatAtomic *int64
	var fiatAsset *string
	if p.FiatPrice != nil {
		atomic := p.FiatPrice.Atomic
		asset := p.FiatPrice.Asset.Code
		fiatAtomic = &atomic
		fiatAsset = &asset
	}

	var cryptoAtomic *int64
	var cryptoAsset *string
	if p.CryptoPrice != nil {
		atomic := p.CryptoPrice.Atomic
		asset := p.CryptoPrice.Asset.Code
		cryptoAtomic = &atomic
		cryptoAsset = &asset
	}

	filter := bson.M{"_id": p.ID}
	update := bson.M{
		"$set": bson.M{
			"description":   p.Description,
			"fiatAtomic":    fiatAtomic,
			"fiatAsset":     fiatAsset,
			"stripePriceId": p.StripePriceID,
			"cryptoAtomic":  cryptoAtomic,
			"cryptoAsset":   cryptoAsset,
			"cryptoAccount": p.CryptoAccount,
			"memoTemplate":  p.MemoTemplate,
			"metadata":      p.Metadata,
			"active":        p.Active,
			"updatedAt":     p.UpdatedAt,
		},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("update product: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrProductNotFound
	}

	return nil
}

// DeleteProduct soft-deletes a product (sets active = false).
func (r *MongoDBRepository) DeleteProduct(ctx context.Context, id string) error {
	filter := bson.M{"_id": id}
	update := bson.M{
		"$set": bson.M{
			"active":    false,
			"updatedAt": time.Now(),
		},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrProductNotFound
	}

	return nil
}

// Close closes the MongoDB connection.
func (r *MongoDBRepository) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return r.client.Disconnect(ctx)
}

// mongoToProduct converts a MongoDB document to a Product.
func mongoToProduct(mp mongoProduct) Product {
	p := Product{
		ID:            mp.ID,
		Description:   mp.Description,
		StripePriceID: mp.StripePriceID,
		CryptoAccount: mp.CryptoAccount,
		MemoTemplate:  mp.MemoTemplate,
		Metadata:      mp.Metadata,
		Active:        mp.Active,
		CreatedAt:     mp.CreatedAt,
		UpdatedAt:     mp.UpdatedAt,
	}

	// Reconstruct FiatPrice from document fields
	if mp.FiatAtomic != nil && mp.FiatAsset != nil {
		if asset, err := money.GetAsset(*mp.FiatAsset); err == nil {
			price := money.New(asset, *mp.FiatAtomic)
			p.FiatPrice = &price
		}
	}

	// Reconstruct CryptoPrice from document fields
	if mp.CryptoAtomic != nil && mp.CryptoAsset != nil {
		if asset, err := money.GetAsset(*mp.CryptoAsset); err == nil {
			price := money.New(asset, *mp.CryptoAtomic)
			p.CryptoPrice = &price
		}
	}

	return p
}

// productToMongo converts a Product to a MongoDB document.
func productToMongo(p Product) mongoProduct {
	mp := mongoProduct{
		ID:            p.ID,
		Description:   p.Description,
		StripePriceID: p.StripePriceID,
		CryptoAccount: p.CryptoAccount,
		MemoTemplate:  p.MemoTemplate,
		Metadata:      p.Metadata,
		Active:        p.Active,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}

	// Extract fiat pricing fields
	if p.FiatPrice != nil {
		atomic := p.FiatPrice.Atomic
		asset := p.FiatPrice.Asset.Code
		mp.FiatAtomic = &atomic
		mp.FiatAsset = &asset
	}

	// Extract crypto pricing fields
	if p.CryptoPrice != nil {
		atomic := p.CryptoPrice.Atomic
		asset := p.CryptoPrice.Asset.Code
		mp.CryptoAtomic = &atomic
		mp.CryptoAsset = &asset
	}

	return mp
}
