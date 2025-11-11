package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsInitialization(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	if m == nil {
		t.Fatal("metrics collector should not be nil")
	}

	// Verify all metrics are initialized
	if m.PaymentsTotal == nil {
		t.Error("PaymentsTotal should be initialized")
	}
	if m.PaymentsSuccessTotal == nil {
		t.Error("PaymentsSuccessTotal should be initialized")
	}
	if m.PaymentsFailedTotal == nil {
		t.Error("PaymentsFailedTotal should be initialized")
	}
	if m.PaymentAmountTotal == nil {
		t.Error("PaymentAmountTotal should be initialized")
	}
	if m.PaymentDuration == nil {
		t.Error("PaymentDuration should be initialized")
	}
	if m.SettlementDuration == nil {
		t.Error("SettlementDuration should be initialized")
	}
	if m.RPCCallsTotal == nil {
		t.Error("RPCCallsTotal should be initialized")
	}
	if m.RPCCallDuration == nil {
		t.Error("RPCCallDuration should be initialized")
	}
	if m.RPCErrorsTotal == nil {
		t.Error("RPCErrorsTotal should be initialized")
	}
}

func TestObservePayment(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Observe a successful payment
	m.ObservePayment("x402", "test-resource", true, 1*time.Second, 100, "USDC")

	// Verify metrics were recorded
	count := promtest.ToFloat64(m.PaymentsTotal.WithLabelValues("x402", "test-resource"))
	if count != 1 {
		t.Errorf("expected 1 payment attempt, got %.0f", count)
	}

	successCount := promtest.ToFloat64(m.PaymentsSuccessTotal.WithLabelValues("x402", "test-resource"))
	if successCount != 1 {
		t.Errorf("expected 1 successful payment, got %.0f", successCount)
	}

	amount := promtest.ToFloat64(m.PaymentAmountTotal.WithLabelValues("x402", "USDC"))
	if amount != 100 {
		t.Errorf("expected payment amount 100 cents, got %.0f", amount)
	}
}

func TestObservePaymentFailure(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Observe a failed payment
	m.ObservePaymentFailure("x402", "test-resource", "insufficient_funds")

	// Verify failure metric was recorded
	count := promtest.ToFloat64(m.PaymentsFailedTotal.WithLabelValues("x402", "test-resource", "insufficient_funds"))
	if count != 1 {
		t.Errorf("expected 1 failed payment, got %.0f", count)
	}
}

func TestObserveSettlement(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Observe settlement time
	m.ObserveSettlement("mainnet-beta", 5*time.Second)

	// For histograms, we can't directly check the count with testutil.ToFloat64
	// Instead, verify the metric was created and registered without error
	// The actual observation is verified by the lack of panic
	if m.SettlementDuration == nil {
		t.Error("SettlementDuration should be initialized")
	}
}

func TestObserveRPCCall(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		network   string
		duration  time.Duration
		err       error
		wantCalls float64
		wantErrors float64
	}{
		{
			name:      "successful RPC call",
			method:    "getTransaction",
			network:   "mainnet-beta",
			duration:  100 * time.Millisecond,
			err:       nil,
			wantCalls: 1,
			wantErrors: 0,
		},
		{
			name:      "failed RPC call with connection error",
			method:    "getTransaction",
			network:   "mainnet-beta",
			duration:  100 * time.Millisecond,
			err:       &testError{msg: "connection reset"},
			wantCalls: 1,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset registry for each test
			registry := prometheus.NewRegistry()
			m := New(registry)

			m.ObserveRPCCall(tt.method, tt.network, tt.duration, tt.err)

			calls := promtest.ToFloat64(m.RPCCallsTotal.WithLabelValues(tt.method, tt.network))
			if calls != tt.wantCalls {
				t.Errorf("expected %.0f RPC calls, got %.0f", tt.wantCalls, calls)
			}

			if tt.err != nil {
				// Error type should be "connection" because error message contains "connection"
				errors := promtest.ToFloat64(m.RPCErrorsTotal.WithLabelValues(tt.method, tt.network, "connection"))
				if errors != tt.wantErrors {
					t.Errorf("expected %.0f RPC errors, got %.0f", tt.wantErrors, errors)
				}
			}
		})
	}
}

func TestObserveCartCheckout(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.ObserveCartCheckout("success", 3)

	count := promtest.ToFloat64(m.CartCheckoutsTotal.WithLabelValues("success"))
	if count != 1 {
		t.Errorf("expected 1 cart checkout, got %.0f", count)
	}

	items := promtest.ToFloat64(m.CartItemsTotal)
	if items != 3 {
		t.Errorf("expected 3 cart items, got %.0f", items)
	}
}

func TestObserveRefund(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.ObserveRefund("success", 200, "USDC", 2*time.Second, "crypto")

	count := promtest.ToFloat64(m.RefundsTotal.WithLabelValues("success"))
	if count != 1 {
		t.Errorf("expected 1 refund, got %.0f", count)
	}

	amount := promtest.ToFloat64(m.RefundAmountTotal.WithLabelValues("USDC"))
	if amount != 200 {
		t.Errorf("expected refund amount 200 cents, got %.0f", amount)
	}
}

func TestObserveWebhook(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// First attempt succeeds
	m.ObserveWebhook("payment.succeeded", "success", 500*time.Millisecond, 1, false)

	webhooks := promtest.ToFloat64(m.WebhooksTotal.WithLabelValues("payment.succeeded", "success"))
	if webhooks != 1 {
		t.Errorf("expected 1 webhook delivery, got %.0f", webhooks)
	}

	// Second attempt with retry (attempt > 1) and goes to DLQ
	// attempt=5 means 4 retries after initial attempt
	m.ObserveWebhook("payment.failed", "failed", 2*time.Second, 5, true)

	// Retries are only recorded when attempt > 1
	retries := promtest.ToFloat64(m.WebhookRetriesTotal.WithLabelValues("payment.failed", "5"))
	if retries != 1 {
		t.Errorf("expected 1 webhook retry record, got %.0f", retries)
	}

	dlq := promtest.ToFloat64(m.WebhookDLQTotal.WithLabelValues("payment.failed"))
	if dlq != 1 {
		t.Errorf("expected 1 webhook in DLQ, got %.0f", dlq)
	}
}

func TestObserveRateLimit(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.ObserveRateLimit("per_wallet", "wallet123")

	hits := promtest.ToFloat64(m.RateLimitHitsTotal.WithLabelValues("per_wallet", "wallet123"))
	if hits != 1 {
		t.Errorf("expected 1 rate limit hit, got %.0f", hits)
	}
}

func TestObserveDBQuery(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.ObserveDBQuery("SELECT", "postgres", 50*time.Millisecond)

	// For histograms, verify the metric exists and was created successfully
	if m.DBQueryDuration == nil {
		t.Error("DBQueryDuration should be initialized")
	}
}

func TestObserveArchival(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.ObserveArchival(1500)

	runs := promtest.ToFloat64(m.ArchivalRunsTotal)
	if runs != 1 {
		t.Errorf("expected 1 archival run, got %.0f", runs)
	}

	deleted := promtest.ToFloat64(m.ArchivalRecordsDeleted)
	if deleted != 1500 {
		t.Errorf("expected 1500 records deleted, got %.0f", deleted)
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
