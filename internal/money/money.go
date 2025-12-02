package money

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Money represents a monetary amount in atomic units for a specific asset.
// All arithmetic is performed on int64 to avoid floating-point precision issues.
//
// Examples:
//   - $10.50 USD  = Money{Asset: USD, Atomic: 1050}       // 1050 cents
//   - 1.5 USDC    = Money{Asset: USDC, Atomic: 1500000}   // 1.5 × 10^6
//   - 0.5 SOL     = Money{Asset: SOL, Atomic: 500000000}  // 0.5 × 10^9
type Money struct {
	Asset  Asset // The currency/token
	Atomic int64 // Amount in smallest unit (cents, lamports, etc.)
}

var (
	// ErrOverflow occurs when an operation would exceed int64 capacity.
	ErrOverflow = errors.New("money: arithmetic overflow")

	// ErrAssetMismatch occurs when operating on different assets.
	ErrAssetMismatch = errors.New("money: asset mismatch")

	// ErrNegativeAmount occurs when negative amount is invalid for operation.
	ErrNegativeAmount = errors.New("money: negative amount not allowed")

	// ErrInvalidFormat occurs when parsing fails.
	ErrInvalidFormat = errors.New("money: invalid format")

	// ErrDivisionByZero occurs when dividing by zero.
	ErrDivisionByZero = errors.New("money: division by zero")
)

// Zero returns a zero amount for the given asset.
func Zero(asset Asset) Money {
	return Money{Asset: asset, Atomic: 0}
}

// New creates a Money from atomic units.
func New(asset Asset, atomic int64) Money {
	return Money{Asset: asset, Atomic: atomic}
}

// FromMajor creates Money from a major unit string (e.g., "10.50").
// Uses half-up rounding for fractional atomic units.
//
// Examples:
//   - FromMajor(USD, "10.50")  → 1050 cents
//   - FromMajor(USDC, "1.5")   → 1500000 micro-USDC
func FromMajor(asset Asset, major string) (Money, error) {
	// Parse the decimal string
	parts := strings.Split(major, ".")
	if len(parts) > 2 {
		return Money{}, fmt.Errorf("%w: too many decimal points", ErrInvalidFormat)
	}

	integerPart := parts[0]
	fractionalPart := ""
	if len(parts) == 2 {
		fractionalPart = parts[1]
	}

	// Parse integer part
	integerVal, err := strconv.ParseInt(integerPart, 10, 64)
	if err != nil {
		return Money{}, fmt.Errorf("%w: %v", ErrInvalidFormat, err)
	}

	// Handle fractional part with proper rounding
	var atomicFromFraction int64
	if fractionalPart != "" {
		// Pad or truncate to match asset decimals
		if len(fractionalPart) > int(asset.Decimals) {
			// Truncate and round (half-up)
			roundDigit := fractionalPart[asset.Decimals] - '0'
			fractionalPart = fractionalPart[:asset.Decimals]

			parsed, _ := strconv.ParseInt(fractionalPart, 10, 64)
			atomicFromFraction = parsed

			// Half-up rounding
			if roundDigit >= 5 {
				atomicFromFraction++
			}
		} else {
			// Pad with zeros
			for len(fractionalPart) < int(asset.Decimals) {
				fractionalPart += "0"
			}
			atomicFromFraction, _ = strconv.ParseInt(fractionalPart, 10, 64)
		}
	}

	// Calculate total atomic units
	multiplier := int64(math.Pow10(int(asset.Decimals)))

	// Check for overflow
	if integerVal > 0 && multiplier > math.MaxInt64/integerVal {
		return Money{}, ErrOverflow
	}
	if integerVal < 0 && multiplier > math.MaxInt64/(-integerVal) {
		return Money{}, ErrOverflow
	}

	atomicFromInteger := integerVal * multiplier

	// Handle sign for fractional part
	if integerVal < 0 {
		atomicFromFraction = -atomicFromFraction
	}

	total := atomicFromInteger + atomicFromFraction

	return Money{Asset: asset, Atomic: total}, nil
}

// FromAtomic creates Money from an atomic units string.
//
// Example:
//   - FromAtomic(USD, "1050")      → $10.50
//   - FromAtomic(USDC, "1500000")  → 1.5 USDC
func FromAtomic(asset Asset, atomic string) (Money, error) {
	value, err := strconv.ParseInt(atomic, 10, 64)
	if err != nil {
		return Money{}, fmt.Errorf("%w: %v", ErrInvalidFormat, err)
	}
	return Money{Asset: asset, Atomic: value}, nil
}

