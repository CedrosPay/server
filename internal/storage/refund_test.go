package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CedrosPay/server/internal/money"
)

func TestGenerateRefundID(t *testing.T) {
	// Generate multiple IDs and verify format
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateRefundID()
		if err != nil {
			t.Fatalf("GenerateRefundID() error = %v", err)
		}

		// Check format
		if !strings.HasPrefix(id, "refund_") {
			t.Errorf("GenerateRefundID() = %q, should start with 'refund_'", id)
		}

		// Check length (refund_ + 32 hex chars)
		expectedLen := 7 + 32 // "refund_" + hex(16 bytes)
		if len(id) != expectedLen {
			t.Errorf("GenerateRefundID() length = %d, want %d", len(id), expectedLen)
		}

		// Check uniqueness
		if ids[id] {
			t.Errorf("GenerateRefundID() generated duplicate: %q", id)
		}
		ids[id] = true
	}
}

func TestRefundQuote_IsExpired(t *testing.T) {
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
			refund := RefundQuote{
				ExpiresAt: tt.expiresAt,
			}
			now := time.Now()
			if got := refund.IsExpiredAt(now); got != tt.want {
				t.Errorf("IsExpiredAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRefundQuote_IsProcessed(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name        string
		processedAt *time.Time
		signature   string
		want        bool
	}{
		{
			name:        "not processed (nil timestamp)",
			processedAt: nil,
			signature:   "",
			want:        false,
		},
		{
			name:        "not processed (no signature)",
			processedAt: &now,
			signature:   "",
			want:        false,
		},
		{
			name:        "not processed (no timestamp)",
			processedAt: nil,
			signature:   "sig123",
			want:        false,
		},
		{
			name:        "processed (both present)",
			processedAt: &now,
			signature:   "sig123",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refund := RefundQuote{
				ProcessedAt: tt.processedAt,
				Signature:   tt.signature,
			}
			if got := refund.IsProcessed(); got != tt.want {
				t.Errorf("IsProcessed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemoryStore_SaveRefundQuote(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	usdc, _ := money.GetAsset("USDC")
	amount, _ := money.FromMajor(usdc, "50.0")

	refundID := "refund_test123"
	quote := RefundQuote{
		ID:                 refundID,
		OriginalPurchaseID: "purchase-1",
		RecipientWallet:    "wallet123",
		Amount:             amount,
		Reason:             "customer request",
		Metadata:           map[string]string{"user_id": "123"},
	}

	err := store.SaveRefundQuote(ctx, quote)
	if err != nil {
		t.Fatalf("SaveRefundQuote() error = %v", err)
	}

	// Verify it was saved
	retrieved, err := store.GetRefundQuote(ctx, refundID)
	if err != nil {
		t.Fatalf("GetRefundQuote() error = %v", err)
	}

	if retrieved.ID != quote.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, quote.ID)
	}
	if retrieved.Amount.ToMajor() != "50.000000" {
		t.Errorf("Amount = %s, want 50.000000", retrieved.Amount.ToMajor())
	}
	if retrieved.RecipientWallet != quote.RecipientWallet {
		t.Errorf("RecipientWallet = %q, want %q", retrieved.RecipientWallet, quote.RecipientWallet)
	}
}

func TestMemoryStore_SaveRefundQuote_AutoTimestamps(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	usdc, _ := money.GetAsset("USDC")
	amount, _ := money.FromMajor(usdc, "50.0")

	refundID := "refund_test456"
	quote := RefundQuote{
		ID:              refundID,
		RecipientWallet: "wallet123",
		Amount:          amount,
		// CreatedAt and ExpiresAt are zero - should be auto-set
	}

	err := store.SaveRefundQuote(ctx, quote)
	if err != nil {
		t.Fatalf("SaveRefundQuote() error = %v", err)
	}

	retrieved, err := store.GetRefundQuote(ctx, refundID)
	if err != nil {
		t.Fatalf("GetRefundQuote() error = %v", err)
	}

	// Verify timestamps were auto-set
	if retrieved.CreatedAt.IsZero() {
		t.Error("CreatedAt should be auto-set")
	}
	if retrieved.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be auto-set")
	}

	// Verify ExpiresAt is CreatedAt + 15 minutes (default TTL)
	expectedExpiry := retrieved.CreatedAt.Add(15 * time.Minute)
	if !retrieved.ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("ExpiresAt = %v, want %v", retrieved.ExpiresAt, expectedExpiry)
	}
}

func TestMemoryStore_SaveRefundQuote_RequiresID(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	usdc, _ := money.GetAsset("USDC")
	amount, _ := money.FromMajor(usdc, "50.0")

	quote := RefundQuote{
		// Missing ID
		RecipientWallet: "wallet123",
		Amount:          amount,
	}

	err := store.SaveRefundQuote(ctx, quote)
	if err == nil {
		t.Error("SaveRefundQuote() should error when ID is missing")
	}
	if !strings.Contains(err.Error(), "requires id") {
		t.Errorf("error message = %q, should mention 'requires id'", err.Error())
	}
}

func TestMemoryStore_GetRefundQuote(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	usdc, _ := money.GetAsset("USDC")
	amount, _ := money.FromMajor(usdc, "50.0")

	tests := []struct {
		name      string
		refundID  string
		setup     func()
		wantErr   bool
		errIs     error
		checkData func(RefundQuote)
	}{
		{
			name:     "not found",
			refundID: "nonexistent",
			setup:    func() {},
			wantErr:  true,
			errIs:    ErrNotFound,
		},
		{
			name:     "found valid quote",
			refundID: "refund_valid",
			setup: func() {
				store.SaveRefundQuote(ctx, RefundQuote{
					ID:              "refund_valid",
					RecipientWallet: "wallet123",
					Amount:          amount,
					ExpiresAt:       time.Now().Add(10 * time.Minute),
				})
			},
			wantErr: false,
			checkData: func(r RefundQuote) {
				if r.ID != "refund_valid" {
					t.Errorf("ID = %q, want 'refund_valid'", r.ID)
				}
			},
		},
		{
			name:     "expired quote (still retrievable)",
			refundID: "refund_expired",
			setup: func() {
				store.SaveRefundQuote(ctx, RefundQuote{
					ID:              "refund_expired",
					RecipientWallet: "wallet123",
					Amount:          amount,
					CreatedAt:       time.Now().Add(-2 * time.Hour),
					ExpiresAt:       time.Now().Add(-1 * time.Hour),
				})
			},
			wantErr: false, // Refund requests never expire - they remain retrievable
			checkData: func(r RefundQuote) {
				if r.ID != "refund_expired" {
					t.Errorf("ID = %q, want 'refund_expired'", r.ID)
				}
				// Verify it's expired (can be re-quoted by admin)
				now := time.Now()
				if !r.IsExpiredAt(now) {
					t.Error("Expected quote to be expired but still retrievable")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			got, err := store.GetRefundQuote(ctx, tt.refundID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetRefundQuote() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.errIs != nil && err != tt.errIs {
				t.Errorf("GetRefundQuote() error = %v, want %v", err, tt.errIs)
			}
			if !tt.wantErr && tt.checkData != nil {
				tt.checkData(got)
			}
		})
	}
}

func TestMemoryStore_MarkRefundProcessed(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	usdc, _ := money.GetAsset("USDC")
	amount, _ := money.FromMajor(usdc, "50.0")

	refundID := "refund_test789"
	quote := RefundQuote{
		ID:              refundID,
		RecipientWallet: "wallet123",
		Amount:          amount,
		ExpiresAt:       time.Now().Add(10 * time.Minute),
	}

	// Save initial quote
	err := store.SaveRefundQuote(ctx, quote)
	if err != nil {
		t.Fatalf("SaveRefundQuote() error = %v", err)
	}

	// Mark as processed
	processedBy := "server_wallet"
	signature := "tx_sig_123"
	err = store.MarkRefundProcessed(ctx, refundID, processedBy, signature)
	if err != nil {
		t.Fatalf("MarkRefundProcessed() error = %v", err)
	}

	// Verify it was marked
	retrieved, err := store.GetRefundQuote(ctx, refundID)
	if err != nil {
		t.Fatalf("GetRefundQuote() error = %v", err)
	}

	if retrieved.ProcessedBy != processedBy {
		t.Errorf("ProcessedBy = %q, want %q", retrieved.ProcessedBy, processedBy)
	}
	if retrieved.Signature != signature {
		t.Errorf("Signature = %q, want %q", retrieved.Signature, signature)
	}
	if retrieved.ProcessedAt == nil {
		t.Error("ProcessedAt should be set")
	}
	if !retrieved.IsProcessed() {
		t.Error("IsProcessed() should be true")
	}
}

func TestMemoryStore_MarkRefundProcessed_NotFound(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	err := store.MarkRefundProcessed(ctx, "nonexistent", "wallet", "sig")
	if err != ErrNotFound {
		t.Errorf("MarkRefundProcessed() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryStore_RemoveExpiredRefunds(t *testing.T) {
	store := NewMemoryStore()
	defer store.Stop()
	ctx := context.Background()

	usdc, _ := money.GetAsset("USDC")
	amount1, _ := money.FromMajor(usdc, "10.0")
	amount2, _ := money.FromMajor(usdc, "20.0")

	// Add expired refund
	store.SaveRefundQuote(ctx, RefundQuote{
		ID:              "refund_expired",
		RecipientWallet: "wallet1",
		Amount:          amount1,
		CreatedAt:       time.Now().Add(-2 * time.Hour),
		ExpiresAt:       time.Now().Add(-1 * time.Hour),
	})

	// Add valid refund
	store.SaveRefundQuote(ctx, RefundQuote{
		ID:              "refund_valid",
		RecipientWallet: "wallet2",
		Amount:          amount2,
		ExpiresAt:       time.Now().Add(10 * time.Minute),
	})

	// Verify both exist
	if _, exists := store.refundQuotes["refund_expired"]; !exists {
		t.Error("expired refund should exist before cleanup")
	}
	if _, exists := store.refundQuotes["refund_valid"]; !exists {
		t.Error("valid refund should exist")
	}

	// Run cleanup
	store.removeExpiredRefunds()

	// Verify expired was removed
	if _, exists := store.refundQuotes["refund_expired"]; exists {
		t.Error("expired refund should be removed after cleanup")
	}

	// Verify valid remains
	if _, exists := store.refundQuotes["refund_valid"]; !exists {
		t.Error("valid refund should still exist after cleanup")
	}
}
