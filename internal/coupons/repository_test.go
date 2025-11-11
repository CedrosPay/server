package coupons

import (
	"testing"
	"time"
)

func TestCoupon_IsValid(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)
	usageLimit := 10

	tests := []struct {
		name    string
		coupon  Coupon
		wantErr bool
		errType error
	}{
		{
			name: "valid coupon",
			coupon: Coupon{
				Code:          "VALID",
				Active:        true,
				DiscountType:  DiscountTypePercentage,
				DiscountValue: 20,
			},
			wantErr: false,
		},
		{
			name: "inactive coupon",
			coupon: Coupon{
				Code:          "INACTIVE",
				Active:        false,
				DiscountType:  DiscountTypePercentage,
				DiscountValue: 20,
			},
			wantErr: true,
		},
		{
			name: "not started yet",
			coupon: Coupon{
				Code:          "FUTURE",
				Active:        true,
				StartsAt:      &future,
				DiscountType:  DiscountTypePercentage,
				DiscountValue: 20,
			},
			wantErr: true,
			errType: ErrCouponNotStarted,
		},
		{
			name: "expired",
			coupon: Coupon{
				Code:          "EXPIRED",
				Active:        true,
				ExpiresAt:     &past,
				DiscountType:  DiscountTypePercentage,
				DiscountValue: 20,
			},
			wantErr: true,
			errType: ErrCouponExpired,
		},
		{
			name: "usage limit reached",
			coupon: Coupon{
				Code:          "MAXED",
				Active:        true,
				UsageLimit:    &usageLimit,
				UsageCount:    10,
				DiscountType:  DiscountTypePercentage,
				DiscountValue: 20,
			},
			wantErr: true,
			errType: ErrCouponUsageLimitReached,
		},
		{
			name: "within date range",
			coupon: Coupon{
				Code:          "ACTIVE",
				Active:        true,
				StartsAt:      &past,
				ExpiresAt:     &future,
				DiscountType:  DiscountTypePercentage,
				DiscountValue: 20,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.coupon.IsValid()
			if (err != nil) != tt.wantErr {
				t.Errorf("IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errType != nil && err != tt.errType {
				t.Errorf("IsValid() error = %v, want %v", err, tt.errType)
			}
		})
	}
}

