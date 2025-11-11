package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EnqueueWebhook adds a webhook to the delivery queue.
func (s *PostgresStore) EnqueueWebhook(ctx context.Context, webhook PendingWebhook) (string, error) {
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

	// Marshal headers to JSON
	headersJSON, err := json.Marshal(webhook.Headers)
	if err != nil {
		return "", fmt.Errorf("marshal headers: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (id, url, payload, headers, event_type, status, attempts, max_attempts, last_error, last_attempt_at, next_attempt_at, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, s.webhookQueueTableName)

	_, err = s.db.ExecContext(ctx, query,
		webhook.ID,
		webhook.URL,
		webhook.Payload,
		headersJSON,
		webhook.EventType,
		webhook.Status,
		webhook.Attempts,
		webhook.MaxAttempts,
		webhook.LastError,
		nullTime(webhook.LastAttemptAt),
		webhook.NextAttemptAt,
		webhook.CreatedAt,
		nullTimePtr(webhook.CompletedAt),
	)
	if err != nil {
		return "", fmt.Errorf("insert webhook: %w", err)
	}

	return webhook.ID, nil
}

// DequeueWebhooks retrieves webhooks ready for delivery.
func (s *PostgresStore) DequeueWebhooks(ctx context.Context, limit int) ([]PendingWebhook, error) {
	query := fmt.Sprintf(`
		SELECT id, url, payload, headers, event_type, status, attempts, max_attempts, last_error, last_attempt_at, next_attempt_at, created_at, completed_at
		FROM %s
		WHERE status = $1 AND next_attempt_at <= $2
		ORDER BY next_attempt_at ASC
		LIMIT $3
	`, s.webhookQueueTableName)

	rows, err := s.db.QueryContext(ctx, query, WebhookStatusPending, time.Now().UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("query webhooks: %w", err)
	}
	defer rows.Close()

	var webhooks []PendingWebhook
	for rows.Next() {
		webhook, err := scanWebhook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan webhook: %w", err)
		}
		webhooks = append(webhooks, webhook)
	}

	return webhooks, rows.Err()
}

// MarkWebhookProcessing updates webhook status to prevent duplicate processing.
func (s *PostgresStore) MarkWebhookProcessing(ctx context.Context, webhookID string) error {
	query := fmt.Sprintf(`
		UPDATE %s
		SET status = $1, last_attempt_at = $2, attempts = attempts + 1
		WHERE id = $3
	`, s.webhookQueueTableName)

	result, err := s.db.ExecContext(ctx, query, WebhookStatusProcessing, time.Now().UTC(), webhookID)
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// MarkWebhookSuccess marks webhook as successfully delivered and removes from queue.
func (s *PostgresStore) MarkWebhookSuccess(ctx context.Context, webhookID string) error {
	query := fmt.Sprintf(`
		DELETE FROM %s
		WHERE id = $1
	`, s.webhookQueueTableName)

	result, err := s.db.ExecContext(ctx, query, webhookID)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// MarkWebhookFailed records failed attempt and schedules retry (or moves to DLQ if exhausted).
func (s *PostgresStore) MarkWebhookFailed(ctx context.Context, webhookID string, errorMsg string, nextAttemptAt time.Time) error {
	// First, check current attempt count to determine if we should mark as failed
	var attempts, maxAttempts int
	checkQuery := fmt.Sprintf("SELECT attempts, max_attempts FROM %s WHERE id = $1", s.webhookQueueTableName)
	err := s.db.QueryRowContext(ctx, checkQuery, webhookID).Scan(&attempts, &maxAttempts)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("query webhook: %w", err)
	}

	var query string
	var args []interface{}

	if attempts >= maxAttempts {
		// Exhausted all retries - mark as permanently failed
		query = fmt.Sprintf(`
			UPDATE %s
			SET status = $1, last_error = $2, last_attempt_at = $3, completed_at = $4
			WHERE id = $5
		`, s.webhookQueueTableName)
		now := time.Now().UTC()
		args = []interface{}{WebhookStatusFailed, errorMsg, now, now, webhookID}
	} else {
		// Schedule retry
		query = fmt.Sprintf(`
			UPDATE %s
			SET status = $1, last_error = $2, last_attempt_at = $3, next_attempt_at = $4
			WHERE id = $5
		`, s.webhookQueueTableName)
		args = []interface{}{WebhookStatusPending, errorMsg, time.Now().UTC(), nextAttemptAt, webhookID}
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// GetWebhook retrieves a webhook by ID (for admin UI).
func (s *PostgresStore) GetWebhook(ctx context.Context, webhookID string) (PendingWebhook, error) {
	query := fmt.Sprintf(`
		SELECT id, url, payload, headers, event_type, status, attempts, max_attempts, last_error, last_attempt_at, next_attempt_at, created_at, completed_at
		FROM %s
		WHERE id = $1
	`, s.webhookQueueTableName)

	row := s.db.QueryRowContext(ctx, query, webhookID)
	webhook, err := scanWebhook(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return PendingWebhook{}, ErrNotFound
		}
		return PendingWebhook{}, fmt.Errorf("scan webhook: %w", err)
	}

	return webhook, nil
}

// ListWebhooks lists webhooks with optional status filter (for admin UI).
func (s *PostgresStore) ListWebhooks(ctx context.Context, status WebhookStatus, limit int) ([]PendingWebhook, error) {
	var query string
	var args []interface{}

	if status == "" {
		query = fmt.Sprintf(`
			SELECT id, url, payload, headers, event_type, status, attempts, max_attempts, last_error, last_attempt_at, next_attempt_at, created_at, completed_at
			FROM %s
			ORDER BY created_at DESC
			LIMIT $1
		`, s.webhookQueueTableName)
		args = []interface{}{limit}
	} else {
		query = fmt.Sprintf(`
			SELECT id, url, payload, headers, event_type, status, attempts, max_attempts, last_error, last_attempt_at, next_attempt_at, created_at, completed_at
			FROM %s
			WHERE status = $1
			ORDER BY created_at DESC
			LIMIT $2
		`, s.webhookQueueTableName)
		args = []interface{}{status, limit}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query webhooks: %w", err)
	}
	defer rows.Close()

	var webhooks []PendingWebhook
	for rows.Next() {
		webhook, err := scanWebhook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan webhook: %w", err)
		}
		webhooks = append(webhooks, webhook)
	}

	return webhooks, rows.Err()
}

// RetryWebhook resets webhook to pending state for manual retry (admin operation).
func (s *PostgresStore) RetryWebhook(ctx context.Context, webhookID string) error {
	query := fmt.Sprintf(`
		UPDATE %s
		SET status = $1, next_attempt_at = $2, last_error = $3
		WHERE id = $4
	`, s.webhookQueueTableName)

	result, err := s.db.ExecContext(ctx, query, WebhookStatusPending, time.Now().UTC(), "", webhookID)
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteWebhook removes webhook from queue (admin operation).
func (s *PostgresStore) DeleteWebhook(ctx context.Context, webhookID string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.webhookQueueTableName)

	result, err := s.db.ExecContext(ctx, query, webhookID)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// scanWebhook is a helper that scans a webhook row from SQL.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanWebhook(s scanner) (PendingWebhook, error) {
	var webhook PendingWebhook
	var headersJSON []byte
	var lastAttemptAt sql.NullTime
	var completedAt sql.NullTime

	err := s.Scan(
		&webhook.ID,
		&webhook.URL,
		&webhook.Payload,
		&headersJSON,
		&webhook.EventType,
		&webhook.Status,
		&webhook.Attempts,
		&webhook.MaxAttempts,
		&webhook.LastError,
		&lastAttemptAt,
		&webhook.NextAttemptAt,
		&webhook.CreatedAt,
		&completedAt,
	)
	if err != nil {
		return PendingWebhook{}, err
	}

	// Unmarshal headers
	if len(headersJSON) > 0 {
		if err := json.Unmarshal(headersJSON, &webhook.Headers); err != nil {
			return PendingWebhook{}, fmt.Errorf("unmarshal headers: %w", err)
		}
	}

	// Convert nullable times
	if lastAttemptAt.Valid {
		webhook.LastAttemptAt = lastAttemptAt.Time
	}
	if completedAt.Valid {
		webhook.CompletedAt = &completedAt.Time
	}

	return webhook, nil
}

// nullTime converts a time.Time to sql.NullTime, handling zero values.
func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// nullTimePtr converts a *time.Time to sql.NullTime.
func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
