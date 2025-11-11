package auth

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/gagliardetto/solana-go"
)

// SignatureVerifier handles Ed25519 signature verification for HTTP requests.
type SignatureVerifier struct{}

// NewSignatureVerifier creates a new signature verifier instance.
func NewSignatureVerifier() *SignatureVerifier {
	return &SignatureVerifier{}
}

// VerificationHeaders contains the signature verification headers from a request.
type VerificationHeaders struct {
	Signature string // X-Signature header (base64-encoded signature)
	Message   string // X-Message header (plain text message that was signed)
	Signer    string // X-Signer header (base58-encoded public key)
}

// ExtractHeaders extracts signature verification headers from an HTTP request.
func (sv *SignatureVerifier) ExtractHeaders(r *http.Request) (VerificationHeaders, error) {
	headers := VerificationHeaders{
		Signature: r.Header.Get("X-Signature"),
		Message:   r.Header.Get("X-Message"),
		Signer:    r.Header.Get("X-Signer"),
	}

	if headers.Signature == "" || headers.Message == "" || headers.Signer == "" {
		return headers, fmt.Errorf("signature required: include X-Signature, X-Message, and X-Signer headers")
	}

	return headers, nil
}

// VerifySignature verifies that the signature is valid for the given message and signer.
func (sv *SignatureVerifier) VerifySignature(headers VerificationHeaders) error {
	// Decode signature from base64
	signatureBytes, err := base64.StdEncoding.DecodeString(headers.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Parse signer public key from base58
	signerPubKey, err := solana.PublicKeyFromBase58(headers.Signer)
	if err != nil {
		return fmt.Errorf("invalid signer address: %w", err)
	}

	// Verify the signature
	signature := solana.SignatureFromBytes(signatureBytes)
	if !signature.Verify(signerPubKey, []byte(headers.Message)) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// VerifyAdminRequest verifies a request is signed by an authorized admin wallet.
// expectedSigner is the configured admin wallet address (e.g., payment address).
// expectedMessage is the message format the client should have signed.
func (sv *SignatureVerifier) VerifyAdminRequest(r *http.Request, expectedSigner string, expectedMessage string) error {
	headers, err := sv.ExtractHeaders(r)
	if err != nil {
		return err
	}

	// SECURITY: Verify cryptographic signature FIRST before checking identity
	// This prevents timing attacks that could leak the expected signer address
	if err := sv.VerifySignature(headers); err != nil {
		return err
	}

	// Now that signature is verified, check signer identity
	if headers.Signer != expectedSigner {
		return fmt.Errorf("unauthorized: only payment address can perform this action")
	}

	// Verify the message matches the expected format
	if headers.Message != expectedMessage {
		return fmt.Errorf("invalid message format (expected: %s)", expectedMessage)
	}

	return nil
}

// VerifyUserRequest verifies a request is signed by one of the allowed wallets.
// allowedSigners contains wallet addresses that are permitted to sign this request.
// expectedMessage is the message format the client should have signed.
func (sv *SignatureVerifier) VerifyUserRequest(r *http.Request, allowedSigners []string, expectedMessage string) error {
	headers, err := sv.ExtractHeaders(r)
	if err != nil {
		return err
	}

	// SECURITY: Verify cryptographic signature FIRST before checking identity
	// This prevents timing attacks that could leak the allowed signer addresses
	if err := sv.VerifySignature(headers); err != nil {
		return err
	}

	// Now that signature is verified, check if signer is in the allowed list
	signerAllowed := false
	for _, allowed := range allowedSigners {
		if headers.Signer == allowed {
			signerAllowed = true
			break
		}
	}
	if !signerAllowed {
		return fmt.Errorf("unauthorized: wallet %s is not authorized for this request", headers.Signer)
	}

	// Verify the message matches the expected format
	if headers.Message != expectedMessage {
		return fmt.Errorf("invalid message format (expected: %s)", expectedMessage)
	}

	return nil
}
