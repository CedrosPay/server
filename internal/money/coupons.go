package money

import (
	"math"
	"math/big"
)

// ApplyPercentageDiscount applies a percentage discount to the Money amount.
// The discount is a value between 0-100 (e.g., 10 for 10% off).
// Uses half-up rounding (standard) for the discount calculation.
// Returns the discounted amount (not the discount itself).
func (m Money) ApplyPercentageDiscount(discountPercent float64) (Money, error) {
	return m.ApplyPercentageDiscountWithRounding(discountPercent, RoundingStandard)
}

// ApplyPercentageDiscountWithRounding applies a percentage discount with configurable rounding.
// The discount is a value between 0-100 (e.g., 10 for 10% off).
// Returns the discounted amount (not the discount itself).
func (m Money) ApplyPercentageDiscountWithRounding(discountPercent float64, mode RoundingMode) (Money, error) {
	if discountPercent < 0 || discountPercent > 100 {
		return m, nil // Invalid discount, return original
	}

	if discountPercent == 0 {
		return m, nil // No discount
	}

	if discountPercent == 100 {
		return Zero(m.Asset), nil // Free
	}

	// Calculate: price * (1 - discount/100)
	// Using basis points for precision: (100 - discount) * 100
	remainingPercent := 100 - discountPercent
	basisPoints := int64(remainingPercent * 100)

	return m.MulBasisPointsWithRounding(basisPoints, mode)
}

// ApplyFixedDiscount subtracts a fixed amount from the Money value.
// Returns zero if the discount exceeds the original amount.
func (m Money) ApplyFixedDiscount(discount Money) (Money, error) {
	if m.Asset.Code != discount.Asset.Code {
		return Money{}, ErrAssetMismatch
	}

	result := m.Atomic - discount.Atomic
	if result < 0 {
		return Zero(m.Asset), nil // Floor at zero
	}

	return Money{Asset: m.Asset, Atomic: result}, nil
}

// SumMoney adds multiple Money values together.
// All values must be the same asset.
// Returns error if assets don't match or overflow occurs.
func SumMoney(amounts ...Money) (Money, error) {
	if len(amounts) == 0 {
		return Money{}, nil
	}

	result := amounts[0]
	for i := 1; i < len(amounts); i++ {
		var err error
		result, err = result.Add(amounts[i])
		if err != nil {
			return Money{}, err
		}
	}

	return result, nil
}

// MultiplyByFloat multiplies Money by a float64 using precise decimal arithmetic.
// This is used for percentage-based calculations where the multiplier is not an integer.
// Uses big.Float for intermediate precision, then rounds half-up to nearest atomic unit.
// WARNING: Only use when necessary (e.g., calculating percentage discounts).
// Prefer integer operations when possible.
func (m Money) MultiplyByFloat(multiplier float64) (Money, error) {
	if multiplier == 0 {
		return Zero(m.Asset), nil
	}

	// Use big.Float for precision
	bigAtomic := new(big.Float).SetInt64(m.Atomic)
	bigMultiplier := new(big.Float).SetFloat64(multiplier)
	bigResult := new(big.Float).Mul(bigAtomic, bigMultiplier)

	// Round half-up
	bigResult.Add(bigResult, big.NewFloat(0.5))

	// Convert to int64
	resultInt64, accuracy := bigResult.Int64()
	if accuracy != big.Exact && accuracy != big.Below {
		// Check for overflow
		if bigResult.Cmp(new(big.Float).SetInt64(math.MaxInt64)) > 0 {
			return Money{}, ErrOverflow
		}
	}

	return Money{Asset: m.Asset, Atomic: resultInt64}, nil
}
