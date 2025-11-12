package money

import (
	"encoding/json"
	"fmt"
)

// MoneyJSON represents the JSON format for Money.
// Uses atomic units for precision:
//
//	{"asset":"USDC", "atomic":"1500000"}
type MoneyJSON struct {
	Asset  string `json:"asset"`  // Asset code (USD, USDC, SOL, etc.)
	Atomic string `json:"atomic"` // Atomic units as string
}

// MarshalJSON implements json.Marshaler for Money.
// Outputs atomic-only JSON:
//
//	{
//	  "asset": "USD",
//	  "atomic": "1050"
//	}
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(MoneyJSON{
		Asset:  m.Asset.Code,
		Atomic: m.ToAtomic(),
	})
}

// UnmarshalJSON implements json.Unmarshaler for Money.
// Accepts atomic format only:
//   - {"asset":"USDC", "atomic":"1500000"}  → Money{USDC, 1500000}
//
// Returns error if:
//   - Asset code is missing or unknown
//   - Atomic field is missing
//   - Parsing fails
func (m *Money) UnmarshalJSON(data []byte) error {
	var mj MoneyJSON
	if err := json.Unmarshal(data, &mj); err != nil {
		return fmt.Errorf("money: invalid JSON: %w", err)
	}

	// Asset is required
	if mj.Asset == "" {
		return fmt.Errorf("money: asset code required")
	}

	// Atomic is required
	if mj.Atomic == "" {
		return fmt.Errorf("money: 'atomic' field required")
	}

	asset, err := GetAsset(mj.Asset)
	if err != nil {
		return err
	}

	parsed, err := FromAtomic(asset, mj.Atomic)
	if err != nil {
		return err
	}

	*m = parsed
	return nil
}

// MoneyRequest is a helper type for API request parsing.
// Use this in request structs for clearer intent.
//
// Example:
//
//	type PaymentRequest struct {
//	    Amount MoneyRequest `json:"amount"`
//	}
type MoneyRequest Money

// MarshalJSON for MoneyRequest uses the same atomic-only format as Money.
func (mr MoneyRequest) MarshalJSON() ([]byte, error) {
	return Money(mr).MarshalJSON()
}

// UnmarshalJSON for MoneyRequest uses the same parsing as Money.
func (mr *MoneyRequest) UnmarshalJSON(data []byte) error {
	return (*Money)(mr).UnmarshalJSON(data)
}

// ToMoney converts MoneyRequest to Money.
func (mr MoneyRequest) ToMoney() Money {
	return Money(mr)
}

// MoneyResponse is a helper type for API response formatting.
// Use this in response structs for clearer intent.
//
// Example:
//
//	type QuoteResponse struct {
//	    Total MoneyResponse `json:"total"`
//	}
type MoneyResponse Money

// MarshalJSON for MoneyResponse uses the same atomic-only format as Money.
func (mr MoneyResponse) MarshalJSON() ([]byte, error) {
	return Money(mr).MarshalJSON()
}

// UnmarshalJSON for MoneyResponse uses the same parsing as Money.
func (mr *MoneyResponse) UnmarshalJSON(data []byte) error {
	return (*Money)(mr).UnmarshalJSON(data)
}

// ToMoney converts MoneyResponse to Money.
func (mr MoneyResponse) ToMoney() Money {
	return Money(mr)
}

// FromMoney creates a MoneyResponse from Money.
func FromMoney(m Money) MoneyResponse {
	return MoneyResponse(m)
}
