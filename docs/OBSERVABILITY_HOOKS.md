# Observability Hooks

Cedros Pay provides a flexible plugin system for custom instrumentation, allowing enterprise customers to integrate their preferred observability platforms (DataDog, New Relic, OpenTelemetry, etc.) without forking the codebase.

## Overview

The observability hooks system dispatches events at key lifecycle points (payment, webhook, refund, etc.) to registered hook implementations. This allows you to:

- ✅ **Emit custom traces** to DataDog APM, New Relic, or OpenTelemetry
- ✅ **Send events** to your company's event bus (Kafka, SQS, EventBridge)
- ✅ **Log structured events** to your centralized logging system
- ✅ **Maintain existing Prometheus metrics** (backward compatible)

## Architecture

```
┌──────────────────┐
│   HTTP Handler   │
└────────┬─────────┘
         │
         v
┌──────────────────────────────────────────────────┐
│         Observability Registry                   │
│  (Dispatches events to all registered hooks)     │
└──────┬──────────┬──────────┬────────────────────┘
       │          │          │
       v          v          v
┌──────────┐ ┌──────────┐ ┌──────────────────┐
│Prometheus│ │ DataDog  │ │ Custom Event Bus │
│   Hook   │ │   Hook   │ │      Hook        │
└──────────┘ └──────────┘ └──────────────────┘
```

## Hook Types

### 1. PaymentHook
Receives events during the payment lifecycle:
- `OnPaymentStarted` - Payment request received
- `OnPaymentCompleted` - Payment succeeded or failed
- `OnPaymentSettled` - On-chain settlement confirmed

### 2. WebhookHook
Receives events during webhook delivery:
- `OnWebhookQueued` - Webhook added to queue
- `OnWebhookDelivered` - Successful delivery
- `OnWebhookFailed` - Delivery failure
- `OnWebhookRetried` - Retry scheduled

### 3. RefundHook
Receives events during refunds:
- `OnRefundRequested` - Refund requested
- `OnRefundProcessed` - Refund processed (success/failure)

### 4. CartHook
Receives events during cart operations:
- `OnCartCreated` - Cart quote created
- `OnCartCheckout` - Checkout attempted

### 5. RPCHook
Receives events from blockchain RPC calls:
- `OnRPCCall` - RPC call with latency and error info

### 6. DatabaseHook
Receives events from database operations:
- `OnDatabaseQuery` - Database query with performance metrics

## Quick Start

### 1. Using the Built-in Logging Hook

```go
import (
    "github.com/CedrosPay/server/internal/observability"
    "github.com/CedrosPay/server/internal/observability/examples"
    "github.com/rs/zerolog"
)

func main() {
    logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

    // Create hook registry
    registry := observability.NewRegistry(logger)

    // Register logging hook (for development/debugging)
    loggingHook := examples.NewLoggingHook(logger)
    registry.RegisterPaymentHook(loggingHook)
    registry.RegisterWebhookHook(loggingHook)

    // Use registry in your handlers...
}
```

### 2. Maintaining Prometheus Metrics (Backward Compatible)

```go
import (
    "github.com/CedrosPay/server/internal/metrics"
    "github.com/CedrosPay/server/internal/observability"
)

func main() {
    // Create Prometheus metrics
    prometheusMetrics := metrics.New(nil)

    // Create hook registry
    registry := observability.NewRegistry(logger)

    // Register Prometheus hook (maintains existing metrics)
    prometheusHook := observability.NewPrometheusHook(prometheusMetrics)
    registry.RegisterPaymentHook(prometheusHook)
    registry.RegisterWebhookHook(prometheusHook)
    registry.RegisterRefundHook(prometheusHook)
    registry.RegisterCartHook(prometheusHook)
    registry.RegisterRPCHook(prometheusHook)
    registry.RegisterDatabaseHook(prometheusHook)
}
```

### 3. Emitting Events from Handlers

```go
// In your payment handler
func (h *PaymentHandler) ProcessPayment(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    startTime := time.Now()

    // Emit payment started event
    h.observability.EmitPaymentStarted(ctx, observability.PaymentStartedEvent{
        Timestamp:  startTime,
        PaymentID:  paymentID,
        Method:     "x402",
        ResourceID: resourceID,
        Amount:     amount,
        Token:      "USDC",
    })

    // Process payment...

    // Emit payment completed event
    h.observability.EmitPaymentCompleted(ctx, observability.PaymentCompletedEvent{
        Timestamp:     time.Now(),
        PaymentID:     paymentID,
        Method:        "x402",
        ResourceID:    resourceID,
        Success:       true,
        Amount:        amount,
        Token:         "USDC",
        Duration:      time.Since(startTime),
        TransactionID: signature,
    })
}
```