func TestCoupon_AppliesToProduct(t *testing.T) {
	tests := []struct {
		name      string
		coupon    Coupon
		productID string
		want      bool
	}{
		{
			name: "scope all applies to any product",
			coupon: Coupon{
				Code:  "ALL",
				Scope: ScopeAll,
			},
			productID: "any-product",
			want:      true,
		},
		{
			name: "scope specific with matching product",
			coupon: Coupon{
				Code:       "SPECIFIC",
				Scope:      ScopeSpecific,
				ProductIDs: []string{"product-1", "product-2"},
			},
			productID: "product-1",
			want:      true,
		},
		{
			name: "scope specific without matching product",
			coupon: Coupon{
				Code:       "SPECIFIC",
				Scope:      ScopeSpecific,
				ProductIDs: []string{"product-1", "product-2"},
			},
			productID: "product-3",
			want:      false,
		},
		{
			name: "scope specific with empty product list",
			coupon: Coupon{
				Code:       "EMPTY",
				Scope:      ScopeSpecific,
				ProductIDs: []string{},
			},
			productID: "any-product",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.coupon.AppliesToProduct(tt.productID); got != tt.want {
				t.Errorf("AppliesToProduct() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoupon_AppliesToPaymentMethod(t *testing.T) {
	tests := []struct {
		name          string
		coupon        Coupon
		paymentMethod PaymentMethod
		want          bool
	}{
		{
			name: "PaymentMethodAny matches stripe",
			coupon: Coupon{
				Code:          "ANY",
				PaymentMethod: PaymentMethodAny,
			},
			paymentMethod: PaymentMethodStripe,
			want:          true,
		},
		{
			name: "PaymentMethodAny matches x402",
			coupon: Coupon{
				Code:          "ANY",
				PaymentMethod: PaymentMethodAny,
			},
			paymentMethod: PaymentMethodX402,
			want:          true,
		},
		{
			name: "PaymentMethodAny matches any (empty string)",
			coupon: Coupon{
				Code:          "ANY",
				PaymentMethod: PaymentMethodAny,
			},
			paymentMethod: PaymentMethodAny,
			want:          true,
		},
		{
			name: "PaymentMethodStripe only matches stripe",
			coupon: Coupon{
				Code:          "STRIPE_ONLY",
				PaymentMethod: PaymentMethodStripe,
			},
			paymentMethod: PaymentMethodStripe,
			want:          true,
		},
		{
			name: "PaymentMethodStripe does not match x402",
			coupon: Coupon{
				Code:          "STRIPE_ONLY",
				PaymentMethod: PaymentMethodStripe,
			},
			paymentMethod: PaymentMethodX402,
			want:          false,
		},
		{
			name: "PaymentMethodX402 only matches x402",
			coupon: Coupon{
				Code:          "X402_ONLY",
				PaymentMethod: PaymentMethodX402,
			},
			paymentMethod: PaymentMethodX402,
			want:          true,
		},
		{
			name: "PaymentMethodX402 does not match stripe",
			coupon: Coupon{
				Code:          "X402_ONLY",
				PaymentMethod: PaymentMethodX402,
			},
			paymentMethod: PaymentMethodStripe,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.coupon.AppliesToPaymentMethod(tt.paymentMethod); got != tt.want {
				t.Errorf("AppliesToPaymentMethod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoupon_ApplyDiscount(t *testing.T) {
	tests := []struct {
		name          string
		coupon        Coupon
		originalPrice float64
		want          float64
	}{
		{
			name: "percentage discount",
			coupon: Coupon{
				Code:          "PERCENT20",
				DiscountType:  DiscountTypePercentage,
				DiscountValue: 20.0, // 20%
			},
			originalPrice: 100.0,
			want:          80.0, // 100 - 20% = 80
		},
		{
			name: "percentage discount rounds correctly",
			coupon: Coupon{
				Code:          "PERCENT25",
				DiscountType:  DiscountTypePercentage,
				DiscountValue: 25.0, // 25%
			},
			originalPrice: 33.33,
			want:          24.9975, // 33.33 * 0.75
		},
		{
			name: "fixed discount",
			coupon: Coupon{
				Code:          "FIXED10",
				DiscountType:  DiscountTypeFixed,
				DiscountValue: 10.0, // $10 off
			},
			originalPrice: 100.0,
			want:          90.0, // 100 - 10 = 90
		},
		{
			name: "fixed discount larger than price returns 0",
			coupon: Coupon{
				Code:          "FIXED50",
				DiscountType:  DiscountTypeFixed,
				DiscountValue: 50.0, // $50 off
			},
			originalPrice: 30.0,
			want:          0.0, // Can't go negative
		},
		{
			name: "fixed discount exactly equals price",
			coupon: Coupon{
				Code:          "FIXED100",
				DiscountType:  DiscountTypeFixed,
				DiscountValue: 100.0, // $100 off
			},
			originalPrice: 100.0,
			want:          0.0, // Free!
		},
		{
			name: "100% discount makes it free",
			coupon: Coupon{
				Code:          "FREE",
				DiscountType:  DiscountTypePercentage,
				DiscountValue: 100.0, // 100%
			},
			originalPrice: 99.99,
			want:          0.0, // Free
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.coupon.ApplyDiscount(tt.originalPrice)
			// Use a small epsilon for float comparison
			epsilon := 0.0001
			if diff := got - tt.want; diff < -epsilon || diff > epsilon {
				t.Errorf("ApplyDiscount() = %v, want %v", got, tt.want)
			}
		})
	}
}
