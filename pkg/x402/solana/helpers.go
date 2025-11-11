package solana

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// pubkeysEqual compares two base58-encoded public keys for equality.
func pubkeysEqual(expected string, actual string) bool {
	exp, err := solana.PublicKeyFromBase58(expected)
	if err != nil {
		return false
	}
	act, err := solana.PublicKeyFromBase58(actual)
	if err != nil {
		return false
	}
	return exp.Equals(act)
}

// maxDuration returns the larger of two durations.
func maxDuration(a, b time.Duration) time.Duration {
	if a >= b {
		return a
	}
	return b
}

// commitmentFromString converts a string to rpc.CommitmentType.
func commitmentFromString(value string) rpc.CommitmentType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "processed":
		return rpc.CommitmentProcessed
	case "confirmed":
		return rpc.CommitmentConfirmed
	case "finalized", "finalised", "":
		return rpc.CommitmentFinalized
	default:
		return rpc.CommitmentFinalized
	}
}

// deriveWebsocketURL converts an HTTP(S) RPC URL to WS(S) format.
func deriveWebsocketURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("rpc url empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "ws", "wss":
		return raw, nil
	case "":
		return "", errors.New("rpc url missing scheme")
	default:
		return "", fmt.Errorf("unsupported rpc url scheme %q", u.Scheme)
	}
	return u.String(), nil
}

// tokenAllowed checks if a token symbol is in the allowed list.
func tokenAllowed(symbol string, allowed []string) bool {
	for _, candidate := range allowed {
		if strings.EqualFold(candidate, symbol) {
			return true
		}
	}
	return false
}

// Error detection helpers

// isAlreadyProcessedError checks if the error indicates the transaction was already processed.
func isAlreadyProcessedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Transaction already processed") || strings.Contains(msg, "already been processed")
}

// isAccountNotFoundError checks if the error indicates an account was not found.
func isAccountNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "account not found") ||
		strings.Contains(msg, "could not find account") ||
		strings.Contains(msg, "invalid account owner") ||
		strings.Contains(msg, "invalidaccountdata") ||
		strings.Contains(msg, "invalid account data")
}

// isInsufficientFundsTokenError checks if the error is due to insufficient token balance.
// Solana returns "custom program error: 0x1" for SPL token insufficient funds.
func isInsufficientFundsTokenError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "custom program error: 0x1") ||
		(strings.Contains(msg, "insufficient funds") && !strings.Contains(msg, "insufficient lamports"))
}

// isInsufficientFundsSOLError checks if the error is due to insufficient SOL for fees.
func isInsufficientFundsSOLError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "insufficient lamports") ||
		(strings.Contains(msg, "insufficient funds") && strings.Contains(msg, "fee payer"))
}
