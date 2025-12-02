package paywall

import (
	"strings"

	"github.com/CedrosPay/server/internal/coupons"
	"github.com/CedrosPay/server/internal/money"
)

// USD-pegged asset codes (1:1 value equivalence)
// All these assets are treated as equivalent for coupon discount purposes.
var usdPeggedAssets = map[string]bool{
	"USD":   true, // Fiat USD (Stripe)
	"USDC":  true, // USD Coin (Circle)
	"USDT":  true, // Tether USD
	"PYUSD": true, // PayPal USD
	"CASH":  true, // CASH USD stablecoin
}

// isUSDPegged checks if an asset code is USD or a USD-pegged stablecoin.
// Returns true for USD, USDC, USDT, PYUSD, CASH (case-insensitive).
// This allows fixed-amount discounts to work across all USD-equivalent assets.
func isUSDPegged(assetCode string) bool {
	return usdPeggedAssets[strings.ToUpper(assetCode)]
}

// StackCouponsOnMoney applies multiple coupons to a Money amount using proper integer arithmetic.
// Coupons are applied in optimal order:
// 1. All percentage discounts are applied first (multiplicatively stacked)
// 2. All fixed-amount discounts are summed and applied at the end
// This ensures maximum discount for the customer and matches existing float64 behavior.
//
// Example:
//
//	Price: $10.00, Coupons: [10% off, 20% off, $1 off, $0.50 off]
//	Step 1: Apply 10%: $10.00 * 0.9 = $9.00
//	Step 2: Apply 20%: $9.00 * 0.8 = $7.20
//	Step 3: Apply $1 + $0.50 = $1.50 off: $7.20 - $1.50 = $5.70
//
// All arithmetic is done using int64 atomic units to avoid floating-point errors.
// The roundingMode parameter controls how fractional cents are rounded.
func StackCouponsOnMoney(originalPrice money.Money, applicableCoupons []coupons.Coupon, roundingMode money.RoundingMode) (money.Money, error) {
	if len(applicableCoupons) == 0 {
		return originalPrice, nil
	}

	price := originalPrice
	var totalFixedDiscount money.Money

	// Single pass: apply percentage coupons and accumulate fixed discounts
	for _, coupon := range applicableCoupons {
		if coupon.DiscountType == coupons.DiscountTypePercentage {
			// Apply percentage discount using precise integer arithmetic with configured rounding
			discounted, err := price.ApplyPercentageDiscountWithRounding(coupon.DiscountValue, roundingMode)
			if err != nil {
				return money.Money{}, err
			}
			price = discounted
		} else if coupon.DiscountType == coupons.DiscountTypeFixed {
			// Apply fixed discounts to USD-pegged assets only (USD, USDC, USDT, PYUSD, CASH)
			// This allows coupons to work across Stripe (USD) and x402 (USDC/USDT/etc)
			// The currency field is now optional and ignored - payment_method controls applicability
			if isUSDPegged(originalPrice.Asset.Code) {
				// Convert fixed discount amount to Money
				fixedDiscount, err := money.FromMajor(originalPrice.Asset, formatFloat(coupon.DiscountValue))
				if err != nil {
					continue // Skip invalid discount
				}

				if totalFixedDiscount.IsZero() {
					totalFixedDiscount = fixedDiscount
				} else {
					var err error
					totalFixedDiscount, err = totalFixedDiscount.Add(fixedDiscount)
					if err != nil {
						continue // Skip on overflow
					}
				}
			}
		}
	}

	// Apply accumulated fixed discounts
	if !totalFixedDiscount.IsZero() {
		discounted, err := price.ApplyFixedDiscount(totalFixedDiscount)
		if err != nil {
			return money.Money{}, err
		}
		price = discounted
	}

	return price, nil
}
