package storage

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestMemoryStore_WebhookQueue(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Create test webhook
	webhook := PendingWebhook{
		URL:           "https://example.com/webhook",
		Payload:       json.RawMessage(`{"event":"payment.succeeded"}`),
		Headers:       map[string]string{"Content-Type": "application/json"},
		EventType:     "payment",
		Status:        WebhookStatusPending,
		Attempts:      0,
		MaxAttempts:   5,
		NextAttemptAt: time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	// Test EnqueueWebhook
	webhookID, err := store.EnqueueWebhook(ctx, webhook)
	if err != nil {
		t.Fatalf("EnqueueWebhook failed: %v", err)
	}
	if webhookID == "" {
		t.Fatal("Expected webhook ID, got empty string")
	}

	// Test GetWebhook
	retrieved, err := store.GetWebhook(ctx, webhookID)
	if err != nil {
		t.Fatalf("GetWebhook failed: %v", err)
	}
	if retrieved.URL != webhook.URL {
		t.Errorf("Expected URL %s, got %s", webhook.URL, retrieved.URL)
	}

	// Test DequeueWebhooks
	webhooks, err := store.DequeueWebhooks(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueWebhooks failed: %v", err)
	}
	if len(webhooks) != 1 {
		t.Fatalf("Expected 1 webhook, got %d", len(webhooks))
	}

	// Test MarkWebhookProcessing
	if err := store.MarkWebhookProcessing(ctx, webhookID); err != nil {
		t.Fatalf("MarkWebhookProcessing failed: %v", err)
	}

	// Test MarkWebhookSuccess
	if err := store.MarkWebhookSuccess(ctx, webhookID); err != nil {
		t.Fatalf("MarkWebhookSuccess failed: %v", err)
	}

	// Webhook should be removed
	_, err = store.GetWebhook(ctx, webhookID)
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound after success, got %v", err)
	}
}

func TestMemoryStore_WebhookQueueRetry(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	webhook := PendingWebhook{
		URL:           "https://example.com/webhook",
		Payload:       json.RawMessage(`{}`),
		EventType:     "payment",
		Status:        WebhookStatusPending,
		Attempts:      0,
		MaxAttempts:   3,
		NextAttemptAt: time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	webhookID, err := store.EnqueueWebhook(ctx, webhook)
	if err != nil {
		t.Fatalf("EnqueueWebhook failed: %v", err)
	}

	// Mark as processing (increments attempts)
	if err := store.MarkWebhookProcessing(ctx, webhookID); err != nil {
		t.Fatalf("MarkWebhookProcessing failed: %v", err)
	}

	// Mark as failed (schedule retry)
	nextAttempt := time.Now().Add(5 * time.Second)
	if err := store.MarkWebhookFailed(ctx, webhookID, "connection timeout", nextAttempt); err != nil {
		t.Fatalf("MarkWebhookFailed failed: %v", err)
	}

	// Check webhook status
	retrieved, err := store.GetWebhook(ctx, webhookID)
	if err != nil {
		t.Fatalf("GetWebhook failed: %v", err)
	}
	if retrieved.Status != WebhookStatusPending {
		t.Errorf("Expected status pending, got %s", retrieved.Status)
	}
	if retrieved.LastError != "connection timeout" {
		t.Errorf("Expected error 'connection timeout', got '%s'", retrieved.LastError)
	}
}

func TestMemoryStore_WebhookQueueListAndDelete(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Create multiple webhooks with slight delays to ensure unique IDs
	for i := 0; i < 5; i++ {
		webhook := PendingWebhook{
			URL:           "https://example.com/webhook",
			Payload:       json.RawMessage(`{}`),
			EventType:     "payment",
			Status:        WebhookStatusPending,
			MaxAttempts:   5,
			NextAttemptAt: time.Now().UTC(),
			CreatedAt:     time.Now().UTC(),
		}
		_, err := store.EnqueueWebhook(ctx, webhook)
		if err != nil {
			t.Fatalf("EnqueueWebhook failed: %v", err)
		}
		time.Sleep(time.Microsecond) // Ensure unique IDs
	}

	// List all webhooks (no status filter)
	webhooks, err := store.ListWebhooks(ctx, "", 10)
	if err != nil {
		t.Fatalf("ListWebhooks failed: %v", err)
	}
	if len(webhooks) != 5 {
		t.Fatalf("Expected 5 webhooks, got %d", len(webhooks))
	}

	// List pending webhooks
	pendingWebhooks, err := store.ListWebhooks(ctx, WebhookStatusPending, 10)
	if err != nil {
		t.Fatalf("ListWebhooks failed: %v", err)
	}
	if len(pendingWebhooks) != 5 {
		t.Fatalf("Expected 5 pending webhooks, got %d", len(pendingWebhooks))
	}

	// Delete first webhook
	if err := store.DeleteWebhook(ctx, webhooks[0].ID); err != nil {
		t.Fatalf("DeleteWebhook failed: %v", err)
	}

	// Verify deletion
	webhooks, err = store.ListWebhooks(ctx, "", 10)
	if err != nil {
		t.Fatalf("ListWebhooks failed: %v", err)
	}
	if len(webhooks) != 4 {
		t.Fatalf("Expected 4 webhooks after deletion, got %d", len(webhooks))
	}
}
