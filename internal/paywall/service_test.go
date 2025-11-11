package paywall

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/CedrosPay/server/internal/callbacks"
	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/products"
	"github.com/CedrosPay/server/internal/storage"
	"github.com/CedrosPay/server/pkg/x402"
)

type stubVerifier struct {
	result x402.VerificationResult
	err    error
}

func (s stubVerifier) Verify(_ context.Context, proof x402.PaymentProof, requirement x402.Requirement) (x402.VerificationResult, error) {
	if s.err != nil {
		return x402.VerificationResult{}, s.err
	}
	res := s.result
	if res.Wallet == "" {
		res.Wallet = proof.Payer
	}
	if res.Signature == "" {
		res.Signature = proof.Signature
	}
	if res.Amount == 0 {
		// Use amount from requirement
		res.Amount = requirement.Amount
	}
	if res.ExpiresAt.IsZero() {
		res.ExpiresAt = time.Now().Add(time.Hour)
	}
	return res, nil
}

func TestGenerateQuoteIncludesStripeAndCrypto(t *testing.T) {
	cfg := testConfig()
	svc := NewService(cfg, storage.NewMemoryStore(), stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	quote, err := svc.GenerateQuote(context.Background(), "demo-content", "")
	if err != nil {
		t.Fatalf("GenerateQuote error: %v", err)
	}
	if quote.Stripe == nil {
		t.Fatal("expected stripe option")
	}
	if quote.Crypto == nil {
		t.Fatal("expected crypto quote")
	}
	if quote.Stripe.AmountCents != cfg.Paywall.Resources["demo-content"].FiatAmountCents {
		t.Logf("Expected: %d, Got: %d", cfg.Paywall.Resources["demo-content"].FiatAmountCents, quote.Stripe.AmountCents)
		t.Fatalf("unexpected amount: %d (expected %d)", quote.Stripe.AmountCents, cfg.Paywall.Resources["demo-content"].FiatAmountCents)
	}
}

func TestAuthorizeRequiresPayment(t *testing.T) {
	cfg := testConfig()
	svc := NewService(cfg, storage.NewMemoryStore(), stubVerifier{}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	result, err := svc.Authorize(context.Background(), "demo-content", "", "", "")
	if err != nil {
		t.Fatalf("Authorize error: %v", err)
	}
	if result.Granted {
		t.Fatal("expected payment requirement")
	}
	if result.Quote == nil {
		t.Fatal("expected quote in response")
	}
}

func TestAuthorizeWithMockPayment(t *testing.T) {
	cfg := testConfig()
	store := storage.NewMemoryStore()
	svc := NewService(cfg, store, stubVerifier{
		result: x402.VerificationResult{
			Wallet:    "payer-wallet",
			Amount:    1.0, // 1.0 USDC
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}, callbacks.NoopNotifier{}, testRepository(cfg), nil, nil)

	// Create x402 Payment Payload
	paymentPayload := x402.PaymentPayload{
		X402Version: 0,
		Scheme:      "solana-spl-transfer",
		Network:     cfg.X402.Network,
		Payload: x402.SolanaPayload{
			Signature:   computeSignature("demo-content", cfg.X402.PaymentAddress, "secret"),
			Transaction: base64.StdEncoding.EncodeToString([]byte("signed-tx")),
		},
	}
	payload, err := json.Marshal(paymentPayload)
	if err != nil {
		t.Fatalf("marshal payment payload: %v", err)
	}
	header := base64.StdEncoding.EncodeToString(payload)

	result, err := svc.Authorize(context.Background(), "demo-content", "", header, "")
	if err != nil {
		t.Fatalf("Authorize error: %v", err)
	}
	if !result.Granted {
		t.Fatal("expected access to be granted")
	}
	if result.Method != "x402" {
		t.Fatalf("unexpected method: %s", result.Method)
	}
	// Verify Settlement response is populated
	if result.Settlement == nil {
		t.Fatal("expected settlement response")
	}
	if !result.Settlement.Success {
		t.Fatal("expected settlement success to be true")
	}
	if result.Settlement.TxHash == nil || *result.Settlement.TxHash == "" {
		t.Fatal("expected settlement txHash")
	}
	if result.Settlement.NetworkID == nil || *result.Settlement.NetworkID != cfg.X402.Network {
		t.Fatalf("expected settlement networkId %s, got %v", cfg.X402.Network, result.Settlement.NetworkID)
	}
	if result.Settlement.Error != nil {
		t.Fatalf("expected no error, got %v", *result.Settlement.Error)
	}
}

// TestAuthorizeWithStripeSession removed - Stripe session tracking has been removed
// System now relies on webhook callbacks for access control

func testConfig() *config.Config {
	cfg := &config.Config{
		Server: config.ServerConfig{},
		Stripe: config.StripeConfig{},
		X402: config.X402Config{
			PaymentAddress: "11111111111111111111111111111111",
			TokenMint:      "So11111111111111111111111111111111111111112",
			Network:        "mainnet-beta",
			RPCURL:         "https://api.mainnet-beta.solana.com",
			WSURL:          "wss://api.mainnet-beta.solana.com",
			AllowedTokens:  []string{"USDC"},
			TokenDecimals:  6,
		},
		Paywall: config.PaywallConfig{
			QuoteTTL: config.Duration{Duration: time.Minute},
			Resources: map[string]config.PaywallResource{
				"demo-content": {
					ResourceID:         "demo-content",
					FiatAmountCents:    100, // 1.0 USD in cents
					FiatCurrency:       "USD", // Must match asset registry (case-sensitive)
					StripePriceID:      "price_123",
					Description:        "demo",
					CryptoAtomicAmount: 1000000, // 1.0 USDC (6 decimals)
					CryptoToken:        "USDC",
					CryptoAccount:      "11111111111111111111111111111111",
				},
			},
		},
	}
	return cfg
}

func testRepository(cfg *config.Config) products.Repository {
	return products.NewYAMLRepository(cfg.Paywall.Resources)
}

func computeSignature(resourceID, recipient, secret string) string {
	msg := resourceID + ":" + recipient
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return base64.RawStdEncoding.EncodeToString(mac.Sum(nil))
}
