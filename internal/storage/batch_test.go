package storage

import (
	"context"
	"testing"
	"time"

	"github.com/CedrosPay/server/internal/money"
)

func TestMemoryStore_SaveCartQuotes(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Create test quotes
	quotes := []CartQuote{
		{
			ID:        "cart_1",
			Items:     []CartItem{{ResourceID: "prod1", Quantity: 1, Price: money.New(money.MustGetAsset("USDC"), 100)}},
			Total:     money.New(money.MustGetAsset("USDC"), 100),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(15 * time.Minute),
		},
		{
			ID:        "cart_2",
			Items:     []CartItem{{ResourceID: "prod2", Quantity: 2, Price: money.New(money.MustGetAsset("USDC"), 200)}},
			Total:     money.New(money.MustGetAsset("USDC"), 400),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(15 * time.Minute),
		},
	}

	// Save batch
	err := store.SaveCartQuotes(ctx, quotes)
	if err != nil {
		t.Fatalf("SaveCartQuotes failed: %v", err)
	}

	// Verify both quotes exist
	for _, expectedQuote := range quotes {
		quote, err := store.GetCartQuote(ctx, expectedQuote.ID)
		if err != nil {
			t.Errorf("Failed to get cart %s: %v", expectedQuote.ID, err)
			continue
		}
		if quote.Total.Atomic != expectedQuote.Total.Atomic {
			t.Errorf("Cart %s: expected total %d, got %d", expectedQuote.ID, expectedQuote.Total.Atomic, quote.Total.Atomic)
		}
	}
}

func TestMemoryStore_GetCartQuotes(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Create and save test quotes
	quotes := []CartQuote{
		{
			ID:        "cart_batch_1",
			Items:     []CartItem{{ResourceID: "prod1", Quantity: 1, Price: money.New(money.MustGetAsset("USDC"), 100)}},
			Total:     money.New(money.MustGetAsset("USDC"), 100),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(15 * time.Minute),
		},
		{
			ID:        "cart_batch_2",
			Items:     []CartItem{{ResourceID: "prod2", Quantity: 1, Price: money.New(money.MustGetAsset("USDC"), 200)}},
			Total:     money.New(money.MustGetAsset("USDC"), 200),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(15 * time.Minute),
		},
	}

	for _, quote := range quotes {
		if err := store.SaveCartQuote(ctx, quote); err != nil {
			t.Fatalf("SaveCartQuote failed: %v", err)
		}
	}

	// Batch retrieve
	cartIDs := []string{"cart_batch_1", "cart_batch_2", "cart_missing"}
	retrieved, err := store.GetCartQuotes(ctx, cartIDs)
	if err != nil {
		t.Fatalf("GetCartQuotes failed: %v", err)
	}

	// Should get 2 quotes (missing one is skipped)
	if len(retrieved) != 2 {
		t.Errorf("Expected 2 quotes, got %d", len(retrieved))
	}
}

func TestMemoryStore_RecordPayments(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Create test transactions
	txs := []PaymentTransaction{
		{
			Signature:  "sig_batch_1",
			ResourceID: "resource1",
			Wallet:     "wallet1",
			Amount:     money.New(money.MustGetAsset("USDC"), 100),
			CreatedAt:  time.Now(),
		},
		{
			Signature:  "sig_batch_2",
			ResourceID: "resource2",
			Wallet:     "wallet2",
			Amount:     money.New(money.MustGetAsset("USDC"), 200),
			CreatedAt:  time.Now(),
		},
	}

	// Record batch
	err := store.RecordPayments(ctx, txs)
	if err != nil {
		t.Fatalf("RecordPayments failed: %v", err)
	}

	// Verify all transactions exist
	for _, tx := range txs {
		processed, err := store.HasPaymentBeenProcessed(ctx, tx.Signature)
		if err != nil {
			t.Errorf("Failed to check payment %s: %v", tx.Signature, err)
		}
		if !processed {
			t.Errorf("Payment %s not found", tx.Signature)
		}
	}
}

func TestMemoryStore_RecordPayments_DuplicateSignature(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Create first transaction
	tx1 := PaymentTransaction{
		Signature:  "sig_duplicate",
		ResourceID: "resource1",
		Wallet:     "wallet1",
		Amount:     money.New(money.MustGetAsset("USDC"), 100),
		CreatedAt:  time.Now(),
	}

	// Record it
	if err := store.RecordPayment(ctx, tx1); err != nil {
		t.Fatalf("RecordPayment failed: %v", err)
	}

	// Try to record batch with duplicate signature
	txs := []PaymentTransaction{
		{
			Signature:  "sig_new",
			ResourceID: "resource2",
			Wallet:     "wallet2",
			Amount:     money.New(money.MustGetAsset("USDC"), 200),
			CreatedAt:  time.Now(),
		},
		{
			Signature:  "sig_duplicate", // Duplicate!
			ResourceID: "resource3",
			Wallet:     "wallet3",
			Amount:     money.New(money.MustGetAsset("USDC"), 300),
			CreatedAt:  time.Now(),
		},
	}

	// Should fail due to duplicate
	err := store.RecordPayments(ctx, txs)
	if err == nil {
		t.Error("Expected error for duplicate signature, got nil")
	}
}

func TestMemoryStore_SaveRefundQuotes(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	quotes := []RefundQuote{
		{
			ID:                 "refund_batch_1",
			OriginalPurchaseID: "purchase1",
			RecipientWallet:    "wallet1",
			Amount:             money.New(money.MustGetAsset("USDC"), 100),
			CreatedAt:          time.Now(),
			ExpiresAt:          time.Now().Add(15 * time.Minute),
		},
		{
			ID:                 "refund_batch_2",
			OriginalPurchaseID: "purchase2",
			RecipientWallet:    "wallet2",
			Amount:             money.New(money.MustGetAsset("USDC"), 200),
			CreatedAt:          time.Now(),
			ExpiresAt:          time.Now().Add(15 * time.Minute),
		},
	}

	// Save batch
	err := store.SaveRefundQuotes(ctx, quotes)
	if err != nil {
		t.Fatalf("SaveRefundQuotes failed: %v", err)
	}

	// Verify all refunds exist
	for _, expectedQuote := range quotes {
		quote, err := store.GetRefundQuote(ctx, expectedQuote.ID)
		if err != nil {
			t.Errorf("Failed to get refund %s: %v", expectedQuote.ID, err)
			continue
		}
		if quote.Amount.Atomic != expectedQuote.Amount.Atomic {
			t.Errorf("Refund %s: expected amount %d, got %d", expectedQuote.ID, expectedQuote.Amount.Atomic, quote.Amount.Atomic)
		}
	}
}
