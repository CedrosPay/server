package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/CedrosPay/server/internal/apikey"
	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/coupons"
	"github.com/CedrosPay/server/internal/idempotency"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/metrics"
	"github.com/CedrosPay/server/internal/paywall"
	"github.com/CedrosPay/server/internal/ratelimit"
	stripesvc "github.com/CedrosPay/server/internal/stripe"
	"github.com/CedrosPay/server/internal/subscriptions"
	"github.com/CedrosPay/server/internal/versioning"
	"github.com/CedrosPay/server/pkg/x402"
)

var (
	serverStartTime = time.Now()
)

// Server wires handlers, middleware, and dependencies.
type Server struct {
	handlers
	httpServer *http.Server
}

type handlers struct {
	cfg              *config.Config
	paywall          *paywall.Service
	stripe           *stripesvc.Client
	cartService      *stripesvc.CartService   // Cart service for multi-item checkouts
	verifier         x402.Verifier
	rpcProxy         *rpcProxyHandlers
	couponRepo       coupons.Repository       // Coupon repository
	idempotencyStore idempotency.Store        // Idempotency store for request deduplication
	metrics          *metrics.Metrics         // Prometheus metrics collector
	subscriptions    *subscriptions.Service   // Subscription management service
	logger           zerolog.Logger           // Structured logger
}

// New builds the HTTP server with configured router.
func New(cfg *config.Config, paywallSvc *paywall.Service, stripeClient *stripesvc.Client, cartService *stripesvc.CartService, verifier x402.Verifier, couponRepo coupons.Repository, idempotencyStore idempotency.Store, metricsCollector *metrics.Metrics, subscriptionsSvc *subscriptions.Service, appLogger zerolog.Logger) *Server {
	router := chi.NewRouter()
	rpcProxy := NewRPCProxyHandlers(cfg)

	s := &Server{
		handlers: handlers{
			cfg:              cfg,
			paywall:          paywallSvc,
			stripe:           stripeClient,
			cartService:      cartService,
			verifier:         verifier,
			rpcProxy:         rpcProxy,
			couponRepo:       couponRepo,
			idempotencyStore: idempotencyStore,
			metrics:          metricsCollector,
			subscriptions:    subscriptionsSvc,
			logger:           appLogger,
		},
		httpServer: &http.Server{
			Addr:         cfg.Server.Address,
			ReadTimeout:  cfg.Server.ReadTimeout.Duration,
			WriteTimeout: cfg.Server.WriteTimeout.Duration,
			IdleTimeout:  cfg.Server.IdleTimeout.Duration,
			Handler:      router,
		},
	}

	ConfigureRouter(router, cfg, paywallSvc, stripeClient, verifier, rpcProxy, cartService, couponRepo, idempotencyStore, metricsCollector, subscriptionsSvc, appLogger)

	return s
}

