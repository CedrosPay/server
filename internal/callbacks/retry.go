package callbacks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/httputil"
	"github.com/CedrosPay/server/internal/metrics"
	"github.com/rs/zerolog"
)

// RetryConfig holds webhook retry configuration.
type RetryConfig struct {
	MaxAttempts     int           // Maximum retry attempts (default: 5)
	InitialInterval time.Duration // Initial backoff interval (default: 1s)
	MaxInterval     time.Duration // Maximum backoff interval (default: 5m)
	Multiplier      float64       // Backoff multiplier (default: 2.0)
	Timeout         time.Duration // Per-attempt timeout (default: 10s)
}

// DefaultRetryConfig returns sensible defaults for webhook retries.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     5,
		InitialInterval: 1 * time.Second,
		MaxInterval:     5 * time.Minute,
		Multiplier:      2.0,
		Timeout:         10 * time.Second,
	}
}

// RetryableClient posts payment events with exponential backoff retry logic.
type RetryableClient struct {
	cfg        config.CallbacksConfig
	retryCfg   RetryConfig
	httpClient *http.Client
	logger     zerolog.Logger
	tmpl       *template.Template
	dlqStore   DLQStore         // Dead Letter Queue for failed webhooks
	metrics    *metrics.Metrics // Prometheus metrics collector
}

// DLQStore persists failed webhook attempts for manual retry or analysis.
type DLQStore interface {
	SaveFailedWebhook(ctx context.Context, webhook FailedWebhook) error
	ListFailedWebhooks(ctx context.Context, limit int) ([]FailedWebhook, error)
	DeleteFailedWebhook(ctx context.Context, id string) error
}

// FailedWebhook represents a webhook that exhausted all retry attempts.
type FailedWebhook struct {
	ID          string            `json:"id"`
	URL         string            `json:"url"`
	Payload     json.RawMessage   `json:"payload"`
	Headers     map[string]string `json:"headers"`
	EventType   string            `json:"eventType"` // "payment" or "refund"
	Attempts    int               `json:"attempts"`
	LastError   string            `json:"lastError"`
	LastAttempt time.Time         `json:"lastAttempt"`
	CreatedAt   time.Time         `json:"createdAt"`
}

// RetryOption customizes the retry client behavior.
type RetryOption func(*RetryableClient)

// WithRetryLogger sets a custom logger for retry operations.
func WithRetryLogger(logger zerolog.Logger) RetryOption {
	return func(c *RetryableClient) {
		c.logger = logger
	}
}

// WithDLQStore enables dead letter queue for failed webhooks.
func WithDLQStore(store DLQStore) RetryOption {
	return func(c *RetryableClient) {
		c.dlqStore = store
	}
}

// WithRetryConfig sets custom retry configuration.
func WithRetryConfig(cfg RetryConfig) RetryOption {
	return func(c *RetryableClient) {
		c.retryCfg = cfg
	}
}

// WithMetrics sets the metrics collector for webhook observability.
func WithMetrics(metrics *metrics.Metrics) RetryOption {
	return func(c *RetryableClient) {
		c.metrics = metrics
	}
}

// NewRetryableClient constructs a callback client with retry support.
func NewRetryableClient(cfg config.CallbacksConfig, opts ...RetryOption) Notifier {
	if cfg.PaymentSuccessURL == "" {
		return NoopNotifier{}
	}

	timeout := cfg.Timeout.Duration
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := &RetryableClient{
		cfg:        cfg,
		retryCfg:   DefaultRetryConfig(),
		httpClient: httputil.NewClient(timeout),
		logger:     zerolog.Nop(), // No-op logger by default
	}

	for _, opt := range opts {
		opt(client)
	}

	if cfg.BodyTemplate != "" {
		tmpl, err := template.New("callback").Parse(cfg.BodyTemplate)
		if err != nil {
			client.logger.Error().Err(err).Msg("callbacks: failed to parse template")
		} else {
			client.tmpl = tmpl
		}
	}

	return client
}

// PaymentSucceeded dispatches the payment event asynchronously with retry logic.
// IMPORTANT: EventID is generated once and preserved across all retry attempts for idempotency.
func (c *RetryableClient) PaymentSucceeded(ctx context.Context, event PaymentEvent) {
	if c == nil || c.cfg.PaymentSuccessURL == "" {
		return
	}

	// Prepare idempotency fields BEFORE serialization
	// This ensures the same EventID is used for all retry attempts
	PreparePaymentEvent(&event)

	go func() {
		payload, err := c.serializePayment(event)
		if err != nil {
			c.logger.Error().Err(err).Msg("callbacks: failed to serialize payment event")
			return
		}

		if err := c.sendWithRetry(context.Background(), payload, "payment"); err != nil {
			c.logger.Error().
				Err(err).
				Str("event_id", event.EventID).
				Msg("callbacks: payment webhook failed after all retries")
			// Save to DLQ if configured
			if c.dlqStore != nil {
				c.saveToDLQ(context.Background(), payload, "payment", err)
			}
		}
	}()
}

// RefundSucceeded dispatches the refund event asynchronously with retry logic.
// IMPORTANT: EventID is generated once and preserved across all retry attempts for idempotency.
func (c *RetryableClient) RefundSucceeded(ctx context.Context, event RefundEvent) {
	if c == nil || c.cfg.PaymentSuccessURL == "" {
		return
	}

	// Prepare idempotency fields BEFORE serialization
	// This ensures the same EventID is used for all retry attempts
	PrepareRefundEvent(&event)

	go func() {
		payload, err := c.serializeRefund(event)
		if err != nil {
			c.logger.Error().Err(err).Msg("callbacks: failed to serialize refund event")
			return
		}

		if err := c.sendWithRetry(context.Background(), payload, "refund"); err != nil {
			c.logger.Error().
				Err(err).
				Str("event_id", event.EventID).
				Msg("callbacks: refund webhook failed after all retries")
			// Save to DLQ if configured
			if c.dlqStore != nil {
				c.saveToDLQ(context.Background(), payload, "refund", err)
			}
		}
	}()
}

