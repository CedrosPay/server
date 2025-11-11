package money

import (
	"fmt"
	"math"
)

// SPLAdapter converts Money to Solana SPL token format.
// SPL tokens use uint64 for amounts, while we use int64 internally.
type SPLAdapter struct{}

// NewSPLAdapter creates a new SPL token adapter.
func NewSPLAdapter() *SPLAdapter {
	return &SPLAdapter{}
}

// ToSPLAmount converts Money to SPL token format.
// Returns (mint address, amount) where:
//   - mint is the base58-encoded Solana token mint address
//   - amount is uint64 in token's atomic units (lamports, micro-USDC, etc.)
//
// Example:
//   - Money{USDC, 1500000} → ("EPjF...", 1500000)  // 1.5 USDC
//   - Money{SOL, 500000000} → ("So11...", 500000000)  // 0.5 SOL
//
// Returns error if:
//   - Asset is not an SPL token
//   - Amount is negative (SPL tokens use uint64, cannot represent negative)
//   - Amount exceeds uint64 max value
func (a *SPLAdapter) ToSPLAmount(m Money) (mint string, amount uint64, err error) {
	if !m.Asset.IsSPLToken() {
		return "", 0, fmt.Errorf("money: %s is not an SPL token", m.Asset.Code)
	}

	if m.Atomic < 0 {
		return "", 0, fmt.Errorf("money: SPL token amount cannot be negative: %d", m.Atomic)
	}

	// int64 max value (9,223,372,036,854,775,807) is less than uint64 max,
	// so positive int64 values always fit in uint64
	solanaMint, err := m.Asset.GetSolanaMint()
	if err != nil {
		return "", 0, err
	}

	return solanaMint, uint64(m.Atomic), nil
}

// FromSPLAmount converts SPL token format to Money.
// Takes mint address and uint64 amount.
//
// Example:
//   - ("EPjF...", 1500000) → Money{USDC, 1500000}  // 1.5 USDC
//   - ("So11...", 500000000) → Money{SOL, 500000000}  // 0.5 SOL
//
// Returns error if:
//   - Mint address is not recognized
//   - Amount exceeds int64 max value (overflow)
func (a *SPLAdapter) FromSPLAmount(mint string, amount uint64) (Money, error) {
	// Find asset by mint address
	var foundAsset *Asset
	for _, asset := range assetRegistry {
		if asset.IsSPLToken() {
			if assetMint, _ := asset.GetSolanaMint(); assetMint == mint {
				foundAsset = &asset
				break
			}
		}
	}

	if foundAsset == nil {
		return Money{}, fmt.Errorf("money: unknown SPL token mint: %s", mint)
	}

	// Check for overflow when converting uint64 to int64
	if amount > math.MaxInt64 {
		return Money{}, fmt.Errorf("money: SPL amount exceeds int64 max: %d", amount)
	}

	return Money{Asset: *foundAsset, Atomic: int64(amount)}, nil
}

// ValidateSPLAmount checks if a Money value is valid for SPL tokens.
// SPL token requirements:
//   - Must be an SPL token asset
//   - Amount must be non-negative (uint64 limitation)
//   - Amount must fit in uint64 (always true for non-negative int64)
func (a *SPLAdapter) ValidateSPLAmount(m Money) error {
	if !m.Asset.IsSPLToken() {
		return fmt.Errorf("money: %s is not an SPL token", m.Asset.Code)
	}

	if m.Atomic < 0 {
		return fmt.Errorf("money: SPL token amount cannot be negative: %d", m.Atomic)
	}

	return nil
}

// GetMintDecimals returns the number of decimals for an SPL token mint.
// This is useful for external callers who have a mint address and need
// to know the token's decimal places.
func (a *SPLAdapter) GetMintDecimals(mint string) (uint8, error) {
	for _, asset := range assetRegistry {
		if asset.IsSPLToken() {
			if assetMint, _ := asset.GetSolanaMint(); assetMint == mint {
				return asset.Decimals, nil
			}
		}
	}
	return 0, fmt.Errorf("money: unknown SPL token mint: %s", mint)
}