// ConfigureRouter attaches Cedros routes to an existing router.
func ConfigureRouter(router chi.Router, cfg *config.Config, paywallSvc *paywall.Service, stripeClient *stripesvc.Client, verifier x402.Verifier, rpcProxy *rpcProxyHandlers, cartService *stripesvc.CartService, couponRepo coupons.Repository, idempotencyStore idempotency.Store, metricsCollector *metrics.Metrics, subscriptionsSvc *subscriptions.Service, appLogger zerolog.Logger) {
	if router == nil {
		return
	}

	handler := handlers{
		cfg:              cfg,
		paywall:          paywallSvc,
		stripe:           stripeClient,
		cartService:      cartService,
		verifier:         verifier,
		rpcProxy:         rpcProxy,
		couponRepo:       couponRepo,
		idempotencyStore: idempotencyStore,
		metrics:          metricsCollector,
		subscriptions:    subscriptionsSvc,
		logger:           appLogger,
	}

	// RPC proxy handlers are already created and passed in

	if len(cfg.Server.CORSAllowedOrigins) > 0 {
		router.Use(cors.New(cors.Options{
			AllowedOrigins:   cfg.Server.CORSAllowedOrigins,
			AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"*"},
			ExposedHeaders:   []string{"Location"},
			AllowCredentials: false,
			MaxAge:           300,
		}).Handler)
	}

	// Security headers middleware (applied first for all responses)
	router.Use(securityHeadersMiddleware)

	// Add structured logging middleware (BEFORE RequestID for context propagation)
	router.Use(logger.Middleware(appLogger))
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)

	// API version negotiation middleware (adds version to context from Accept header)
	router.Use(versioning.Negotiation)

	// API key authentication middleware (BEFORE rate limiting)
	// Extracts X-API-Key header and stores tier in context for rate limit exemptions
	apiKeyCfg := apikey.Config{
		Enabled: cfg.APIKey.Enabled,
		APIKeys: make(map[string]apikey.Tier),
	}
	for key, tierStr := range cfg.APIKey.Keys {
		apiKeyCfg.APIKeys[key] = apikey.Tier(tierStr)
	}
	router.Use(apikey.Middleware(apiKeyCfg))

	// Rate limiting middleware (applied globally)
	// Convert config to ratelimit.Config
	rateLimitCfg := ratelimit.Config{
		GlobalEnabled:    cfg.RateLimit.GlobalEnabled,
		GlobalLimit:      cfg.RateLimit.GlobalLimit,
		GlobalWindow:     cfg.RateLimit.GlobalWindow.Duration,
		GlobalBurst:      cfg.RateLimit.GlobalLimit / 10, // Burst = 10% of limit
		PerWalletEnabled: cfg.RateLimit.PerWalletEnabled,
		PerWalletLimit:   cfg.RateLimit.PerWalletLimit,
		PerWalletWindow:  cfg.RateLimit.PerWalletWindow.Duration,
		PerWalletBurst:   cfg.RateLimit.PerWalletLimit / 6, // Burst = ~17% of limit
		PerIPEnabled:     cfg.RateLimit.PerIPEnabled,
		PerIPLimit:       cfg.RateLimit.PerIPLimit,
		PerIPWindow:      cfg.RateLimit.PerIPWindow.Duration,
		PerIPBurst:       cfg.RateLimit.PerIPLimit / 6, // Burst = ~17% of limit
		Metrics:          metricsCollector,             // Pass metrics collector to rate limiter
	}
	router.Use(ratelimit.GlobalLimiter(rateLimitCfg))
	router.Use(ratelimit.WalletLimiter(rateLimitCfg))
	router.Use(ratelimit.IPLimiter(rateLimitCfg))

	// NOTE: Timeout middleware is applied selectively per route group below
	// to avoid imposing 60s timeout on lightweight discovery/health endpoints

	// Apply route prefix if configured
	prefix := cfg.Server.RoutePrefix

	// Lightweight endpoints with 5s timeout (health checks, discovery, documentation, metrics)
	router.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(5 * time.Second))
		r.Get("/cedros-health", handler.health)
		r.Get("/.well-known/payment-options", handler.wellKnownPaymentOptions)
		r.Get("/.well-known/agent.json", handler.agentCard)
		r.Get("/openapi.json", handler.openAPISpec)
		r.Post("/resources/list", handler.mcpResourcesList)
		// Prometheus metrics endpoint (respects route prefix to avoid conflicts)
		// Protected by optional admin API key (ADMIN_METRICS_API_KEY env var)
		r.With(adminMetricsAuth(cfg.Server.AdminMetricsAPIKey)).Handle(prefix+"/metrics", promhttp.Handler())
	})

	// Idempotency middleware (24 hour cache for payment requests)
	idempotencyMW := idempotency.Middleware(idempotencyStore, 24*time.Hour)

	// Payment processing endpoints with 60s timeout (blockchain confirmations, external API calls)
	router.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(60 * time.Second))

		// Stripe webhook endpoints (keep at root for webhook URL stability)
		// NOTE: Webhooks are NOT versioned - Stripe needs stable URLs
		r.Get(prefix+"/webhook/stripe", handler.stripeWebhookInfo)
		r.Post(prefix+"/webhook/stripe", handler.handleStripeWebhook)
		r.Get(prefix+"/stripe/success", handler.stripeSuccess)
		r.Get(prefix+"/stripe/cancel", handler.stripeCancel)

		// API v1 - Paywall endpoints
		r.Post(prefix+"/paywall/v1/quote", handler.paywallQuote)
		r.Post(prefix+"/paywall/v1/verify", handler.paywallVerify)
		r.With(idempotencyMW).Post(prefix+"/paywall/v1/stripe-session", handler.createStripeSession)
		r.Get(prefix+"/paywall/v1/stripe-session/verify", handler.verifyStripeSession)
		r.Get(prefix+"/paywall/v1/x402-transaction/verify", handler.verifyX402Transaction)
		r.With(idempotencyMW).Post(prefix+"/paywall/v1/cart/checkout", handler.createCartCheckout)
		r.With(idempotencyMW).Post(prefix+"/paywall/v1/cart/quote", handler.requestCartQuote)
		r.Post(prefix+"/paywall/v1/gasless-transaction", handler.buildGaslessTransaction)

		// API v1 - Refund endpoints
		r.With(idempotencyMW).Post(prefix+"/paywall/v1/refunds/request", handler.requestRefund)
		r.Post(prefix+"/paywall/v1/refunds/approve", handler.getRefundQuote)
		r.Post(prefix+"/paywall/v1/refunds/deny", handler.denyRefund)
		r.Post(prefix+"/paywall/v1/refunds/pending", handler.listPendingRefunds)

		// API v1 - Admin nonce generation (for replay protection)
		r.Post(prefix+"/paywall/v1/nonce", handler.generateNonce)

		// API v1 - Products endpoint (cached)
		r.Get(prefix+"/paywall/v1/products", handler.listProducts)

		// API v1 - Coupon validation endpoint
		r.Post(prefix+"/paywall/v1/coupons/validate", handler.validateCoupon)

		// API v1 - Subscription endpoints (matches frontend BACKEND_SUBSCRIPTION_API.md spec)
		r.Get(prefix+"/paywall/v1/subscription/status", handler.getSubscriptionStatus)
		r.With(idempotencyMW).Post(prefix+"/paywall/v1/subscription/stripe-session", handler.createStripeSubscription)
		r.With(idempotencyMW).Post(prefix+"/paywall/v1/subscription/quote", handler.getSubscriptionQuote)
		// Subscription management endpoints
		r.Post(prefix+"/paywall/v1/subscription/cancel", handler.cancelSubscription)
		r.Post(prefix+"/paywall/v1/subscription/portal", handler.getBillingPortal)
		r.With(idempotencyMW).Post(prefix+"/paywall/v1/subscription/x402/activate", handler.createX402Subscription)
		// Upgrade/downgrade/reactivate endpoints
		r.With(idempotencyMW).Post(prefix+"/paywall/v1/subscription/change", handler.changeSubscription)
		r.Post(prefix+"/paywall/v1/subscription/reactivate", handler.reactivateSubscription)
	})
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
