package money

import (
	"testing"
)

func TestValidateStablecoinMint(t *testing.T) {
	tests := []struct {
		name      string
		mint      string
		wantSymbol string
		wantErr   bool
	}{
		{
			name:      "USDC mainnet",
			mint:      "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
			wantSymbol: "USDC",
			wantErr:   false,
		},
		{
			name:      "USDT mainnet",
			mint:      "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
			wantSymbol: "USDT",
			wantErr:   false,
		},
		{
			name:      "PYUSD mainnet",
			mint:      "2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo",
			wantSymbol: "PYUSD",
			wantErr:   false,
		},
		{
			name:      "CASH mainnet",
			mint:      "CASHx9KJUStyftLFWGvEVf59SGeG9sh5FfcnZMVPCASH",
			wantSymbol: "CASH",
			wantErr:   false,
		},
		{
			name:      "SOL (non-stablecoin)",
			mint:      "So11111111111111111111111111111111111111112",
			wantSymbol: "",
			wantErr:   true,
		},
		{
			name:      "BONK (non-stablecoin)",
			mint:      "DezXAZ8z7PnrnRJjz3wXBoRgixCa6xjnB7YaB1pPB263",
			wantSymbol: "",
			wantErr:   true,
		},
		{
			name:      "invalid mint address",
			mint:      "invalid-mint-address",
			wantSymbol: "",
			wantErr:   true,
		},
		{
			name:      "typo in USDC mint",
			mint:      "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1X", // Changed last char
			wantSymbol: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			symbol, err := ValidateStablecoinMint(tt.mint)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStablecoinMint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if symbol != tt.wantSymbol {
				t.Errorf("ValidateStablecoinMint() symbol = %v, want %v", symbol, tt.wantSymbol)
			}
		})
	}
}

func TestIsStablecoin(t *testing.T) {
	tests := []struct {
		name string
		mint string
		want bool
	}{
		{"USDC", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", true},
		{"USDT", "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB", true},
		{"PYUSD", "2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo", true},
		{"CASH", "CASHx9KJUStyftLFWGvEVf59SGeG9sh5FfcnZMVPCASH", true},
		{"SOL", "So11111111111111111111111111111111111111112", false},
		{"BONK", "DezXAZ8z7PnrnRJjz3wXBoRgixCa6xjnB7YaB1pPB263", false},
		{"invalid", "invalid-mint", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsStablecoin(tt.mint); got != tt.want {
				t.Errorf("IsStablecoin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetStablecoinSymbol(t *testing.T) {
	tests := []struct {
		name string
		mint string
		want string
	}{
		{"USDC", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", "USDC"},
		{"USDT", "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB", "USDT"},
		{"PYUSD", "2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo", "PYUSD"},
		{"CASH", "CASHx9KJUStyftLFWGvEVf59SGeG9sh5FfcnZMVPCASH", "CASH"},
		{"SOL (not stablecoin)", "So11111111111111111111111111111111111111112", ""},
		{"unknown", "unknown-mint", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetStablecoinSymbol(tt.mint); got != tt.want {
				t.Errorf("GetStablecoinSymbol() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetMintAddressForSymbol(t *testing.T) {
	tests := []struct {
		name   string
		symbol string
		want   string
	}{
		{"USDC", "USDC", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"},
		{"USDT", "USDT", "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB"},
		{"PYUSD", "PYUSD", "2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo"},
		{"CASH", "CASH", "CASHx9KJUStyftLFWGvEVf59SGeG9sh5FfcnZMVPCASH"},
		{"SOL (not supported)", "SOL", ""},
		{"BONK (not supported)", "BONK", ""},
		{"unknown", "UNKNOWN", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetMintAddressForSymbol(tt.symbol); got != tt.want {
				t.Errorf("GetMintAddressForSymbol() = %v, want %v", got, tt.want)
			}
		})
	}
}
