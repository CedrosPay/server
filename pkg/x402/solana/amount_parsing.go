package solana

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/gagliardetto/solana-go/rpc"
)

// extractInstructionInfo extracts the info and type from a parsed instruction.
func extractInstructionInfo(inst *rpc.ParsedInstruction) (map[string]interface{}, string, error) {
	payload, err := inst.Parsed.MarshalJSON()
	if err != nil {
		return nil, "", err
	}
	var decoded struct {
		Info map[string]interface{} `json:"info"`
		Type string                 `json:"type"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, "", err
	}
	if decoded.Info == nil {
		return nil, decoded.Type, errors.New("instruction info missing")
	}
	return decoded.Info, decoded.Type, nil
}

// parseAmount extracts and converts a token amount from instruction info.
func parseAmount(info map[string]interface{}, decimals uint8) (float64, error) {
	// Try tokenAmount structure first (parsed format)
	if tokenAmount, ok := mapValue(info["tokenAmount"]); ok {
		if ui, ok := floatValue(tokenAmount["uiAmount"]); ok && ui > 0 {
			return ui, nil
		}
		if str := stringValue(tokenAmount["uiAmountString"]); str != "" {
			if f, err := strconv.ParseFloat(str, 64); err == nil {
				return f, nil
			}
		}
		if raw := stringValue(tokenAmount["amount"]); raw != "" {
			return rawAmountToFloat(raw, decimals)
		}
	}

	// Try direct amount field
	if raw := info["amount"]; raw != nil {
		return numericToFloat(raw, decimals)
	}
	// Try uiAmountString field
	if raw := info["uiAmountString"]; raw != nil {
		return numericToFloat(raw, decimals)
	}
	return 0, errors.New("token amount missing")
}

// numericToFloat converts various numeric types to float64.
func numericToFloat(value interface{}, decimals uint8) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case string:
		if strings.Contains(v, ".") {
			return strconv.ParseFloat(v, 64)
		}
		return rawAmountToFloat(v, decimals)
	case json.Number:
		return v.Float64()
	default:
		return 0, fmt.Errorf("unsupported amount type %T", value)
	}
}

// rawAmountToFloat converts a raw token amount (as string) to float using decimals.
func rawAmountToFloat(amount string, decimals uint8) (float64, error) {
	if amount == "" {
		return 0, errors.New("empty amount")
	}
	val, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return 0, fmt.Errorf("invalid integer amount %q", amount)
	}
	denominator := math.Pow10(int(decimals))
	result, _ := new(big.Float).Quo(new(big.Float).SetInt(val), big.NewFloat(denominator)).Float64()
	return result, nil
}

// mapValue safely extracts a map from an interface{}.
func mapValue(value interface{}) (map[string]interface{}, bool) {
	m, ok := value.(map[string]interface{})
	return m, ok
}

// floatValue safely extracts a float64 from an interface{}.
func floatValue(value interface{}) (float64, bool) {
	if v, ok := value.(float64); ok {
		return v, true
	}
	return 0, false
}

// stringValue safely extracts a string from an interface{}.
func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}
