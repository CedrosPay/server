package metrics

import (
	"time"
)

// MeasureDBQuery wraps a database operation with timing instrumentation.
// Usage:
//
//	defer metrics.MeasureDBQuery(m, "get_product", "postgres")()
//
// Or with explicit start time:
//
//	start := time.Now()
//	// ... do database work ...
//	metrics.RecordDBQuery(m, "get_product", "postgres", time.Since(start))
func MeasureDBQuery(m *Metrics, operation, backend string) func() {
	if m == nil {
		return func() {}
	}
	start := time.Now()
	return func() {
		m.ObserveDBQuery(operation, backend, time.Since(start))
	}
}

// RecordDBQuery records a database query duration directly (when timing is already captured).
func RecordDBQuery(m *Metrics, operation, backend string, duration time.Duration) {
	if m == nil {
		return
	}
	m.ObserveDBQuery(operation, backend, duration)
}
