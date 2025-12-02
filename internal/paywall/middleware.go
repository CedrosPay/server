package paywall

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/CedrosPay/server/pkg/responders"
)

type contextKey string

const (
	contextKeyAuthorization contextKey = "paywall.authorization"
	contextKeyResourceID    contextKey = "paywall.resourceID"
)

// ResourceResolver extracts the paywall resource identifier from the request.
type ResourceResolver func(*http.Request) (string, error)

// Middleware enforces paywall checks before calling the downstream handler.
func (s *Service) Middleware(resolver ResourceResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resourceID, err := resolver(r)
			if err != nil {
				if errors.Is(err, ErrResourceNotConfigured) {
					responders.JSON(w, http.StatusNotFound, map[string]any{
						"error": "resource not found",
					})
					return
				}
				responders.JSON(w, http.StatusBadRequest, map[string]any{
					"error": err.Error(),
				})
				return
			}

			stripeSession := r.Header.Get("X-Stripe-Session")
			paymentHeader := r.Header.Get("X-PAYMENT")
			paymentHeader = strings.TrimSpace(paymentHeader)
			couponCode := r.URL.Query().Get("couponCode")
			wallet := r.Header.Get("X-Wallet") // For subscription access checks

			result, err := s.AuthorizeWithWallet(r.Context(), resourceID, stripeSession, paymentHeader, couponCode, wallet)
			if err != nil {
				if errors.Is(err, ErrStripeSessionPending) {
					responders.JSON(w, http.StatusPaymentRequired, map[string]any{"error": err.Error()})
					return
				}
				responders.JSON(w, http.StatusForbidden, map[string]any{
					"error": err.Error(),
				})
				return
			}

			if !result.Granted {
				// Build x402 compliant Payment Required Response
				// Reference: https://github.com/coinbase/x402
				response := map[string]any{
					"x402Version": 0,
					"error":       "payment required",
				}

				// Build accepts array with payment requirements
				var accepts []any
				if result.Quote != nil && result.Quote.Crypto != nil {
					accepts = append(accepts, result.Quote.Crypto)
				}
				if len(accepts) > 0 {
					response["accepts"] = accepts
				}

				responders.JSON(w, http.StatusPaymentRequired, response)
				return
			}

			ctx := context.WithValue(r.Context(), contextKeyAuthorization, result)
			ctx = context.WithValue(ctx, contextKeyResourceID, resourceID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthorizationFromContext retrieves the authorization result for logging or auditing.
func AuthorizationFromContext(ctx context.Context) (AuthorizationResult, bool) {
	val := ctx.Value(contextKeyAuthorization)
	if val == nil {
		return AuthorizationResult{}, false
	}
	result, ok := val.(AuthorizationResult)
	return result, ok
}

// ResourceIDFromContext retrieves the resolved resource identifier.
func ResourceIDFromContext(ctx context.Context) (string, bool) {
	val := ctx.Value(contextKeyResourceID)
	if id, ok := val.(string); ok {
		return id, true
	}
	return "", false
}
