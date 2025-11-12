package errors

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the standardized error format returned to clients.
// It provides machine-readable error codes for robust error handling.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error code, message, and optional context.
type ErrorDetail struct {
	Code      ErrorCode              `json:"code"`              // Machine-readable error code
	Message   string                 `json:"message"`           // Human-readable error message
	Retryable bool                   `json:"retryable"`         // Whether the client should retry
	Details   map[string]interface{} `json:"details,omitempty"` // Optional context (resourceId, etc.)
}

// NewErrorResponse creates a standardized error response.
func NewErrorResponse(code ErrorCode, message string, details map[string]interface{}) ErrorResponse {
	return ErrorResponse{
		Error: ErrorDetail{
			Code:      code,
			Message:   message,
			Retryable: code.IsRetryable(),
			Details:   details,
		},
	}
}

// WriteJSON writes the error response as JSON to the HTTP response writer.
func (e ErrorResponse) WriteJSON(w http.ResponseWriter) {
	status := e.Error.Code.HTTPStatus()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(e)
}

// WriteError is a convenience function to write an error response in one call.
func WriteError(w http.ResponseWriter, code ErrorCode, message string, details map[string]interface{}) {
	resp := NewErrorResponse(code, message, details)
	resp.WriteJSON(w)
}

// WriteSimpleError writes an error with no additional details.
func WriteSimpleError(w http.ResponseWriter, code ErrorCode, message string) {
	WriteError(w, code, message, nil)
}

// WriteErrorWithDetail writes an error with a single detail field.
func WriteErrorWithDetail(w http.ResponseWriter, code ErrorCode, message string, key string, value interface{}) {
	WriteError(w, code, message, map[string]interface{}{key: value})
}
