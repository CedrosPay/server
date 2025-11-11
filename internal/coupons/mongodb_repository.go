package coupons

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBRepository implements Repository using MongoDB.
type MongoDBRepository struct {
	client     *mongo.Client
	collection *mongo.Collection
}

// mongoCoupon represents the MongoDB document structure.
type mongoCoupon struct {
	Code          string            `bson:"_id"`
	DiscountType  string            `bson:"discountType"`
	DiscountValue float64           `bson:"discountValue"`
	Currency      string            `bson:"currency"`
	Scope         string            `bson:"scope"`
	ProductIDs    []string          `bson:"productIds"`
	PaymentMethod string            `bson:"paymentMethod,omitempty"` // "stripe", "x402", or "" for any
	AutoApply     bool              `bson:"autoApply"`
	AppliesAt     string            `bson:"appliesAt,omitempty"` // "catalog", "checkout", or "" for backward compatibility
	UsageLimit    *int              `bson:"usageLimit"`
	UsageCount    int               `bson:"usageCount"`
	StartsAt      *time.Time        `bson:"startsAt,omitempty"`
	ExpiresAt     *time.Time        `bson:"expiresAt,omitempty"`
	Active        bool              `bson:"active"`
	Metadata      map[string]string `bson:"metadata"`
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
			Keys: bson.D{{Key: "expiresAt", Value: 1}},
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

// GetCoupon retrieves a coupon by code.
func (r *MongoDBRepository) GetCoupon(ctx context.Context, code string) (Coupon, error) {
	filter := bson.M{"_id": code, "active": true}

	var mc mongoCoupon
	err := r.collection.FindOne(ctx, filter).Decode(&mc)
	if err == mongo.ErrNoDocuments {
		return Coupon{}, ErrCouponNotFound
	}
	if err != nil {
		return Coupon{}, fmt.Errorf("find coupon: %w", err)
	}

	return mongoToCoupon(mc), nil
}

// ListCoupons returns all active coupons.
func (r *MongoDBRepository) ListCoupons(ctx context.Context) ([]Coupon, error) {
	filter := bson.M{"active": true}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find coupons: %w", err)
	}
	defer cursor.Close(ctx)

	var coupons []Coupon
	for cursor.Next(ctx) {
		var mc mongoCoupon
		if err := cursor.Decode(&mc); err != nil {
			return nil, fmt.Errorf("decode coupon: %w", err)
		}
		coupons = append(coupons, mongoToCoupon(mc))
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate coupons: %w", err)
	}

	return coupons, nil
}

// GetAutoApplyCouponsForPayment returns auto-apply coupons filtered by payment method.
// Optimized to filter by payment method at database level.
func (r *MongoDBRepository) GetAutoApplyCouponsForPayment(ctx context.Context, productID string, paymentMethod PaymentMethod) ([]Coupon, error) {
	now := time.Now()

	// Build MongoDB filter with proper $and array for multiple $or conditions
	filter := bson.M{
		"$and": []bson.M{
			{"autoApply": true},
			{"active": true},
			{
				"$or": []bson.M{
					{"startsAt": bson.M{"$exists": false}},
					{"startsAt": nil},
					{"startsAt": bson.M{"$lte": now}},
				},
			},
			{
				"$or": []bson.M{
					{"expiresAt": bson.M{"$exists": false}},
					{"expiresAt": nil},
					{"expiresAt": bson.M{"$gt": now}},
				},
			},
			{
				"$or": []bson.M{
					{"scope": "all"},
					{"productIds": productID},
				},
			},
			{
				// Payment method filter: "" (any) OR matches specified method
				"$or": []bson.M{
					{"paymentMethod": ""},
					{"paymentMethod": string(paymentMethod)},
				},
			},
		},
		// Usage limit check with $expr for field comparison
		"$expr": bson.M{
			"$or": []interface{}{
				bson.M{"$eq": []interface{}{"$usageLimit", nil}},
				bson.M{"$lt": []interface{}{"$usageCount", "$usageLimit"}},
			},
		},
	}

	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find auto-apply coupons: %w", err)
	}
	defer cursor.Close(ctx)

	var coupons []Coupon
	for cursor.Next(ctx) {
		var mc mongoCoupon
		if err := cursor.Decode(&mc); err != nil {
			return nil, fmt.Errorf("decode coupon: %w", err)
		}
		coupons = append(coupons, mongoToCoupon(mc))
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate coupons: %w", err)
	}

	return coupons, nil
}

