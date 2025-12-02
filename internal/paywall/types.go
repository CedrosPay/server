package paywall

import (
	"errors"
	"time"
)

// ErrResourceNotConfigured indicates the requested resource lacks pricing metadata.
var ErrResourceNotConfigured = errors.New("paywall: resource not configured")

// ErrStripeSessionPending indicates a Stripe session is still awaiting webhook confirmation.
var ErrStripeSessionPending = errors.New("paywall: stripe session pending")

// AuthorizationResult captures the outcome of an access attempt.
type AuthorizationResult struct {
	Granted      bool
	Method       string
	Wallet       string
	Quote        *Quote
	Settlement   *SettlementResponse
	Subscription *SubscriptionInfo // Present when access granted via subscription
}

// SubscriptionInfo contains subscription details when access is granted via subscription.
type SubscriptionInfo struct {
	ID               string    `json:"id"`
	Status           string    `json:"status"`
	CurrentPeriodEnd time.Time `json:"currentPeriodEnd"`
}

// Quote contains the pricing metadata shared with the caller.
type Quote struct {
	ResourceID string
	ExpiresAt  time.Time
	Stripe     *StripeOption
	Crypto     *CryptoQuote
}

// StripeOption exposes fiat checkout metadata.
type StripeOption struct {
	PriceID     string
	AmountCents int64
	Currency    string
	Description string
	Metadata    map[string]string
}

// CryptoQuote models the x402 paymentRequirements following the official spec.
// Reference: https://github.com/coinbase/x402
type CryptoQuote struct {
	// x402 standard fields
	Scheme            string `json:"scheme"`
	Network           string `json:"network"`
	MaxAmountRequired string `json:"maxAmountRequired"` // in atomic units
	Resource          string `json:"resource"`
	Description       string `json:"description"`
	MimeType          string `json:"mimeType"`
	OutputSchema      any    `json:"outputSchema,omitempty"`
	PayTo             string `json:"payTo"`
	MaxTimeoutSeconds int    `json:"maxTimeoutSeconds"`
	Asset             string `json:"asset"`
	Extra             any    `json:"extra,omitempty"`
}

// SettlementResponse communicates blockchain transaction details to the client.
// Sent via X-PAYMENT-RESPONSE header following x402 specification.
// Reference: https://github.com/coinbase/x402
type SettlementResponse struct {
	Success   bool    `json:"success"`
	Error     *string `json:"error"`
	TxHash    *string `json:"txHash"`
	NetworkID *string `json:"networkId"`
}
