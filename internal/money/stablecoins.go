package money

import "fmt"

// KnownStablecoins maps Solana token mint addresses to their stablecoin symbols.
// These are the ONLY tokens that should be used for payments to ensure proper
// decimal handling (all stablecoins use 2-6 decimals and are pegged to $1).
//
// WARNING: Using non-stablecoin tokens (SOL, BONK, etc.) will cause precision
// issues because the system rounds to 2 decimal places (cents).
var KnownStablecoins = map[string]string{
	"CASHx9KJUStyftLFWGvEVf59SGeG9sh5FfcnZMVPCASH": "CASH",  // CASH stablecoin
	"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v": "USDC",  // USDC mainnet
	"Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB": "USDT",  // USDT mainnet
	"2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo": "PYUSD", // PayPal USD
}

// ValidateStablecoinMint checks if a token mint address is a known stablecoin.
// Returns the stablecoin symbol if valid, or an error if not.
//
// Why this matters:
//   - Typo in token mint = payments go to wrong token = permanent loss
//   - Non-stablecoins have unpredictable values (1 SOL ≠ $1, 1 BONK ≠ $1)
//   - System rounds to 2 decimal places assuming $1 peg
func ValidateStablecoinMint(mintAddress string) (string, error) {
	symbol, ok := KnownStablecoins[mintAddress]
	if !ok {
		return "", fmt.Errorf(
			"token mint %s is not a recognized stablecoin - only stablecoins are supported (USDC, USDT, PYUSD, CASH)",
			mintAddress,
		)
	}
	return symbol, nil
}

// IsStablecoin returns true if the mint address is a known stablecoin.
func IsStablecoin(mintAddress string) bool {
	_, ok := KnownStablecoins[mintAddress]
	return ok
}

// GetStablecoinSymbol returns the symbol for a stablecoin mint address.
// Returns empty string if not a known stablecoin.
func GetStablecoinSymbol(mintAddress string) string {
	return KnownStablecoins[mintAddress]
}

// GetMintAddressForSymbol returns the mint address for a stablecoin symbol.
// Returns empty string if symbol not found.
func GetMintAddressForSymbol(symbol string) string {
	for mint, sym := range KnownStablecoins {
		if sym == symbol {
			return mint
		}
	}
	return ""
}
