package examples

import (
	"context"

	"github.com/CedrosPay/server/internal/observability"
	"github.com/rs/zerolog"
)

// LoggingHook logs all observability events using zerolog.
// Useful for debugging and development environments.
type LoggingHook struct {
	logger zerolog.Logger
}

// NewLoggingHook creates a hook that logs all events.
func NewLoggingHook(logger zerolog.Logger) *LoggingHook {
	return &LoggingHook{logger: logger}
}

func (h *LoggingHook) Name() string {
	return "logging"
}

// ===============================================
// PaymentHook Implementation
// ===============================================

func (h *LoggingHook) OnPaymentStarted(ctx context.Context, event observability.PaymentStartedEvent) {
	h.logger.Info().
		Str("payment_id", event.PaymentID).
		Str("method", event.Method).
		Str("resource", event.ResourceID).
		Int64("amount", event.Amount).
		Str("token", event.Token).
		Msg("payment started")
}

func (h *LoggingHook) OnPaymentCompleted(ctx context.Context, event observability.PaymentCompletedEvent) {
	log := h.logger.Info()
	if !event.Success {
		log = h.logger.Warn().Str("error", event.ErrorReason)
	}

	log.Str("payment_id", event.PaymentID).
		Str("method", event.Method).
		Str("resource", event.ResourceID).
		Bool("success", event.Success).
		Dur("duration", event.Duration).
		Int64("amount", event.Amount).
		Str("token", event.Token).
		Str("tx_id", event.TransactionID).
		Msg("payment completed")
}

func (h *LoggingHook) OnPaymentSettled(ctx context.Context, event observability.PaymentSettledEvent) {
	h.logger.Info().
		Str("payment_id", event.PaymentID).
		Str("network", event.Network).
		Str("tx_id", event.TransactionID).
		Int("confirmations", event.Confirmations).
		Dur("settlement_duration", event.SettlementDuration).
		Msg("payment settled on-chain")
}

// ===============================================
// WebhookHook Implementation
// ===============================================

func (h *LoggingHook) OnWebhookQueued(ctx context.Context, event observability.WebhookQueuedEvent) {
	h.logger.Debug().
		Str("webhook_id", event.WebhookID).
		Str("event_type", event.EventType).
		Str("event_id", event.EventID).
		Str("url", event.URL).
		Msg("webhook queued")
}

func (h *LoggingHook) OnWebhookDelivered(ctx context.Context, event observability.WebhookDeliveredEvent) {
	h.logger.Info().
		Str("webhook_id", event.WebhookID).
		Str("event_type", event.EventType).
		Str("event_id", event.EventID).
		Int("attempts", event.Attempts).
		Dur("duration", event.Duration).
		Int("status_code", event.StatusCode).
		Msg("webhook delivered")
}

func (h *LoggingHook) OnWebhookFailed(ctx context.Context, event observability.WebhookFailedEvent) {
	h.logger.Warn().
		Str("webhook_id", event.WebhookID).
		Str("event_type", event.EventType).
		Str("event_id", event.EventID).
		Int("attempts", event.Attempts).
		Bool("final_failure", event.FinalFailure).
		Str("error", event.Error).
		Msg("webhook delivery failed")
}

func (h *LoggingHook) OnWebhookRetried(ctx context.Context, event observability.WebhookRetriedEvent) {
	h.logger.Debug().
		Str("webhook_id", event.WebhookID).
		Str("event_type", event.EventType).
		Int("attempt", event.CurrentAttempt).
		Int("max_attempts", event.MaxAttempts).
		Time("next_retry", event.NextRetryAt).
		Float64("backoff_seconds", event.BackoffSeconds).
		Msg("webhook scheduled for retry")
}

// ===============================================
// RefundHook Implementation
// ===============================================

func (h *LoggingHook) OnRefundRequested(ctx context.Context, event observability.RefundRequestedEvent) {
	h.logger.Info().
		Str("refund_id", event.RefundID).
		Str("original_purchase", event.OriginalPurchaseID).
		Str("recipient", event.RecipientWallet).
		Int64("amount", event.Amount).
		Str("token", event.Token).
		Str("reason", event.Reason).
		Msg("refund requested")
}

func (h *LoggingHook) OnRefundProcessed(ctx context.Context, event observability.RefundProcessedEvent) {
	log := h.logger.Info()
	if !event.Success {
		log = h.logger.Warn().Str("error", event.ErrorReason)
	}

	log.Str("refund_id", event.RefundID).
		Str("original_purchase", event.OriginalPurchaseID).
		Bool("success", event.Success).
		Dur("duration", event.Duration).
		Int64("amount", event.Amount).
		Str("tx_id", event.TransactionID).
		Msg("refund processed")
}

// ===============================================
// CartHook Implementation
// ===============================================

func (h *LoggingHook) OnCartCreated(ctx context.Context, event observability.CartCreatedEvent) {
	h.logger.Debug().
		Str("cart_id", event.CartID).
		Int("item_count", event.ItemCount).
		Int64("total", event.TotalAmount).
		Str("token", event.Token).
		Time("expires_at", event.ExpiresAt).
		Msg("cart created")
}

func (h *LoggingHook) OnCartCheckout(ctx context.Context, event observability.CartCheckoutEvent) {
	h.logger.Info().
		Str("cart_id", event.CartID).
		Int("item_count", event.ItemCount).
		Int64("total", event.TotalAmount).
		Str("status", event.Status).
		Str("payment_method", event.PaymentMethod).
		Msg("cart checkout")
}

// ===============================================
// RPCHook Implementation
// ===============================================

func (h *LoggingHook) OnRPCCall(ctx context.Context, event observability.RPCCallEvent) {
	log := h.logger.Debug()
	if !event.Success {
		log = h.logger.Warn().Str("error_type", event.ErrorType)
	}

	log.Str("method", event.Method).
		Str("network", event.Network).
		Dur("duration", event.Duration).
		Bool("success", event.Success).
		Msg("RPC call")
}

// ===============================================
// DatabaseHook Implementation
// ===============================================

func (h *LoggingHook) OnDatabaseQuery(ctx context.Context, event observability.DatabaseQueryEvent) {
	log := h.logger.Debug()
	if !event.Success {
		log = h.logger.Warn().Str("error", event.Error)
	}

	log.Str("operation", event.Operation).
		Str("backend", event.Backend).
		Dur("duration", event.Duration).
		Bool("success", event.Success).
		Msg("database query")
}
