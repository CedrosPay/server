package coupons

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockRepository implements Repository for testing.
type mockRepository struct {
	getCouponFunc              func(ctx context.Context, code string) (Coupon, error)
	listCouponsFunc            func(ctx context.Context) ([]Coupon, error)
	getAutoApplyForPaymentFunc func(ctx context.Context, productID string, paymentMethod PaymentMethod) ([]Coupon, error)
	createCouponFunc           func(ctx context.Context, coupon Coupon) error
	updateCouponFunc           func(ctx context.Context, coupon Coupon) error
	incrementUsageFunc         func(ctx context.Context, code string) error
	deleteCouponFunc           func(ctx context.Context, code string) error
	closeFunc                  func() error
	getCouponCallCount         int
	listCouponsCallCount       int
}

func (m *mockRepository) GetCoupon(ctx context.Context, code string) (Coupon, error) {
	m.getCouponCallCount++
	if m.getCouponFunc != nil {
		return m.getCouponFunc(ctx, code)
	}
	return Coupon{}, ErrCouponNotFound
}

func (m *mockRepository) ListCoupons(ctx context.Context) ([]Coupon, error) {
	m.listCouponsCallCount++
	if m.listCouponsFunc != nil {
		return m.listCouponsFunc(ctx)
	}
	return nil, nil
}

func (m *mockRepository) GetAutoApplyCouponsForPayment(ctx context.Context, productID string, paymentMethod PaymentMethod) ([]Coupon, error) {
	if m.getAutoApplyForPaymentFunc != nil {
		return m.getAutoApplyForPaymentFunc(ctx, productID, paymentMethod)
	}
	return nil, nil
}

func (m *mockRepository) GetAllAutoApplyCouponsForPayment(ctx context.Context, paymentMethod PaymentMethod) (map[string][]Coupon, error) {
	// Not used in cached repository tests
	return make(map[string][]Coupon), nil
}

func (m *mockRepository) CreateCoupon(ctx context.Context, coupon Coupon) error {
	if m.createCouponFunc != nil {
		return m.createCouponFunc(ctx, coupon)
	}
	return nil
}

func (m *mockRepository) UpdateCoupon(ctx context.Context, coupon Coupon) error {
	if m.updateCouponFunc != nil {
		return m.updateCouponFunc(ctx, coupon)
	}
	return nil
}

func (m *mockRepository) IncrementUsage(ctx context.Context, code string) error {
	if m.incrementUsageFunc != nil {
		return m.incrementUsageFunc(ctx, code)
	}
	return nil
}

func (m *mockRepository) DeleteCoupon(ctx context.Context, code string) error {
	if m.deleteCouponFunc != nil {
		return m.deleteCouponFunc(ctx, code)
	}
	return nil
}

