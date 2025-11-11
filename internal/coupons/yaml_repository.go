package coupons

import (
	"context"
	"errors"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/rs/zerolog/log"
)

// YAMLRepository implements Repository using in-memory YAML config.
type YAMLRepository struct {
	coupons map[string]config.Coupon
}

// NewYAMLRepository creates a repository from YAML config.
func NewYAMLRepository(coupons map[string]config.Coupon) *YAMLRepository {
	// Warn about coupons with usage_limit set
	for code, coupon := range coupons {
		if coupon.UsageLimit != nil && *coupon.UsageLimit > 0 {
			log.Warn().
				Str("coupon_code", code).
				Int("usage_limit", *coupon.UsageLimit).
				Msg("yaml_coupon.usage_limit_not_tracked")
		}
	}

	return &YAMLRepository{
		coupons: coupons,
	}
}

// GetCoupon retrieves a coupon by code.
func (r *YAMLRepository) GetCoupon(_ context.Context, code string) (Coupon, error) {
	cfgCoupon, ok := r.coupons[code]
	if !ok {
		return Coupon{}, ErrCouponNotFound
	}

	return configToCoupon(cfgCoupon, code), nil
}

// ListCoupons returns all active coupons.
func (r *YAMLRepository) ListCoupons(_ context.Context) ([]Coupon, error) {
	coupons := make([]Coupon, 0, len(r.coupons))

	for code, cfgCoupon := range r.coupons {
		coupons = append(coupons, configToCoupon(cfgCoupon, code))
	}

	return coupons, nil
}

// GetAutoApplyCouponsForPayment returns auto-apply coupons that match the product ID and payment method.
func (r *YAMLRepository) GetAutoApplyCouponsForPayment(ctx context.Context, productID string, paymentMethod PaymentMethod) ([]Coupon, error) {
	coupons := make([]Coupon, 0)

	for code, cfgCoupon := range r.coupons {
		coupon := configToCoupon(cfgCoupon, code)

		// Filter: AutoApply must be true
		if !coupon.AutoApply {
			continue
		}

		// Filter: Must be valid (active, not expired, started)
		if err := coupon.IsValid(); err != nil {
			continue
		}

		// Filter: Must apply to the product
		if !coupon.AppliesToProduct(productID) {
			continue
		}

		// Filter: Must apply to the payment method
		if !coupon.AppliesToPaymentMethod(paymentMethod) {
			continue
		}

		coupons = append(coupons, coupon)
	}

	return coupons, nil
}

// GetAllAutoApplyCouponsForPayment returns all auto-apply coupons grouped by product ID.
func (r *YAMLRepository) GetAllAutoApplyCouponsForPayment(_ context.Context, paymentMethod PaymentMethod) (map[string][]Coupon, error) {
	result := make(map[string][]Coupon)

	for code, cfgCoupon := range r.coupons {
		coupon := configToCoupon(cfgCoupon, code)

		// Filter: AutoApply must be true
		if !coupon.AutoApply {
			continue
		}

		// Filter: Must be valid (active, not expired, started)
		if err := coupon.IsValid(); err != nil {
			continue
		}

		// Filter: Must apply to the payment method
		if !coupon.AppliesToPaymentMethod(paymentMethod) {
			continue
		}

		// Group by product IDs
		if coupon.Scope == ScopeAll {
			// For "all" scope coupons, we'll need to add them to each product later
			// Store under a special key that the caller can handle
			result["*"] = append(result["*"], coupon)
		} else {
			// For specific products, add to each product ID
			for _, productID := range coupon.ProductIDs {
				result[productID] = append(result[productID], coupon)
			}
		}
	}

	return result, nil
}

// CreateCoupon is not supported for YAML repository (read-only).
func (r *YAMLRepository) CreateCoupon(_ context.Context, _ Coupon) error {
	return errors.New("yaml repository is read-only")
}

// UpdateCoupon is not supported for YAML repository (read-only).
func (r *YAMLRepository) UpdateCoupon(_ context.Context, _ Coupon) error {
	return errors.New("yaml repository is read-only")
}

// IncrementUsage is not supported for YAML repository (read-only).
func (r *YAMLRepository) IncrementUsage(_ context.Context, _ string) error {
	return errors.New("yaml repository is read-only")
}

// DeleteCoupon is not supported for YAML repository (read-only).
func (r *YAMLRepository) DeleteCoupon(_ context.Context, _ string) error {
	return errors.New("yaml repository is read-only")
}

// Close is a no-op for YAML repository.
func (r *YAMLRepository) Close() error {
	return nil
}

// configToCoupon converts a config.Coupon to a coupons.Coupon.
func configToCoupon(cfg config.Coupon, code string) Coupon {
	var discountType DiscountType
	if cfg.DiscountType == "fixed" {
		discountType = DiscountTypeFixed
	} else {
		discountType = DiscountTypePercentage
	}

	var scope Scope
	if cfg.Scope == "specific" {
		scope = ScopeSpecific
	} else {
		scope = ScopeAll
	}

	var paymentMethod PaymentMethod
	switch cfg.PaymentMethod {
	case "stripe":
		paymentMethod = PaymentMethodStripe
	case "x402":
		paymentMethod = PaymentMethodX402
	default:
		paymentMethod = PaymentMethodAny
	}

	var appliesAt AppliesAt
	switch cfg.AppliesAt {
	case "catalog":
		appliesAt = AppliesAtCatalog
	case "checkout":
		appliesAt = AppliesAtCheckout
	default:
		// Default to empty string for backward compatibility with existing coupons
		appliesAt = ""
	}

	return Coupon{
		Code:          code,
		DiscountType:  discountType,
		DiscountValue: cfg.DiscountValue,
		Currency:      cfg.Currency,
		Scope:         scope,
		ProductIDs:    cfg.ProductIDs,
		PaymentMethod: paymentMethod,
		AutoApply:     cfg.AutoApply,
		AppliesAt:     appliesAt,
		UsageLimit:    cfg.UsageLimit,
		UsageCount:    cfg.UsageCount, // Use configured usage count
		StartsAt:      parseTime(cfg.StartsAt),
		ExpiresAt:     parseTime(cfg.ExpiresAt),
		Active:        cfg.Active,
		Metadata:      cfg.Metadata,
		CreatedAt:     time.Time{}, // Zero value - YAML repos are read-only
		UpdatedAt:     time.Time{}, // Zero value - YAML repos are read-only
	}
}

// parseTime converts an RFC3339 timestamp string to *time.Time.
// Returns nil if the string is empty or invalid.
func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}
