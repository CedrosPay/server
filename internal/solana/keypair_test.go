package solana

import (
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
)

func TestParsePrivateKey_Base58Format(t *testing.T) {
	// Generate a test keypair
	testKey, err := solana.NewRandomPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Convert to base58 string
	base58Key := testKey.String()

	// Parse it back
	parsed, err := ParsePrivateKey(base58Key)
	if err != nil {
		t.Fatalf("Failed to parse base58 private key: %v", err)
	}

	// Verify the parsed key matches the original
	if !parsed.PublicKey().Equals(testKey.PublicKey()) {
		t.Errorf("Parsed key public key mismatch:\nExpected: %s\nGot: %s",
			testKey.PublicKey().String(), parsed.PublicKey().String())
	}
}

func TestParsePrivateKey_JSONArrayFormat(t *testing.T) {
	// Generate a test keypair
	testKey, err := solana.NewRandomPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Convert to JSON array format: [1,2,3,...,64]
	jsonArray := "["
	for i, b := range testKey {
		if i > 0 {
			jsonArray += ","
		}
		jsonArray += fmt.Sprintf("%d", b)
	}
	jsonArray += "]"

	t.Logf("JSON array format: %s...", jsonArray[:50])

	// Parse it back
	parsed, err := ParsePrivateKey(jsonArray)
	if err != nil {
		t.Fatalf("Failed to parse JSON array private key: %v", err)
	}

	// Verify the parsed key matches the original
	if !parsed.PublicKey().Equals(testKey.PublicKey()) {
		t.Errorf("Parsed key public key mismatch:\nExpected: %s\nGot: %s",
			testKey.PublicKey().String(), parsed.PublicKey().String())
	}
}

func TestParsePrivateKey_EmptyString(t *testing.T) {
	_, err := ParsePrivateKey("")
	if err == nil {
		t.Error("Expected error for empty string, got nil")
	}
}

func TestParsePrivateKey_InvalidBase58(t *testing.T) {
	_, err := ParsePrivateKey("invalid_base58_string_with_invalid_chars!!!")
	if err == nil {
		t.Error("Expected error for invalid base58, got nil")
	}
}

func TestParsePrivateKey_InvalidJSONArray(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"wrong length", "[1,2,3,4,5]"},
		{"invalid byte value", "[1,2,3,abc,5]"},
		{"byte out of range", "[256,2,3,4,5]"},
		{"missing bracket", "1,2,3,4,5]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePrivateKey(tt.input)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestParsePrivateKey_BothFormatsProduceSameKey(t *testing.T) {
	// Generate a test keypair
	testKey, err := solana.NewRandomPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Convert to both formats
	base58Key := testKey.String()

	jsonArray := "["
	for i, b := range testKey {
		if i > 0 {
			jsonArray += ","
		}
		jsonArray += fmt.Sprintf("%d", b)
	}
	jsonArray += "]"

	// Parse both
	parsedBase58, err := ParsePrivateKey(base58Key)
	if err != nil {
		t.Fatalf("Failed to parse base58: %v", err)
	}

	parsedArray, err := ParsePrivateKey(jsonArray)
	if err != nil {
		t.Fatalf("Failed to parse JSON array: %v", err)
	}

	// Both should produce the same public key
	if !parsedBase58.PublicKey().Equals(parsedArray.PublicKey()) {
		t.Errorf("Public keys don't match:\nBase58: %s\nJSON Array: %s",
			parsedBase58.PublicKey().String(), parsedArray.PublicKey().String())
	}
}