// ToMajor converts Money to major unit string with proper decimal places.
//
// Examples:
//   - Money{USD, 1050}.ToMajor()      → "10.50"
//   - Money{USDC, 1500000}.ToMajor()  → "1.500000"
//   - Money{USDC, 1500000}.ToMajor()  → "1.500000"
func (m Money) ToMajor() string {
	if m.Atomic == 0 {
		if m.Asset.Decimals == 0 {
			return "0"
		}
		return "0." + strings.Repeat("0", int(m.Asset.Decimals))
	}

	divisor := int64(math.Pow10(int(m.Asset.Decimals)))
	integerPart := m.Atomic / divisor
	fractionalPart := m.Atomic % divisor

	// Handle negative numbers
	if fractionalPart < 0 {
		fractionalPart = -fractionalPart
	}

	if m.Asset.Decimals == 0 {
		return strconv.FormatInt(integerPart, 10)
	}

	// Format fractional part with leading zeros using efficient string building
	// Pre-allocate buffer based on actual value size to minimize waste
	// Calculate digits needed for integer part
	digits := 1
	absInt := integerPart
	if absInt < 0 {
		absInt = -absInt
		digits++ // Account for negative sign
	}
	if absInt >= 10 {
		// Fast path for common amounts ($0.01 - $99.99): most are 1-2 digits
		if absInt < 100 {
			digits++
		} else if absInt < 1000 {
			digits += 2
		} else {
			// For larger amounts, use logarithm
			digits += int(math.Log10(float64(absInt)))
		}
	}

	// Total allocation: integer digits + '.' + decimal digits
	var buf strings.Builder
	buf.Grow(digits + 1 + int(m.Asset.Decimals))

	// Write integer part
	buf.WriteString(strconv.FormatInt(integerPart, 10))
	buf.WriteByte('.')

	// Write fractional part with leading zeros
	fractionalStr := strconv.FormatInt(fractionalPart, 10)
	leadingZeros := int(m.Asset.Decimals) - len(fractionalStr)
	for i := 0; i < leadingZeros; i++ {
		buf.WriteByte('0')
	}
	buf.WriteString(fractionalStr)

	return buf.String()
}

// ToAtomic returns the atomic units as a string.
func (m Money) ToAtomic() string {
	return strconv.FormatInt(m.Atomic, 10)
}

// Add returns the sum of two Money values.
// Returns error if assets don't match or overflow occurs.
func (m Money) Add(other Money) (Money, error) {
	if m.Asset.Code != other.Asset.Code {
		return Money{}, fmt.Errorf("%w: cannot add %s and %s", ErrAssetMismatch, m.Asset.Code, other.Asset.Code)
	}

	// Check for overflow
	result := m.Atomic + other.Atomic
	if (result > m.Atomic) != (other.Atomic > 0) {
		return Money{}, ErrOverflow
	}

	return Money{Asset: m.Asset, Atomic: result}, nil
}

// Sub returns the difference of two Money values.
func (m Money) Sub(other Money) (Money, error) {
	if m.Asset.Code != other.Asset.Code {
		return Money{}, fmt.Errorf("%w: cannot subtract %s and %s", ErrAssetMismatch, m.Asset.Code, other.Asset.Code)
	}

	// Check for underflow
	result := m.Atomic - other.Atomic
	if (result < m.Atomic) != (other.Atomic > 0) {
		return Money{}, ErrOverflow
	}

	return Money{Asset: m.Asset, Atomic: result}, nil
}

// Mul multiplies Money by an integer scalar.
func (m Money) Mul(multiplier int64) (Money, error) {
	if multiplier == 0 {
		return Zero(m.Asset), nil
	}

	// Check for overflow using big.Int
	bigResult := new(big.Int).Mul(big.NewInt(m.Atomic), big.NewInt(multiplier))
	if !bigResult.IsInt64() {
		return Money{}, ErrOverflow
	}

	return Money{Asset: m.Asset, Atomic: bigResult.Int64()}, nil
}

// RoundingMode determines how fractional cents are rounded.
type RoundingMode int

const (
	// RoundingStandard uses half-up rounding (0.5 rounds up).
	// This matches Stripe's default behavior: $0.025 → $0.03, $0.024 → $0.02
	RoundingStandard RoundingMode = iota

	// RoundingCeiling always rounds up to the next cent.
	// Example: $0.024 → $0.03, $0.001 → $0.01
	RoundingCeiling
)

// ParseRoundingMode converts a string to RoundingMode.
// Accepts "standard", "ceiling", or empty string (defaults to standard).
func ParseRoundingMode(mode string) RoundingMode {
	switch mode {
	case "ceiling":
		return RoundingCeiling
	case "standard", "":
		return RoundingStandard
	default:
		return RoundingStandard // Default to standard for invalid values
	}
}

// MulBasisPoints multiplies Money by basis points (1/100th of a percent).
// Example: amount.MulBasisPoints(250) applies a 2.5% rate.
// Uses half-up rounding (standard).
func (m Money) MulBasisPoints(basisPoints int64) (Money, error) {
	return m.MulBasisPointsWithRounding(basisPoints, RoundingStandard)
}

