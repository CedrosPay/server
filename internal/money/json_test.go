package money

import (
	"encoding/json"
	"testing"
)

func TestMoney_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		money    Money
		wantJSON string
	}{
		{
			name:     "USD 10.50",
			money:    Money{USD, 1050},
			wantJSON: `{"asset":"USD","atomic":"1050"}`,
		},
		{
			name:     "USDC 1.5",
			money:    Money{USDC, 1500000},
			wantJSON: `{"asset":"USDC","atomic":"1500000"}`,
		},
		{
			name:     "SOL 0.5",
			money:    Money{SOL, 500000000},
			wantJSON: `{"asset":"SOL","atomic":"500000000"}`,
		},
		{
			name:     "zero amount",
			money:    Money{USD, 0},
			wantJSON: `{"asset":"USD","atomic":"0"}`,
		},
		{
			name:     "negative amount",
			money:    Money{USD, -525},
			wantJSON: `{"asset":"USD","atomic":"-525"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.money)
			if err != nil {
				t.Fatalf("MarshalJSON() error = %v", err)
			}
			if string(got) != tt.wantJSON {
				t.Errorf("MarshalJSON() = %s, want %s", string(got), tt.wantJSON)
			}
		})
	}
}

func TestMoney_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		jsonInput  string
		wantAtomic int64
		wantAsset  string
		wantErr    bool
	}{
		{
			name:       "atomic form - USDC",
			jsonInput:  `{"asset":"USDC","atomic":"1500000"}`,
			wantAtomic: 1500000,
			wantAsset:  "USDC",
			wantErr:    false,
		},
		{
			name:       "atomic form - USD",
			jsonInput:  `{"asset":"USD","atomic":"1050"}`,
			wantAtomic: 1050,
			wantAsset:  "USD",
			wantErr:    false,
		},
		{
			name:       "SOL atomic form",
			jsonInput:  `{"asset":"SOL","atomic":"500000000"}`,
			wantAtomic: 500000000,
			wantAsset:  "SOL",
			wantErr:    false,
		},
		{
			name:       "zero amount",
			jsonInput:  `{"asset":"USD","atomic":"0"}`,
			wantAtomic: 0,
			wantAsset:  "USD",
			wantErr:    false,
		},
		{
			name:       "negative amount",
			jsonInput:  `{"asset":"USD","atomic":"-525"}`,
			wantAtomic: -525,
			wantAsset:  "USD",
			wantErr:    false,
		},
		{
			name:      "missing asset",
			jsonInput: `{"atomic":"1050"}`,
			wantErr:   true,
		},
		{
			name:      "unknown asset",
			jsonInput: `{"asset":"BTC","atomic":"1000"}`,
			wantErr:   true,
		},
		{
			name:      "missing atomic",
			jsonInput: `{"asset":"USD"}`,
			wantErr:   true,
		},
		{
			name:      "invalid atomic",
			jsonInput: `{"asset":"USD","atomic":"invalid"}`,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Money
			err := json.Unmarshal([]byte(tt.jsonInput), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Atomic != tt.wantAtomic {
					t.Errorf("UnmarshalJSON() atomic = %v, want %v", got.Atomic, tt.wantAtomic)
				}
				if got.Asset.Code != tt.wantAsset {
					t.Errorf("UnmarshalJSON() asset = %v, want %v", got.Asset.Code, tt.wantAsset)
				}
			}
		})
	}
}

func TestMoney_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		money Money
	}{
		{"USD 10.50", Money{USD, 1050}},
		{"USDC 1.5", Money{USDC, 1500000}},
		{"SOL 0.5", Money{SOL, 500000000}},
		{"EUR 25.00", Money{EUR, 2500}},
		{"zero", Money{USD, 0}},
		{"negative", Money{USD, -525}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.money)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// Unmarshal back
			var roundTrip Money
			if err := json.Unmarshal(data, &roundTrip); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			// Verify we got the same value
			if !tt.money.Equal(roundTrip) {
				t.Errorf("Round trip failed: %v → %s → %v", tt.money, string(data), roundTrip)
			}
		})
	}
}

func TestMoneyRequest_JSON(t *testing.T) {
	// Test MoneyRequest helper type
	req := struct {
		Amount MoneyRequest `json:"amount"`
	}{
		Amount: MoneyRequest(Money{USD, 1050}),
	}

	// Marshal
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Should contain atomic-only form
	expected := `{"amount":{"asset":"USD","atomic":"1050"}}`
	if string(data) != expected {
		t.Errorf("Marshal() = %s, want %s", string(data), expected)
	}

	// Unmarshal back
	var parsed struct {
		Amount MoneyRequest `json:"amount"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !Money(parsed.Amount).Equal(Money{USD, 1050}) {
		t.Errorf("Unmarshal() = %v, want %v", parsed.Amount, Money{USD, 1050})
	}
}

func TestMoneyResponse_JSON(t *testing.T) {
	// Test MoneyResponse helper type
	resp := struct {
		Total MoneyResponse `json:"total"`
	}{
		Total: FromMoney(Money{USDC, 1500000}),
	}

	// Marshal
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Should contain atomic-only form
	expected := `{"total":{"asset":"USDC","atomic":"1500000"}}`
	if string(data) != expected {
		t.Errorf("Marshal() = %s, want %s", string(data), expected)
	}

	// Unmarshal back
	var parsed struct {
		Total MoneyResponse `json:"total"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !parsed.Total.ToMoney().Equal(Money{USDC, 1500000}) {
		t.Errorf("Unmarshal() = %v, want %v", parsed.Total, Money{USDC, 1500000})
	}
}

