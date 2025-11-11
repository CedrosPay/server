package httpserver

import (
	"encoding/json"
	"net/http"
)

// openAPISpec handles GET /openapi.json
// Returns OpenAPI 3.0 specification for Cedros Pay API
func (h *handlers) openAPISpec(w http.ResponseWriter, r *http.Request) {
	spec := h.buildOpenAPISpec(r)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600") // 1-hour cache
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(spec); err != nil {
		http.Error(w, `{"error":"encoding failed"}`, http.StatusInternalServerError)
	}
}

// buildOpenAPISpec constructs the OpenAPI 3.0 specification
func (h *handlers) buildOpenAPISpec(r *http.Request) map[string]interface{} {
	baseURL := h.getServiceEndpoint(r)
	prefix := h.cfg.Server.RoutePrefix

	return map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":   "Cedros Pay API",
			"version": "1.0.0",
			"description": "Unified payment gateway supporting both fiat (Stripe) and cryptocurrency (x402/Solana) payments for AI agents and applications.\n\n" +
				"## API Versioning\n\n" +
				"This API uses **content negotiation** for versioning. URLs remain constant, but you can request specific API versions via headers:\n\n" +
				"**Method 1: X-API-Version Header (Recommended)**\n" +
				"```\nX-API-Version: v2\n```\n\n" +
				"**Method 2: Vendor-Specific Media Type**\n" +
				"```\nAccept: application/vnd.cedros.v2+json\n```\n\n" +
				"**Method 3: Version Parameter**\n" +
				"```\nAccept: application/json; version=2\n```\n\n" +
				"If no version is specified, the server defaults to **v1** (current stable).\n\n" +
				"All responses include `X-API-Version` header indicating the version used. " +
				"Deprecated versions will receive `Deprecation`, `Sunset`, and `Warning` headers.",
			"contact": map[string]string{
				"url": "https://github.com/CedrosPay/server",
			},
			"license": map[string]string{
				"name": "MIT",
				"url":  "https://github.com/CedrosPay/server/blob/main/LICENSE",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         baseURL,
				"description": "Cedros Pay Server",
			},
		},
		"paths": map[string]interface{}{
			"/cedros-health": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Health Check",
					"description": "Check server health and get route prefix configuration",
					"operationId": "healthCheck",
					"tags":        []string{"System"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Server is healthy",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"status":      map[string]string{"type": "string", "example": "ok"},
											"routePrefix": map[string]string{"type": "string", "example": "/api"},
										},
									},
								},
							},
						},
					},
				},
			},
			"/.well-known/payment-options": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Payment Options Discovery (RFC 8615)",
					"description": "Web-discoverable endpoint for AI agents to find available paid resources and payment methods",
					"operationId": "getPaymentOptions",
					"tags":        []string{"Discovery"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Payment configuration and resources",
						},
					},
				},
			},
			"/.well-known/agent.json": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Agent Card (A2A Protocol)",
					"description": "Google Agent2Agent protocol agent card for agent discovery",
					"operationId": "getAgentCard",
					"tags":        []string{"Discovery"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Agent capabilities and configuration",
						},
					},
				},
			},
			"/resources/list": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "List Resources (MCP)",
					"description": "Model Context Protocol (MCP) JSON-RPC 2.0 endpoint for resource discovery",
					"operationId": "listResources",
					"tags":        []string{"Discovery"},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"jsonrpc": map[string]string{"type": "string", "example": "2.0"},
										"id":      map[string]interface{}{"oneOf": []map[string]string{{"type": "string"}, {"type": "number"}}},
										"method":  map[string]string{"type": "string", "example": "resources/list"},
									},
									"required": []string{"jsonrpc", "id", "method"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "JSON-RPC response with resource list",
						},
					},
				},
			},
			prefix + "/paywall/v1/products": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List Products",
					"description": "Get all available products with pricing information",
					"operationId": "listProducts",
					"tags":        []string{"Products"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of products",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"$ref": "#/components/schemas/Product",
										},
									},
								},
							},
						},
					},
				},
			},
			prefix + "/paywall/v1/stripe-session": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Create Stripe Checkout Session",
					"description": "Create a Stripe checkout session for a single product",
					"operationId": "createStripeSession",
					"tags":        []string{"Stripe"},
					"parameters": []map[string]interface{}{
						{
							"name":        "Idempotency-Key",
							"in":          "header",
							"description": "Unique key for request deduplication (24h window)",
							"required":    false,
							"schema":      map[string]string{"type": "string"},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"resource":      map[string]string{"type": "string", "description": "Product ID"},
										"customerEmail": map[string]string{"type": "string", "format": "email"},
										"couponCode":    map[string]string{"type": "string"},
									},
									"required": []string{"resource"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Checkout session created",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"sessionId": map[string]string{"type": "string"},
											"url":       map[string]string{"type": "string", "format": "uri"},
										},
									},
								},
							},
						},
					},
				},
			},
			prefix + "/paywall/{resourceId}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Access Paywalled Resource",
					"description": "Get paywalled content with x402 payment or Stripe session verification",
					"operationId": "getPaywalledResource",
					"tags":        []string{"Paywall"},
					"parameters": []map[string]interface{}{
						{
							"name":        "resourceId",
							"in":          "path",
							"description": "Resource identifier",
							"required":    true,
							"schema":      map[string]string{"type": "string"},
						},
						{
							"name":        "X-PAYMENT",
							"in":          "header",
							"description": "x402 payment proof (base64-encoded JSON)",
							"required":    false,
							"schema":      map[string]string{"type": "string"},
						},
						{
							"name":        "X-Stripe-Session",
							"in":          "header",
							"description": "Stripe checkout session ID",
							"required":    false,
							"schema":      map[string]string{"type": "string"},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Payment verified, resource delivered",
						},
						"402": map[string]interface{}{
							"description": "Payment required (x402 quote)",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/X402Quote",
									},
								},
							},
						},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"Product": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":            map[string]string{"type": "string"},
						"description":   map[string]string{"type": "string"},
						"fiatAmount":    map[string]string{"type": "number", "format": "double"},
						"fiatCurrency":  map[string]string{"type": "string"},
						"stripePriceId": map[string]string{"type": "string"},
						"cryptoAmount":  map[string]string{"type": "number", "format": "double"},
						"cryptoToken":   map[string]string{"type": "string"},
						"metadata":      map[string]string{"type": "object"},
					},
				},
				"X402Quote": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"x402Version": map[string]string{"type": "integer", "example": "0"},
						"accepts": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"scheme":            map[string]string{"type": "string"},
									"network":           map[string]string{"type": "string"},
									"maxAmountRequired": map[string]string{"type": "string"},
									"resource":          map[string]string{"type": "string"},
									"payTo":             map[string]string{"type": "string"},
									"maxTimeoutSeconds": map[string]string{"type": "integer"},
									"asset":             map[string]string{"type": "string"},
								},
							},
						},
					},
				},
			},
			"securitySchemes": map[string]interface{}{
				"x402": map[string]interface{}{
					"type":        "apiKey",
					"in":          "header",
					"name":        "X-PAYMENT",
					"description": "x402 payment proof (base64-encoded JSON)",
				},
				"stripeSession": map[string]interface{}{
					"type":        "apiKey",
					"in":          "header",
					"name":        "X-Stripe-Session",
					"description": "Stripe checkout session ID",
				},
			},
		},
		"tags": []map[string]string{
			{"name": "System", "description": "System endpoints"},
			{"name": "Discovery", "description": "Agent discovery endpoints"},
			{"name": "Products", "description": "Product catalog"},
			{"name": "Stripe", "description": "Stripe payment endpoints"},
			{"name": "Paywall", "description": "Protected resource access"},
		},
	}
}
