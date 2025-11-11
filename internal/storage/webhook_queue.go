package storage

import (
	"encoding/json"
	"time"
)

// WebhookStatus represents the current state of a webhook in the queue.
type WebhookStatus string

const (
	WebhookStatusPending    WebhookStatus = "pending"    // Waiting for delivery
	WebhookStatusProcessing WebhookStatus = "processing" // Currently being delivered
	WebhookStatusFailed     WebhookStatus = "failed"     // Failed after all retries (DLQ)
	WebhookStatusSuccess    WebhookStatus = "success"    // Successfully delivered
)

// PendingWebhook represents a webhook waiting for delivery or retry.
// This struct is persisted to the database to ensure delivery across server restarts.
type PendingWebhook struct {
	ID              string            `json:"id"`              // Unique webhook identifier (webhook_123...)
	URL             string            `json:"url"`             // Destination URL
	Payload         json.RawMessage   `json:"payload"`         // JSON payload to send
	Headers         map[string]string `json:"headers"`         // HTTP headers
	EventType       string            `json:"eventType"`       // "payment" or "refund"
	Status          WebhookStatus     `json:"status"`          // Current status
	Attempts        int               `json:"attempts"`        // Number of delivery attempts
	MaxAttempts     int               `json:"maxAttempts"`     // Maximum retry attempts (e.g., 5)
	LastError       string            `json:"lastError"`       // Error from last attempt
	LastAttemptAt   time.Time         `json:"lastAttemptAt"`   // When last attempt was made
	NextAttemptAt   time.Time         `json:"nextAttemptAt"`   // When next attempt should be made
	CreatedAt       time.Time         `json:"createdAt"`       // When webhook was created
	CompletedAt     *time.Time        `json:"completedAt"`     // When webhook was successfully delivered or failed permanently
}

// IsReadyForDelivery returns true if the webhook should be processed now.
func (w PendingWebhook) IsReadyForDelivery() bool {
	if w.Status != WebhookStatusPending {
		return false
	}
	return time.Now().After(w.NextAttemptAt) || w.NextAttemptAt.IsZero()
}

// IsFinallyFailed returns true if the webhook has exhausted all retries.
func (w PendingWebhook) IsFinallyFailed() bool {
	return w.Attempts >= w.MaxAttempts && w.Status == WebhookStatusFailed
}
