package paywall

import (
	"context"
	"fmt"

	"github.com/CedrosPay/server/internal/callbacks"
	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/coupons"
	"github.com/CedrosPay/server/internal/metrics"
	"github.com/CedrosPay/server/internal/products"
	solanaKeypair "github.com/CedrosPay/server/internal/solana"
	"github.com/CedrosPay/server/internal/storage"
	"github.com/CedrosPay/server/internal/subscriptions"
	"github.com/CedrosPay/server/pkg/x402"
)

// SubscriptionChecker provides subscription access verification.
type SubscriptionChecker interface {
	// HasAccess checks if a wallet has active subscription access to a product.
	HasAccess(ctx context.Context, wallet, productID string) (bool, *subscriptions.Subscription, error)
}

// Service orchestrates paywall pricing, quotes, and authorization.
type Service struct {
	cfg           *config.Config
	store         storage.Store
	verifier      x402.Verifier
	notifier      callbacks.Notifier
	repository    products.Repository
	coupons       coupons.Repository
	subscriptions SubscriptionChecker // Optional subscription access checker
	metrics       *metrics.Metrics    // Prometheus metrics collector
}

// NewService constructs a paywall service.
func NewService(cfg *config.Config, store storage.Store, verifier x402.Verifier, notifier callbacks.Notifier, repository products.Repository, couponRepo coupons.Repository, metricsCollector *metrics.Metrics) *Service {
	if notifier == nil {
		notifier = callbacks.NoopNotifier{}
	}

	return &Service{
		cfg:        cfg,
		store:      store,
		verifier:   verifier,
		notifier:   notifier,
		repository: repository,
		coupons:    couponRepo,
		metrics:    metricsCollector,
	}
}

// SetSubscriptionChecker sets the subscription checker for access verification.
// This is optional - if not set, subscription-based access control is disabled.
func (s *Service) SetSubscriptionChecker(checker SubscriptionChecker) {
	s.subscriptions = checker
}

// getFeePayerPublicKey returns the server wallet public key for gasless transactions.
// This is a lightweight operation (microseconds) and does not require caching.
func (s *Service) getFeePayerPublicKey() string {
	if !s.cfg.X402.GaslessEnabled || len(s.cfg.X402.ServerWalletKeys) == 0 {
		return ""
	}

	serverWalletKey, err := solanaKeypair.ParsePrivateKey(s.cfg.X402.ServerWalletKeys[0])
	if err != nil {
		return ""
	}

	return serverWalletKey.PublicKey().String()
}

// ResourceDefinition resolves the pricing config for a resource ID.
// Accepts a context for cancellation and timeout propagation.
func (s *Service) ResourceDefinition(ctx context.Context, resourceID string) (config.PaywallResource, error) {
	if resourceID == "" {
		return config.PaywallResource{}, ErrResourceNotConfigured
	}

	// Use repository to fetch product (thread context for cancellation)
	product, err := s.repository.GetProduct(ctx, resourceID)
	if err != nil {
		if err == products.ErrProductNotFound {
			return config.PaywallResource{}, ErrResourceNotConfigured
		}
		return config.PaywallResource{}, fmt.Errorf("get product: %w", err)
	}

	// Convert Product to PaywallResource for backward compatibility
	return product.ToPaywallResource(), nil
}

// ResourceDefinitionByStripePriceID resolves a resource by reverse-looking up its Stripe price ID.
// This is used for coupon validation when clients submit priceId-only cart items.
// Returns ErrResourceNotConfigured if no product matches the given price ID.
func (s *Service) ResourceDefinitionByStripePriceID(ctx context.Context, stripePriceID string) (config.PaywallResource, error) {
	if stripePriceID == "" {
		return config.PaywallResource{}, ErrResourceNotConfigured
	}

	// Use dedicated repository method with database indexing (fixes N+1 query bug)
	product, err := s.repository.GetProductByStripePriceID(ctx, stripePriceID)
	if err != nil {
		if err == products.ErrProductNotFound {
			return config.PaywallResource{}, ErrResourceNotConfigured
		}
		return config.PaywallResource{}, fmt.Errorf("get product by stripe price id: %w", err)
	}

	return product.ToPaywallResource(), nil
}

// ListProducts returns all active products from the repository (uses cache if enabled).
func (s *Service) ListProducts(ctx context.Context) ([]products.Product, error) {
	return s.repository.ListProducts(ctx)
}

// GetProduct retrieves a product by ID.
func (s *Service) GetProduct(ctx context.Context, productID string) (products.Product, error) {
	return s.repository.GetProduct(ctx, productID)
}

// HasPaymentBeenProcessed checks if a payment signature has been processed by this server.
// This is used for refund request validation to ensure refunds can only be requested for actual payments.
func (s *Service) HasPaymentBeenProcessed(ctx context.Context, signature string) (bool, error) {
	return s.store.HasPaymentBeenProcessed(ctx, signature)
}

// GetPayment retrieves payment transaction details by signature.
// This is used for refund wallet validation to ensure refunds go to the original payer.
func (s *Service) GetPayment(ctx context.Context, signature string) (storage.PaymentTransaction, error) {
	payment, err := s.store.GetPayment(ctx, signature)
	if err == storage.ErrNotFound {
		return storage.PaymentTransaction{}, fmt.Errorf("paywall: payment not found")
	}
	if err != nil {
		return storage.PaymentTransaction{}, fmt.Errorf("paywall: get payment: %w", err)
	}
	return payment, nil
}

// CreateNonce stores a new admin nonce for replay protection.
func (s *Service) CreateNonce(ctx context.Context, nonce storage.AdminNonce) error {
	return s.store.CreateNonce(ctx, nonce)
}

// ConsumeNonce marks a nonce as consumed (one-time use).
func (s *Service) ConsumeNonce(ctx context.Context, nonceID string) error {
	return s.store.ConsumeNonce(ctx, nonceID)
}

// validateManualCoupon validates a manual coupon code with optional product and payment method filters.
// Returns the coupon if valid and passes all applicable filters, nil otherwise.
// Silently ignores invalid coupons (matches Stripe behavior).
// Pass empty string for productID or empty PaymentMethod to skip that filter.
func (s *Service) validateManualCoupon(ctx context.Context, couponCode string, productID string, paymentMethod coupons.PaymentMethod) *coupons.Coupon {
	if couponCode == "" || s.coupons == nil {
		return nil
	}
	coupon, err := s.coupons.GetCoupon(ctx, couponCode)
	if err != nil || coupon.IsValid() != nil {
		return nil
	}

	// Check product applicability if productID is provided
	if productID != "" && !coupon.AppliesToProduct(productID) {
		return nil
	}

	// Check payment method applicability if paymentMethod is provided
	if paymentMethod != "" && !coupon.AppliesToPaymentMethod(paymentMethod) {
		return nil
	}

	return &coupon
}
