package httpserver

import (
	"net/http"

	"github.com/CedrosPay/server/pkg/responders"
)

// MCPResourcesListRequest represents a JSON-RPC 2.0 request for resources/list
type MCPResourcesListRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// MCPResourcesListResponse represents a JSON-RPC 2.0 response for resources/list
type MCPResourcesListResponse struct {
	JSONRPC string                  `json:"jsonrpc"`
	ID      interface{}             `json:"id"`
	Result  *MCPResourcesListResult `json:"result,omitempty"`
	Error   *MCPError               `json:"error,omitempty"`
}

// MCPResourcesListResult contains the list of available resources
type MCPResourcesListResult struct {
	Resources []MCPResource `json:"resources"`
}

// MCPResource represents a single MCP resource
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPError represents a JSON-RPC 2.0 error
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// mcpResourcesList handles POST /resources/list
// This follows the Model Context Protocol (MCP) JSON-RPC 2.0 specification
// for resource discovery by AI agents.
//
// MCP Specification: https://modelcontextprotocol.io/docs/concepts/resources/
func (h *handlers) mcpResourcesList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// Parse JSON-RPC request
	var req MCPResourcesListRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		h.sendMCPError(w, nil, -32700, "Parse error", nil)
		return
	}

	// Validate JSON-RPC version
	if req.JSONRPC != "2.0" {
		h.sendMCPError(w, req.ID, -32600, "Invalid Request: jsonrpc must be '2.0'", nil)
		return
	}

	// Validate method
	if req.Method != "resources/list" {
		h.sendMCPError(w, req.ID, -32601, "Method not found", nil)
		return
	}

	// Fetch all products from paywall service
	products, err := h.paywall.ListProducts(r.Context())
	if err != nil {
		h.sendMCPError(w, req.ID, -32603, "Internal error: failed to fetch resources", err.Error())
		return
	}

	// Build MCP resources from products
	resources := make([]MCPResource, 0, len(products))
	for _, p := range products {
		// Construct resource URI following a standard pattern
		// Format: cedros-pay://paywall/{product_id}
		uri := "cedros-pay://paywall/" + p.ID

		resource := MCPResource{
			URI:         uri,
			Name:        p.Description,
			Description: p.Description,
			MimeType:    "application/json", // All our resources return JSON
		}
		resources = append(resources, resource)
	}

	// Build JSON-RPC response
	response := MCPResourcesListResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: &MCPResourcesListResult{
			Resources: resources,
		},
	}

	// Send response
	responders.JSON(w, http.StatusOK, response)
}

// sendMCPError sends a JSON-RPC 2.0 error response
func (h *handlers) sendMCPError(w http.ResponseWriter, id interface{}, code int, message string, data interface{}) {
	response := MCPResourcesListResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	responders.JSON(w, http.StatusOK, response) // JSON-RPC errors still return 200 OK
}
