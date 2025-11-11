package money

import (
	"testing"
)

var (
	USD  = MustGetAsset("USD")
	EUR  = MustGetAsset("EUR")
	USDC = MustGetAsset("USDC")
	SOL  = MustGetAsset("SOL")
	USDT = MustGetAsset("USDT")
)

func TestFromMajor(t *testing.T) {
	tests := []struct {
		name       string
		asset      Asset
		major      string
		wantAtomic int64
		wantErr    bool
	}{
		// USD (2 decimals)
		{"USD 10.50", USD, "10.50", 1050, false},
		{"USD 0.01", USD, "0.01", 1, false},
		{"USD 100", USD, "100", 10000, false},
		{"USD -5.25", USD, "-5.25", -525, false},
		{"USD rounding up", USD, "10.555", 1056, false},
		{"USD rounding down", USD, "10.554", 1055, false},

		// USDC (6 decimals)
		{"USDC 1.5", USDC, "1.5", 1500000, false},
		{"USDC 10", USDC, "10", 10000000, false},
		{"USDC 0.000001", USDC, "0.000001", 1, false},

		// SOL (9 decimals)
		{"SOL 0.5", SOL, "0.5", 500000000, false},
		{"SOL 1", SOL, "1", 1000000000, false},

		// Errors
		{"invalid format", USD, "10.50.30", 0, true},
		{"invalid number", USD, "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromMajor(tt.asset, tt.major)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromMajor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.Atomic != tt.wantAtomic {
				t.Errorf("FromMajor() atomic = %v, want %v", got.Atomic, tt.wantAtomic)
			}
		})
	}
}

func TestToMajor(t *testing.T) {
	tests := []struct {
		name   string
		money  Money
		want   string
	}{
		{"USD 10.50", Money{USD, 1050}, "10.50"},
		{"USD 0.01", Money{USD, 1}, "0.01"},
		{"USD 100", Money{USD, 10000}, "100.00"},
		{"USD -5.25", Money{USD, -525}, "-5.25"},
		{"USD zero", Money{USD, 0}, "0.00"},

		{"USDC 1.5", Money{USDC, 1500000}, "1.500000"},
		{"USDC 10", Money{USDC, 10000000}, "10.000000"},

		{"SOL 0.5", Money{SOL, 500000000}, "0.500000000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.money.ToMajor()
			if got != tt.want {
				t.Errorf("ToMajor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromAtomic(t *testing.T) {
	tests := []struct {
		name       string
		asset      Asset
		atomic     string
		wantAtomic int64
		wantErr    bool
	}{
		{"USD 1050", USD, "1050", 1050, false},
		{"USDC 1500000", USDC, "1500000", 1500000, false},
		{"invalid", USD, "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromAtomic(tt.asset, tt.atomic)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromAtomic() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.Atomic != tt.wantAtomic {
				t.Errorf("FromAtomic() = %v, want %v", got.Atomic, tt.wantAtomic)
			}
		})
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name    string
		a       Money
		b       Money
		want    int64
		wantErr bool
	}{
		{"same asset", Money{USD, 1000}, Money{USD, 500}, 1500, false},
		{"negative", Money{USD, 1000}, Money{USD, -500}, 500, false},
		{"different assets", Money{USD, 1000}, Money{USDC, 500}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.a.Add(tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Add() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.Atomic != tt.want {
				t.Errorf("Add() = %v, want %v", got.Atomic, tt.want)
			}
		})
	}
}

func TestSub(t *testing.T) {
	tests := []struct {
		name    string
		a       Money
		b       Money
		want    int64
		wantErr bool
	}{
		{"positive result", Money{USD, 1000}, Money{USD, 500}, 500, false},
		{"negative result", Money{USD, 500}, Money{USD, 1000}, -500, false},
		{"different assets", Money{USD, 1000}, Money{USDC, 500}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.a.Sub(tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Sub() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.Atomic != tt.want {
				t.Errorf("Sub() = %v, want %v", got.Atomic, tt.want)
			}
		})
	}
}

func TestMul(t *testing.T) {
	tests := []struct {
		name       string
		money      Money
		multiplier int64
		want       int64
		wantErr    bool
	}{
		{"double", Money{USD, 1000}, 2, 2000, false},
		{"zero", Money{USD, 1000}, 0, 0, false},
		{"negative", Money{USD, 1000}, -2, -2000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.money.Mul(tt.multiplier)
			if (err != nil) != tt.wantErr {
				t.Errorf("Mul() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.Atomic != tt.want {
				t.Errorf("Mul() = %v, want %v", got.Atomic, tt.want)
			}
		})
	}
}

func TestMulBasisPoints(t *testing.T) {
	tests := []struct {
		name        string
		money       Money
		basisPoints int64
		want        int64
		wantErr     bool
	}{
		{"2.5% of $100", Money{USD, 10000}, 250, 250, false},   // $100 * 2.5% = $2.50
		{"10% of $50", Money{USD, 5000}, 1000, 500, false},     // $50 * 10% = $5.00
		{"100% of $10", Money{USD, 1000}, 10000, 1000, false},  // $10 * 100% = $10
		{"0%", Money{USD, 10000}, 0, 0, false},
		{"rounding half-up", Money{USD, 1005}, 1000, 101, false}, // $10.05 * 10% = $1.005 → $1.01
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.money.MulBasisPoints(tt.basisPoints)
			if (err != nil) != tt.wantErr {
				t.Errorf("MulBasisPoints() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.Atomic != tt.want {
				t.Errorf("MulBasisPoints() = %v, want %v", got.Atomic, tt.want)
			}
		})
	}
}

func TestMulPercent(t *testing.T) {
	tests := []struct {
		name    string
		money   Money
		percent int64
		want    int64
	}{
		{"10% of $100", Money{USD, 10000}, 10, 1000},  // $100 * 10% = $10
		{"50% of $20", Money{USD, 2000}, 50, 1000},    // $20 * 50% = $10
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := tt.money.MulPercent(tt.percent)
			if got.Atomic != tt.want {
				t.Errorf("MulPercent() = %v, want %v", got.Atomic, tt.want)
			}
		})
	}
}