## Creating Custom Hooks

### Example: Event Bus Hook

```go
package customhooks

import (
    "context"
    "encoding/json"

    "github.com/CedrosPay/server/internal/observability"
    "github.com/aws/aws-sdk-go/service/sqs"
)

// EventBusHook sends observability events to an SQS queue.
type EventBusHook struct {
    sqsClient *sqs.SQS
    queueURL  string
}

func NewEventBusHook(sqsClient *sqs.SQS, queueURL string) *EventBusHook {
    return &EventBusHook{
        sqsClient: sqsClient,
        queueURL:  queueURL,
    }
}

func (h *EventBusHook) Name() string {
    return "event-bus"
}

// Implement PaymentHook interface
func (h *EventBusHook) OnPaymentStarted(ctx context.Context, event observability.PaymentStartedEvent) {
    // Marshal event to JSON
    payload, _ := json.Marshal(map[string]interface{}{
        "eventType": "payment.started",
        "timestamp": event.Timestamp,
        "data":      event,
    })

    // Send to SQS
    h.sqsClient.SendMessage(&sqs.SendMessageInput{
        QueueUrl:    &h.queueURL,
        MessageBody: aws.String(string(payload)),
    })
}

func (h *EventBusHook) OnPaymentCompleted(ctx context.Context, event observability.PaymentCompletedEvent) {
    payload, _ := json.Marshal(map[string]interface{}{
        "eventType": "payment.completed",
        "timestamp": event.Timestamp,
        "data":      event,
    })

    h.sqsClient.SendMessage(&sqs.SendMessageInput{
        QueueUrl:    &h.queueURL,
        MessageBody: aws.String(string(payload)),
    })
}

func (h *EventBusHook) OnPaymentSettled(ctx context.Context, event observability.PaymentSettledEvent) {
    // Similar implementation...
}
```

### Example: DataDog APM Integration

See `internal/observability/examples/datadog_hook.go` for a complete template.

To integrate with DataDog:

```go
import (
    "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
    "github.com/CedrosPay/server/internal/observability/examples"
)

func main() {
    // Initialize DataDog tracer
    tracer.Start(
        tracer.WithService("cedros-pay"),
        tracer.WithEnv("production"),
        tracer.WithAgentAddr("datadog-agent:8126"),
    )
    defer tracer.Stop()

    // Register DataDog hook
    ddHook := examples.NewDataDogHook()
    registry.RegisterPaymentHook(ddHook)
    registry.RegisterWebhookHook(ddHook)
}
```

### Example: OpenTelemetry Integration

See `internal/observability/examples/opentelemetry_hook.go` for a complete template.

To integrate with OpenTelemetry:

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/jaeger"
    "go.opentelemetry.io/otel/sdk/trace"
    "github.com/CedrosPay/server/internal/observability/examples"
)

func main() {
    // Initialize OpenTelemetry
    exporter, _ := jaeger.New(jaeger.WithCollectorEndpoint())
    tp := trace.NewTracerProvider(trace.WithBatcher(exporter))
    otel.SetTracerProvider(tp)

    // Register OpenTelemetry hook
    otelHook := examples.NewOpenTelemetryHook()
    registry.RegisterPaymentHook(otelHook)
}
```

## Event Schema Reference

### PaymentStartedEvent
```go
type PaymentStartedEvent struct {
    Timestamp   time.Time
    PaymentID   string
    Method      string // "stripe" or "x402"
    ResourceID  string
    Amount      int64  // Atomic units (cents, lamports)
    Token       string // "USD", "USDC", etc.
    Wallet      string // Payer wallet (for x402)
    Metadata    map[string]string
}
```

### PaymentCompletedEvent
```go
type PaymentCompletedEvent struct {
    Timestamp      time.Time
    PaymentID      string
    Method         string
    ResourceID     string
    Success        bool
    ErrorReason    string // Set if Success=false
    Amount         int64
    Token          string
    Wallet         string
    Duration       time.Duration
    TransactionID  string // Blockchain tx or Stripe session ID
    Metadata       map[string]string
}
```

### WebhookDeliveredEvent
```go
type WebhookDeliveredEvent struct {
    Timestamp   time.Time
    WebhookID   string
    EventType   string // "payment" or "refund"
    URL         string
    EventID     string // Idempotency key
    Attempts    int
    Duration    time.Duration
    StatusCode  int
}
```

See `internal/observability/hooks.go` for complete event schemas.

## Best Practices

### 1. Panic Recovery
The registry automatically recovers from panics in hook implementations, so one bad hook won't crash the server:

```go
// This hook will panic, but won't crash the server
type BuggyHook struct{}

