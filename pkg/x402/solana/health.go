package solana

import (
	"context"
	"sync"
	"time"

	"github.com/CedrosPay/server/internal/logger"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rs/zerolog"
)

const (
	// MinHealthyBalance is the minimum SOL balance required for a wallet to be considered healthy.
	// This threshold accounts for:
	// - Rent-exempt minimum: ~0.00089 SOL for wallet account data
	// - Token account creation: ~0.002 SOL for recipient token account (if auto-create enabled)
	// - Transaction fees: ~0.000005 SOL per transaction
	// At 0.005 SOL, wallet can maintain rent-exemption, create token accounts, and process ~1,000 transactions.
	MinHealthyBalance = 0.005 // SOL

	// CriticalBalance is the balance at which we consider the wallet critically low.
	// Below this threshold, wallet may not have enough for token account creation + rent.
	CriticalBalance = 0.001 // SOL

	// HealthCheckInterval is how often we check wallet balances.
	// More frequent than monitoring (15min) to catch issues faster.
	HealthCheckInterval = 5 * time.Minute

	// HealthCheckTimeout is the RPC timeout for balance queries.
	HealthCheckTimeout = 10 * time.Second
)

// WalletHealth tracks the health status of a server wallet.
type WalletHealth struct {
	PublicKey      solana.PublicKey
	Balance        float64   // Current SOL balance
	IsHealthy      bool      // true if balance >= MinHealthyBalance
	IsCritical     bool      // true if balance <= CriticalBalance
	LastChecked    time.Time // When balance was last checked
	LastCheckError error     // Error from last balance check (if any)
}

// WalletHealthChecker monitors server wallet balances and tracks health status.
type WalletHealthChecker struct {
	mu         sync.RWMutex
	rpcClient  *rpc.Client
	wallets    []solana.PrivateKey
	health     map[string]*WalletHealth // pubkey string -> health
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	onCritical func(wallet WalletHealth) // Callback when wallet becomes critical
	log        zerolog.Logger            // Structured logger
}

// NewWalletHealthChecker creates a new health checker.
func NewWalletHealthChecker(rpcClient *rpc.Client, wallets []solana.PrivateKey) *WalletHealthChecker {
	ctx, cancel := context.WithCancel(context.Background())

	// Create logger with task context
	log := logger.FromContext(ctx).With().
		Str("component", "wallet_health_checker").
		Logger()

	checker := &WalletHealthChecker{
		rpcClient: rpcClient,
		wallets:   wallets,
		health:    make(map[string]*WalletHealth),
		ctx:       ctx,
		cancel:    cancel,
		log:       log,
	}

	// Initialize health map
	for _, wallet := range wallets {
		pubkey := wallet.PublicKey()
		checker.health[pubkey.String()] = &WalletHealth{
			PublicKey:   pubkey,
			Balance:     0,
			IsHealthy:   false, // Will be updated on first check
			IsCritical:  true,  // Assume critical until proven otherwise
			LastChecked: time.Time{},
		}
	}

	return checker
}

// SetCriticalCallback sets a callback to be invoked when a wallet's balance becomes critical.
func (w *WalletHealthChecker) SetCriticalCallback(fn func(wallet WalletHealth)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onCritical = fn
}

// Start begins background health checking.
func (w *WalletHealthChecker) Start() {
	// Do an immediate check on startup
	w.CheckAll()

	// Start background checker
	w.wg.Add(1)
	go w.healthCheckLoop()

	w.log.Info().
		Dur("interval", HealthCheckInterval).
		Float64("healthy_threshold_sol", MinHealthyBalance).
		Float64("critical_threshold_sol", CriticalBalance).
		Msg("wallet_health_checker.started")
}

// Stop gracefully stops the health checker.
func (w *WalletHealthChecker) Stop() {
	w.log.Info().Msg("wallet_health_checker.stopping")
	w.cancel()
	w.wg.Wait()
	w.log.Info().Msg("wallet_health_checker.stopped")
}

// healthCheckLoop runs periodic health checks.
func (w *WalletHealthChecker) healthCheckLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.CheckAll()
		}
	}
}

// CheckAll checks the balance of all wallets and updates their health status.
func (w *WalletHealthChecker) CheckAll() {
	for _, wallet := range w.wallets {
		w.checkWallet(wallet)
	}
}