// GetAllAutoApplyCouponsForPayment returns all auto-apply coupons grouped by product ID.
func (r *MongoDBRepository) GetAllAutoApplyCouponsForPayment(ctx context.Context, paymentMethod PaymentMethod) (map[string][]Coupon, error) {
	now := time.Now()

	// Build MongoDB filter for all auto-apply coupons for the payment method
	filter := bson.M{
		"$and": []bson.M{
			{"autoApply": true},
			{"active": true},
			{
				"$or": []bson.M{
					{"startsAt": bson.M{"$exists": false}},
					{"startsAt": nil},
					{"startsAt": bson.M{"$lte": now}},
				},
			},
			{
				"$or": []bson.M{
					{"expiresAt": bson.M{"$exists": false}},
					{"expiresAt": nil},
					{"expiresAt": bson.M{"$gt": now}},
				},
			},
			{
				// Payment method filter: "" (any) OR matches specified method
				"$or": []bson.M{
					{"paymentMethod": ""},
					{"paymentMethod": string(paymentMethod)},
				},
			},
		},
		// Usage limit check with $expr for field comparison
		"$expr": bson.M{
			"$or": []interface{}{
				bson.M{"$eq": []interface{}{"$usageLimit", nil}},
				bson.M{"$lt": []interface{}{"$usageCount", "$usageLimit"}},
			},
		},
	}

	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find all auto-apply coupons: %w", err)
	}
	defer cursor.Close(ctx)

	result := make(map[string][]Coupon)

	for cursor.Next(ctx) {
		var mc mongoCoupon
		if err := cursor.Decode(&mc); err != nil {
			return nil, fmt.Errorf("decode coupon: %w", err)
		}

		coupon := mongoToCoupon(mc)

		// Group by product IDs
		if coupon.Scope == ScopeAll {
			// For "all" scope coupons, store under special key
			result["*"] = append(result["*"], coupon)
		} else {
			// For specific products, add to each product ID
			for _, productID := range coupon.ProductIDs {
				result[productID] = append(result[productID], coupon)
			}
		}
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate coupons: %w", err)
	}

	return result, nil
}

// CreateCoupon creates a new coupon.
func (r *MongoDBRepository) CreateCoupon(ctx context.Context, c Coupon) error {
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()

	mc := couponToMongo(c)
	_, err := r.collection.InsertOne(ctx, mc)
	if err != nil {
		// Check if this is a duplicate key error
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("coupon already exists: %s", c.Code)
		}
		return fmt.Errorf("insert coupon: %w", err)
	}

	return nil
}

// UpdateCoupon updates an existing coupon.
func (r *MongoDBRepository) UpdateCoupon(ctx context.Context, c Coupon) error {
	c.UpdatedAt = time.Now()

	filter := bson.M{"_id": c.Code}
	update := bson.M{
		"$set": bson.M{
			"discountType":  string(c.DiscountType),
			"discountValue": c.DiscountValue,
			"currency":      c.Currency,
			"scope":         string(c.Scope),
			"productIds":    c.ProductIDs,
			"paymentMethod": string(c.PaymentMethod),
			"autoApply":     c.AutoApply,
			"usageLimit":    c.UsageLimit,
			"usageCount":    c.UsageCount,
			"startsAt":      c.StartsAt,
			"expiresAt":     c.ExpiresAt,
			"active":        c.Active,
			"metadata":      c.Metadata,
			"updatedAt":     c.UpdatedAt,
		},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("update coupon: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrCouponNotFound
	}

	return nil
}

// IncrementUsage atomically increments the usage count.
func (r *MongoDBRepository) IncrementUsage(ctx context.Context, code string) error {
	filter := bson.M{"_id": code, "active": true}
	update := bson.M{
		"$inc": bson.M{"usageCount": 1},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("increment usage: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrCouponNotFound
	}

	return nil
}

// DeleteCoupon soft-deletes a coupon (sets active = false).
func (r *MongoDBRepository) DeleteCoupon(ctx context.Context, code string) error {
	filter := bson.M{"_id": code}
	update := bson.M{
		"$set": bson.M{
			"active":    false,
			"updatedAt": time.Now(),
		},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("delete coupon: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrCouponNotFound
	}

	return nil
}

// Close closes the MongoDB connection.
func (r *MongoDBRepository) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return r.client.Disconnect(ctx)
}

// mongoToCoupon converts a MongoDB document to a Coupon.
func mongoToCoupon(mc mongoCoupon) Coupon {
	return Coupon{
		Code:          mc.Code,
		DiscountType:  DiscountType(mc.DiscountType),
		DiscountValue: mc.DiscountValue,
		Currency:      mc.Currency,
		Scope:         Scope(mc.Scope),
		ProductIDs:    mc.ProductIDs,
		PaymentMethod: PaymentMethod(mc.PaymentMethod),
		AutoApply:     mc.AutoApply,
		AppliesAt:     AppliesAt(mc.AppliesAt),
		UsageLimit:    mc.UsageLimit,
		UsageCount:    mc.UsageCount,
		StartsAt:      mc.StartsAt,
		ExpiresAt:     mc.ExpiresAt,
		Active:        mc.Active,
		Metadata:      mc.Metadata,
		CreatedAt:     mc.CreatedAt,
		UpdatedAt:     mc.UpdatedAt,
	}
}

// couponToMongo converts a Coupon to a MongoDB document.
func couponToMongo(c Coupon) mongoCoupon {
	return mongoCoupon{
		Code:          c.Code,
		DiscountType:  string(c.DiscountType),
		DiscountValue: c.DiscountValue,
		Currency:      c.Currency,
		Scope:         string(c.Scope),
		ProductIDs:    c.ProductIDs,
		PaymentMethod: string(c.PaymentMethod),
		AutoApply:     c.AutoApply,
		AppliesAt:     string(c.AppliesAt),
		UsageLimit:    c.UsageLimit,
		UsageCount:    c.UsageCount,
		StartsAt:      c.StartsAt,
		ExpiresAt:     c.ExpiresAt,
		Active:        c.Active,
		Metadata:      c.Metadata,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}