func (h *BuggyHook) OnPaymentStarted(ctx context.Context, event PaymentStartedEvent) {
    panic("oops") // Registry will recover and log the error
}
```

### 2. Non-Blocking Hooks
Hook methods should execute quickly to avoid blocking the request path. For expensive operations (e.g., network calls), use goroutines:

```go
func (h *SlowHook) OnPaymentCompleted(ctx context.Context, event PaymentCompletedEvent) {
    // Don't block the handler - emit async
    go func() {
        h.sendToExternalAPI(event)
    }()
}
```

### 3. Context Propagation
Use the provided context for tracing and cancellation:

```go
func (h *CustomHook) OnPaymentStarted(ctx context.Context, event PaymentStartedEvent) {
    // Extract trace context
    span := trace.SpanFromContext(ctx)

    // Add custom attributes
    span.SetAttributes(
        attribute.String("payment.id", event.PaymentID),
    )
}
```

### 4. Hook Naming
Use descriptive names for debugging:

```go
func (h *MyHook) Name() string {
    return "my-company-event-emitter"
}
```

The name appears in logs when hooks panic or are registered.

## Migrating from Direct Metrics Calls

**Before:**
```go
metrics.ObservePayment("x402", "resource_1", true, duration, 1000, "USDC")
```

**After:**
```go
observability.EmitPaymentCompleted(ctx, PaymentCompletedEvent{
    Timestamp:  time.Now(),
    PaymentID:  paymentID,
    Method:     "x402",
    ResourceID: "resource_1",
    Success:    true,
    Amount:     1000,
    Token:      "USDC",
    Duration:   duration,
})
```

The `PrometheusHook` will automatically convert events to Prometheus metrics, maintaining backward compatibility.

## Troubleshooting

### Hook Not Receiving Events
- Verify the hook is registered: Check logs for "registered [type] hook" messages
- Ensure the hook implements the correct interface (`PaymentHook`, `WebhookHook`, etc.)
- Check that events are being emitted from handlers

### Hook Panicking
- Check logs for "observability hook panicked (recovered)" messages
- The panic error will be logged with the hook name
- Other hooks will continue to receive events

### Performance Impact
- Hooks run synchronously in the request path - keep them fast
- Use goroutines for expensive operations (network calls, database writes)
- Monitor hook execution time in production

## FAQ

**Q: Can I use multiple observability platforms simultaneously?**
A: Yes! Register multiple hooks (e.g., Prometheus + DataDog + Custom Event Bus).

**Q: Are hooks required?**
A: No. If no hooks are registered, events are silently discarded.

**Q: Can I filter which events a hook receives?**
A: Yes. Your hook implementation can check event fields and return early.

**Q: Is there a performance overhead?**
A: Minimal. Event emission is fast (struct copying), and hooks can be async.

**Q: Can I modify the event data in a hook?**
A: No. Events are passed by value, so modifications won't affect other hooks.

## Support

For questions or issues with the observability hooks system:
- GitHub Issues: https://github.com/CedrosPay/server/issues
- Documentation: https://docs.cedros.pay/observability

## Example: Full Integration

```go
package main

import (
    "github.com/CedrosPay/server/internal/metrics"
    "github.com/CedrosPay/server/internal/observability"
    "github.com/CedrosPay/server/internal/observability/examples"
    "github.com/rs/zerolog"
)

func main() {
    logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

    // Create observability registry
    registry := observability.NewRegistry(logger)

    // 1. Prometheus (backward compatible)
    prometheusMetrics := metrics.New(nil)
    prometheusHook := observability.NewPrometheusHook(prometheusMetrics)
    registry.RegisterPaymentHook(prometheusHook)
    registry.RegisterWebhookHook(prometheusHook)
    registry.RegisterRefundHook(prometheusHook)

    // 2. Logging (for development)
    loggingHook := examples.NewLoggingHook(logger)
    registry.RegisterPaymentHook(loggingHook)

    // 3. Custom event bus (your implementation)
    eventBusHook := NewEventBusHook(sqsClient, queueURL)
    registry.RegisterPaymentHook(eventBusHook)

    // Pass registry to your HTTP server
    server := NewHTTPServer(registry)
    server.Start()
}
```
