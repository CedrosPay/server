package storage

import (
	"context"
	"sort"
	"time"
)

// EnqueueWebhook adds a webhook to the delivery queue.
func (s *FileStore) EnqueueWebhook(ctx context.Context, webhook PendingWebhook) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	// Store webhook
	if s.data.WebhookQueue == nil {
		s.data.WebhookQueue = make(map[string]PendingWebhook)
	}
	s.data.WebhookQueue[webhook.ID] = webhook

	return webhook.ID, s.persist()
}

// DequeueWebhooks retrieves webhooks ready for delivery.
func (s *FileStore) DequeueWebhooks(ctx context.Context, limit int) ([]PendingWebhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var ready []PendingWebhook

	for _, webhook := range s.data.WebhookQueue {
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
func (s *FileStore) MarkWebhookProcessing(ctx context.Context, webhookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	webhook, ok := s.data.WebhookQueue[webhookID]
	if !ok {
		return ErrNotFound
	}

	webhook.Status = WebhookStatusProcessing
	webhook.LastAttemptAt = time.Now().UTC()
	webhook.Attempts++
	s.data.WebhookQueue[webhookID] = webhook

	return s.persist()
}

// MarkWebhookSuccess marks webhook as successfully delivered and removes from queue.
func (s *FileStore) MarkWebhookSuccess(ctx context.Context, webhookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data.WebhookQueue[webhookID]; !ok {
		return ErrNotFound
	}

	// Remove from queue
	delete(s.data.WebhookQueue, webhookID)

	return s.persist()
}

// MarkWebhookFailed records failed attempt and schedules retry (or moves to DLQ if exhausted).
func (s *FileStore) MarkWebhookFailed(ctx context.Context, webhookID string, errorMsg string, nextAttemptAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	webhook, ok := s.data.WebhookQueue[webhookID]
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

	s.data.WebhookQueue[webhookID] = webhook
	return s.persist()
}

// GetWebhook retrieves a webhook by ID (for admin UI).
func (s *FileStore) GetWebhook(ctx context.Context, webhookID string) (PendingWebhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	webhook, ok := s.data.WebhookQueue[webhookID]
	if !ok {
		return PendingWebhook{}, ErrNotFound
	}

	return webhook, nil
}

// ListWebhooks lists webhooks with optional status filter (for admin UI).
func (s *FileStore) ListWebhooks(ctx context.Context, status WebhookStatus, limit int) ([]PendingWebhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var webhooks []PendingWebhook

	for _, webhook := range s.data.WebhookQueue {
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
func (s *FileStore) RetryWebhook(ctx context.Context, webhookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	webhook, ok := s.data.WebhookQueue[webhookID]
	if !ok {
		return ErrNotFound
	}

	// Reset to pending for immediate retry
	webhook.Status = WebhookStatusPending
	webhook.NextAttemptAt = time.Now().UTC()
	webhook.LastError = "" // Clear previous error
	s.data.WebhookQueue[webhookID] = webhook

	return s.persist()
}

// DeleteWebhook removes webhook from queue (admin operation).
func (s *FileStore) DeleteWebhook(ctx context.Context, webhookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data.WebhookQueue[webhookID]; !ok {
		return ErrNotFound
	}

	delete(s.data.WebhookQueue, webhookID)
	return s.persist()
}
