package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/paywall"
	"github.com/CedrosPay/server/pkg/responders"
	"github.com/CedrosPay/server/pkg/x402"
)

// resourceKey returns the appropriate JSON key name for a resource type.
func resourceKey(resourceType string) string {
	switch resourceType {
	case "cart":
		return "cartId"
	case "refund":
		return "refundId"
	default:
		return "resourceId"
	}
}

// paymentRequiredResponse sends a 402 Payment Required response.
func paymentRequiredResponse(w http.ResponseWriter, message, resourceID, resourceType string) {
	apierrors.WriteError(w, apierrors.ErrCodeMissingField, message, map[string]interface{}{
		resourceKey(resourceType): resourceID,
	})
}

// paymentVerificationFailedResponse sends a 402 response when payment verification fails.
func paymentVerificationFailedResponse(w http.ResponseWriter, err error, resourceID, resourceType string) {
	// Check if it's a VerificationError with specific error code
	if vErr, ok := err.(x402.VerificationError); ok {
		apierrors.WriteError(w, vErr.Code, vErr.Message, map[string]interface{}{
			resourceKey(resourceType): resourceID,
		})
		return
	}
	apierrors.WriteError(w, apierrors.ErrCodeTransactionFailed, err.Error(), map[string]interface{}{
		resourceKey(resourceType): resourceID,
	})
}

// paymentNotGrantedResponse sends a 402 response when payment is not granted.
func paymentNotGrantedResponse(w http.ResponseWriter, message, resourceID, resourceType string) {
	apierrors.WriteError(w, apierrors.ErrCodeTransactionFailed, message, map[string]interface{}{
		resourceKey(resourceType): resourceID,
	})
}

// paymentSuccessResponse sends a 200 OK response when payment is verified.
func paymentSuccessResponse(w http.ResponseWriter, resourceID, resourceType string, result paywall.AuthorizationResult) {
	message := fmt.Sprintf("Payment verified for %s %s", resourceType, resourceID)
	if resourceType == "refund" {
		message = fmt.Sprintf("Refund verified for %s", resourceID)
	}

	response := map[string]any{
		"success":                 true,
		"message":                 message,
		"method":                  result.Method,
		resourceKey(resourceType): resourceID,
	}

	// Add optional wallet and signature if present
	if result.Wallet != "" {
		response["wallet"] = result.Wallet
	}
	if result.Settlement != nil && result.Settlement.TxHash != nil {
		response["signature"] = *result.Settlement.TxHash
	}

	// Add X-PAYMENT-RESPONSE header per x402 spec
	addSettlementHeader(w, result.Settlement)

	responders.JSON(w, http.StatusOK, response)
}

// addSettlementHeader adds X-PAYMENT-RESPONSE header if settlement exists.
// This header contains the base64-encoded JSON settlement proof per x402 spec.
func addSettlementHeader(w http.ResponseWriter, settlement *paywall.SettlementResponse) {
	if settlement == nil {
		return
	}
	settlementJSON, err := json.Marshal(settlement)
	if err != nil {
		return
	}
	settlementHeader := base64.StdEncoding.EncodeToString(settlementJSON)
	w.Header().Set("X-PAYMENT-RESPONSE", settlementHeader)
}
