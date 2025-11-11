package money

import (
	"fmt"
	"sync"
)

// Asset represents a currency or token with its properties.
type Asset struct {
	Code     string // Asset code (USD, USDC, SOL, etc.)
	Decimals uint8  // Number of decimal places (2 for USD, 6 for USDC, 9 for SOL)
	Type     AssetType
	Metadata AssetMetadata
}

// AssetType categorizes the asset for different backends.
type AssetType int

const (
	AssetTypeFiat AssetType = iota // Fiat currency (Stripe)
	AssetTypeSPL                    // Solana SPL token
)

// AssetMetadata contains backend-specific information.
type AssetMetadata struct {
	StripeCurrency string // Stripe currency code (lowercase: "usd", "eur")
	SolanaMint     string // Solana token mint address (base58)
}

// Global asset registry with concurrent access protection
var (
	assetRegistry   = map[string]Asset{
	// Fiat currencies (Stripe)
	"USD": {
		Code:     "USD",
		Decimals: 2, // cents
		Type:     AssetTypeFiat,
		Metadata: AssetMetadata{
			StripeCurrency: "usd",
		},
	},
	"EUR": {
		Code:     "EUR",
		Decimals: 2, // cents
		Type:     AssetTypeFiat,
		Metadata: AssetMetadata{
			StripeCurrency: "eur",
		},
	},

	// Solana SPL Tokens
	"USDC": {
		Code:     "USDC",
		Decimals: 6, // micro-USDC
		Type:     AssetTypeSPL,
		Metadata: AssetMetadata{
			SolanaMint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", // USDC mainnet
		},
	},
	"SOL": {
		Code:     "SOL",
		Decimals: 9, // lamports
		Type:     AssetTypeSPL,
		Metadata: AssetMetadata{
			SolanaMint: "So11111111111111111111111111111111111111112", // Wrapped SOL
		},
	},
	"USDT": {
		Code:     "USDT",
		Decimals: 6, // micro-USDT
		Type:     AssetTypeSPL,
		Metadata: AssetMetadata{
			SolanaMint: "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB", // USDT mainnet
		},
	},
	"PYUSD": {
		Code:     "PYUSD",
		Decimals: 6, // micro-PYUSD (PayPal USD)
		Type:     AssetTypeSPL,
		Metadata: AssetMetadata{
			SolanaMint: "2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo", // PYUSD mainnet
		},
	},
	"CASH": {
		Code:     "CASH",
		Decimals: 6, // micro-CASH
		Type:     AssetTypeSPL,
		Metadata: AssetMetadata{
			SolanaMint: "CASHx9KJUStyftLFWGvEVf59SGeG9sh5FfcnZMVPCASH", // CASH mainnet
		},
	},
	}
	assetRegistryMu sync.RWMutex
)

// GetAsset retrieves an asset from the registry.
func GetAsset(code string) (Asset, error) {
	assetRegistryMu.RLock()
	asset, ok := assetRegistry[code]
	assetRegistryMu.RUnlock()

	if !ok {
		return Asset{}, fmt.Errorf("money: unknown asset: %s", code)
	}
	return asset, nil
}

// MustGetAsset retrieves an asset and panics if not found (for tests/constants).
func MustGetAsset(code string) Asset {
	asset, err := GetAsset(code)
	if err != nil {
		panic(err)
	}
	return asset
}

// RegisterAsset adds a new asset to the registry (for testing or dynamic tokens).
func RegisterAsset(asset Asset) error {
	if asset.Code == "" {
		return fmt.Errorf("money: asset code required")
	}
	if asset.Decimals > 18 {
		return fmt.Errorf("money: decimals must be ≤ 18")
	}

	assetRegistryMu.Lock()
	assetRegistry[asset.Code] = asset
	assetRegistryMu.Unlock()

	return nil
}

// ListAssets returns all registered assets.
func ListAssets() []Asset {
	assetRegistryMu.RLock()
	assets := make([]Asset, 0, len(assetRegistry))
	for _, asset := range assetRegistry {
		assets = append(assets, asset)
	}
	assetRegistryMu.RUnlock()

	return assets
}

// IsStripeCurrency returns true if the asset is a Stripe fiat currency.
func (a Asset) IsStripeCurrency() bool {
	return a.Type == AssetTypeFiat
}

// IsSPLToken returns true if the asset is a Solana SPL token.
func (a Asset) IsSPLToken() bool {
	return a.Type == AssetTypeSPL
}

// GetStripeCurrency returns the Stripe currency code or error.
func (a Asset) GetStripeCurrency() (string, error) {
	if !a.IsStripeCurrency() {
		return "", fmt.Errorf("money: %s is not a Stripe currency", a.Code)
	}
	return a.Metadata.StripeCurrency, nil
}

// GetSolanaMint returns the Solana mint address or error.
func (a Asset) GetSolanaMint() (string, error) {
	if !a.IsSPLToken() {
		return "", fmt.Errorf("money: %s is not an SPL token", a.Code)
	}
	return a.Metadata.SolanaMint, nil
}
