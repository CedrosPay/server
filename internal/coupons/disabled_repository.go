package coupons

import (
	"context"
)

// DisabledRepository is a no-op repository for when coupons are disabled.
type DisabledRepository struct{}

// NewDisabledRepository creates a disabled repository.
func NewDisabledRepository() *DisabledRepository {
	return &DisabledRepository{}
}

// GetCoupon always returns ErrCouponNotFound when coupons are disabled.
func (r *DisabledRepository) GetCoupon(_ context.Context, _ string) (Coupon, error) {
	return Coupon{}, ErrCouponNotFound
}

// ListCoupons returns an empty list when coupons are disabled.
func (r *DisabledRepository) ListCoupons(_ context.Context) ([]Coupon, error) {
	return []Coupon{}, nil
}

// GetAutoApplyCouponsForPayment returns an empty list when coupons are disabled.
func (r *DisabledRepository) GetAutoApplyCouponsForPayment(_ context.Context, _ string, _ PaymentMethod) ([]Coupon, error) {
	return []Coupon{}, nil
}

// GetAllAutoApplyCouponsForPayment returns an empty map when coupons are disabled.
func (r *DisabledRepository) GetAllAutoApplyCouponsForPayment(_ context.Context, _ PaymentMethod) (map[string][]Coupon, error) {
	return make(map[string][]Coupon), nil
}

// CreateCoupon is a no-op when coupons are disabled.
func (r *DisabledRepository) CreateCoupon(_ context.Context, _ Coupon) error {
	return nil
}

// UpdateCoupon is a no-op when coupons are disabled.
func (r *DisabledRepository) UpdateCoupon(_ context.Context, _ Coupon) error {
	return nil
}

// IncrementUsage is a no-op when coupons are disabled.
func (r *DisabledRepository) IncrementUsage(_ context.Context, _ string) error {
	return nil
}

// DeleteCoupon is a no-op when coupons are disabled.
func (r *DisabledRepository) DeleteCoupon(_ context.Context, _ string) error {
	return nil
}

// Close is a no-op when coupons are disabled.
func (r *DisabledRepository) Close() error {
	return nil
}