func (m *mockRepository) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestCachedRepository_GetCoupon_CacheHit(t *testing.T) {
	mockRepo := &mockRepository{
		getCouponFunc: func(ctx context.Context, code string) (Coupon, error) {
			return Coupon{
				Code:          code,
				DiscountType:  DiscountTypePercentage,
				DiscountValue: 10.0,
				Active:        true,
			}, nil
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	ctx := context.Background()

	// First call should hit the underlying repo
	coupon1, err := cached.GetCoupon(ctx, "TEST10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if coupon1.Code != "TEST10" {
		t.Errorf("expected code TEST10, got %s", coupon1.Code)
	}
	if mockRepo.getCouponCallCount != 1 {
		t.Errorf("expected 1 call to underlying repo, got %d", mockRepo.getCouponCallCount)
	}

	// Second call should hit the cache
	coupon2, err := cached.GetCoupon(ctx, "TEST10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if coupon2.Code != "TEST10" {
		t.Errorf("expected code TEST10, got %s", coupon2.Code)
	}
	if mockRepo.getCouponCallCount != 1 {
		t.Errorf("expected 1 call to underlying repo (cached), got %d", mockRepo.getCouponCallCount)
	}
}

func TestCachedRepository_GetCoupon_CacheMiss(t *testing.T) {
	callCount := 0
	mockRepo := &mockRepository{
		getCouponFunc: func(ctx context.Context, code string) (Coupon, error) {
			callCount++
			return Coupon{
				Code:          code,
				DiscountType:  DiscountTypePercentage,
				DiscountValue: float64(callCount) * 10.0, // Different value each time
				Active:        true,
			}, nil
		},
	}

	cached := NewCachedRepository(mockRepo, 100*time.Millisecond)
	ctx := context.Background()

	// First call
	coupon1, err := cached.GetCoupon(ctx, "EXPIRE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if coupon1.DiscountValue != 10.0 {
		t.Errorf("expected discount 10.0, got %f", coupon1.DiscountValue)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Second call should hit underlying repo again
	coupon2, err := cached.GetCoupon(ctx, "EXPIRE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if coupon2.DiscountValue != 20.0 {
		t.Errorf("expected discount 20.0 (cache expired), got %f", coupon2.DiscountValue)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls to underlying repo, got %d", callCount)
	}
}

func TestCachedRepository_GetCoupon_NoCaching(t *testing.T) {
	mockRepo := &mockRepository{
		getCouponFunc: func(ctx context.Context, code string) (Coupon, error) {
			return Coupon{Code: code, Active: true}, nil
		},
	}

	cached := NewCachedRepository(mockRepo, 0) // Zero TTL = no caching
	ctx := context.Background()

	cached.GetCoupon(ctx, "TEST")
	cached.GetCoupon(ctx, "TEST")

	if mockRepo.getCouponCallCount != 2 {
		t.Errorf("expected 2 calls with no caching, got %d", mockRepo.getCouponCallCount)
	}
}

func TestCachedRepository_GetCoupon_Error(t *testing.T) {
	expectedErr := errors.New("database error")
	mockRepo := &mockRepository{
		getCouponFunc: func(ctx context.Context, code string) (Coupon, error) {
			return Coupon{}, expectedErr
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	ctx := context.Background()

	_, err := cached.GetCoupon(ctx, "ERROR")
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestCachedRepository_ListCoupons_CacheHit(t *testing.T) {
	mockRepo := &mockRepository{
		listCouponsFunc: func(ctx context.Context) ([]Coupon, error) {
			return []Coupon{
				{Code: "COUPON1", Active: true},
				{Code: "COUPON2", Active: true},
			}, nil
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	ctx := context.Background()

	// First call
	coupons1, err := cached.ListCoupons(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coupons1) != 2 {
		t.Errorf("expected 2 coupons, got %d", len(coupons1))
	}
	if mockRepo.listCouponsCallCount != 1 {
		t.Errorf("expected 1 call to underlying repo, got %d", mockRepo.listCouponsCallCount)
	}

	// Second call should hit cache
	coupons2, err := cached.ListCoupons(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coupons2) != 2 {
		t.Errorf("expected 2 coupons, got %d", len(coupons2))
	}
	if mockRepo.listCouponsCallCount != 1 {
		t.Errorf("expected 1 call to underlying repo (cached), got %d", mockRepo.listCouponsCallCount)
	}
}

func TestCachedRepository_ListCoupons_CacheMiss(t *testing.T) {
	callCount := 0
	mockRepo := &mockRepository{
		listCouponsFunc: func(ctx context.Context) ([]Coupon, error) {
			callCount++
			return []Coupon{
				{Code: "COUPON", DiscountValue: float64(callCount)},
			}, nil
		},
	}

	cached := NewCachedRepository(mockRepo, 100*time.Millisecond)
	ctx := context.Background()

	// First call
	coupons1, err := cached.ListCoupons(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if coupons1[0].DiscountValue != 1.0 {
		t.Errorf("expected discount 1.0, got %f", coupons1[0].DiscountValue)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Second call should hit underlying repo again
	coupons2, err := cached.ListCoupons(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if coupons2[0].DiscountValue != 2.0 {
		t.Errorf("expected discount 2.0 (cache expired), got %f", coupons2[0].DiscountValue)
	}
}

func TestCachedRepository_GetAutoApplyCouponsForPayment_NoCache(t *testing.T) {
	callCount := 0
	mockRepo := &mockRepository{
		getAutoApplyForPaymentFunc: func(ctx context.Context, productID string, paymentMethod PaymentMethod) ([]Coupon, error) {
			callCount++
			return []Coupon{
				{Code: "AUTO", ProductIDs: []string{productID}, AutoApply: true, PaymentMethod: paymentMethod},
			}, nil
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	ctx := context.Background()

	// Auto-apply coupons are never cached (always fresh)
	cached.GetAutoApplyCouponsForPayment(ctx, "product-1", PaymentMethodStripe)
	cached.GetAutoApplyCouponsForPayment(ctx, "product-1", PaymentMethodStripe)

	if callCount != 2 {
		t.Errorf("expected 2 calls (auto-apply not cached), got %d", callCount)
	}
}

func TestCachedRepository_CreateCoupon_InvalidatesCache(t *testing.T) {
	mockRepo := &mockRepository{
		getCouponFunc: func(ctx context.Context, code string) (Coupon, error) {
			return Coupon{Code: code, Active: true}, nil
		},
		createCouponFunc: func(ctx context.Context, coupon Coupon) error {
			return nil
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	ctx := context.Background()

	// Prime the cache
	cached.GetCoupon(ctx, "TEST")
	if mockRepo.getCouponCallCount != 1 {
		t.Errorf("expected 1 call, got %d", mockRepo.getCouponCallCount)
	}

	// Create a new coupon (should invalidate cache)
	err := cached.CreateCoupon(ctx, Coupon{Code: "NEW", Active: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Next GetCoupon should hit underlying repo again
	cached.GetCoupon(ctx, "TEST")
	if mockRepo.getCouponCallCount != 2 {
		t.Errorf("expected 2 calls (cache invalidated), got %d", mockRepo.getCouponCallCount)
	}
}

func TestCachedRepository_UpdateCoupon_InvalidatesCache(t *testing.T) {
	mockRepo := &mockRepository{
		getCouponFunc: func(ctx context.Context, code string) (Coupon, error) {
			return Coupon{Code: code, Active: true}, nil
		},
		updateCouponFunc: func(ctx context.Context, coupon Coupon) error {
			return nil
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	ctx := context.Background()

	// Prime the cache
	cached.GetCoupon(ctx, "TEST")
	if mockRepo.getCouponCallCount != 1 {
		t.Errorf("expected 1 call, got %d", mockRepo.getCouponCallCount)
	}

	// Update coupon (should invalidate cache)
	err := cached.UpdateCoupon(ctx, Coupon{Code: "TEST", Active: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Next GetCoupon should hit underlying repo again
	cached.GetCoupon(ctx, "TEST")
	if mockRepo.getCouponCallCount != 2 {
		t.Errorf("expected 2 calls (cache invalidated), got %d", mockRepo.getCouponCallCount)
	}
}

func TestCachedRepository_IncrementUsage_InvalidatesSpecificCoupon(t *testing.T) {
	mockRepo := &mockRepository{
		getCouponFunc: func(ctx context.Context, code string) (Coupon, error) {
			return Coupon{Code: code, Active: true}, nil
		},
		incrementUsageFunc: func(ctx context.Context, code string) error {
			return nil
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	ctx := context.Background()

	// Prime the cache with two coupons
	cached.GetCoupon(ctx, "TEST1")
	cached.GetCoupon(ctx, "TEST2")
	if mockRepo.getCouponCallCount != 2 {
		t.Errorf("expected 2 calls, got %d", mockRepo.getCouponCallCount)
	}

	// Increment usage for TEST1 (should invalidate only TEST1)
	err := cached.IncrementUsage(ctx, "TEST1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// TEST1 should hit underlying repo again
	cached.GetCoupon(ctx, "TEST1")
	if mockRepo.getCouponCallCount != 3 {
		t.Errorf("expected 3 calls (TEST1 invalidated), got %d", mockRepo.getCouponCallCount)
	}

	// TEST2 should still be cached
	cached.GetCoupon(ctx, "TEST2")
	if mockRepo.getCouponCallCount != 3 {
		t.Errorf("expected 3 calls (TEST2 cached), got %d", mockRepo.getCouponCallCount)
	}
}

func TestCachedRepository_DeleteCoupon_InvalidatesCache(t *testing.T) {
	mockRepo := &mockRepository{
		getCouponFunc: func(ctx context.Context, code string) (Coupon, error) {
			return Coupon{Code: code, Active: true}, nil
		},
		deleteCouponFunc: func(ctx context.Context, code string) error {
			return nil
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	ctx := context.Background()

	// Prime the cache
	cached.GetCoupon(ctx, "TEST")
	if mockRepo.getCouponCallCount != 1 {
		t.Errorf("expected 1 call, got %d", mockRepo.getCouponCallCount)
	}

	// Delete coupon (should invalidate cache)
	err := cached.DeleteCoupon(ctx, "TEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Next GetCoupon should hit underlying repo again
	cached.GetCoupon(ctx, "TEST")
	if mockRepo.getCouponCallCount != 2 {
		t.Errorf("expected 2 calls (cache invalidated), got %d", mockRepo.getCouponCallCount)
	}
}

func TestCachedRepository_InvalidateCache(t *testing.T) {
	mockRepo := &mockRepository{
		getCouponFunc: func(ctx context.Context, code string) (Coupon, error) {
			return Coupon{Code: code, Active: true}, nil
		},
		listCouponsFunc: func(ctx context.Context) ([]Coupon, error) {
			return []Coupon{{Code: "COUPON1"}}, nil
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	ctx := context.Background()

	// Prime both caches
	cached.GetCoupon(ctx, "TEST")
	cached.ListCoupons(ctx)
	if mockRepo.getCouponCallCount != 1 || mockRepo.listCouponsCallCount != 1 {
		t.Errorf("expected 1 call each, got GetCoupon:%d ListCoupons:%d",
			mockRepo.getCouponCallCount, mockRepo.listCouponsCallCount)
	}

	// Invalidate all caches
	cached.InvalidateCache()

	// Both should hit underlying repo again
	cached.GetCoupon(ctx, "TEST")
	cached.ListCoupons(ctx)
	if mockRepo.getCouponCallCount != 2 || mockRepo.listCouponsCallCount != 2 {
		t.Errorf("expected 2 calls each (cache invalidated), got GetCoupon:%d ListCoupons:%d",
			mockRepo.getCouponCallCount, mockRepo.listCouponsCallCount)
	}
}

func TestCachedRepository_Close(t *testing.T) {
	closeCalled := false
	mockRepo := &mockRepository{
		closeFunc: func() error {
			closeCalled = true
			return nil
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	err := cached.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !closeCalled {
		t.Error("expected Close to be called on underlying repository")
	}
}

func TestCachedRepository_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("underlying error")
	mockRepo := &mockRepository{
		createCouponFunc: func(ctx context.Context, coupon Coupon) error {
			return expectedErr
		},
		updateCouponFunc: func(ctx context.Context, coupon Coupon) error {
			return expectedErr
		},
		incrementUsageFunc: func(ctx context.Context, code string) error {
			return expectedErr
		},
		deleteCouponFunc: func(ctx context.Context, code string) error {
			return expectedErr
		},
	}

	cached := NewCachedRepository(mockRepo, 5*time.Minute)
	ctx := context.Background()

	// Test error propagation for all write methods
	if err := cached.CreateCoupon(ctx, Coupon{}); err != expectedErr {
		t.Errorf("CreateCoupon: expected error %v, got %v", expectedErr, err)
	}
	if err := cached.UpdateCoupon(ctx, Coupon{}); err != expectedErr {
		t.Errorf("UpdateCoupon: expected error %v, got %v", expectedErr, err)
	}
	if err := cached.IncrementUsage(ctx, "TEST"); err != expectedErr {
		t.Errorf("IncrementUsage: expected error %v, got %v", expectedErr, err)
	}
	if err := cached.DeleteCoupon(ctx, "TEST"); err != expectedErr {
		t.Errorf("DeleteCoupon: expected error %v, got %v", expectedErr, err)
	}
}
