package solana

import (
	"encoding/json"
	"math"
	"testing"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		name     string
		info     map[string]interface{}
		decimals uint8
		want     float64
		wantErr  bool
	}{
		{
			name: "tokenAmount with uiAmount",
			info: map[string]interface{}{
				"tokenAmount": map[string]interface{}{
					"uiAmount": 10.5,
				},
			},
			decimals: 6,
			want:     10.5,
			wantErr:  false,
		},
		{
			name: "tokenAmount with uiAmountString",
			info: map[string]interface{}{
				"tokenAmount": map[string]interface{}{
					"uiAmountString": "25.75",
				},
			},
			decimals: 6,
			want:     25.75,
			wantErr:  false,
		},
		{
			name: "tokenAmount with raw amount (6 decimals)",
			info: map[string]interface{}{
				"tokenAmount": map[string]interface{}{
					"amount": "1000000",
				},
			},
			decimals: 6,
			want:     1.0,
			wantErr:  false,
		},
		{
			name: "direct amount field as string",
			info: map[string]interface{}{
				"amount": "5000000",
			},
			decimals: 6,
			want:     5.0,
			wantErr:  false,
		},
		{
			name: "direct amount field as float",
			info: map[string]interface{}{
				"amount": 15.5,
			},
			decimals: 6,
			want:     15.5,
			wantErr:  false,
		},
		{
			name: "uiAmountString field",
			info: map[string]interface{}{
				"uiAmountString": "7.25",
			},
			decimals: 6,
			want:     7.25,
			wantErr:  false,
		},
		{
			name:     "missing amount",
			info:     map[string]interface{}{},
			decimals: 6,
			want:     0,
			wantErr:  true,
		},
		{
			name: "9 decimals (SOL)",
			info: map[string]interface{}{
				"amount": "1000000000",
			},
			decimals: 9,
			want:     1.0,
			wantErr:  false,
		},
		{
			name: "2 decimals (cents)",
			info: map[string]interface{}{
				"amount": "100",
			},
			decimals: 2,
			want:     1.0,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAmount(tt.info, tt.decimals)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAmount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("parseAmount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNumericToFloat(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		decimals uint8
		want     float64
		wantErr  bool
	}{
		{
			name:     "float64 value",
			value:    12.34,
			decimals: 6,
			want:     12.34,
			wantErr:  false,
		},
		{
			name:     "string with decimal",
			value:    "15.75",
			decimals: 6,
			want:     15.75,
			wantErr:  false,
		},
		{
			name:     "raw amount string (no decimal)",
			value:    "1000000",
			decimals: 6,
			want:     1.0,
			wantErr:  false,
		},
		{
			name:     "json.Number",
			value:    json.Number("25.5"),
			decimals: 6,
			want:     25.5,
			wantErr:  false,
		},
		{
			name:     "invalid string",
			value:    "invalid",
			decimals: 6,
			want:     0,
			wantErr:  true,
		},
		{
			name:     "unsupported type (int)",
			value:    int(42),
			decimals: 6,
			want:     0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := numericToFloat(tt.value, tt.decimals)
			if (err != nil) != tt.wantErr {
				t.Errorf("numericToFloat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("numericToFloat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRawAmountToFloat(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		decimals uint8
		want     float64
		wantErr  bool
	}{
		{
			name:     "1 USDC (6 decimals)",
			amount:   "1000000",
			decimals: 6,
			want:     1.0,
			wantErr:  false,
		},
		{
			name:     "1 SOL (9 decimals)",
			amount:   "1000000000",
			decimals: 9,
			want:     1.0,
			wantErr:  false,
		},
		{
			name:     "0.5 USDC",
			amount:   "500000",
			decimals: 6,
			want:     0.5,
			wantErr:  false,
		},
		{
			name:     "10.123456 USDC",
			amount:   "10123456",
			decimals: 6,
			want:     10.123456,
			wantErr:  false,
		},
		{
			name:     "zero amount",
			amount:   "0",
			decimals: 6,
			want:     0.0,
			wantErr:  false,
		},
		{
			name:     "empty string",
			amount:   "",
			decimals: 6,
			want:     0,
			wantErr:  true,
		},
		{
			name:     "invalid integer",
			amount:   "abc",
			decimals: 6,
			want:     0,
			wantErr:  true,
		},
		{
			name:     "negative amount",
			amount:   "-1000000",
			decimals: 6,
			want:     -1.0,
			wantErr:  false,
		},
		{
			name:     "very large amount",
			amount:   "9999999999999999",
			decimals: 6,
			want:     9999999999.999999,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rawAmountToFloat(tt.amount, tt.decimals)
			if (err != nil) != tt.wantErr {
				t.Errorf("rawAmountToFloat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && math.Abs(got-tt.want) > 0.0000001 {
				t.Errorf("rawAmountToFloat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapValue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  map[string]interface{}
		ok    bool
	}{
		{
			name:  "valid map",
			value: map[string]interface{}{"key": "value"},
			want:  map[string]interface{}{"key": "value"},
			ok:    true,
		},
		{
			name:  "not a map (string)",
			value: "string",
			want:  nil,
			ok:    false,
		},
		{
			name:  "not a map (number)",
			value: 123,
			want:  nil,
			ok:    false,
		},
		{
			name:  "nil value",
			value: nil,
			want:  nil,
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := mapValue(tt.value)
			if ok != tt.ok {
				t.Errorf("mapValue() ok = %v, want %v", ok, tt.ok)
			}
			if ok && len(got) != len(tt.want) {
				t.Errorf("mapValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFloatValue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  float64
		ok    bool
	}{
		{
			name:  "valid float64",
			value: 12.34,
			want:  12.34,
			ok:    true,
		},
		{
			name:  "not a float64 (string)",
			value: "12.34",
			want:  0,
			ok:    false,
		},
		{
			name:  "not a float64 (int)",
			value: 42,
			want:  0,
			ok:    false,
		},
		{
			name:  "nil value",
			value: nil,
			want:  0,
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := floatValue(tt.value)
			if ok != tt.ok {
				t.Errorf("floatValue() ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("floatValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringValue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{
			name:  "string value",
			value: "hello",
			want:  "hello",
		},
		{
			name:  "integer value",
			value: 123,
			want:  "123",
		},
		{
			name:  "float value",
			value: 12.34,
			want:  "12.34",
		},
		{
			name:  "nil value",
			value: nil,
			want:  "",
		},
		{
			name:  "boolean value",
			value: true,
			want:  "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringValue(tt.value)
			if got != tt.want {
				t.Errorf("stringValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractInstructionInfo(t *testing.T) {
	// Note: This test requires constructing rpc.ParsedInstruction which is complex.
	// We'll test the basic structure parsing.
	t.Run("valid instruction with info and type", func(t *testing.T) {
		// This is a mock test showing the expected behavior
		// In real usage, this would come from Solana RPC responses
		jsonData := `{"info":{"amount":"1000000","authority":"...","destination":"..."},"type":"transfer"}`

		var parsedData struct {
			Info map[string]interface{} `json:"info"`
			Type string                 `json:"type"`
		}
		if err := json.Unmarshal([]byte(jsonData), &parsedData); err != nil {
			t.Fatalf("failed to parse test data: %v", err)
		}

		if parsedData.Info == nil {
			t.Error("expected info to be parsed")
		}
		if parsedData.Type != "transfer" {
			t.Errorf("expected type 'transfer', got %s", parsedData.Type)
		}
		if amount, ok := parsedData.Info["amount"].(string); !ok || amount != "1000000" {
			t.Errorf("expected amount '1000000', got %v", parsedData.Info["amount"])
		}
	})
}
