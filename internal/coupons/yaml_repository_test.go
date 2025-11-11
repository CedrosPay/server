package coupons

import (
	"context"
	"testing"
	"time"

	"github.com/CedrosPay/server/internal/config"
)

func TestYAMLRepository_GetCoupon(t *testing.T) {
	usageLimit := 100
	coupons := map[string]config.Coupon{
		"SAVE20": {
			DiscountType:  "percentage",
			DiscountValue: 20.0,
			Active:        true,
		},
		"FIXED10": {
			DiscountType:  "fixed",
			DiscountValue: 10.0,
			Active:        true,
		},
		"SPECIFIC": {
			DiscountType:  "percentage",
			DiscountValue: 15.0,
			Scope:         "specific",
			ProductIDs:    []string{"product-1", "product-2"},
			Active:        true,
		},
		"LIMITED": {
			DiscountType:  "percentage",
			DiscountValue: 30.0,
			UsageLimit:    &usageLimit,
			Active:        true,
		},
	}

	repo := NewYAMLRepository(coupons)
	ctx := context.Background()

	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "existing percentage coupon",
			code:    "SAVE20",
			wantErr: false,
		},
		{
			name:    "existing fixed coupon",
			code:    "FIXED10",
			wantErr: false,
		},
		{
			name:    "non-existent coupon",
			code:    "NOTFOUND",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coupon, err := repo.GetCoupon(ctx, tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCoupon() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if coupon.Code != tt.code {
					t.Errorf("GetCoupon() code = %v, want %v", coupon.Code, tt.code)
				}
			}
		})
	}
}

func TestYAMLRepository_ListCoupons(t *testing.T) {
	coupons := map[string]config.Coupon{
		"COUPON1": {
			DiscountType:  "percentage",
			DiscountValue: 10.0,
			Active:        true,
		},
		"COUPON2": {
			DiscountType:  "fixed",
			DiscountValue: 5.0,
			Active:        false,
		},
		"COUPON3": {
			DiscountType:  "percentage",
			DiscountValue: 25.0,
			Active:        true,
		},
	}

	repo := NewYAMLRepository(coupons)
	ctx := context.Background()

	result, err := repo.ListCoupons(ctx)
	if err != nil {
		t.Fatalf("ListCoupons() error = %v", err)
	}

	if len(result) != 3 {
		t.Errorf("ListCoupons() returned %d coupons, want 3", len(result))
	}

	// Verify all coupons are returned (order may vary)
	codes := make(map[string]bool)
	for _, c := range result {
		codes[c.Code] = true
	}

	if !codes["COUPON1"] || !codes["COUPON2"] || !codes["COUPON3"] {
		t.Error("ListCoupons() did not return all coupons")
	}
}

func TestYAMLRepository_GetAutoApplyCouponsForPayment(t *testing.T) {
	future := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	past := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)

	coupons := map[string]config.Coupon{
		"AUTO1": {
			DiscountType:  "percentage",
			DiscountValue: 10.0,
			AutoApply:     true,
			Active:        true,
		},
		"AUTO2": {
			DiscountType:  "percentage",
			DiscountValue: 15.0,
			Scope:         "specific",
			ProductIDs:    []string{"product-1"},
			AutoApply:     true,
			Active:        true,
		},
		"AUTO3": {
			DiscountType:  "percentage",
			DiscountValue: 20.0,
			Scope:         "specific",
			ProductIDs:    []string{"product-2"},
			AutoApply:     true,
			Active:        true,
		},
		"MANUAL": {
			DiscountType:  "percentage",
			DiscountValue: 5.0,
			AutoApply:     false,
			Active:        true,
		},
		"EXPIRED": {
			DiscountType:  "percentage",
			DiscountValue: 50.0,
			AutoApply:     true,
			ExpiresAt:     past,
			Active:        true,
		},
		"NOTSTARTED": {
			DiscountType:  "percentage",
			DiscountValue: 50.0,
			AutoApply:     true,
			StartsAt:      future,
			Active:        true,
		},
		"INACTIVE": {
			DiscountType:  "percentage",
			DiscountValue: 30.0,
			AutoApply:     true,
			Active:        false,
		},
	}

	repo := NewYAMLRepository(coupons)
	ctx := context.Background()

	tests := []struct {
		name      string
		productID string
		wantCodes []string
	}{
		{
			name:      "product-1 gets AUTO1 (all) and AUTO2 (specific)",
			productID: "product-1",
			wantCodes: []string{"AUTO1", "AUTO2"},
		},
		{
			name:      "product-2 gets AUTO1 (all) and AUTO3 (specific)",
			productID: "product-2",
			wantCodes: []string{"AUTO1", "AUTO3"},
		},
		{
			name:      "product-3 gets only AUTO1 (all)",
			productID: "product-3",
			wantCodes: []string{"AUTO1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetAutoApplyCouponsForPayment(ctx, tt.productID, PaymentMethodAny)
			if err != nil {
				t.Fatalf("GetAutoApplyCouponsForPayment() error = %v", err)
			}

			if len(result) != len(tt.wantCodes) {
				t.Errorf("GetAutoApplyCouponsForPayment() returned %d coupons, want %d", len(result), len(tt.wantCodes))
			}

			codes := make(map[string]bool)
			for _, c := range result {
				codes[c.Code] = true
			}

			for _, wantCode := range tt.wantCodes {
				if !codes[wantCode] {
					t.Errorf("GetAutoApplyCouponsForPayment() missing coupon %s", wantCode)
				}
			}
		})
	}
}