func TestDiv(t *testing.T) {
	tests := []struct {
		name    string
		money   Money
		divisor int64
		want    int64
		wantErr bool
	}{
		{"divide by 2", Money{USD, 1000}, 2, 500, false},
		{"divide by 3 with rounding", Money{USD, 1000}, 3, 333, false}, // Half-up rounding
		{"divide by zero", Money{USD, 1000}, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.money.Div(tt.divisor)
			if (err != nil) != tt.wantErr {
				t.Errorf("Div() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.Atomic != tt.want {
				t.Errorf("Div() = %v, want %v", got.Atomic, tt.want)
			}
		})
	}
}

func TestComparisons(t *testing.T) {
	a := Money{USD, 1000}
	b := Money{USD, 500}
	c := Money{USD, 1000}
	d := Money{USDC, 1000}

	if !a.GreaterThan(b) {
		t.Error("Expected a > b")
	}
	if !b.LessThan(a) {
		t.Error("Expected b < a")
	}
	if !a.Equal(c) {
		t.Error("Expected a == c")
	}
	if a.Equal(d) {
		t.Error("Expected a != d (different assets)")
	}
}

func TestChecks(t *testing.T) {
	positive := Money{USD, 100}
	negative := Money{USD, -100}
	zero := Money{USD, 0}

	if !positive.IsPositive() || positive.IsNegative() || positive.IsZero() {
		t.Error("Positive check failed")
	}
	if !negative.IsNegative() || negative.IsPositive() || negative.IsZero() {
		t.Error("Negative check failed")
	}
	if !zero.IsZero() || zero.IsPositive() || zero.IsNegative() {
		t.Error("Zero check failed")
	}
}

