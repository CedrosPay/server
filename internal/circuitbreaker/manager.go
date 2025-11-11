package circuitbreaker

import (
	"fmt"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/sony/gobreaker"
)

// ServiceType identifies different external services for circuit breaker isolation.
type ServiceType string

const (
	ServiceSolanaRPC ServiceType = "solana_rpc"
	ServiceStripe    ServiceType = "stripe_api"
	ServiceWebhook   ServiceType = "webhook"
)

// Manager manages circuit breakers for different external services.
// Provides bulkhead isolation - each service has its own circuit breaker
// to prevent cascading failures across service boundaries.
type Manager struct {
	breakers map[ServiceType]*gobreaker.CircuitBreaker
	config   Config
}

// Config holds circuit breaker configuration for all services.
type Config struct {
	// Global enable/disable toggle
	Enabled bool

	// Solana RPC circuit breaker config
	SolanaRPC BreakerConfig

	// Stripe API circuit breaker config
	StripeAPI BreakerConfig

	// Webhook delivery circuit breaker config
	Webhook BreakerConfig
}

// BreakerConfig configures a single circuit breaker.
type BreakerConfig struct {
	// MaxRequests is the maximum number of requests allowed to pass through
	// when the circuit breaker is half-open. Default: 1
	MaxRequests uint32

	// Interval is the cyclic period in closed state to clear the internal counts.
	// If 0, never clears. Default: 60s
	Interval time.Duration

	// Timeout is the period of the open state after which the state becomes half-open.
	// Default: 30s
	Timeout time.Duration

	// ReadyToTrip is called whenever a request fails in the closed state.
	// If it returns true, the circuit breaker trips to open state.
	// Default: 5 consecutive failures or 50% failure rate over 10 requests
	ConsecutiveFailures uint32
	FailureRatio        float64
	MinRequests         uint32
}

// NewManagerFromConfig creates a circuit breaker manager from application config.
func NewManagerFromConfig(cfg config.CircuitBreakerConfig) *Manager {
	return NewManager(Config{
		Enabled: cfg.Enabled,
		SolanaRPC: BreakerConfig{
			MaxRequests:         cfg.SolanaRPC.MaxRequests,
			Interval:            cfg.SolanaRPC.Interval.Duration,
			Timeout:             cfg.SolanaRPC.Timeout.Duration,
			ConsecutiveFailures: cfg.SolanaRPC.ConsecutiveFailures,
			FailureRatio:        cfg.SolanaRPC.FailureRatio,
			MinRequests:         cfg.SolanaRPC.MinRequests,
		},
		StripeAPI: BreakerConfig{
			MaxRequests:         cfg.StripeAPI.MaxRequests,
			Interval:            cfg.StripeAPI.Interval.Duration,
			Timeout:             cfg.StripeAPI.Timeout.Duration,
			ConsecutiveFailures: cfg.StripeAPI.ConsecutiveFailures,
			FailureRatio:        cfg.StripeAPI.FailureRatio,
			MinRequests:         cfg.StripeAPI.MinRequests,
		},
		Webhook: BreakerConfig{
			MaxRequests:         cfg.Webhook.MaxRequests,
			Interval:            cfg.Webhook.Interval.Duration,
			Timeout:             cfg.Webhook.Timeout.Duration,
			ConsecutiveFailures: cfg.Webhook.ConsecutiveFailures,
			FailureRatio:        cfg.Webhook.FailureRatio,
			MinRequests:         cfg.Webhook.MinRequests,
		},
	})
}

