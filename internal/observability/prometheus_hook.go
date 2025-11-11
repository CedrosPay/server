package observability

import (
	"context"

	"github.com/CedrosPay/server/internal/metrics"
)

// PrometheusHook adapts the existing Prometheus metrics to the hook interface.
// This maintains backward compatibility while allowing new hooks to be added.
type PrometheusHook struct {
	metrics *metrics.Metrics
}

// NewPrometheusHook creates a hook that emits events to Prometheus metrics.
func NewPrometheusHook(m *metrics.Metrics) *PrometheusHook {
	return &PrometheusHook{metrics: m}
}

func (h *PrometheusHook) Name() string {
	return "prometheus"
}

// ===============================================
// PaymentHook Implementation
// ===============================================

func (h *PrometheusHook) OnPaymentStarted(ctx context.Context, event PaymentStartedEvent) {
	// Prometheus doesn't track "started" events separately - only completions
}

func (h *PrometheusHook) OnPaymentCompleted(ctx context.Context, event PaymentCompletedEvent) {
	h.metrics.ObservePayment(
		event.Method,
		event.ResourceID,
		event.Success,
		event.Duration,
		event.Amount,
		event.Token,
	)

	if !event.Success && event.ErrorReason != "" {
		h.metrics.ObservePaymentFailure(event.Method, event.ResourceID, event.ErrorReason)
	}
}

func (h *PrometheusHook) OnPaymentSettled(ctx context.Context, event PaymentSettledEvent) {
	h.metrics.ObserveSettlement(event.Network, event.SettlementDuration)
}

// ===============================================
// WebhookHook Implementation
// ===============================================

func (h *PrometheusHook) OnWebhookQueued(ctx context.Context, event WebhookQueuedEvent) {
	// Prometheus doesn't track queued events separately
}

func (h *PrometheusHook) OnWebhookDelivered(ctx context.Context, event WebhookDeliveredEvent) {
	h.metrics.ObserveWebhook(event.EventType, "success", event.Duration, event.Attempts, false)
}

func (h *PrometheusHook) OnWebhookFailed(ctx context.Context, event WebhookFailedEvent) {
	status := "failed"
	if !event.FinalFailure {
		status = "retry"
	}
	h.metrics.ObserveWebhook(event.EventType, status, 0, event.Attempts, event.FinalFailure)
}

func (h *PrometheusHook) OnWebhookRetried(ctx context.Context, event WebhookRetriedEvent) {
	// Retries are tracked in OnWebhookFailed
}

// ===============================================
// RefundHook Implementation
// ===============================================

func (h *PrometheusHook) OnRefundRequested(ctx context.Context, event RefundRequestedEvent) {
	// Prometheus doesn't track "requested" events separately
}

func (h *PrometheusHook) OnRefundProcessed(ctx context.Context, event RefundProcessedEvent) {
	status := "success"
	if !event.Success {
		status = "failed"
	}

	h.metrics.ObserveRefund(status, event.Amount, event.Token, event.Duration, "x402")
}

// ===============================================
// CartHook Implementation
// ===============================================

func (h *PrometheusHook) OnCartCreated(ctx context.Context, event CartCreatedEvent) {
	// Prometheus doesn't track "created" events separately
}

func (h *PrometheusHook) OnCartCheckout(ctx context.Context, event CartCheckoutEvent) {
	h.metrics.ObserveCartCheckout(event.Status, event.ItemCount)
}

// ===============================================
// RPCHook Implementation
// ===============================================

func (h *PrometheusHook) OnRPCCall(ctx context.Context, event RPCCallEvent) {
	var err error
	if !event.Success {
		err = &rpcError{errorType: event.ErrorType}
	}
	h.metrics.ObserveRPCCall(event.Method, event.Network, event.Duration, err)
}

// ===============================================
// DatabaseHook Implementation
// ===============================================

func (h *PrometheusHook) OnDatabaseQuery(ctx context.Context, event DatabaseQueryEvent) {
	h.metrics.ObserveDBQuery(event.Operation, event.Backend, event.Duration)
}

// rpcError is a minimal error type for Prometheus hook.
type rpcError struct {
	errorType string
}

func (e *rpcError) Error() string {
	return e.errorType
}
