package products

import (
	"context"
	"errors"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/money"
)

// YAMLRepository implements Repository using in-memory YAML config.
type YAMLRepository struct {
	resources map[string]config.PaywallResource
}

var (
	// zeroTime is a reusable zero value for timestamps in read-only repositories
	zeroTime = time.Time{}
)

// NewYAMLRepository creates a repository from YAML config.
func NewYAMLRepository(resources map[string]config.PaywallResource) *YAMLRepository {
	return &YAMLRepository{
		resources: resources,
	}
}

// GetProduct retrieves a product by ID.
func (r *YAMLRepository) GetProduct(_ context.Context, id string) (Product, error) {
	resource, ok := r.resources[id]
	if !ok {
		return Product{}, ErrProductNotFound
	}

	p := Product{
		ID:            id,
		Description:   resource.Description,
		StripePriceID: resource.StripePriceID,
		CryptoAccount: resource.CryptoAccount,
		MemoTemplate:  resource.MemoTemplate,
		Metadata:      cloneMetadata(resource.Metadata),
		Active:        true, // YAML resources are always active
		CreatedAt:     zeroTime,
		UpdatedAt:     zeroTime,
	}

	// Convert fiat pricing if present
	if resource.FiatAmountCents > 0 && resource.FiatCurrency != "" {
		// Asset registry uses uppercase codes (USD, EUR, etc.)
		assetCode := toUpperCase(resource.FiatCurrency)
		if asset, err := money.GetAsset(assetCode); err == nil {
			price := money.New(asset, resource.FiatAmountCents)
			p.FiatPrice = &price
		}
	}

	// Convert crypto pricing if present
	if resource.CryptoAtomicAmount > 0 && resource.CryptoToken != "" {
		// Asset registry uses uppercase codes (USDC, USDT, SOL, etc.)
		assetCode := toUpperCase(resource.CryptoToken)
		if asset, err := money.GetAsset(assetCode); err == nil {
			price := money.New(asset, resource.CryptoAtomicAmount)
			p.CryptoPrice = &price
		}
	}

	// Convert subscription config if present
	if resource.Subscription != nil && resource.Subscription.BillingPeriod != "" {
		p.Subscription = &SubscriptionConfig{
			BillingPeriod:    resource.Subscription.BillingPeriod,
			BillingInterval:  resource.Subscription.BillingInterval,
			TrialDays:        resource.Subscription.TrialDays,
			StripePriceID:    resource.Subscription.StripePriceID,
			AllowX402:        resource.Subscription.AllowX402,
			GracePeriodHours: resource.Subscription.GracePeriodHours,
		}
		// Default billing interval to 1 if not set
		if p.Subscription.BillingInterval < 1 {
			p.Subscription.BillingInterval = 1
		}
	}

	return p, nil
}

// GetProductByStripePriceID retrieves a product by its Stripe Price ID.
func (r *YAMLRepository) GetProductByStripePriceID(ctx context.Context, stripePriceID string) (Product, error) {
	// Linear search through YAML resources to find matching Stripe Price ID
	for id, resource := range r.resources {
		if resource.StripePriceID == stripePriceID {
			// Reuse GetProduct logic to ensure consistent conversion
			return r.GetProduct(ctx, id)
		}
	}
	return Product{}, ErrProductNotFound
}

// ListProducts returns all active products.
func (r *YAMLRepository) ListProducts(ctx context.Context) ([]Product, error) {
	products := make([]Product, 0, len(r.resources))

	for id := range r.resources {
		// Reuse GetProduct logic to ensure consistent conversion
		p, err := r.GetProduct(ctx, id)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}

	return products, nil
}

// CreateProduct is not supported for YAML repository (read-only).
func (r *YAMLRepository) CreateProduct(_ context.Context, _ Product) error {
	return errors.New("yaml repository is read-only")
}

// UpdateProduct is not supported for YAML repository (read-only).
func (r *YAMLRepository) UpdateProduct(_ context.Context, _ Product) error {
	return errors.New("yaml repository is read-only")
}

// DeleteProduct is not supported for YAML repository (read-only).
func (r *YAMLRepository) DeleteProduct(_ context.Context, _ string) error {
	return errors.New("yaml repository is read-only")
}

// Close is a no-op for YAML repository.
func (r *YAMLRepository) Close() error {
	return nil
}

func cloneMetadata(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	clone := make(map[string]string, len(m))
	for k, v := range m {
		clone[k] = v
	}
	return clone
}

// toUpperCase converts a string to uppercase for asset code lookup.
// Allows YAML config to use lowercase codes (usd, usdc) while the asset registry uses uppercase (USD, USDC).
func toUpperCase(s string) string {
	// Simple ASCII uppercase conversion (avoids importing strings package)
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			result[i] = c - 32 // Convert to uppercase
		} else {
			result[i] = c
		}
	}
	return string(result)
}
