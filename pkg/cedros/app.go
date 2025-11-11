package cedros

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/CedrosPay/server/internal/callbacks"
	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/coupons"
	"github.com/CedrosPay/server/internal/httpserver"
	"github.com/CedrosPay/server/internal/idempotency"
	"github.com/CedrosPay/server/internal/lifecycle"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/metrics"
	"github.com/CedrosPay/server/internal/paywall"
	"github.com/CedrosPay/server/internal/products"
	"github.com/CedrosPay/server/internal/storage"
	stripesvc "github.com/CedrosPay/server/internal/stripe"
	"github.com/CedrosPay/server/pkg/x402"
	"github.com/CedrosPay/server/pkg/x402/solana"
	"github.com/prometheus/client_golang/prometheus"
)

// App wires the Cedros paywall components for reuse or standalone serving.
type App struct {
	Config           *config.Config
	Store            storage.Store
	Verifier         x402.Verifier
	Notifier         callbacks.Notifier
	Paywall          *paywall.Service
	Stripe           *stripesvc.Client
	CartService      *stripesvc.CartService // NEW: Cart service for multi-item checkouts
	Coupons          coupons.Repository     // NEW: Coupon repository
	IdempotencyStore *idempotency.MemoryStore

	router           chi.Router
	resourceManager  *lifecycle.Manager
	metricsCollector *metrics.Metrics
}

// Option configures App construction.
type Option func(*options)

type options struct {
	store    storage.Store
	notifier callbacks.Notifier
	verifier x402.Verifier
	router   chi.Router
}

// WithStore sets a custom storage backend.
func WithStore(store storage.Store) Option {
	return func(o *options) {
		o.store = store
	}
}

// WithNotifier injects a payment callback notifier.
func WithNotifier(notifier callbacks.Notifier) Option {
	return func(o *options) {
		o.notifier = notifier
	}
}

// WithVerifier injects a custom x402 verifier (responsible for validation and settlement).
func WithVerifier(verifier x402.Verifier) Option {
	return func(o *options) {
		o.verifier = verifier
	}
}

// WithRouter allows callers to provide an existing chi.Router to register routes onto.
func WithRouter(router chi.Router) Option {
	return func(o *options) {
		o.router = router
	}
}

