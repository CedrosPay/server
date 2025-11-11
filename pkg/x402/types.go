package x402

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PaymentPayload follows the x402 specification for the X-PAYMENT header.
// Reference: https://github.com/coinbase/x402
type PaymentPayload struct {
	X402Version int    `json:"x402Version"`
	Scheme      string `json:"scheme"`
	Network     string `json:"network"`
	Payload     any    `json:"payload"` // scheme-dependent
}

// SolanaPayload is the scheme-specific payload for Solana SPL transfers.
// Extends the x402 standard with Solana-specific fields.
type SolanaPayload struct {
	// Required fields
	Signature   string `json:"signature"`
	Transaction string `json:"transaction"`

	// Resource identification (prevents resource ID leakage in URL paths)
	Resource     string `json:"resource,omitempty"`     // Resource ID (product, cart, refund)
	ResourceType string `json:"resourceType,omitempty"` // "regular" | "cart" | "refund"

	// Optional Solana-specific extensions
	FeePayer              string            `json:"feePayer,omitempty"` // Server wallet (pays transaction fees in gasless mode)
	Memo                  string            `json:"memo,omitempty"`
	RecipientTokenAccount string            `json:"recipientTokenAccount,omitempty"` // SPL token account
	Metadata              map[string]string `json:"metadata,omitempty"`
}

// PaymentProof is the internal representation after parsing and validation.
type PaymentProof struct {
	X402Version int
	Scheme      string
	Network     string
	Signature   string
	Payer       string
	Transaction string
	Memo        string
	Metadata    map[string]string

	// Resource identification (prevents resource ID leakage in URL paths)
	Resource     string // Resource ID from payment payload
	ResourceType string // "regular" | "cart" | "refund"

	// Solana-specific
	RecipientTokenAccount string
	FeePayer              string // Server wallet that pays fees (gasless mode)
}

// Requirement describes the verification constraints for a resource.
type Requirement struct {
	ResourceID            string
	RecipientOwner        string
	RecipientTokenAccount string
	TokenMint             string
	Amount                float64
	Network               string
	TokenDecimals         uint8
	AllowedTokens         []string
	QuoteTTL              time.Duration
	SkipPreflight         bool
	Commitment            string
}

// VerificationResult captures the verifier outcome.
type VerificationResult struct {
	Wallet    string
	Amount    float64
	Signature string
	ExpiresAt time.Time
}

// Verifier validates incoming payments before the protected handler executes.
type Verifier interface {
	Verify(ctx context.Context, proof PaymentProof, requirement Requirement) (VerificationResult, error)
}

// ParsePaymentProof decodes the X-PAYMENT header into a PaymentProof.
// Follows the x402 specification: https://github.com/coinbase/x402
func ParsePaymentProof(header string) (PaymentProof, error) {
	raw := strings.TrimSpace(header)
	if raw == "" {
		return PaymentProof{}, errors.New("x402: empty payment header")
	}

	// Decode base64 (or accept raw JSON for testing)
	var data []byte
	if strings.HasPrefix(raw, "{") {
		data = []byte(raw)
	} else {
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(raw)
			if err != nil {
				return PaymentProof{}, fmt.Errorf("x402: decode base64: %w", err)
			}
		}
		data = decoded
	}

	// Parse x402 Payment Payload
	var payload PaymentPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return PaymentProof{}, fmt.Errorf("x402: parse payment payload: %w", err)
	}

	proof := PaymentProof{
		X402Version: payload.X402Version,
		Scheme:      payload.Scheme,
		Network:     payload.Network,
	}

	// Extract scheme-specific payload
	payloadJSON, err := json.Marshal(payload.Payload)
	if err != nil {
		return proof, fmt.Errorf("x402: marshal payload: %w", err)
	}

	switch payload.Scheme {
	case "solana-spl-transfer", "solana":
		var solPayload SolanaPayload
		if err := json.Unmarshal(payloadJSON, &solPayload); err != nil {
			return proof, fmt.Errorf("x402: parse solana payload: %w", err)
		}
		proof.Signature = solPayload.Signature
		proof.Transaction = solPayload.Transaction
		proof.FeePayer = solPayload.FeePayer
		// Note: proof.Payer (user wallet) is extracted from the transaction by the verifier
		proof.Memo = solPayload.Memo
		proof.RecipientTokenAccount = solPayload.RecipientTokenAccount
		proof.Metadata = solPayload.Metadata
		proof.Resource = solPayload.Resource
		proof.ResourceType = solPayload.ResourceType

	default:
		return proof, fmt.Errorf("x402: unsupported scheme %q (supported: solana-spl-transfer, solana)", payload.Scheme)
	}

	// Validation
	if proof.Transaction == "" {
		return proof, errors.New("x402: payment payload missing transaction")
	}
	// Note: signature is optional - verifier will extract it from transaction if not provided

	return proof, nil
}