// NewManager creates a circuit breaker manager with the given configuration.
func NewManager(cfg Config) *Manager {
	m := &Manager{
		breakers: make(map[ServiceType]*gobreaker.CircuitBreaker),
		config:   cfg,
	}

	if !cfg.Enabled {
		// Return manager with no breakers (pass-through)
		return m
	}

	// Initialize circuit breakers for each service
	m.breakers[ServiceSolanaRPC] = gobreaker.NewCircuitBreaker(toGobreakerSettings(string(ServiceSolanaRPC), cfg.SolanaRPC))
	m.breakers[ServiceStripe] = gobreaker.NewCircuitBreaker(toGobreakerSettings(string(ServiceStripe), cfg.StripeAPI))
	m.breakers[ServiceWebhook] = gobreaker.NewCircuitBreaker(toGobreakerSettings(string(ServiceWebhook), cfg.Webhook))

	return m
}

// Execute wraps a function call with circuit breaker protection.
// If circuit breaker is disabled or not configured for the service, executes directly.
func (m *Manager) Execute(service ServiceType, fn func() (interface{}, error)) (interface{}, error) {
	if !m.config.Enabled {
		// Circuit breaker disabled - pass through
		return fn()
	}

	breaker, ok := m.breakers[service]
	if !ok {
		// No circuit breaker configured for this service - pass through
		return fn()
	}

	return breaker.Execute(fn)
}

// State returns the current state of a circuit breaker.
// Returns "disabled" if circuit breakers are not enabled or service not found.
func (m *Manager) State(service ServiceType) string {
	if !m.config.Enabled {
		return "disabled"
	}

	breaker, ok := m.breakers[service]
	if !ok {
		return "not_configured"
	}

	return breaker.State().String()
}

// Counts returns the current counts for a circuit breaker.
func (m *Manager) Counts(service ServiceType) Counts {
	if !m.config.Enabled {
		return Counts{}
	}

	breaker, ok := m.breakers[service]
	if !ok {
		return Counts{}
	}

	c := breaker.Counts()
	return Counts{
		Requests:             c.Requests,
		TotalSuccesses:       c.TotalSuccesses,
		TotalFailures:        c.TotalFailures,
		ConsecutiveSuccesses: c.ConsecutiveSuccesses,
		ConsecutiveFailures:  c.ConsecutiveFailures,
	}
}

// Counts represents circuit breaker statistics.
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// toGobreakerSettings converts our config to gobreaker.Settings.
func toGobreakerSettings(name string, cfg BreakerConfig) gobreaker.Settings {
	return gobreaker.Settings{
		Name:        name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip if we've hit consecutive failures threshold
			if cfg.ConsecutiveFailures > 0 && counts.ConsecutiveFailures >= cfg.ConsecutiveFailures {
				return true
			}

			// Trip if we've hit failure ratio threshold (and have minimum requests)
			if cfg.FailureRatio > 0 && cfg.MinRequests > 0 {
				if counts.Requests >= cfg.MinRequests {
					failureRate := float64(counts.TotalFailures) / float64(counts.Requests)
					if failureRate >= cfg.FailureRatio {
						return true
					}
				}
			}

			return false
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// Log state transitions for observability
			fmt.Printf("Circuit breaker %s: %s -> %s\n", name, from.String(), to.String())
		},
	}
}

// DefaultConfig returns sensible defaults for circuit breaker configuration.
func DefaultConfig() Config {
	return Config{
		Enabled: true,
		SolanaRPC: BreakerConfig{
			MaxRequests:         3,
			Interval:            60 * time.Second,
			Timeout:             30 * time.Second,
			ConsecutiveFailures: 5,
			FailureRatio:        0.5,
			MinRequests:         10,
		},
		StripeAPI: BreakerConfig{
			MaxRequests:         3,
			Interval:            60 * time.Second,
			Timeout:             30 * time.Second,
			ConsecutiveFailures: 5,
			FailureRatio:        0.5,
			MinRequests:         10,
		},
		Webhook: BreakerConfig{
			MaxRequests:         5,
			Interval:            60 * time.Second,
			Timeout:             60 * time.Second, // Longer timeout for webhooks
			ConsecutiveFailures: 10,               // More tolerant for webhooks
			FailureRatio:        0.7,
			MinRequests:         20,
		},
	}
}
