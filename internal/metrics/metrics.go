package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for Cedros Pay.
type Metrics struct {
	// Payment metrics
	PaymentsTotal        *prometheus.CounterVec
	PaymentsSuccessTotal *prometheus.CounterVec
	PaymentsFailedTotal  *prometheus.CounterVec
	PaymentAmountTotal   *prometheus.CounterVec
	PaymentDuration      *prometheus.HistogramVec
	SettlementDuration   *prometheus.HistogramVec

	// RPC call metrics
	RPCCallsTotal   *prometheus.CounterVec
	RPCCallDuration *prometheus.HistogramVec
	RPCErrorsTotal  *prometheus.CounterVec

	// Cart metrics
	CartCheckoutsTotal *prometheus.CounterVec
	CartItemsTotal     prometheus.Counter
	CartAverageValue   prometheus.Gauge

	// Refund metrics
	RefundsTotal      *prometheus.CounterVec
	RefundAmountTotal *prometheus.CounterVec
	RefundDuration    *prometheus.HistogramVec

	// Webhook metrics
	WebhooksTotal       *prometheus.CounterVec
	WebhookRetriesTotal *prometheus.CounterVec
	WebhookDLQTotal     *prometheus.CounterVec
	WebhookDuration     *prometheus.HistogramVec

	// Rate limiting metrics
	RateLimitHitsTotal *prometheus.CounterVec

	// Database metrics
	DBQueryDuration     *prometheus.HistogramVec
	DBConnectionsActive prometheus.Gauge

	// System metrics
	ArchivalRunsTotal      prometheus.Counter
	ArchivalRecordsDeleted prometheus.Counter
}

// New creates and registers all Prometheus metrics.
func New(registry prometheus.Registerer) *Metrics {
	if registry == nil {
		registry = prometheus.DefaultRegisterer
	}

	factory := promauto.With(registry)

	return &Metrics{
		// Payment metrics
		PaymentsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_payments_total",
				Help: "Total number of payment attempts",
			},
			[]string{"method", "resource"},
		),
		PaymentsSuccessTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_payments_success_total",
				Help: "Total number of successful payments",
			},
			[]string{"method", "resource"},
		),
		PaymentsFailedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_payments_failed_total",
				Help: "Total number of failed payments",
			},
			[]string{"method", "resource", "reason"},
		),
		PaymentAmountTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_payment_amount_total",
				Help: "Total payment amount in USD cents",
			},
			[]string{"method", "token"},
		),
		PaymentDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cedros_payment_duration_seconds",
				Help:    "Time taken to process payment (supports p50, p95, p99 percentiles)",
				Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"method", "resource"},
		),
		SettlementDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cedros_settlement_duration_seconds",
				Help:    "Time from payment initiation to on-chain settlement",
				Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
			},
			[]string{"network"},
		),

		// RPC call metrics
		RPCCallsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_rpc_calls_total",
				Help: "Total number of RPC calls to blockchain",
			},
			[]string{"method", "network"},
		),
		RPCCallDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cedros_rpc_call_duration_seconds",
				Help:    "Duration of RPC calls to blockchain (supports p50, p95, p99 percentiles)",
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
			},
			[]string{"method", "network"},
		),
		RPCErrorsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_rpc_errors_total",
				Help: "Total number of RPC errors",
			},
			[]string{"method", "network", "error_type"},
		),

		// Cart metrics
		CartCheckoutsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_cart_checkouts_total",
				Help: "Total number of cart checkouts",
			},
			[]string{"status"},
		),
		CartItemsTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "cedros_cart_items_total",
				Help: "Total number of items added to carts",
			},
		),
		CartAverageValue: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "cedros_cart_average_value_cents",
				Help: "Average cart value in cents",
			},
		),

		// Refund metrics
		RefundsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_refunds_total",
				Help: "Total number of refund requests",
			},
			[]string{"status"},
		),
		RefundAmountTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_refund_amount_total",
				Help: "Total refund amount in USD cents",
			},
			[]string{"token"},
		),
		RefundDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cedros_refund_duration_seconds",
				Help:    "Time taken to process refund",
				Buckets: []float64{1, 5, 10, 30, 60, 300},
			},
			[]string{"method"},
		),

		// Webhook metrics
		WebhooksTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_webhooks_total",
				Help: "Total number of webhook deliveries",
			},
			[]string{"event_type", "status"},
		),
		WebhookRetriesTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_webhook_retries_total",
				Help: "Total number of webhook retry attempts",
			},
			[]string{"event_type", "attempt"},
		),
		WebhookDLQTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_webhook_dlq_total",
				Help: "Total number of webhooks sent to DLQ",
			},
			[]string{"event_type"},
		),
		WebhookDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cedros_webhook_duration_seconds",
				Help:    "Time taken for webhook delivery",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
			},
			[]string{"event_type"},
		),

		// Rate limiting metrics
		RateLimitHitsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cedros_rate_limit_hits_total",
				Help: "Total number of rate limit hits",
			},
			[]string{"limit_type", "identifier"},
		),

		// Database metrics
		DBQueryDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cedros_db_query_duration_seconds",
				Help:    "Database query duration (supports p50, p95, p99 percentiles)",
				Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.5, 1, 2},
			},
			[]string{"operation", "backend"},
		),
		DBConnectionsActive: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "cedros_db_connections_active",
				Help: "Number of active database connections",
			},
		),

		// System metrics
		ArchivalRunsTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "cedros_archival_runs_total",
				Help: "Total number of archival runs",
			},
		),
		ArchivalRecordsDeleted: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "cedros_archival_records_deleted_total",
				Help: "Total number of records deleted by archival",
			},
		),
	}
}

