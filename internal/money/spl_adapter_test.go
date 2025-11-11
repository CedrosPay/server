package money

import (
	"math"
	"testing"
)

func TestSPLAdapter_ToSPLAmount(t *testing.T) {
	adapter := NewSPLAdapter()

	tests := []struct {
		name       string
		money      Money
		wantMint   string
		wantAmount uint64
		wantErr    bool
	}{
		{
			name:       "USDC 1.5",
			money:      Money{USDC, 1500000},
			wantMint:   "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
			wantAmount: 1500000,
			wantErr:    false,
		},
		{
			name:       "SOL 0.5",
			money:      Money{SOL, 500000000},
			wantMint:   "So11111111111111111111111111111111111111112",
			wantAmount: 500000000,
			wantErr:    false,
		},
		{
			name:       "USDT 10.0",
			money:      Money{USDT, 10000000},
			wantMint:   "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
			wantAmount: 10000000,
			wantErr:    false,
		},
		{
			name:    "USD not SPL",
			money:   Money{USD, 1050},
			wantErr: true,
		},
		{
			name:    "negative amount",
			money:   Money{USDC, -1000},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mint, amount, err := adapter.ToSPLAmount(tt.money)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToSPLAmount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if mint != tt.wantMint {
					t.Errorf("ToSPLAmount() mint = %v, want %v", mint, tt.wantMint)
				}
				if amount != tt.wantAmount {
					t.Errorf("ToSPLAmount() amount = %v, want %v", amount, tt.wantAmount)
				}
			}
		})
	}
}

func TestSPLAdapter_FromSPLAmount(t *testing.T) {
	adapter := NewSPLAdapter()

	tests := []struct {
		name       string
		mint       string
		amount     uint64
		wantAtomic int64
		wantAsset  string
		wantErr    bool
	}{
		{
			name:       "USDC 1.5",
			mint:       "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
			amount:     1500000,
			wantAtomic: 1500000,
			wantAsset:  "USDC",
			wantErr:    false,
		},
		{
			name:       "SOL 0.5",
			mint:       "So11111111111111111111111111111111111111112",
			amount:     500000000,
			wantAtomic: 500000000,
			wantAsset:  "SOL",
			wantErr:    false,
		},
		{
			name:       "USDT 10.0",
			mint:       "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
			amount:     10000000,
			wantAtomic: 10000000,
			wantAsset:  "USDT",
			wantErr:    false,
		},
		{
			name:    "unknown mint",
			mint:    "UnknownMint1111111111111111111111111111",
			amount:  1000,
			wantErr: true,
		},
		{
			name:    "amount exceeds int64 max",
			mint:    "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
			amount:  math.MaxUint64, // Will exceed int64 max
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adapter.FromSPLAmount(tt.mint, tt.amount)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromSPLAmount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Atomic != tt.wantAtomic {
					t.Errorf("FromSPLAmount() atomic = %v, want %v", got.Atomic, tt.wantAtomic)
				}
				if got.Asset.Code != tt.wantAsset {
					t.Errorf("FromSPLAmount() asset = %v, want %v", got.Asset.Code, tt.wantAsset)
				}
			}
		})
	}
}

func TestSPLAdapter_ValidateSPLAmount(t *testing.T) {
	adapter := NewSPLAdapter()

	tests := []struct {
		name    string
		money   Money
		wantErr bool
	}{
		{
			name:    "valid USDC",
			money:   Money{USDC, 1500000},
			wantErr: false,
		},
		{
			name:    "valid SOL",
			money:   Money{SOL, 500000000},
			wantErr: false,
		},
		{
			name:    "negative amount",
			money:   Money{USDC, -1000},
			wantErr: true,
		},
		{
			name:    "USD not SPL",
			money:   Money{USD, 1050},
			wantErr: true,
		},
		{
			name:    "zero amount",
			money:   Money{USDC, 0},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ValidateSPLAmount(tt.money)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSPLAmount() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSPLAdapter_GetMintDecimals(t *testing.T) {
	adapter := NewSPLAdapter()

	tests := []struct {
		name         string
		mint         string
		wantDecimals uint8
		wantErr      bool
	}{
		{
			name:         "USDC",
			mint:         "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
			wantDecimals: 6,
			wantErr:      false,
		},
		{
			name:         "SOL",
			mint:         "So11111111111111111111111111111111111111112",
			wantDecimals: 9,
			wantErr:      false,
		},
		{
			name:         "USDT",
			mint:         "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
			wantDecimals: 6,
			wantErr:      false,
		},
		{
			name:    "unknown mint",
			mint:    "UnknownMint1111111111111111111111111111",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decimals, err := adapter.GetMintDecimals(tt.mint)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMintDecimals() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if decimals != tt.wantDecimals {
					t.Errorf("GetMintDecimals() = %v, want %v", decimals, tt.wantDecimals)
				}
			}
		})
	}
}

func TestSPLAdapter_RoundTrip(t *testing.T) {
	adapter := NewSPLAdapter()

	tests := []struct {
		name  string
		money Money
	}{
		{"USDC 1.5", Money{USDC, 1500000}},
		{"SOL 0.5", Money{SOL, 500000000}},
		{"USDT 10.0", Money{USDT, 10000000}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to SPL format
			mint, amount, err := adapter.ToSPLAmount(tt.money)
			if err != nil {
				t.Fatalf("ToSPLAmount() error = %v", err)
			}

			// Convert back to Money
			roundTrip, err := adapter.FromSPLAmount(mint, amount)
			if err != nil {
				t.Fatalf("FromSPLAmount() error = %v", err)
			}

			// Verify we got the same value back
			if !tt.money.Equal(roundTrip) {
				t.Errorf("Round trip failed: %v → (%s, %d) → %v", tt.money, mint, amount, roundTrip)
			}
		})
	}
}