// NewApp assembles Cedros paywall services for embedding.
func NewApp(cfg *config.Config, opts ...Option) (*App, error) {
	if cfg == nil {
		return nil, errors.New("cedros: config required")
	}

	optState := options{}
	for _, opt := range opts {
		opt(&optState)
	}

	app := &App{
		Config:          cfg,
		resourceManager: lifecycle.NewManager(),
	}

	if optState.store != nil {
		app.Store = optState.store
	} else {
		app.Store = storage.NewMemoryStore()
		app.resourceManager.Register("storage", app.Store)
		log.Warn().
			Msg("cedros: defaulting to in-memory store – do not use this backend in production")
	}

	// Initialize Prometheus metrics collector (needed for callback notifier)
	metricsCollector := metrics.New(prometheus.DefaultRegisterer)
	app.metricsCollector = metricsCollector

	if optState.notifier != nil {
		app.Notifier = optState.notifier
	} else {
		// Initialize DLQ store for failed webhooks (if enabled)
		var dlqStore callbacks.DLQStore
		if cfg.Callbacks.DLQEnabled {
			var err error
			dlqStore, err = callbacks.NewFileDLQStore(cfg.Callbacks.DLQPath)
			if err != nil {
				return nil, fmt.Errorf("init DLQ store: %w", err)
			}
		}

		// Convert config retry settings to callbacks.RetryConfig
		retryConfig := callbacks.RetryConfig{
			MaxAttempts:     cfg.Callbacks.Retry.MaxAttempts,
			InitialInterval: cfg.Callbacks.Retry.InitialInterval.Duration,
			MaxInterval:     cfg.Callbacks.Retry.MaxInterval.Duration,
			Multiplier:      cfg.Callbacks.Retry.Multiplier,
			Timeout:         cfg.Callbacks.Timeout.Duration,
		}

		// Create retryable callback notifier with metrics and optional DLQ
		callbackOpts := []callbacks.RetryOption{
			callbacks.WithRetryConfig(retryConfig),
			callbacks.WithMetrics(metricsCollector), // Add metrics for webhook observability
		}
		if dlqStore != nil {
			callbackOpts = append(callbackOpts, callbacks.WithDLQStore(dlqStore))
		}
		app.Notifier = callbacks.NewRetryableClient(cfg.Callbacks, callbackOpts...)
	}

	if optState.verifier != nil {
		app.Verifier = optState.verifier
	} else {
		verifier, err := solana.NewSolanaVerifier(cfg.X402.RPCURL, cfg.X402.WSURL)
		if err != nil {
			return nil, err
		}
		app.Verifier = verifier
		app.resourceManager.RegisterFunc("solana-verifier", func() error {
			verifier.Close()
			return nil
		})
	}

	// Initialize product repository based on config
	productRepository, err := products.NewRepository(cfg.Paywall)
	if err != nil {
		return nil, err
	}
	app.resourceManager.Register("product-repository", productRepository)

	// Initialize coupon repository based on config
	couponRepository, err := coupons.NewRepository(cfg.Coupons)
	if err != nil {
		return nil, err
	}
	app.resourceManager.Register("coupon-repository", couponRepository)

	// Use the metrics collector created earlier (for consistency across all services)
	app.Paywall = paywall.NewService(cfg, app.Store, app.Verifier, app.Notifier, productRepository, couponRepository, metricsCollector)
	app.Stripe = stripesvc.NewClient(cfg.Stripe, app.Store, app.Notifier, couponRepository, metricsCollector)

	// NEW: Create cart service for multi-item checkouts
	app.CartService = stripesvc.NewCartService(cfg.Stripe, app.Store, app.Notifier, couponRepository, metricsCollector)

	// NEW: Store coupon repository in app
	app.Coupons = couponRepository

	if optState.router != nil {
		app.router = optState.router
	} else {
		app.router = chi.NewRouter()
	}

	// Create RPC proxy handlers for frontend endpoints
	rpcProxy := httpserver.NewRPCProxyHandlers(cfg)

	// Create shared idempotency store (single goroutine for cleanup)
	app.IdempotencyStore = idempotency.NewMemoryStore()

	// Register cleanup for idempotency store
	app.resourceManager.RegisterFunc("idempotency-store", func() error {
		app.IdempotencyStore.Stop()
		return nil
	})

	// Create logger for HTTP server
	appLogger := logger.New(logger.Config{
		Level:       cfg.Logging.Level,
		Format:      cfg.Logging.Format,
		Service:     "cedros-pay-embedded",
		Environment: cfg.Logging.Environment,
	})

	httpserver.ConfigureRouter(app.router, cfg, app.Paywall, app.Stripe, app.Verifier, rpcProxy, app.CartService, app.Coupons, app.IdempotencyStore, metricsCollector, appLogger)

	return app, nil
}

// Router returns the chi router with Cedros routes registered.
func (a *App) Router() chi.Router {
	return a.router
}

// Handler exposes the router as an http.Handler.
func (a *App) Handler() http.Handler {
	return a.router
}

// Close releases resources owned by the app (verifier, etc).
func (a *App) Close() error {
	return a.resourceManager.Close()
}

// RegisterRoutes attaches Cedros endpoints to the provided router using an existing App.
func RegisterRoutes(router chi.Router, app *App) {
	if router == nil || app == nil {
		return
	}
	// Create RPC proxy handlers for frontend endpoints
	rpcProxy := httpserver.NewRPCProxyHandlers(app.Config)

	// Create logger for HTTP server
	appLogger := logger.New(logger.Config{
		Level:       app.Config.Logging.Level,
		Format:      app.Config.Logging.Format,
		Service:     "cedros-pay-embedded",
		Environment: app.Config.Logging.Environment,
	})

	// Reuse the app's metrics collector (already registered in NewApp)
	collector := app.metricsCollector
	if collector == nil {
		collector = metrics.New(prometheus.DefaultRegisterer)
	}

	// Reuse the app's idempotency store (already created and managed by app lifecycle)
	httpserver.ConfigureRouter(router, app.Config, app.Paywall, app.Stripe, app.Verifier, rpcProxy, app.CartService, app.Coupons, app.IdempotencyStore, collector, appLogger)
}

// NewHandler is a convenience that constructs an App and returns its handler.
func NewHandler(cfg *config.Config, opts ...Option) (http.Handler, func(context.Context) error, error) {
	app, err := NewApp(cfg, opts...)
	if err != nil {
		return nil, nil, err
	}
	shutdown := func(context.Context) error {
		return app.Close()
	}
	return app.Handler(), shutdown, nil
}

// Config is an exported alias of the internal configuration struct for embedding use.
type Config = config.Config

// LoadConfig wraps the internal loader for consumers embedding Cedros Pay.
func LoadConfig(path string) (*config.Config, error) {
	return config.Load(path)
}