// serializePayment converts a payment event to JSON payload.
func (c *RetryableClient) serializePayment(event PaymentEvent) ([]byte, error) {
	if c.cfg.Body != "" {
		return []byte(c.cfg.Body), nil
	}
	if c.tmpl != nil {
		var buf bytes.Buffer
		if err := c.tmpl.Execute(&buf, event); err != nil {
			return nil, fmt.Errorf("execute template: %w", err)
		}
		return buf.Bytes(), nil
	}
	return json.Marshal(event)
}

// serializeRefund converts a refund event to JSON payload.
func (c *RetryableClient) serializeRefund(event RefundEvent) ([]byte, error) {
	if c.cfg.Body != "" {
		return []byte(c.cfg.Body), nil
	}
	if c.tmpl != nil {
		var buf bytes.Buffer
		if err := c.tmpl.Execute(&buf, event); err != nil {
			return nil, fmt.Errorf("execute template: %w", err)
		}
		return buf.Bytes(), nil
	}
	return json.Marshal(event)
}

// sendWithRetry attempts to send the webhook with exponential backoff.
func (c *RetryableClient) sendWithRetry(ctx context.Context, payload []byte, eventType string) error {
	var lastErr error
	interval := c.retryCfg.InitialInterval
	startTime := time.Now()

	// If retries are disabled, only attempt once
	if !c.cfg.Retry.Enabled {
		reqCtx, cancel := context.WithTimeout(ctx, c.retryCfg.Timeout)
		err := c.sendHTTP(reqCtx, payload)
		cancel()
		if c.metrics != nil {
			status := "success"
			if err != nil {
				status = "failed"
			}
			c.metrics.ObserveWebhook(eventType, status, time.Since(startTime), 1, false)
		}
		return err
	}

	for attempt := 1; attempt <= c.retryCfg.MaxAttempts; attempt++ {
		reqCtx, cancel := context.WithTimeout(ctx, c.retryCfg.Timeout)
		err := c.sendHTTP(reqCtx, payload)
		cancel()

		if err == nil {
			duration := time.Since(startTime)

			// Record successful webhook delivery
			if c.metrics != nil {
				c.metrics.ObserveWebhook(eventType, "success", duration, attempt, false)
			}

			if attempt > 1 {
				c.logger.Info().
					Int("attempt", attempt).
					Str("eventType", eventType).
					Msg("callbacks: webhook succeeded after retry")
			}
			return nil
		}

		lastErr = err
		c.logger.Warn().
			Err(err).
			Int("attempt", attempt).
			Int("maxAttempts", c.retryCfg.MaxAttempts).
			Str("eventType", eventType).
			Dur("nextRetry", interval).
			Msg("callbacks: webhook attempt failed")

		// Don't sleep after the last attempt
		if attempt < c.retryCfg.MaxAttempts {
			time.Sleep(interval)
			// Exponential backoff with max cap
			interval = time.Duration(float64(interval) * c.retryCfg.Multiplier)
			if interval > c.retryCfg.MaxInterval {
				interval = c.retryCfg.MaxInterval
			}
		}
	}

	duration := time.Since(startTime)

	// Record failed webhook (after all retries exhausted)
	if c.metrics != nil {
		c.metrics.ObserveWebhook(eventType, "failed", duration, c.retryCfg.MaxAttempts, false)
	}

	return fmt.Errorf("webhook failed after %d attempts: %w", c.retryCfg.MaxAttempts, lastErr)
}

// sendHTTP performs the actual HTTP request.
func (c *RetryableClient) sendHTTP(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.PaymentSuccessURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	contentType := c.cfg.Headers["Content-Type"]
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)

	for k, v := range c.cfg.Headers {
		if k == "" {
			continue
		}
		if strings.EqualFold(k, "content-type") {
			continue
		}
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("received status %d from %s", resp.StatusCode, c.cfg.PaymentSuccessURL)
	}

	return nil
}

// saveToDLQ persists a failed webhook to the dead letter queue.
func (c *RetryableClient) saveToDLQ(ctx context.Context, payload []byte, eventType string, lastErr error) {
	webhook := FailedWebhook{
		ID:          generateWebhookID(),
		URL:         c.cfg.PaymentSuccessURL,
		Payload:     json.RawMessage(payload),
		Headers:     c.cfg.Headers,
		EventType:   eventType,
		Attempts:    c.retryCfg.MaxAttempts,
		LastError:   lastErr.Error(),
		LastAttempt: time.Now().UTC(),
		CreatedAt:   time.Now().UTC(),
	}

	if err := c.dlqStore.SaveFailedWebhook(ctx, webhook); err != nil {
		c.logger.Error().Err(err).Str("webhookID", webhook.ID).Msg("callbacks: failed to save to DLQ")
	} else {
		// Record DLQ metric
		if c.metrics != nil {
			// Calculate total duration (use a reasonable estimate based on max attempts)
			totalDuration := time.Duration(webhook.Attempts) * c.retryCfg.InitialInterval
			c.metrics.ObserveWebhook(eventType, "dlq", totalDuration, webhook.Attempts, true)
		}

		c.logger.Info().
			Str("webhookID", webhook.ID).
			Str("eventType", eventType).
			Int("attempts", webhook.Attempts).
			Msg("callbacks: saved failed webhook to DLQ")
	}
}

// generateWebhookID creates a unique identifier for failed webhooks.
func generateWebhookID() string {
	return fmt.Sprintf("webhook_%d", time.Now().UnixNano())
}
