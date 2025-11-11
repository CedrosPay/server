package storage

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const webhookQueueCollection = "webhook_queue"

// EnqueueWebhook adds a webhook to the delivery queue.
func (s *MongoDBStore) EnqueueWebhook(ctx context.Context, webhook PendingWebhook) (string, error) {
	// Generate webhook ID if not provided
	if webhook.ID == "" {
		webhook.ID = generateWebhookID()
	}

	// Set defaults
	if webhook.Status == "" {
		webhook.Status = WebhookStatusPending
	}
	if webhook.CreatedAt.IsZero() {
		webhook.CreatedAt = time.Now().UTC()
	}
	if webhook.NextAttemptAt.IsZero() {
		webhook.NextAttemptAt = time.Now().UTC()
	}
	if webhook.MaxAttempts == 0 {
		webhook.MaxAttempts = 5 // Default from retry config
	}

	coll := s.db.Collection(webhookQueueCollection)
	_, err := coll.InsertOne(ctx, webhook)
	if err != nil {
		return "", fmt.Errorf("insert webhook: %w", err)
	}

	return webhook.ID, nil
}

// DequeueWebhooks retrieves webhooks ready for delivery.
func (s *MongoDBStore) DequeueWebhooks(ctx context.Context, limit int) ([]PendingWebhook, error) {
	coll := s.db.Collection(webhookQueueCollection)

	filter := bson.M{
		"status": WebhookStatusPending,
		"nextattemptat": bson.M{"$lte": time.Now().UTC()},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "nextattemptat", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("query webhooks: %w", err)
	}
	defer cursor.Close(ctx)

	var webhooks []PendingWebhook
	if err := cursor.All(ctx, &webhooks); err != nil {
		return nil, fmt.Errorf("decode webhooks: %w", err)
	}

	return webhooks, nil
}

// MarkWebhookProcessing updates webhook status to prevent duplicate processing.
func (s *MongoDBStore) MarkWebhookProcessing(ctx context.Context, webhookID string) error {
	coll := s.db.Collection(webhookQueueCollection)

	filter := bson.M{"id": webhookID}
	update := bson.M{
		"$set": bson.M{
			"status":         WebhookStatusProcessing,
			"lastattemptat":  time.Now().UTC(),
		},
		"$inc": bson.M{"attempts": 1},
	}

	result, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}

	return nil
}

// MarkWebhookSuccess marks webhook as successfully delivered and removes from queue.
func (s *MongoDBStore) MarkWebhookSuccess(ctx context.Context, webhookID string) error {
	coll := s.db.Collection(webhookQueueCollection)

	filter := bson.M{"id": webhookID}
	result, err := coll.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}

	return nil
}

// MarkWebhookFailed records failed attempt and schedules retry (or moves to DLQ if exhausted).
func (s *MongoDBStore) MarkWebhookFailed(ctx context.Context, webhookID string, errorMsg string, nextAttemptAt time.Time) error {
	coll := s.db.Collection(webhookQueueCollection)

	// First, get current webhook to check attempt count
	var webhook PendingWebhook
	filter := bson.M{"id": webhookID}
	err := coll.FindOne(ctx, filter).Decode(&webhook)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return ErrNotFound
		}
		return fmt.Errorf("query webhook: %w", err)
	}

	var update bson.M
	if webhook.Attempts >= webhook.MaxAttempts {
		// Exhausted all retries - mark as permanently failed
		now := time.Now().UTC()
		update = bson.M{
			"$set": bson.M{
				"status":        WebhookStatusFailed,
				"lasterror":     errorMsg,
				"lastattemptat": now,
				"completedat":   now,
			},
		}
	} else {
		// Schedule retry
		update = bson.M{
			"$set": bson.M{
				"status":         WebhookStatusPending,
				"lasterror":      errorMsg,
				"lastattemptat":  time.Now().UTC(),
				"nextattemptat":  nextAttemptAt,
			},
		}
	}

	result, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}

	return nil
}

// GetWebhook retrieves a webhook by ID (for admin UI).
func (s *MongoDBStore) GetWebhook(ctx context.Context, webhookID string) (PendingWebhook, error) {
	coll := s.db.Collection(webhookQueueCollection)

	var webhook PendingWebhook
	filter := bson.M{"id": webhookID}
	err := coll.FindOne(ctx, filter).Decode(&webhook)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return PendingWebhook{}, ErrNotFound
		}
		return PendingWebhook{}, fmt.Errorf("query webhook: %w", err)
	}

	return webhook, nil
}

// ListWebhooks lists webhooks with optional status filter (for admin UI).
func (s *MongoDBStore) ListWebhooks(ctx context.Context, status WebhookStatus, limit int) ([]PendingWebhook, error) {
	coll := s.db.Collection(webhookQueueCollection)

	filter := bson.M{}
	if status != "" {
		filter["status"] = status
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdat", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("query webhooks: %w", err)
	}
	defer cursor.Close(ctx)

	var webhooks []PendingWebhook
	if err := cursor.All(ctx, &webhooks); err != nil {
		return nil, fmt.Errorf("decode webhooks: %w", err)
	}

	return webhooks, nil
}

// RetryWebhook resets webhook to pending state for manual retry (admin operation).
func (s *MongoDBStore) RetryWebhook(ctx context.Context, webhookID string) error {
	coll := s.db.Collection(webhookQueueCollection)

	filter := bson.M{"id": webhookID}
	update := bson.M{
		"$set": bson.M{
			"status":         WebhookStatusPending,
			"nextattemptat":  time.Now().UTC(),
			"lasterror":      "",
		},
	}

	result, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteWebhook removes webhook from queue (admin operation).
func (s *MongoDBStore) DeleteWebhook(ctx context.Context, webhookID string) error {
	coll := s.db.Collection(webhookQueueCollection)

	filter := bson.M{"id": webhookID}
	result, err := coll.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}

	return nil
}