func TestYAMLRepository_ReadOnlyMethods(t *testing.T) {
	repo := NewYAMLRepository(map[string]config.Coupon{})
	ctx := context.Background()

	testCoupon := Coupon{Code: "TEST"}

	// Test that write operations return errors
	if err := repo.CreateCoupon(ctx, testCoupon); err == nil {
		t.Error("CreateCoupon() should return error for read-only repository")
	}

	if err := repo.UpdateCoupon(ctx, testCoupon); err == nil {
		t.Error("UpdateCoupon() should return error for read-only repository")
	}

	if err := repo.IncrementUsage(ctx, "TEST"); err == nil {
		t.Error("IncrementUsage() should return error for read-only repository")
	}

	if err := repo.DeleteCoupon(ctx, "TEST"); err == nil {
		t.Error("DeleteCoupon() should return error for read-only repository")
	}

	// Test Close (should not error)
	if err := repo.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestConfigToCoupon(t *testing.T) {
	usageLimit := 50
	now := time.Now().Format(time.RFC3339)

	tests := []struct {
		name   string
		config config.Coupon
		code   string
		checks func(*testing.T, Coupon)
	}{
		{
			name: "percentage discount",
			config: config.Coupon{
				DiscountType:  "percentage",
				DiscountValue: 25.0,
				Active:        true,
			},
			code: "PERCENT25",
			checks: func(t *testing.T, c Coupon) {
				if c.DiscountType != DiscountTypePercentage {
					t.Error("expected DiscountTypePercentage")
				}
				if c.DiscountValue != 25.0 {
					t.Errorf("expected 25.0, got %f", c.DiscountValue)
				}
			},
		},
		{
			name: "fixed discount",
			config: config.Coupon{
				DiscountType:  "fixed",
				DiscountValue: 10.0,
				Active:        true,
			},
			code: "FIXED10",
			checks: func(t *testing.T, c Coupon) {
				if c.DiscountType != DiscountTypeFixed {
					t.Error("expected DiscountTypeFixed")
				}
			},
		},
		{
			name: "scope specific",
			config: config.Coupon{
				DiscountType:  "percentage",
				DiscountValue: 15.0,
				Scope:         "specific",
				ProductIDs:    []string{"p1", "p2"},
				Active:        true,
			},
			code: "SPECIFIC",
			checks: func(t *testing.T, c Coupon) {
				if c.Scope != ScopeSpecific {
					t.Error("expected ScopeSpecific")
				}
				if len(c.ProductIDs) != 2 {
					t.Errorf("expected 2 product IDs, got %d", len(c.ProductIDs))
				}
			},
		},
		{
			name: "scope all (default)",
			config: config.Coupon{
				DiscountType:  "percentage",
				DiscountValue: 20.0,
				Active:        true,
			},
			code: "ALL",
			checks: func(t *testing.T, c Coupon) {
				if c.Scope != ScopeAll {
					t.Error("expected ScopeAll")
				}
			},
		},
		{
			name: "with usage limit",
			config: config.Coupon{
				DiscountType:  "percentage",
				DiscountValue: 30.0,
				UsageLimit:    &usageLimit,
				Active:        true,
			},
			code: "LIMITED",
			checks: func(t *testing.T, c Coupon) {
				if c.UsageLimit == nil || *c.UsageLimit != 50 {
					t.Error("expected usage limit of 50")
				}
			},
		},
		{
			name: "with timestamps",
			config: config.Coupon{
				DiscountType:  "percentage",
				DiscountValue: 10.0,
				StartsAt:      now,
				ExpiresAt:     now,
				Active:        true,
			},
			code: "TIMED",
			checks: func(t *testing.T, c Coupon) {
				if c.StartsAt == nil {
					t.Error("expected StartsAt to be set")
				}
				if c.ExpiresAt == nil {
					t.Error("expected ExpiresAt to be set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coupon := configToCoupon(tt.config, tt.code)
			if coupon.Code != tt.code {
				t.Errorf("expected code %s, got %s", tt.code, coupon.Code)
			}
			tt.checks(t, coupon)
		})
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
	}{
		{
			name:    "valid RFC3339",
			input:   "2024-01-15T10:30:00Z",
			wantNil: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantNil: true,
		},
		{
			name:    "invalid format",
			input:   "not-a-date",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTime(tt.input)
			if (result == nil) != tt.wantNil {
				t.Errorf("parseTime() nil = %v, wantNil %v", result == nil, tt.wantNil)
			}
		})
	}
}