// ObservePayment records a payment attempt and its outcome.
func (m *Metrics) ObservePayment(method, resource string, success bool, duration time.Duration, amountCents int64, token string) {
	m.PaymentsTotal.WithLabelValues(method, resource).Inc()
	if success {
		m.PaymentsSuccessTotal.WithLabelValues(method, resource).Inc()
		m.PaymentAmountTotal.WithLabelValues(method, token).Add(float64(amountCents))
	}
	m.PaymentDuration.WithLabelValues(method, resource).Observe(duration.Seconds())
}

// ObservePaymentFailure records a failed payment with reason.
func (m *Metrics) ObservePaymentFailure(method, resource, reason string) {
	m.PaymentsFailedTotal.WithLabelValues(method, resource, reason).Inc()
}

// ObserveSettlement records blockchain settlement time.
func (m *Metrics) ObserveSettlement(network string, duration time.Duration) {
	m.SettlementDuration.WithLabelValues(network).Observe(duration.Seconds())
}

// ObserveRPCCall records an RPC call to the blockchain.
func (m *Metrics) ObserveRPCCall(method, network string, duration time.Duration, err error) {
	m.RPCCallsTotal.WithLabelValues(method, network).Inc()
	m.RPCCallDuration.WithLabelValues(method, network).Observe(duration.Seconds())

	if err != nil {
		errorType := "unknown"
		// Categorize errors
		if errStr := err.Error(); errStr != "" {
			switch {
			case contains(errStr, "timeout"):
				errorType = "timeout"
			case contains(errStr, "rate limit"):
				errorType = "rate_limit"
			case contains(errStr, "connection"):
				errorType = "connection"
			case contains(errStr, "not found"):
				errorType = "not_found"
			default:
				errorType = "other"
			}
		}
		m.RPCErrorsTotal.WithLabelValues(method, network, errorType).Inc()
	}
}

// ObserveCartCheckout records a cart checkout.
func (m *Metrics) ObserveCartCheckout(status string, itemCount int) {
	m.CartCheckoutsTotal.WithLabelValues(status).Inc()
	m.CartItemsTotal.Add(float64(itemCount))
}

// ObserveRefund records a refund operation.
func (m *Metrics) ObserveRefund(status string, amountCents int64, token string, duration time.Duration, method string) {
	m.RefundsTotal.WithLabelValues(status).Inc()
	if status == "success" {
		m.RefundAmountTotal.WithLabelValues(token).Add(float64(amountCents))
	}
	m.RefundDuration.WithLabelValues(method).Observe(duration.Seconds())
}

// ObserveWebhook records webhook delivery.
func (m *Metrics) ObserveWebhook(eventType, status string, duration time.Duration, attempt int, sentToDLQ bool) {
	m.WebhooksTotal.WithLabelValues(eventType, status).Inc()
	m.WebhookDuration.WithLabelValues(eventType).Observe(duration.Seconds())

	if attempt > 1 {
		m.WebhookRetriesTotal.WithLabelValues(eventType, formatAttempt(attempt)).Inc()
	}

	if sentToDLQ {
		m.WebhookDLQTotal.WithLabelValues(eventType).Inc()
	}
}

// ObserveRateLimit records a rate limit hit.
func (m *Metrics) ObserveRateLimit(limitType, identifier string) {
	m.RateLimitHitsTotal.WithLabelValues(limitType, identifier).Inc()
}

// ObserveDBQuery records a database query.
func (m *Metrics) ObserveDBQuery(operation, backend string, duration time.Duration) {
	m.DBQueryDuration.WithLabelValues(operation, backend).Observe(duration.Seconds())
}

// ObserveArchival records an archival run.
func (m *Metrics) ObserveArchival(recordsDeleted int64) {
	m.ArchivalRunsTotal.Inc()
	m.ArchivalRecordsDeleted.Add(float64(recordsDeleted))
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && contains(s[1:], substr)
}

func formatAttempt(attempt int) string {
	if attempt <= 5 {
		return string(rune('0' + attempt))
	}
	return "5+"
}
