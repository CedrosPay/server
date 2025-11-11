package httpserver

import (
	"encoding/json"
	"net/http"
)

// A2AAgentCard represents the Google Agent2Agent protocol agent card
// Published at /.well-known/agent.json for agent discovery
//
// Specification: https://a2a-protocol.org/
type A2AAgentCard struct {
	Name            string             `json:"name"`
	Version         string             `json:"version"`
	Description     string             `json:"description"`
	ServiceEndpoint string             `json:"service_endpoint"`
	Capabilities    []string           `json:"capabilities"`
	Authentication  A2AAuthentication  `json:"authentication"`
	PaymentMethods  []A2APaymentMethod `json:"payment_methods"`
	Metadata        map[string]string  `json:"metadata,omitempty"`
}

// A2AAuthentication describes supported authentication methods
type A2AAuthentication struct {
	Type    string   `json:"type"`
	Schemes []string `json:"schemes"`
}

// A2APaymentMethod describes supported payment options
type A2APaymentMethod struct {
	Type        string `json:"type"`
	Protocol    string `json:"protocol"`
	Network     string `json:"network,omitempty"`
	Description string `json:"description"`
}

// agentCard handles GET /.well-known/agent.json
// This endpoint implements Google's Agent2Agent (A2A) protocol for agent discovery.
// It allows AI agents to discover Cedros Pay's capabilities, supported payment methods,
// and integration details.
//
// A2A Spec: https://a2a-protocol.org/
func (h *handlers) agentCard(w http.ResponseWriter, r *http.Request) {
	// Build agent card from configuration
	card := A2AAgentCard{
		Name:            "Cedros Pay",
		Version:         "1.0",
		Description:     "Unified payment gateway supporting both fiat (Stripe) and cryptocurrency (x402/Solana) payments for AI agents and applications",
		ServiceEndpoint: h.getServiceEndpoint(r),
		Capabilities: []string{
			"payment-processing",
			"x402-payment",
			"stripe-checkout",
			"product-catalog",
			"webhook-notifications",
			"coupon-support",
		},
		Authentication: A2AAuthentication{
			Type: "hybrid",
			Schemes: []string{
				"x402",           // Crypto payment verification
				"stripe-session", // Stripe session-based auth
				"none",           // Public endpoints (discovery)
			},
		},
		PaymentMethods: []A2APaymentMethod{
			{
				Type:        "cryptocurrency",
				Protocol:    "x402-solana-spl-transfer",
				Network:     h.cfg.X402.Network,
				Description: "Instant USDC payments on Solana via x402 protocol",
			},
			{
				Type:        "fiat",
				Protocol:    "stripe",
				Description: "Credit/debit card payments via Stripe Checkout",
			},
		},
		Metadata: map[string]string{
			"project_url":       "https://github.com/CedrosPay/server",
			"documentation":     "https://github.com/CedrosPay/server#readme",
			"discovery_rfc8615": "/.well-known/payment-options",
			"discovery_mcp":     "POST /resources/list",
			"openapi_spec":      "/openapi.json",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600") // 1-hour cache
	w.Header().Set("Access-Control-Allow-Origin", "*")      // Allow cross-origin discovery

	if err := json.NewEncoder(w).Encode(card); err != nil {
		http.Error(w, `{"error":"encoding failed"}`, http.StatusInternalServerError)
	}
}

// getServiceEndpoint constructs the full service endpoint URL from the request
func (h *handlers) getServiceEndpoint(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}

	host := r.Host
	if host == "" {
		host = "localhost:8080" // Fallback for local development
	}

	return scheme + "://" + host
}
