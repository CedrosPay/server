package callbacks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/httputil"
	"github.com/CedrosPay/server/internal/metrics"
	"github.com/CedrosPay/server/internal/storage"
	"github.com/rs/zerolog"
)

// WebhookQueueWorker processes webhooks from the persistent queue.
type WebhookQueueWorker struct {
	store        storage.Store
	cfg          config.CallbacksConfig
	retryCfg     RetryConfig
	httpClient   *http.Client
	logger       zerolog.Logger
	metrics      *metrics.Metrics
	stopChan     chan struct{}
	doneChan     chan struct{}
	pollInterval time.Duration
}

// WebhookQueueWorkerOptions configures the webhook queue worker.
type WebhookQueueWorkerOptions struct {
	Store        storage.Store
	Config       config.CallbacksConfig
	RetryConfig  RetryConfig
	Logger       zerolog.Logger
	Metrics      *metrics.Metrics
	PollInterval time.Duration // How often to poll for pending webhooks (default: 5s)
}

// NewWebhookQueueWorker creates a new webhook queue worker.
func NewWebhookQueueWorker(opts WebhookQueueWorkerOptions) *WebhookQueueWorker {
	if opts.PollInterval == 0 {
		opts.PollInterval = 5 * time.Second
	}
	if opts.RetryConfig.Timeout == 0 {
		opts.RetryConfig = DefaultRetryConfig()
	}
	if opts.Logger.GetLevel() == zerolog.Disabled {
		opts.Logger = zerolog.Nop()
	}

	timeout := opts.Config.Timeout.Duration
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &WebhookQueueWorker{
		store:        opts.Store,
		cfg:          opts.Config,
		retryCfg:     opts.RetryConfig,
		httpClient:   httputil.NewClient(timeout),
		logger:       opts.Logger,
		metrics:      opts.Metrics,
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
		pollInterval: opts.PollInterval,
	}
}

// Start begins processing webhooks from the queue.
func (w *WebhookQueueWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop gracefully stops the worker.
func (w *WebhookQueueWorker) Stop() {
	close(w.stopChan)
	<-w.doneChan
}

// run is the main worker loop that polls the queue and processes webhooks.
func (w *WebhookQueueWorker) run(ctx context.Context) {
	defer close(w.doneChan)

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	w.logger.Info().
		Dur("pollInterval", w.pollInterval).
		Msg("webhook queue worker started")

	for {
		select {
		case <-w.stopChan:
			w.logger.Info().Msg("webhook queue worker stopping")
			return
		case <-ticker.C:
			w.processQueue(ctx)
		}
	}
}

// processQueue fetches and processes pending webhooks.
func (w *WebhookQueueWorker) processQueue(ctx context.Context) {
	// Dequeue up to 10 webhooks per poll
	webhooks, err := w.store.DequeueWebhooks(ctx, 10)
	if err != nil {
		w.logger.Error().Err(err).Msg("failed to dequeue webhooks")
		return
	}

	if len(webhooks) == 0 {
		return
	}

	w.logger.Debug().Int("count", len(webhooks)).Msg("processing webhooks from queue")

	for _, webhook := range webhooks {
		w.processWebhook(ctx, webhook)
	}
}

// processWebhook processes a single webhook delivery attempt.
func (w *WebhookQueueWorker) processWebhook(ctx context.Context, webhook storage.PendingWebhook) {
	// Mark as processing to prevent duplicate delivery
	if err := w.store.MarkWebhookProcessing(ctx, webhook.ID); err != nil {
		w.logger.Error().
			Err(err).
			Str("webhookID", webhook.ID).
			Msg("failed to mark webhook as processing")
		return
	}
	webhook.Attempts++

	startTime := time.Now()

	// Attempt delivery
	reqCtx, cancel := context.WithTimeout(ctx, w.retryCfg.Timeout)
	err := w.sendWebhook(reqCtx, webhook)
	cancel()

	duration := time.Since(startTime)

	if err == nil {
		// Success - remove from queue
		if markErr := w.store.MarkWebhookSuccess(ctx, webhook.ID); markErr != nil {
			w.logger.Error().
				Err(markErr).
				Str("webhookID", webhook.ID).
				Msg("failed to mark webhook as successful")
		}

		// Record metrics
		if w.metrics != nil {
			w.metrics.ObserveWebhook(webhook.EventType, "success", duration, webhook.Attempts, false)
		}

		w.logger.Info().
			Str("webhookID", webhook.ID).
			Str("eventType", webhook.EventType).
			Int("attempts", webhook.Attempts).
			Dur("duration", duration).
			Msg("webhook delivered successfully")

		return
	}

	// Failed - schedule retry or move to DLQ
	w.handleWebhookFailure(ctx, webhook, err)
}

// handleWebhookFailure schedules a retry or marks webhook as permanently failed.
func (w *WebhookQueueWorker) handleWebhookFailure(ctx context.Context, webhook storage.PendingWebhook, deliveryErr error) {
	// Calculate next retry time using exponential backoff
	backoffDuration := w.calculateBackoff(webhook.Attempts)
	nextAttemptAt := time.Now().Add(backoffDuration)

	// Mark webhook as failed (will schedule retry or move to DLQ)
	err := w.store.MarkWebhookFailed(ctx, webhook.ID, deliveryErr.Error(), nextAttemptAt)
	if err != nil {
		w.logger.Error().
			Err(err).
			Str("webhookID", webhook.ID).
			Msg("failed to mark webhook as failed")
		return
	}

	if webhook.Attempts >= webhook.MaxAttempts {
		// Exhausted all retries - permanently failed (DLQ)
		if w.metrics != nil {
			w.metrics.ObserveWebhook(webhook.EventType, "dlq", time.Since(webhook.CreatedAt), webhook.Attempts, true)
		}

		w.logger.Warn().
			Str("webhookID", webhook.ID).
			Str("eventType", webhook.EventType).
			Int("attempts", webhook.Attempts).
			Err(deliveryErr).
			Msg("webhook failed permanently after all retries")
	} else {
		// Scheduled for retry
		w.logger.Warn().
			Str("webhookID", webhook.ID).
			Str("eventType", webhook.EventType).
			Int("attempts", webhook.Attempts).
			Time("nextAttempt", nextAttemptAt).
			Err(deliveryErr).
			Msg("webhook delivery failed, scheduled for retry")
	}
}

// calculateBackoff calculates the backoff duration for the given attempt number.
func (w *WebhookQueueWorker) calculateBackoff(attempt int) time.Duration {
	// Start with initial interval
	backoff := w.retryCfg.InitialInterval

	// Apply exponential multiplier for each attempt
	for i := 1; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * w.retryCfg.Multiplier)
		if backoff > w.retryCfg.MaxInterval {
			backoff = w.retryCfg.MaxInterval
			break
		}
	}

	return backoff
}

