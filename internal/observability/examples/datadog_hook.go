package examples

import (
	"context"

	"github.com/CedrosPay/server/internal/observability"
)

// DataDogHook emits events to DataDog APM.
// This is a template implementation - requires DataDog SDK integration.
//
// To use this hook:
//  1. Import DataDog SDK: "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
//  2. Initialize DataDog tracer in main()
//  3. Register this hook with the observability registry
//
// Example integration:
//
//	import "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
//
//	func main() {
//	    tracer.Start(tracer.WithService("cedros-pay"))
//	    defer tracer.Stop()
//
//	    hook := examples.NewDataDogHook()
//	    registry.RegisterPaymentHook(hook)
//	}
type DataDogHook struct {
	// Add DataDog tracer reference here when integrating
	// tracer ddtrace.Tracer
}

// NewDataDogHook creates a hook that emits events to DataDog.
func NewDataDogHook() *DataDogHook {
	return &DataDogHook{}
}

func (h *DataDogHook) Name() string {
	return "datadog"
}

// ===============================================
// PaymentHook Implementation
// ===============================================

func (h *DataDogHook) OnPaymentStarted(ctx context.Context, event observability.PaymentStartedEvent) {
	// Example DataDog integration:
	//
	// span, ctx := tracer.StartSpanFromContext(ctx, "payment.process",
	//     tracer.Tag("payment.id", event.PaymentID),
	//     tracer.Tag("payment.method", event.Method),
	//     tracer.Tag("payment.resource", event.ResourceID),
	//     tracer.Tag("payment.amount", event.Amount),
	//     tracer.Tag("payment.token", event.Token),
	// )
	// defer span.Finish()
	//
	// // Store span in context for OnPaymentCompleted
	// ctx = context.WithValue(ctx, "datadog_span", span)
}

func (h *DataDogHook) OnPaymentCompleted(ctx context.Context, event observability.PaymentCompletedEvent) {
	// Example DataDog integration:
	//
	// // Retrieve span from context (set in OnPaymentStarted)
	// span, ok := ctx.Value("datadog_span").(ddtrace.Span)
	// if !ok {
	//     return
	// }
	//
	// span.SetTag("payment.success", event.Success)
	// span.SetTag("payment.duration_ms", event.Duration.Milliseconds())
	// span.SetTag("payment.tx_id", event.TransactionID)
	//
	// if !event.Success {
	//     span.SetTag("error", true)
	//     span.SetTag("error.msg", event.ErrorReason)
	// }
}

func (h *DataDogHook) OnPaymentSettled(ctx context.Context, event observability.PaymentSettledEvent) {
	// Example DataDog integration:
	//
	// span, _ := tracer.StartSpanFromContext(ctx, "payment.settlement",
	//     tracer.Tag("payment.id", event.PaymentID),
	//     tracer.Tag("payment.network", event.Network),
	//     tracer.Tag("payment.tx_id", event.TransactionID),
	//     tracer.Tag("payment.confirmations", event.Confirmations),
	//     tracer.Tag("settlement.duration_ms", event.SettlementDuration.Milliseconds()),
	// )
	// defer span.Finish()
}

// ===============================================
// WebhookHook Implementation
// ===============================================

func (h *DataDogHook) OnWebhookQueued(ctx context.Context, event observability.WebhookQueuedEvent) {
	// Similar pattern - create span with webhook metadata
}

func (h *DataDogHook) OnWebhookDelivered(ctx context.Context, event observability.WebhookDeliveredEvent) {
	// Track successful webhook delivery with status code
}

func (h *DataDogHook) OnWebhookFailed(ctx context.Context, event observability.WebhookFailedEvent) {
	// Track webhook failures with error details
}

func (h *DataDogHook) OnWebhookRetried(ctx context.Context, event observability.WebhookRetriedEvent) {
	// Track webhook retry attempts and backoff
}

// ===============================================
// RefundHook Implementation
// ===============================================

func (h *DataDogHook) OnRefundRequested(ctx context.Context, event observability.RefundRequestedEvent) {
	// Track refund requests with metadata
}

func (h *DataDogHook) OnRefundProcessed(ctx context.Context, event observability.RefundProcessedEvent) {
	// Track refund processing outcome
}

// ===============================================
// CartHook Implementation
// ===============================================

func (h *DataDogHook) OnCartCreated(ctx context.Context, event observability.CartCreatedEvent) {
	// Track cart creation
}

func (h *DataDogHook) OnCartCheckout(ctx context.Context, event observability.CartCheckoutEvent) {
	// Track checkout attempts
}

// ===============================================
// RPCHook Implementation
// ===============================================

func (h *DataDogHook) OnRPCCall(ctx context.Context, event observability.RPCCallEvent) {
	// Track RPC calls to blockchain with latency
}

// ===============================================
// DatabaseHook Implementation
// ===============================================

func (h *DataDogHook) OnDatabaseQuery(ctx context.Context, event observability.DatabaseQueryEvent) {
	// Track database query performance
}
