package paywall

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/CedrosPay/server/internal/callbacks"
	"github.com/CedrosPay/server/internal/money"
	"github.com/CedrosPay/server/internal/storage"
	"github.com/CedrosPay/server/pkg/x402"
)

func TestGenerateRefundQuote_ValidRequest(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()
	svc := NewService(cfg, store, stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	req := RefundQuoteRequest{
		OriginalPurchaseID: "purchase_123",
		RecipientWallet:    "11111111111111111111111111111111", // Valid Solana address
		Amount:             10.5,
		Token:              "USDC",
		Reason:             "customer request",
		Metadata:           map[string]string{"order_id": "456"},
	}

	// First create the refund request
	refundQuote, err := svc.CreateRefundRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}

	// Then regenerate quote to get RefundQuoteResponse
	resp, err := svc.RegenerateRefundQuote(context.Background(), refundQuote.ID)
	if err != nil {
		t.Fatalf("RegenerateRefundQuote() error = %v", err)
	}

	// Verify response structure
	if resp.RefundID == "" {
		t.Error("RefundID should not be empty")
	}
	if resp.Quote == nil {
		t.Fatal("Quote should not be nil")
	}
	if resp.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set")
	}

	// Verify quote details
	if resp.Quote.Scheme != "solana-spl-transfer" {
		t.Errorf("Scheme = %q, want 'solana-spl-transfer'", resp.Quote.Scheme)
	}
	if resp.Quote.Network != cfg.X402.Network {
		t.Errorf("Network = %q, want %q", resp.Quote.Network, cfg.X402.Network)
	}
	if resp.Quote.PayTo != req.RecipientWallet {
		t.Errorf("PayTo = %q, want %q", resp.Quote.PayTo, req.RecipientWallet)
	}
	if resp.Quote.Asset != cfg.X402.TokenMint {
		t.Errorf("Asset = %q, want %q", resp.Quote.Asset, cfg.X402.TokenMint)
	}

	// Verify refund was saved to storage
	savedRefund, err := store.GetRefundQuote(context.Background(), resp.RefundID)
	if err != nil {
		t.Fatalf("GetRefundQuote() error = %v", err)
	}
	if savedRefund.OriginalPurchaseID != req.OriginalPurchaseID {
		t.Errorf("OriginalPurchaseID = %q, want %q", savedRefund.OriginalPurchaseID, req.OriginalPurchaseID)
	}
	// Compare Money amount with float64 request
	savedAmount, _ := strconv.ParseFloat(savedRefund.Amount.ToMajor(), 64)
	if savedAmount != req.Amount {
		t.Errorf("Amount = %f, want %f", savedAmount, req.Amount)
	}
}

