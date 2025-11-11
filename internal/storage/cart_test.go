package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CedrosPay/server/internal/money"
)

func TestGenerateCartID(t *testing.T) {
	// Generate multiple IDs and verify format
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateCartID()
		if err != nil {
			t.Fatalf("GenerateCartID() error = %v", err)
		}

		// Check format
		if !strings.HasPrefix(id, "cart_") {
			t.Errorf("GenerateCartID() = %q, should start with 'cart_'", id)
		}

		// Check length (cart_ + 32 hex chars)
		expectedLen := 5 + 32 // "cart_" + hex(16 bytes)
		if len(id) != expectedLen {
			t.Errorf("GenerateCartID() length = %d, want %d", len(id), expectedLen)
		}

		// Check uniqueness
		if ids[id] {
			t.Errorf("GenerateCartID() generated duplicate: %q", id)
		}
		ids[id] = true
	}
}

func TestCartQuote_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "not expired (future)",
			expiresAt: time.Now().Add(10 * time.Minute),
			want:      false,
		},
		{
			name:      "expired (past)",
			expiresAt: time.Now().Add(-10 * time.Minute),
			want:      true,
		},
		{
			name:      "expired (just now)",
			expiresAt: time.Now().Add(-1 * time.Millisecond),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cart := CartQuote{
				ExpiresAt: tt.expiresAt,
			}
			now := time.Now()
			if got := cart.IsExpiredAt(now); got != tt.want {
				t.Errorf("IsExpiredAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemoryStore_SaveCartQuote(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	cartID := "cart_test123"
	// Create Money for item price and total
	usdc, _ := money.GetAsset("USDC")
	itemPrice, _ := money.FromMajor(usdc, "10.0")
	totalPrice, _ := money.FromMajor(usdc, "20.0")

	quote := CartQuote{
		ID: cartID,
		Items: []CartItem{
			{
				ResourceID: "item-1",
				Quantity:   2,
				Price:      itemPrice,
			},
		},
		Total:    totalPrice,
		Metadata: map[string]string{"user_id": "123"},
	}

	err := store.SaveCartQuote(ctx, quote)
	if err != nil {
		t.Fatalf("SaveCartQuote() error = %v", err)
	}

	// Verify it was saved
	retrieved, err := store.GetCartQuote(ctx, cartID)
	if err != nil {
		t.Fatalf("GetCartQuote() error = %v", err)
	}

	if retrieved.ID != cartID {
		t.Errorf("GetCartQuote() ID = %q, want %q", retrieved.ID, cartID)
	}
	if retrieved.Total.ToMajor() != "20.000000" {
		t.Errorf("GetCartQuote() Total = %s, want 20.000000", retrieved.Total.ToMajor())
	}
	if len(retrieved.Items) != 1 {
		t.Errorf("GetCartQuote() Items length = %d, want 1", len(retrieved.Items))
	}
}

func TestMemoryStore_SaveCartQuote_AutoTimestamps(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	// Save quote without CreatedAt or ExpiresAt
	usdc, _ := money.GetAsset("USDC")
	total, _ := money.FromMajor(usdc, "10.0")
	quote := CartQuote{
		ID:    "cart_auto",
		Total: total,
	}

	err := store.SaveCartQuote(ctx, quote)
	if err != nil {
		t.Fatalf("SaveCartQuote() error = %v", err)
	}

	// Retrieve and verify timestamps were auto-filled
	retrieved, err := store.GetCartQuote(ctx, "cart_auto")
	if err != nil {
		t.Fatalf("GetCartQuote() error = %v", err)
	}

	if retrieved.CreatedAt.IsZero() {
		t.Error("SaveCartQuote() should auto-fill CreatedAt")
	}
	if retrieved.ExpiresAt.IsZero() {
		t.Error("SaveCartQuote() should auto-fill ExpiresAt")
	}

	// Verify ExpiresAt is 15 minutes (default TTL) after CreatedAt
	expectedExpiry := retrieved.CreatedAt.Add(15 * time.Minute)
	if !retrieved.ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("ExpiresAt = %v, want %v", retrieved.ExpiresAt, expectedExpiry)
	}
}

func TestMemoryStore_SaveCartQuote_RequiresID(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	usdc, _ := money.GetAsset("USDC")
	total, _ := money.FromMajor(usdc, "10.0")
	quote := CartQuote{
		// No ID
		Total: total,
	}

	err := store.SaveCartQuote(ctx, quote)
	if err == nil {
		t.Error("SaveCartQuote() should return error when ID is empty")
	}
}

func TestMemoryStore_GetCartQuote_NotFound(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	_, err := store.GetCartQuote(ctx, "cart_nonexistent")
	if err != ErrNotFound {
		t.Errorf("GetCartQuote() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryStore_GetCartQuote_Expired(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	// Save expired cart
	usdc, _ := money.GetAsset("USDC")
	total, _ := money.FromMajor(usdc, "10.0")
	quote := CartQuote{
		ID:        "cart_expired",
		Total:     total,
		CreatedAt: time.Now().Add(-1 * time.Hour),
		ExpiresAt: time.Now().Add(-30 * time.Minute), // Expired
	}

	err := store.SaveCartQuote(ctx, quote)
	if err != nil {
		t.Fatalf("SaveCartQuote() error = %v", err)
	}

	// Try to retrieve expired cart
	_, err = store.GetCartQuote(ctx, "cart_expired")
	if err != ErrCartExpired {
		t.Errorf("GetCartQuote() error = %v, want %v", err, ErrCartExpired)
	}
}

func TestMemoryStore_MarkCartPaid(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	cartID := "cart_payment"
	wallet := "wallet123"

	// Save cart
	usdc, _ := money.GetAsset("USDC")
	total, _ := money.FromMajor(usdc, "10.0")
	quote := CartQuote{
		ID:    cartID,
		Total: total,
	}
	store.SaveCartQuote(ctx, quote)

	// Mark as paid
	err := store.MarkCartPaid(ctx, cartID, wallet)
	if err != nil {
		t.Fatalf("MarkCartPaid() error = %v", err)
	}

	// Verify payment was recorded
	retrieved, err := store.GetCartQuote(ctx, cartID)
	if err != nil {
		t.Fatalf("GetCartQuote() error = %v", err)
	}

	if retrieved.WalletPaidBy != wallet {
		t.Errorf("WalletPaidBy = %q, want %q", retrieved.WalletPaidBy, wallet)
	}
}

func TestMemoryStore_MarkCartPaid_NotFound(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	err := store.MarkCartPaid(ctx, "cart_nonexistent", "wallet123")
	if err != ErrNotFound {
		t.Errorf("MarkCartPaid() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryStore_HasCartAccess(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	cartID := "cart_access"
	wallet := "wallet123"

	// Save cart
	usdc, _ := money.GetAsset("USDC")
	total, _ := money.FromMajor(usdc, "10.0")
	quote := CartQuote{
		ID:    cartID,
		Total: total,
	}
	store.SaveCartQuote(ctx, quote)

	// Should not have access before payment
	hasAccess := store.HasCartAccess(ctx, cartID, wallet)
	if hasAccess {
		t.Error("HasCartAccess() should be false before payment")
	}

	// Mark as paid
	store.MarkCartPaid(ctx, cartID, wallet)

	// Should have access after payment
	hasAccess = store.HasCartAccess(ctx, cartID, wallet)
	if !hasAccess {
		t.Error("HasCartAccess() should be true after payment")
	}

	// Different wallet should not have access
	hasAccess = store.HasCartAccess(ctx, cartID, "wallet999")
	if hasAccess {
		t.Error("HasCartAccess() should be false for different wallet")
	}

	// Non-existent cart should not have access
	hasAccess = store.HasCartAccess(ctx, "cart_nonexistent", wallet)
	if hasAccess {
		t.Error("HasCartAccess() should be false for non-existent cart")
	}
}

func TestMemoryStore_RemoveExpiredCarts(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	// Create Money values
	usdc, _ := money.GetAsset("USDC")
	expiredTotal, _ := money.FromMajor(usdc, "10.0")
	validTotal, _ := money.FromMajor(usdc, "20.0")

	// Add expired cart
	expiredCart := CartQuote{
		ID:        "cart_expired",
		Total:     expiredTotal,
		CreatedAt: time.Now().Add(-1 * time.Hour),
		ExpiresAt: time.Now().Add(-30 * time.Minute),
	}
	store.SaveCartQuote(ctx, expiredCart)

	// Add valid cart
	validCart := CartQuote{
		ID:        "cart_valid",
		Total:     validTotal,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	store.SaveCartQuote(ctx, validCart)

	// Manually trigger cleanup
	store.removeExpiredCarts()

	// Expired cart should be removed (GetCartQuote will return ErrNotFound, not ErrCartExpired)
	store.mu.RLock()
	_, exists := store.cartQuotes["cart_expired"]
	store.mu.RUnlock()
	if exists {
		t.Error("Expired cart should be removed by cleanup")
	}

	// Valid cart should remain
	_, err := store.GetCartQuote(ctx, "cart_valid")
	if err != nil {
		t.Errorf("Valid cart should remain after cleanup, got error: %v", err)
	}
}

func TestMemoryStore_CartWithMetadata(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	metadata := map[string]string{
		"user_id":   "123",
		"campaign":  "summer-sale",
		"affiliate": "partner-xyz",
	}

	usdc, _ := money.GetAsset("USDC")
	itemPrice, _ := money.FromMajor(usdc, "10.0")
	totalPrice, _ := money.FromMajor(usdc, "10.0")

	quote := CartQuote{
		ID: "cart_metadata",
		Items: []CartItem{
			{
				ResourceID: "item-1",
				Quantity:   1,
				Price:      itemPrice,
				Metadata:   map[string]string{"color": "red"},
			},
		},
		Total:    totalPrice,
		Metadata: metadata,
	}

	store.SaveCartQuote(ctx, quote)

	retrieved, err := store.GetCartQuote(ctx, "cart_metadata")
	if err != nil {
		t.Fatalf("GetCartQuote() error = %v", err)
	}

	// Verify cart-level metadata
	if retrieved.Metadata["user_id"] != "123" {
		t.Errorf("Metadata user_id = %q, want '123'", retrieved.Metadata["user_id"])
	}
	if retrieved.Metadata["campaign"] != "summer-sale" {
		t.Errorf("Metadata campaign = %q, want 'summer-sale'", retrieved.Metadata["campaign"])
	}

	// Verify item-level metadata
	if retrieved.Items[0].Metadata["color"] != "red" {
		t.Errorf("Item metadata color = %q, want 'red'", retrieved.Items[0].Metadata["color"])
	}
}

func TestMemoryStore_MultipleItems(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	usdc, _ := money.GetAsset("USDC")
	price1, _ := money.FromMajor(usdc, "10.0")
	price2, _ := money.FromMajor(usdc, "25.0")
	price3, _ := money.FromMajor(usdc, "5.0")
	totalPrice, _ := money.FromMajor(usdc, "60.0") // (2*10) + (1*25) + (3*5)

	items := []CartItem{
		{
			ResourceID: "item-1",
			Quantity:   2,
			Price:      price1,
		},
		{
			ResourceID: "item-2",
			Quantity:   1,
			Price:      price2,
		},
		{
			ResourceID: "item-3",
			Quantity:   3,
			Price:      price3,
		},
	}

	quote := CartQuote{
		ID:    "cart_multiple",
		Items: items,
		Total: totalPrice,
	}

	store.SaveCartQuote(ctx, quote)

	retrieved, err := store.GetCartQuote(ctx, "cart_multiple")
	if err != nil {
		t.Fatalf("GetCartQuote() error = %v", err)
	}

	if len(retrieved.Items) != 3 {
		t.Errorf("Items length = %d, want 3", len(retrieved.Items))
	}

	if retrieved.Total.ToMajor() != "60.000000" {
		t.Errorf("Total = %s, want 60.000000", retrieved.Total.ToMajor())
	}

	// Verify each item
	for i, item := range retrieved.Items {
		if item.ResourceID != items[i].ResourceID {
			t.Errorf("Item %d ResourceID = %q, want %q", i, item.ResourceID, items[i].ResourceID)
		}
		if item.Quantity != items[i].Quantity {
			t.Errorf("Item %d Quantity = %d, want %d", i, item.Quantity, items[i].Quantity)
		}
	}
}
