package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// WellKnownPaymentOptions represents the /.well-known/payment-options response
// This follows the RFC 8615 well-known URI standard for service discovery
type WellKnownPaymentOptions struct {
	Version   string                   `json:"version"`   // x402 protocol version
	Server    string                   `json:"server"`    // Server identifier
	Resources []WellKnownResourceEntry `json:"resources"` // Available paid resources
	Payment   WellKnownPaymentInfo     `json:"payment"`   // Payment configuration
}

// WellKnownResourceEntry represents a single resource in the discovery response
type WellKnownResourceEntry struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Endpoint    string             `json:"endpoint"`
	Price       WellKnownPriceInfo `json:"price"`
	Metadata    map[string]string  `json:"metadata,omitempty"`
}

// WellKnownPriceInfo contains pricing information
type WellKnownPriceInfo struct {
	Fiat   *WellKnownFiatPrice   `json:"fiat,omitempty"`
	Crypto *WellKnownCryptoPrice `json:"crypto,omitempty"`
}

// WellKnownFiatPrice contains fiat pricing
type WellKnownFiatPrice struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// WellKnownCryptoPrice contains crypto pricing
type WellKnownCryptoPrice struct {
	Amount float64 `json:"amount"`
	Token  string  `json:"token"`
}

// WellKnownPaymentInfo describes supported payment methods
type WellKnownPaymentInfo struct {
	Methods []string    `json:"methods"` // e.g., ["stripe", "x402-solana-spl-transfer"]
	X402    *X402Config `json:"x402,omitempty"`
}

// X402Config describes x402 payment configuration
type X402Config struct {
	Network        string `json:"network"`        // e.g., "mainnet-beta"
	PaymentAddress string `json:"paymentAddress"` // Recipient address
	TokenMint      string `json:"tokenMint"`      // Token contract address
}

// wellKnownPaymentOptions handles GET /.well-known/payment-options
// This is a standard endpoint for AI agents to discover paid resources.
//
// Follows RFC 8615: https://tools.ietf.org/html/rfc8615
func (h *handlers) wellKnownPaymentOptions(w http.ResponseWriter, r *http.Request) {
	// Fetch all products
	products, err := h.paywall.ListProducts(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to fetch resources"}`, http.StatusInternalServerError)
		return
	}

	// Build resource entries
	resources := make([]WellKnownResourceEntry, 0, len(products))
	for _, p := range products {
		// Extract pricing from Money types
		var fiatAmount, cryptoAmount float64
		var fiatCurrency, cryptoToken string

		if p.FiatPrice != nil {
			fiatAmountStr := p.FiatPrice.ToMajor()
			fiatAmount, _ = strconv.ParseFloat(fiatAmountStr, 64)
			fiatCurrency = p.FiatPrice.Asset.Code
		}

		if p.CryptoPrice != nil {
			cryptoAmountStr := p.CryptoPrice.ToMajor()
			cryptoAmount, _ = strconv.ParseFloat(cryptoAmountStr, 64)
			cryptoToken = p.CryptoPrice.Asset.Code
		}

		entry := WellKnownResourceEntry{
			ID:          p.ID,
			Name:        p.Description,
			Description: p.Description,
			Endpoint:    "/paywall/" + p.ID,
			Price: WellKnownPriceInfo{
				Fiat: &WellKnownFiatPrice{
					Amount:   fiatAmount,
					Currency: fiatCurrency,
				},
				Crypto: &WellKnownCryptoPrice{
					Amount: cryptoAmount,
					Token:  cryptoToken,
				},
			},
			Metadata: p.Metadata,
		}
		resources = append(resources, entry)
	}

	response := WellKnownPaymentOptions{
		Version:   "1.0",
		Server:    "cedros-pay",
		Resources: resources,
		Payment: WellKnownPaymentInfo{
			Methods: []string{"stripe", "x402-solana-spl-transfer"},
			X402: &X402Config{
				Network:        h.cfg.X402.Network,
				PaymentAddress: h.cfg.X402.PaymentAddress,
				TokenMint:      h.cfg.X402.TokenMint,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300") // 5-minute cache
	w.Header().Set("Access-Control-Allow-Origin", "*")     // Allow cross-origin discovery

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, `{"error":"encoding failed"}`, http.StatusInternalServerError)
	}
}