// sendWebhook performs the actual HTTP request to deliver the webhook.
func (w *WebhookQueueWorker) sendWebhook(ctx context.Context, webhook storage.PendingWebhook) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(webhook.Payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	// Set headers
	for key, value := range webhook.Headers {
		if key == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	// Ensure Content-Type is set
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("received status %d from %s", resp.StatusCode, webhook.URL)
	}

	return nil
}

// EnqueuePaymentWebhook adds a payment webhook to the persistent queue.
func (w *WebhookQueueWorker) EnqueuePaymentWebhook(ctx context.Context, event PaymentEvent) error {
	// Prepare idempotency fields
	PreparePaymentEvent(&event)

	// Serialize payload
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal payment event: %w", err)
	}

	// Create pending webhook
	webhook := storage.PendingWebhook{
		URL:           w.cfg.PaymentSuccessURL,
		Payload:       json.RawMessage(payload),
		Headers:       w.cfg.Headers,
		EventType:     "payment",
		Status:        storage.WebhookStatusPending,
		Attempts:      0,
		MaxAttempts:   w.retryCfg.MaxAttempts,
		NextAttemptAt: time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	// Enqueue to storage
	webhookID, err := w.store.EnqueueWebhook(ctx, webhook)
	if err != nil {
		return fmt.Errorf("enqueue webhook: %w", err)
	}

	w.logger.Debug().
		Str("webhookID", webhookID).
		Str("eventID", event.EventID).
		Msg("payment webhook enqueued")

	return nil
}

// EnqueueRefundWebhook adds a refund webhook to the persistent queue.
func (w *WebhookQueueWorker) EnqueueRefundWebhook(ctx context.Context, event RefundEvent) error {
	// Prepare idempotency fields
	PrepareRefundEvent(&event)

	// Serialize payload
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal refund event: %w", err)
	}

	// Create pending webhook
	webhook := storage.PendingWebhook{
		URL:           w.cfg.PaymentSuccessURL,
		Payload:       json.RawMessage(payload),
		Headers:       w.cfg.Headers,
		EventType:     "refund",
		Status:        storage.WebhookStatusPending,
		Attempts:      0,
		MaxAttempts:   w.retryCfg.MaxAttempts,
		NextAttemptAt: time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	// Enqueue to storage
	webhookID, err := w.store.EnqueueWebhook(ctx, webhook)
	if err != nil {
		return fmt.Errorf("enqueue webhook: %w", err)
	}

	w.logger.Debug().
		Str("webhookID", webhookID).
		Str("eventID", event.EventID).
		Msg("refund webhook enqueued")

	return nil
}
