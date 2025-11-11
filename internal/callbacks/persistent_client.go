package callbacks

import (
	"context"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/metrics"
	"github.com/CedrosPay/server/internal/storage"
	"github.com/rs/zerolog"
)

// PersistentCallbackClient delivers webhooks via a persistent queue.
// Unlike RetryableClient which uses goroutines (lost on restart), this client
// persists webhooks to the database for guaranteed delivery across server restarts.
type PersistentCallbackClient struct {
	worker *WebhookQueueWorker
	logger zerolog.Logger
}

// PersistentCallbackOptions configures the persistent callback client.
type PersistentCallbackOptions struct {
	Store       storage.Store
	Config      config.CallbacksConfig
	RetryConfig RetryConfig
	Logger      zerolog.Logger
	Metrics     *metrics.Metrics
}

// NewPersistentCallbackClient creates a callback client with persistent queue backing.
func NewPersistentCallbackClient(opts PersistentCallbackOptions) *PersistentCallbackClient {
	if opts.Config.PaymentSuccessURL == "" {
		return nil
	}

	if opts.RetryConfig.Timeout == 0 {
		opts.RetryConfig = DefaultRetryConfig()
	}

	worker := NewWebhookQueueWorker(WebhookQueueWorkerOptions{
		Store:       opts.Store,
		Config:      opts.Config,
		RetryConfig: opts.RetryConfig,
		Logger:      opts.Logger,
		Metrics:     opts.Metrics,
	})

	// Start worker in background
	worker.Start(context.Background())

	return &PersistentCallbackClient{
		worker: worker,
		logger: opts.Logger,
	}
}

// PaymentSucceeded queues a payment success webhook for persistent delivery.
func (c *PersistentCallbackClient) PaymentSucceeded(ctx context.Context, event PaymentEvent) {
	if c == nil || c.worker == nil {
		return
	}

	if err := c.worker.EnqueuePaymentWebhook(ctx, event); err != nil {
		c.logger.Error().
			Err(err).
			Str("eventID", event.EventID).
			Msg("failed to enqueue payment webhook")
	}
}

// RefundSucceeded queues a refund success webhook for persistent delivery.
func (c *PersistentCallbackClient) RefundSucceeded(ctx context.Context, event RefundEvent) {
	if c == nil || c.worker == nil {
		return
	}

	if err := c.worker.EnqueueRefundWebhook(ctx, event); err != nil {
		c.logger.Error().
			Err(err).
			Str("eventID", event.EventID).
			Msg("failed to enqueue refund webhook")
	}
}

// Close gracefully stops the webhook worker.
func (c *PersistentCallbackClient) Close() error {
	if c == nil || c.worker == nil {
		return nil
	}

	c.worker.Stop()
	return nil
}
