package money

import (
	"testing"
)

func TestStripeAdapter_ToStripeAmount(t *testing.T) {
	adapter := NewStripeAdapter()

	tests := []struct {
		name         string
		money        Money
		wantCurrency string
		wantAmount   int64
		wantErr      bool
	}{
		{
			name:         "USD 10.50",
			money:        Money{USD, 1050},
			wantCurrency: "usd",
			wantAmount:   1050,
			wantErr:      false,
		},
		{
			name:         "EUR 25.00",
			money:        Money{EUR, 2500},
			wantCurrency: "eur",
			wantAmount:   2500,
			wantErr:      false,
		},
		{
			name:         "USD 0.01",
			money:        Money{USD, 1},
			wantCurrency: "usd",
			wantAmount:   1,
			wantErr:      false,
		},
		{
			name:    "USDC not supported",
			money:   Money{USDC, 1500000},
			wantErr: true,
		},
		{
			name:    "SOL not supported",
			money:   Money{SOL, 1000000000},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currency, amount, err := adapter.ToStripeAmount(tt.money)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToStripeAmount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if currency != tt.wantCurrency {
					t.Errorf("ToStripeAmount() currency = %v, want %v", currency, tt.wantCurrency)
				}
				if amount != tt.wantAmount {
					t.Errorf("ToStripeAmount() amount = %v, want %v", amount, tt.wantAmount)
				}
			}
		})
	}
}

func TestStripeAdapter_FromStripeAmount(t *testing.T) {
	adapter := NewStripeAdapter()

	tests := []struct {
		name       string
		currency   string
		amount     int64
		wantAtomic int64
		wantAsset  string
		wantErr    bool
	}{
		{
			name:       "usd 1050",
			currency:   "usd",
			amount:     1050,
			wantAtomic: 1050,
			wantAsset:  "USD",
			wantErr:    false,
		},
		{
			name:       "eur 2500",
			currency:   "eur",
			amount:     2500,
			wantAtomic: 2500,
			wantAsset:  "EUR",
			wantErr:    false,
		},
		{
			name:     "unsupported currency",
			currency: "gbp",
			amount:   1000,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adapter.FromStripeAmount(tt.currency, tt.amount)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromStripeAmount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Atomic != tt.wantAtomic {
					t.Errorf("FromStripeAmount() atomic = %v, want %v", got.Atomic, tt.wantAtomic)
				}
				if got.Asset.Code != tt.wantAsset {
					t.Errorf("FromStripeAmount() asset = %v, want %v", got.Asset.Code, tt.wantAsset)
				}
			}
		})
	}
}

func TestStripeAdapter_ValidateStripeAmount(t *testing.T) {
	adapter := NewStripeAdapter()

	tests := []struct {
		name    string
		money   Money
		wantErr bool
	}{
		{
			name:    "valid USD",
			money:   Money{USD, 1050},
			wantErr: false,
		},
		{
			name:    "valid EUR",
			money:   Money{EUR, 2500},
			wantErr: false,
		},
		{
			name:    "negative amount",
			money:   Money{USD, -100},
			wantErr: true,
		},
		{
			name:    "exceeds max",
			money:   Money{USD, 100_000_000}, // $1,000,000.00 - exceeds Stripe limit
			wantErr: true,
		},
		{
			name:    "at max limit",
			money:   Money{USD, 99_999_999}, // $999,999.99 - at limit
			wantErr: false,
		},
		{
			name:    "USDC not supported",
			money:   Money{USDC, 1500000},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ValidateStripeAmount(tt.money)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStripeAmount() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStripeAdapter_RoundTrip(t *testing.T) {
	adapter := NewStripeAdapter()

	tests := []struct {
		name  string
		money Money
	}{
		{"USD 10.50", Money{USD, 1050}},
		{"EUR 25.00", Money{EUR, 2500}},
		{"USD 0.01", Money{USD, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to Stripe format
			currency, amount, err := adapter.ToStripeAmount(tt.money)
			if err != nil {
				t.Fatalf("ToStripeAmount() error = %v", err)
			}

			// Convert back to Money
			roundTrip, err := adapter.FromStripeAmount(currency, amount)
			if err != nil {
				t.Fatalf("FromStripeAmount() error = %v", err)
			}

			// Verify we got the same value back
			if !tt.money.Equal(roundTrip) {
				t.Errorf("Round trip failed: %v → (%s, %d) → %v", tt.money, currency, amount, roundTrip)
			}
		})
	}
}