func TestGenerateRefundQuote_MissingOriginalPurchaseID(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()
	svc := NewService(cfg, store, stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	req := RefundQuoteRequest{
		RecipientWallet: "11111111111111111111111111111111",
		Amount:          10.0,
		Token:           "USDC",
	}

	_, err := svc.CreateRefundRequest(context.Background(), req)
	if err == nil {
		t.Error("CreateRefundRequest() should error when OriginalPurchaseID is missing")
	}
	if err != nil && err.Error() != "paywall: originalPurchaseId required" {
		t.Errorf("error = %q, want 'paywall: originalPurchaseId required'", err.Error())
	}
}

func TestGenerateRefundQuote_MissingRecipientWallet(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()
	svc := NewService(cfg, store, stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	req := RefundQuoteRequest{
		OriginalPurchaseID: "purchase_123",
		Amount:             10.0,
		Token:              "USDC",
	}

	_, err := svc.CreateRefundRequest(context.Background(), req)
	if err == nil {
		t.Error("GenerateRefundQuote() should error when RecipientWallet is missing")
	}
	if err != nil && err.Error() != "paywall: recipientWallet required" {
		t.Errorf("error = %q, want 'paywall: recipientWallet required'", err.Error())
	}
}

func TestGenerateRefundQuote_InvalidAmount(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()
	svc := NewService(cfg, store, stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	tests := []struct {
		name   string
		amount float64
	}{
		{"zero amount", 0},
		{"negative amount", -10.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := RefundQuoteRequest{
				OriginalPurchaseID: "purchase_123",
				RecipientWallet:    "11111111111111111111111111111111",
				Amount:             tt.amount,
				Token:              "USDC",
			}

			_, err := svc.CreateRefundRequest(context.Background(), req)
			if err == nil {
				t.Error("CreateRefundRequest() should error for invalid amount")
			}
		})
	}
}

func TestGenerateRefundQuote_MissingToken(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()
	svc := NewService(cfg, store, stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	req := RefundQuoteRequest{
		OriginalPurchaseID: "purchase_123",
		RecipientWallet:    "11111111111111111111111111111111",
		Amount:             10.0,
	}

	_, err := svc.CreateRefundRequest(context.Background(), req)
	if err == nil {
		t.Error("CreateRefundRequest() should error when Token is missing")
	}
}

func TestGenerateRefundQuote_InvalidWalletAddress(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()
	svc := NewService(cfg, store, stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	req := RefundQuoteRequest{
		OriginalPurchaseID: "purchase_123",
		RecipientWallet:    "invalid-wallet",
		Amount:             10.0,
		Token:              "USDC",
	}

	_, err := svc.CreateRefundRequest(context.Background(), req)
	if err == nil {
		t.Error("CreateRefundRequest() should error for invalid wallet address")
	}
}

func TestAuthorizeRefund_Success(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()

	// Setup successful verifier
	svc := NewService(cfg, store, stubVerifier{
		result: x402.VerificationResult{
			Wallet:    cfg.X402.PaymentAddress, // Server wallet
			Amount:    10.5,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	// Generate refund quote first
	req := RefundQuoteRequest{
		OriginalPurchaseID: "purchase_123",
		RecipientWallet:    "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin", // Valid Solana address
		Amount:             10.5,
		Token:              "USDC",
		Reason:             "customer request",
	}

	// First create the refund request
	refundQuote, err := svc.CreateRefundRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}

	// Then regenerate quote to get RefundQuoteResponse
	resp, err := svc.RegenerateRefundQuote(context.Background(), refundQuote.ID)
	if err != nil {
		t.Fatalf("RegenerateRefundQuote() error = %v", err)
	}

	// Build payment proof (server executing refund)
	refundID := resp.RefundID
	paymentPayload := x402.PaymentPayload{
		X402Version: 0,
		Scheme:      "solana-spl-transfer",
		Network:     cfg.X402.Network,
		Payload: x402.SolanaPayload{
			Signature:   "refund_signature_123",
			Transaction: base64.StdEncoding.EncodeToString([]byte("signed-refund-tx")),
		},
	}
	payload, err := json.Marshal(paymentPayload)
	if err != nil {
		t.Fatalf("marshal payment payload: %v", err)
	}
	header := base64.StdEncoding.EncodeToString(payload)

	// Authorize refund
	result, err := svc.Authorize(context.Background(), refundID, "", header, "")
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}

	// Verify authorization was granted
	if !result.Granted {
		t.Error("Authorize() granted = false, want true")
	}
	if result.Method != "x402-refund" {
		t.Errorf("Method = %q, want 'x402-refund'", result.Method)
	}
	if result.Wallet != cfg.X402.PaymentAddress {
		t.Errorf("Wallet = %q, want %q", result.Wallet, cfg.X402.PaymentAddress)
	}

	// Verify settlement response
	if result.Settlement == nil {
		t.Fatal("Settlement should not be nil")
	}
	if !result.Settlement.Success {
		t.Error("Settlement.Success = false, want true")
	}
	if result.Settlement.TxHash == nil || *result.Settlement.TxHash == "" {
		t.Error("Settlement.TxHash should be set")
	}

	// Verify refund was marked as processed
	savedRefund, err := store.GetRefundQuote(context.Background(), refundID)
	if err != nil {
		t.Fatalf("GetRefundQuote() error = %v", err)
	}
	if !savedRefund.IsProcessed() {
		t.Error("Refund should be marked as processed")
	}
	if savedRefund.ProcessedBy != cfg.X402.PaymentAddress {
		t.Errorf("ProcessedBy = %q, want %q", savedRefund.ProcessedBy, cfg.X402.PaymentAddress)
	}
	if savedRefund.Signature != "refund_signature_123" {
		t.Errorf("Signature = %q, want 'refund_signature_123'", savedRefund.Signature)
	}
}

func TestAuthorizeRefund_MissingPaymentHeader(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()
	svc := NewService(cfg, store, stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	// Generate refund quote
	req := RefundQuoteRequest{
		OriginalPurchaseID: "purchase_123",
		RecipientWallet:    "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin", // Valid Solana address
		Amount:             10.5,
		Token:              "USDC",
	}

	// First create the refund request
	refundQuote, err := svc.CreateRefundRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}

	// Then regenerate quote to get RefundQuoteResponse
	resp, err := svc.RegenerateRefundQuote(context.Background(), refundQuote.ID)
	if err != nil {
		t.Fatalf("RegenerateRefundQuote() error = %v", err)
	}

	// Try to authorize refund via normal Authorize method - it should route to authorizeRefund
	result, err := svc.Authorize(context.Background(), resp.RefundID, "", "", "")
	if err == nil {
		t.Error("Authorize() should error when payment header is missing")
	}
	// Should still get a result even if not granted
	if result.Granted {
		t.Error("Authorize() should not grant access without payment header")
	}
}

func TestAuthorizeRefund_ExpiredQuote(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()
	svc := NewService(cfg, store, stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	// Manually create expired refund quote
	refundID := "refund_expired_123"
	usdc, _ := money.GetAsset("USDC")
	refundAmount, _ := money.FromMajor(usdc, "10.5")
	expiredQuote := storage.RefundQuote{
		ID:                 refundID,
		OriginalPurchaseID: "purchase_123",
		RecipientWallet:    "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin", // Valid Solana address
		Amount:             refundAmount,
		CreatedAt:          time.Now().Add(-2 * time.Hour),
		ExpiresAt:          time.Now().Add(-1 * time.Hour), // Expired
	}

	err := store.SaveRefundQuote(context.Background(), expiredQuote)
	if err != nil {
		t.Fatalf("SaveRefundQuote() error = %v", err)
	}

	// Build payment proof
	paymentPayload := x402.PaymentPayload{
		X402Version: 0,
		Scheme:      "solana-spl-transfer",
		Network:     cfg.X402.Network,
		Payload: x402.SolanaPayload{
			Signature:   "sig_123",
			Transaction: base64.StdEncoding.EncodeToString([]byte("tx")),
		},
	}
	payload, _ := json.Marshal(paymentPayload)
	header := base64.StdEncoding.EncodeToString(payload)

	// Try to authorize expired refund
	_, err = svc.Authorize(context.Background(), refundID, "", header, "")
	if err == nil {
		t.Error("Authorize() should error for expired refund")
	}
	if err != nil && err.Error() != "refund quote expired, please request a new quote" {
		t.Errorf("error = %q, want 'refund quote expired, please request a new quote'", err.Error())
	}
}

func TestAuthorizeRefund_AlreadyProcessed(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()

	svc := NewService(cfg, store, stubVerifier{
		result: x402.VerificationResult{
			Wallet:    cfg.X402.PaymentAddress,
			Amount:    10.5,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	// Generate refund quote
	req := RefundQuoteRequest{
		OriginalPurchaseID: "purchase_123",
		RecipientWallet:    "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin", // Valid Solana address
		Amount:             10.5,
		Token:              "USDC",
	}

	// First create the refund request
	refundQuote, err := svc.CreateRefundRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}

	// Then regenerate quote to get RefundQuoteResponse
	resp, err := svc.RegenerateRefundQuote(context.Background(), refundQuote.ID)
	if err != nil {
		t.Fatalf("RegenerateRefundQuote() error = %v", err)
	}

	// Mark refund as already processed
	err = store.MarkRefundProcessed(context.Background(), resp.RefundID, cfg.X402.PaymentAddress, "existing_sig")
	if err != nil {
		t.Fatalf("MarkRefundProcessed() error = %v", err)
	}

	// Build payment proof
	paymentPayload := x402.PaymentPayload{
		X402Version: 0,
		Scheme:      "solana-spl-transfer",
		Network:     cfg.X402.Network,
		Payload: x402.SolanaPayload{
			Signature:   "sig_123",
			Transaction: base64.StdEncoding.EncodeToString([]byte("tx")),
		},
	}
	payload, _ := json.Marshal(paymentPayload)
	header := base64.StdEncoding.EncodeToString(payload)

	// Try to authorize already processed refund
	_, err = svc.Authorize(context.Background(), resp.RefundID, "", header, "")
	if err == nil {
		t.Error("Authorize() should error for already processed refund")
	}
}

func TestAuthorizeRefund_UnauthorizedPayer(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()

	svc := NewService(cfg, store, stubVerifier{
		result: x402.VerificationResult{
			Wallet:    "DifferentWa11et1111111111111111111111111", // Valid but unauthorized
			Amount:    10.5,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	// Generate refund quote
	req := RefundQuoteRequest{
		OriginalPurchaseID: "purchase_123",
		RecipientWallet:    "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin", // Valid Solana address
		Amount:             10.5,
		Token:              "USDC",
	}

	// First create the refund request
	refundQuote, err := svc.CreateRefundRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}

	// Then regenerate quote to get RefundQuoteResponse
	resp, err := svc.RegenerateRefundQuote(context.Background(), refundQuote.ID)
	if err != nil {
		t.Fatalf("GenerateRefundQuote() error = %v", err)
	}

	// Build payment proof with unauthorized payer
	paymentPayload := x402.PaymentPayload{
		X402Version: 0,
		Scheme:      "solana-spl-transfer",
		Network:     cfg.X402.Network,
		Payload: x402.SolanaPayload{
			Signature:   "sig_123",
			Transaction: base64.StdEncoding.EncodeToString([]byte("tx")),
		},
	}
	payload, _ := json.Marshal(paymentPayload)
	header := base64.StdEncoding.EncodeToString(payload)

	// Try to authorize with unauthorized payer
	_, err = svc.Authorize(context.Background(), resp.RefundID, "", header, "")
	if err == nil {
		t.Error("Authorize() should error when payer is not the configured payment address")
	}
}

func TestAuthorizeRefund_RefundNotFound(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	defer store.Stop()
	svc := NewService(cfg, store, stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	// Build payment proof for non-existent refund
	paymentPayload := x402.PaymentPayload{
		X402Version: 0,
		Scheme:      "solana-spl-transfer",
		Network:     cfg.X402.Network,
		Payload: x402.SolanaPayload{
			Signature:   "sig_123",
			Transaction: base64.StdEncoding.EncodeToString([]byte("tx")),
		},
	}
	payload, _ := json.Marshal(paymentPayload)
	header := base64.StdEncoding.EncodeToString(payload)

	// Try to authorize non-existent refund
	_, err := svc.Authorize(context.Background(), "refund_nonexistent", "", header, "")
	if err == nil {
		t.Error("Authorize() should error for non-existent refund")
	}
}