// MulBasisPointsWithRounding multiplies Money by basis points with configurable rounding.
// Example: amount.MulBasisPointsWithRounding(250, RoundingCeiling) applies a 2.5% rate with ceiling rounding.
func (m Money) MulBasisPointsWithRounding(basisPoints int64, mode RoundingMode) (Money, error) {
	if basisPoints == 0 {
		return Zero(m.Asset), nil
	}

	// Multiply by basis points, then divide by 10000
	// Using big.Int for intermediate calculation to avoid overflow
	bigAtomic := big.NewInt(m.Atomic)
	bigBP := big.NewInt(basisPoints)
	bigDivisor := big.NewInt(10000)

	// result = (atomic * basisPoints) / 10000
	bigResult := new(big.Int).Mul(bigAtomic, bigBP)

	// Apply rounding based on mode
	switch mode {
	case RoundingStandard:
		// Half-up rounding: add 5000 before dividing by 10000
		if bigResult.Sign() >= 0 {
			bigResult.Add(bigResult, big.NewInt(5000))
		} else {
			bigResult.Sub(bigResult, big.NewInt(5000))
		}
	case RoundingCeiling:
		// Ceiling rounding: add 9999 before dividing by 10000
		// This ensures any fractional amount rounds up
		if bigResult.Sign() >= 0 {
			bigResult.Add(bigResult, big.NewInt(9999))
		} else {
			bigResult.Sub(bigResult, big.NewInt(9999))
		}
	}

	bigResult.Div(bigResult, bigDivisor)

	if !bigResult.IsInt64() {
		return Money{}, ErrOverflow
	}

	return Money{Asset: m.Asset, Atomic: bigResult.Int64()}, nil
}

// MulPercent multiplies Money by a percentage (0-100).
// Example: amount.MulPercent(10) applies a 10% rate.
func (m Money) MulPercent(percent int64) (Money, error) {
	return m.MulBasisPoints(percent * 100)
}

// Div divides Money by an integer divisor.
// Uses half-up rounding for remainders.
func (m Money) Div(divisor int64) (Money, error) {
	if divisor == 0 {
		return Money{}, ErrDivisionByZero
	}

	quotient := m.Atomic / divisor
	remainder := m.Atomic % divisor

	// Half-up rounding
	if remainder*2 >= divisor {
		quotient++
	} else if remainder*2 <= -divisor {
		quotient--
	}

	return Money{Asset: m.Asset, Atomic: quotient}, nil
}

// IsPositive returns true if amount is greater than zero.
func (m Money) IsPositive() bool {
	return m.Atomic > 0
}

// IsNegative returns true if amount is less than zero.
func (m Money) IsNegative() bool {
	return m.Atomic < 0
}

// IsZero returns true if amount is exactly zero.
func (m Money) IsZero() bool {
	return m.Atomic == 0
}

// LessThan returns true if m < other (same asset required).
func (m Money) LessThan(other Money) bool {
	if m.Asset.Code != other.Asset.Code {
		return false // Cannot compare different assets
	}
	return m.Atomic < other.Atomic
}

// GreaterThan returns true if m > other (same asset required).
func (m Money) GreaterThan(other Money) bool {
	if m.Asset.Code != other.Asset.Code {
		return false
	}
	return m.Atomic > other.Atomic
}

// Equal returns true if m == other (same asset and amount).
func (m Money) Equal(other Money) bool {
	return m.Asset.Code == other.Asset.Code && m.Atomic == other.Atomic
}

// Abs returns the absolute value.
func (m Money) Abs() Money {
	if m.Atomic < 0 {
		return Money{Asset: m.Asset, Atomic: -m.Atomic}
	}
	return m
}

// Negate returns the negated amount.
func (m Money) Negate() Money {
	return Money{Asset: m.Asset, Atomic: -m.Atomic}
}

// String returns a human-readable representation.
// Example: Money{USD, 1050} → "$10.50 USD"
func (m Money) String() string {
	return fmt.Sprintf("%s %s", m.ToMajor(), m.Asset.Code)
}

// RoundUpToCents rounds the amount up to the nearest cent (2 decimal places).
// This is used for pricing to ensure we never undercharge.
// Only affects assets with more than 2 decimals (like USDC with 6 decimals).
//
// For positive amounts: rounds up (ceiling) - $0.184 → $0.19
// For negative amounts: rounds towards zero (floor) - -$0.184 → -$0.18
// This ensures consistent behavior for both payments and refunds.
func (m Money) RoundUpToCents() Money {
	if m.Asset.Decimals <= 2 {
		return m // Already at cent precision or less
	}

	// Calculate the divisor to get to cents (e.g., for USDC: 10^6 / 10^2 = 10^4)
	centDivisor := int64(math.Pow10(int(m.Asset.Decimals - 2)))

	var rounded int64
	if m.Atomic < 0 {
		// For negative amounts (refunds), round towards zero (less negative)
		// Formula: floor(x/d) for negative x
		// Example: -10001 / 10000 = -1, -1 * 10000 = -10000
		rounded = (m.Atomic / centDivisor) * centDivisor
	} else {
		// For positive amounts, round up (ceiling)
		// Formula: ceil(x/d) = (x + d - 1) / d
		// Example: 10001 / 10000 = 2 (after adding d-1), 2 * 10000 = 20000
		rounded = ((m.Atomic + centDivisor - 1) / centDivisor) * centDivisor
	}

	return Money{Asset: m.Asset, Atomic: rounded}
}
