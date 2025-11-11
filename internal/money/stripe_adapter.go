package money

import (
	"fmt"
)

// StripeAdapter converts Money to Stripe API format.
// Stripe expects amounts in the currency's smallest unit (cents for USD).
type StripeAdapter struct{}

// NewStripeAdapter creates a new Stripe adapter.
func NewStripeAdapter() *StripeAdapter {
	return &StripeAdapter{}
}

// ToStripeAmount converts Money to Stripe format.
// Returns (currency, amount) where:
//   - currency is lowercase Stripe currency code (e.g., "usd", "eur")
//   - amount is int64 in smallest unit (cents)
//
// Example:
//   - Money{USD, 1050} → ("usd", 1050)  // $10.50
//   - Money{EUR, 2500} → ("eur", 2500)  // €25.00
//
// Returns error if asset is not a Stripe-supported fiat currency.
func (a *StripeAdapter) ToStripeAmount(m Money) (currency string, amount int64, err error) {
	if !m.Asset.IsStripeCurrency() {
		return "", 0, fmt.Errorf("money: %s is not a Stripe-supported currency", m.Asset.Code)
	}

	stripeCurrency, err := m.Asset.GetStripeCurrency()
	if err != nil {
		return "", 0, err
	}

	return stripeCurrency, m.Atomic, nil
}

// FromStripeAmount converts Stripe format to Money.
// Takes Stripe currency code (lowercase) and amount in smallest unit.
//
// Example:
//   - ("usd", 1050) → Money{USD, 1050}  // $10.50
//   - ("eur", 2500) → Money{EUR, 2500}  // €25.00
//
// Returns error if currency code is not recognized.
func (a *StripeAdapter) FromStripeAmount(currency string, amount int64) (Money, error) {
	// Map Stripe currency codes to our asset codes
	var assetCode string
	switch currency {
	case "usd":
		assetCode = "USD"
	case "eur":
		assetCode = "EUR"
	default:
		return Money{}, fmt.Errorf("money: unsupported Stripe currency: %s", currency)
	}

	asset, err := GetAsset(assetCode)
	if err != nil {
		return Money{}, err
	}

	return Money{Asset: asset, Atomic: amount}, nil
}

// ValidateStripeAmount checks if a Money value is valid for Stripe.
// Stripe has specific requirements:
//   - Must be a fiat currency
//   - Amount must be non-negative
//   - Amount must be within Stripe's limits (0 to 99,999,999 for most currencies)
func (a *StripeAdapter) ValidateStripeAmount(m Money) error {
	if !m.Asset.IsStripeCurrency() {
		return fmt.Errorf("money: %s is not a Stripe-supported currency", m.Asset.Code)
	}

	if m.Atomic < 0 {
		return fmt.Errorf("money: Stripe amount cannot be negative: %d", m.Atomic)
	}

	// Stripe's maximum amount is 99,999,999 in smallest unit for most currencies
	// This is $999,999.99 for USD
	const maxStripeAmount = 99_999_999
	if m.Atomic > maxStripeAmount {
		return fmt.Errorf("money: Stripe amount exceeds maximum (%d > %d)", m.Atomic, maxStripeAmount)
	}

	return nil
}