// checkWallet checks a single wallet's balance and updates its health.
func (w *WalletHealthChecker) checkWallet(wallet solana.PrivateKey) {
	ctx, cancel := context.WithTimeout(w.ctx, HealthCheckTimeout)
	defer cancel()

	pubkey := wallet.PublicKey()
	pubkeyStr := pubkey.String()

	// Query balance using confirmed commitment for faster monitoring
	// Confirmed is sufficient for health checks as we're monitoring trends, not processing payments
	balanceLamports, err := w.rpcClient.GetBalance(ctx, pubkey, rpc.CommitmentConfirmed)
	if err != nil {
		w.mu.Lock()
		if h, ok := w.health[pubkeyStr]; ok {
			h.LastCheckError = err
			h.LastChecked = time.Now()
			// If we can't check balance, assume unhealthy to be safe
			h.IsHealthy = false
		}
		w.mu.Unlock()
		w.log.Error().
			Err(err).
			Str("wallet", logger.TruncateAddress(pubkey.String())).
			Msg("wallet_health.balance_check_failed")
		return
	}

	// Convert lamports to SOL (1 SOL = 1e9 lamports)
	balance := float64(balanceLamports.Value) / 1e9

	w.mu.Lock()
	defer w.mu.Unlock()

	health, ok := w.health[pubkeyStr]
	if !ok {
		// Shouldn't happen, but handle gracefully
		health = &WalletHealth{PublicKey: pubkey}
		w.health[pubkeyStr] = health
	}

	// Track previous state for change detection
	wasHealthy := health.IsHealthy
	wasCritical := health.IsCritical

	// Update health
	health.Balance = balance
	health.IsHealthy = balance >= MinHealthyBalance
	health.IsCritical = balance <= CriticalBalance
	health.LastChecked = time.Now()
	health.LastCheckError = nil

	// Log balance updates
	status := "healthy"
	if health.IsCritical {
		status = "CRITICAL"
	} else if !health.IsHealthy {
		status = "low"
	}

	w.log.Debug().
		Str("wallet", logger.TruncateAddress(pubkey.String())).
		Float64("balance_sol", balance).
		Str("status", status).
		Msg("wallet_health.balance_checked")

	// Log transitions
	if !wasHealthy && health.IsHealthy {
		w.log.Info().
			Str("wallet", logger.TruncateAddress(pubkey.String())).
			Float64("balance_sol", balance).
			Msg("wallet_health.now_healthy")
	} else if wasHealthy && !health.IsHealthy {
		w.log.Warn().
			Str("wallet", logger.TruncateAddress(pubkey.String())).
			Float64("balance_sol", balance).
			Msg("wallet_health.now_unhealthy")
	}

	if !wasCritical && health.IsCritical {
		w.log.Error().
			Str("wallet", logger.TruncateAddress(pubkey.String())).
			Float64("balance_sol", balance).
			Msg("wallet_health.now_critical")
		// Invoke critical callback if set
		if w.onCritical != nil {
			// Make a copy to avoid holding lock during callback
			healthCopy := *health
			go w.onCritical(healthCopy)
		}
	} else if wasCritical && !health.IsCritical {
		w.log.Info().
			Str("wallet", logger.TruncateAddress(pubkey.String())).
			Float64("balance_sol", balance).
			Msg("wallet_health.no_longer_critical")
	}
}

// GetHealthyWallet returns the next healthy wallet using round-robin selection.
// Returns nil if no healthy wallets are available.
func (w *WalletHealthChecker) GetHealthyWallet(currentIndex *uint64) *solana.PrivateKey {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if len(w.wallets) == 0 {
		return nil
	}

	// Try all wallets starting from current index
	for i := 0; i < len(w.wallets); i++ {
		// Increment and wrap
		idx := (int(*currentIndex) + i) % len(w.wallets)
		wallet := w.wallets[idx]
		pubkeyStr := wallet.PublicKey().String()

		// Check if healthy
		if health, ok := w.health[pubkeyStr]; ok && health.IsHealthy {
			// Update index for next call
			*currentIndex = uint64(idx + 1)
			return &wallet
		}
	}

	// No healthy wallets found
	return nil
}

// GetHealth returns the current health status of all wallets.
func (w *WalletHealthChecker) GetHealth() []WalletHealth {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make([]WalletHealth, 0, len(w.health))
	for _, h := range w.health {
		result = append(result, *h)
	}
	return result
}

// GetWalletHealth returns health for a specific wallet.
func (w *WalletHealthChecker) GetWalletHealth(pubkey solana.PublicKey) (*WalletHealth, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	h, ok := w.health[pubkey.String()]
	if !ok {
		return nil, false
	}
	// Return a copy
	healthCopy := *h
	return &healthCopy, true
}

// HealthySummary returns a summary of wallet health.
func (w *WalletHealthChecker) HealthySummary() (healthy, unhealthy, critical int) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, h := range w.health {
		if h.IsCritical {
			critical++
		} else if !h.IsHealthy {
			unhealthy++
		} else {
			healthy++
		}
	}
	return
}
