package storage

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// EnqueueWebhook adds a webhook to the delivery queue.
func (m *MemoryStore) EnqueueWebhook(ctx context.Context, webhook PendingWebhook) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	m.webhookQueue[webhook.ID] = webhook
	return webhook.ID, nil
}

// DequeueWebhooks retrieves webhooks ready for delivery.
func (m *MemoryStore) DequeueWebhooks(ctx context.Context, limit int) ([]PendingWebhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	var ready []PendingWebhook

	for _, webhook := range m.webhookQueue {
		if webhook.Status == WebhookStatusPending && (webhook.NextAttemptAt.IsZero() || webhook.NextAttemptAt.Before(now)) {
			ready = append(ready, webhook)
		}
	}

	// Sort by next attempt time (earliest first)
	sort.Slice(ready, func(i, j int) bool {
		return ready[i].NextAttemptAt.Before(ready[j].NextAttemptAt)
	})

	// Limit results
	if limit > 0 && len(ready) > limit {
		ready = ready[:limit]
	}

	return ready, nil
}

// MarkWebhookProcessing updates webhook status to prevent duplicate processing.
func (m *MemoryStore) MarkWebhookProcessing(ctx context.Context, webhookID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	webhook, ok := m.webhookQueue[webhookID]
	if !ok {
		return ErrNotFound
	}

	webhook.Status = WebhookStatusProcessing
	webhook.LastAttemptAt = time.Now().UTC()
	webhook.Attempts++
	m.webhookQueue[webhookID] = webhook

	return nil
}

// MarkWebhookSuccess marks webhook as successfully delivered and removes from queue.
func (m *MemoryStore) MarkWebhookSuccess(ctx context.Context, webhookID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	webhook, ok := m.webhookQueue[webhookID]
	if !ok {
		return ErrNotFound
	}

	webhook.Status = WebhookStatusSuccess
	now := time.Now().UTC()
	webhook.CompletedAt = &now
	m.webhookQueue[webhookID] = webhook

	// Remove from queue after marking success (webhooks are kept for a short time for audit)
	delete(m.webhookQueue, webhookID)

	return nil
}

// MarkWebhookFailed records failed attempt and schedules retry (or moves to DLQ if exhausted).
func (m *MemoryStore) MarkWebhookFailed(ctx context.Context, webhookID string, errorMsg string, nextAttemptAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	webhook, ok := m.webhookQueue[webhookID]
	if !ok {
		return ErrNotFound
	}

	webhook.LastError = errorMsg
	webhook.LastAttemptAt = time.Now().UTC()

	// Check if retries exhausted
	if webhook.Attempts >= webhook.MaxAttempts {
		webhook.Status = WebhookStatusFailed
		now := time.Now().UTC()
		webhook.CompletedAt = &now
	} else {
		webhook.Status = WebhookStatusPending
		webhook.NextAttemptAt = nextAttemptAt
	}

	m.webhookQueue[webhookID] = webhook
	return nil
}

// GetWebhook retrieves a webhook by ID (for admin UI).
func (m *MemoryStore) GetWebhook(ctx context.Context, webhookID string) (PendingWebhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	webhook, ok := m.webhookQueue[webhookID]
	if !ok {
		return PendingWebhook{}, ErrNotFound
	}

	return webhook, nil
}

// ListWebhooks lists webhooks with optional status filter (for admin UI).
func (m *MemoryStore) ListWebhooks(ctx context.Context, status WebhookStatus, limit int) ([]PendingWebhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var webhooks []PendingWebhook

	for _, webhook := range m.webhookQueue {
		if status == "" || webhook.Status == status {
			webhooks = append(webhooks, webhook)
		}
	}

	// Sort by created time (newest first)
	sort.Slice(webhooks, func(i, j int) bool {
		return webhooks[i].CreatedAt.After(webhooks[j].CreatedAt)
	})

	// Limit results
	if limit > 0 && len(webhooks) > limit {
		webhooks = webhooks[:limit]
	}

	return webhooks, nil
}

// RetryWebhook resets webhook to pending state for manual retry (admin operation).
func (m *MemoryStore) RetryWebhook(ctx context.Context, webhookID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	webhook, ok := m.webhookQueue[webhookID]
	if !ok {
		return ErrNotFound
	}

	// Reset to pending for immediate retry
	webhook.Status = WebhookStatusPending
	webhook.NextAttemptAt = time.Now().UTC()
	webhook.LastError = "" // Clear previous error
	m.webhookQueue[webhookID] = webhook

	return nil
}

// DeleteWebhook removes webhook from queue (admin operation).
func (m *MemoryStore) DeleteWebhook(ctx context.Context, webhookID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.webhookQueue[webhookID]; !ok {
		return ErrNotFound
	}

	delete(m.webhookQueue, webhookID)
	return nil
}

// generateWebhookID creates a unique identifier for webhooks.
func generateWebhookID() string {
	return fmt.Sprintf("webhook_%d", time.Now().UnixNano())
}
