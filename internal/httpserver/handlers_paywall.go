package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gagliardetto/solana-go/rpc"

	"github.com/CedrosPay/server/internal/paywall"
	"github.com/CedrosPay/server/pkg/responders"
	x402solana "github.com/CedrosPay/server/pkg/x402/solana"
)

// health returns service health status including RPC connectivity and wallet health.
func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	now := time.Now()
	uptime := now.Sub(serverStartTime)
	rpcHealthy := h.checkRPCHealth(ctx)

	// Determine overall health status
	status := "ok"
	statusCode := http.StatusOK
	hasCriticalWallets := false

	// Check wallet health if gasless is enabled
	var walletHealth map[string]any
	if h.cfg.X402.GaslessEnabled {
		walletHealth = h.getWalletHealth()
		if walletHealth != nil {
			// Extract critical wallet count from summary
			if summary, ok := walletHealth["summary"].(map[string]int); ok {
				if summary["critical"] > 0 {
					hasCriticalWallets = true
				}
			}
		}
	}

	// Service is degraded if RPC is down or wallets are critical
	if !rpcHealthy || hasCriticalWallets {
		status = "degraded"
		statusCode = http.StatusServiceUnavailable // 503
	}

	response := map[string]any{
		"status":     status,
		"uptime":     uptime.String(),
		"timestamp":  now.UTC(),
		"rpcHealthy": rpcHealthy,
	}

	// Include route prefix for frontend discovery
	if h.cfg.Server.RoutePrefix != "" {
		response["routePrefix"] = h.cfg.Server.RoutePrefix
	}

	// Include network info
	response["network"] = h.cfg.X402.Network

	// Include enabled features
	features := []string{}
	if h.cfg.X402.GaslessEnabled {
		features = append(features, "gasless")
	}
	if h.cfg.X402.AutoCreateTokenAccount {
		features = append(features, "auto-create-token-accounts")
	}
	if h.cfg.Monitoring.LowBalanceAlertURL != "" {
		features = append(features, "balance-monitoring")
	}
	if len(features) > 0 {
		response["features"] = features
	}

	// Include wallet health if available
	if walletHealth != nil {
		response["walletHealth"] = walletHealth
	}

	responders.JSON(w, statusCode, response)
}

// checkRPCHealth verifies Solana RPC connectivity.
func (h *handlers) checkRPCHealth(ctx context.Context) bool {
	// Quick health check - just try to get slot
	verifier, ok := h.verifier.(interface{ RPCClient() *rpc.Client })
	if !ok {
		return false
	}
	client := verifier.RPCClient()
	if client == nil {
		return false
	}
	_, err := client.GetSlot(ctx, rpc.CommitmentFinalized)
	return err == nil
}

// getWalletHealth retrieves health status of all server wallets.
func (h *handlers) getWalletHealth() map[string]any {
	// Try to get health checker from verifier
	type healthProvider interface {
		GetHealthChecker() *x402solana.WalletHealthChecker
	}
	verifier, ok := h.verifier.(healthProvider)
	if !ok {
		return nil
	}

	checker := verifier.GetHealthChecker()
	if checker == nil {
		return nil
	}

	healthy, unhealthy, critical := checker.HealthySummary()
	allHealth := checker.GetHealth()

	// Build wallet details
	wallets := make([]map[string]any, 0, len(allHealth))
	for _, wh := range allHealth {
		status := "healthy"
		if wh.IsCritical {
			status = "critical"
		} else if !wh.IsHealthy {
			status = "unhealthy"
		}

		wallet := map[string]any{
			"publicKey":   wh.PublicKey.String()[:8] + "...", // Truncate for privacy
			"balance":     fmt.Sprintf("%.6f", wh.Balance),
			"status":      status,
			"lastChecked": wh.LastChecked.Format(time.RFC3339),
		}

		if wh.LastCheckError != nil {
			wallet["error"] = wh.LastCheckError.Error()
		}

		wallets = append(wallets, wallet)
	}

	return map[string]any{
		"summary": map[string]int{
			"healthy":   healthy,
			"unhealthy": unhealthy,
			"critical":  critical,
			"total":     len(allHealth),
		},
		"wallets": wallets,
	}
}

// paywalledContent serves protected content after payment verification.
func (h *handlers) paywalledContent(w http.ResponseWriter, r *http.Request) {
	resourceID, _ := paywall.ResourceIDFromContext(r.Context())
	auth, _ := paywall.AuthorizationFromContext(r.Context())

	payload := map[string]any{
		"resource": resourceID,
		"granted":  true,
		"method":   auth.Method,
	}
	if auth.Wallet != "" {
		payload["wallet"] = auth.Wallet
	}
	if auth.Settlement != nil && auth.Settlement.TxHash != nil {
		payload["signature"] = *auth.Settlement.TxHash
	}

	// Add X-PAYMENT-RESPONSE header per x402 spec
	// Reference: https://github.com/coinbase/x402
	addSettlementHeader(w, auth.Settlement)

	responders.JSON(w, http.StatusOK, payload)
}