func TestAbsNegate(t *testing.T) {
	positive := Money{USD, 100}
	negative := Money{USD, -100}

	if positive.Abs().Atomic != 100 {
		t.Error("Abs of positive failed")
	}
	if negative.Abs().Atomic != 100 {
		t.Error("Abs of negative failed")
	}
	if positive.Negate().Atomic != -100 {
		t.Error("Negate of positive failed")
	}
	if negative.Negate().Atomic != 100 {
		t.Error("Negate of negative failed")
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name  string
		money Money
		want  string
	}{
		{"USD positive", Money{USD, 1050}, "10.50 USD"},
		{"USDC", Money{USDC, 1500000}, "1.500000 USDC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.money.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoundTripMajor(t *testing.T) {
	tests := []struct {
		asset Asset
		major string
	}{
		{USD, "10.50"},
		{USDC, "1.5"},
		{SOL, "0.123456789"},
	}

	for _, tt := range tests {
		t.Run(tt.asset.Code+" "+tt.major, func(t *testing.T) {
			m, err := FromMajor(tt.asset, tt.major)
			if err != nil {
				t.Fatalf("FromMajor() error = %v", err)
			}

			roundTrip, err := FromMajor(tt.asset, m.ToMajor())
			if err != nil {
				t.Fatalf("Round trip FromMajor() error = %v", err)
			}

			if m.Atomic != roundTrip.Atomic {
				t.Errorf("Round trip failed: %v → %v → %v", tt.major, m.Atomic, roundTrip.Atomic)
			}
		})
	}
}

func TestRoundUpToCents(t *testing.T) {
	tests := []struct {
		name       string
		money      Money
		wantAtomic int64
	}{
		// USDC (6 decimals) - positive amounts
		{"USDC positive fractional small", Money{USDC, 1}, 10000},           // 0.000001 → 0.01
		{"USDC positive fractional large", Money{USDC, 9999}, 10000},        // 0.009999 → 0.01
		{"USDC positive at boundary", Money{USDC, 10000}, 10000},            // 0.01 → 0.01
		{"USDC positive above boundary", Money{USDC, 10001}, 20000},         // 0.010001 → 0.02
		{"USDC positive $1.50", Money{USDC, 1500000}, 1500000},              // 1.50 → 1.50
		{"USDC positive $1.501", Money{USDC, 1501000}, 1510000},             // 1.501 → 1.51

		// USDC (6 decimals) - negative amounts (refunds)
		{"USDC negative fractional small", Money{USDC, -1}, 0},              // -0.000001 → 0.00
		{"USDC negative fractional large", Money{USDC, -9999}, 0},           // -0.009999 → 0.00
		{"USDC negative at boundary", Money{USDC, -10000}, -10000},          // -0.01 → -0.01
		{"USDC negative above boundary", Money{USDC, -10001}, -10000},       // -0.010001 → -0.01
		{"USDC negative $1.50", Money{USDC, -1500000}, -1500000},            // -1.50 → -1.50
		{"USDC negative $1.501", Money{USDC, -1501000}, -1500000},           // -1.501 → -1.50

		// USD (2 decimals) - should return unchanged
		{"USD positive no rounding needed", Money{USD, 1050}, 1050},         // $10.50 → $10.50
		{"USD negative no rounding needed", Money{USD, -1050}, -1050},       // -$10.50 → -$10.50

		// SOL (9 decimals) - positive amounts
		{"SOL positive fractional", Money{SOL, 1000000}, 10000000},          // 0.001 → 0.01
		{"SOL positive at boundary", Money{SOL, 10000000}, 10000000},        // 0.01 → 0.01
		{"SOL positive above boundary", Money{SOL, 10000001}, 20000000},     // 0.010000001 → 0.02

		// SOL (9 decimals) - negative amounts
		{"SOL negative fractional", Money{SOL, -1000000}, 0},                // -0.001 → 0.00
		{"SOL negative at boundary", Money{SOL, -10000000}, -10000000},      // -0.01 → -0.01
		{"SOL negative above boundary", Money{SOL, -10000001}, -10000000},   // -0.010000001 → -0.01

		// Edge cases
		{"USDC zero", Money{USDC, 0}, 0},                                    // 0.00 → 0.00
		{"USDC large positive", Money{USDC, 100000000}, 100000000},          // $100 → $100
		{"USDC large negative", Money{USDC, -100000000}, -100000000},        // -$100 → -$100
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.money.RoundUpToCents()
			if got.Atomic != tt.wantAtomic {
				t.Errorf("RoundUpToCents() = %v, want %v (input: %v)", got.Atomic, tt.wantAtomic, tt.money.Atomic)
			}
			if got.Asset.Code != tt.money.Asset.Code {
				t.Errorf("RoundUpToCents() changed asset from %v to %v", tt.money.Asset.Code, got.Asset.Code)
			}
		})
	}
}
