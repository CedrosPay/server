package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"text/template"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/httputil"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// BalanceMonitor periodically checks server wallet balances and sends alerts when balances are low.
type BalanceMonitor struct {
	cfg        *config.Config
	rpcClient  *rpc.Client
	wallets    []solana.PublicKey
	httpClient *http.Client

	mu          sync.Mutex
	alertedKeys map[string]time.Time // Track which wallets we've already alerted about (key -> last alert time)

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// BalanceAlert contains information about a wallet with low balance.
type BalanceAlert struct {
	Wallet    string    `json:"wallet"`
	Balance   float64   `json:"balance"`
	Threshold float64   `json:"threshold"`
	Timestamp time.Time `json:"timestamp"`
}

// NewBalanceMonitor creates a new balance monitor for the configured server wallets.
func NewBalanceMonitor(cfg *config.Config, rpcClient *rpc.Client, wallets []solana.PrivateKey) *BalanceMonitor {
	// Extract public keys from private keys
	publicKeys := make([]solana.PublicKey, len(wallets))
	for i, wallet := range wallets {
		publicKeys[i] = wallet.PublicKey()
	}

	return &BalanceMonitor{
		cfg:         cfg,
		rpcClient:   rpcClient,
		wallets:     publicKeys,
		httpClient:  httputil.NewClient(cfg.Monitoring.Timeout.Duration),
		alertedKeys: make(map[string]time.Time),
		stopCh:      make(chan struct{}),
	}
}

// Start begins the balance monitoring loop.
func (m *BalanceMonitor) Start(ctx context.Context) {
	// Don't start if no alert URL configured
	if m.cfg.Monitoring.LowBalanceAlertURL == "" {
		log.Info().Msg("balance_monitor.disabled_no_url")
		return
	}

	if len(m.wallets) == 0 {
		log.Info().Msg("balance_monitor.no_wallets")
		return
	}

	log.Info().
		Int("wallet_count", len(m.wallets)).
		Dur("check_interval", m.cfg.Monitoring.CheckInterval.Duration).
		Float64("threshold_sol", m.cfg.Monitoring.LowBalanceThreshold).
		Msg("balance_monitor.started")

	m.wg.Add(1)
	go m.monitorLoop(ctx)
}

// Stop gracefully stops the balance monitoring loop.
func (m *BalanceMonitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	log.Info().Msg("balance_monitor.stopped")
}

// monitorLoop runs the periodic balance checks.
func (m *BalanceMonitor) monitorLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cfg.Monitoring.CheckInterval.Duration)
	defer ticker.Stop()

	// Run initial check immediately
	m.checkBalances(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkBalances(ctx)
		}
	}
}

// checkBalances checks all wallet balances and sends alerts for low balances.
func (m *BalanceMonitor) checkBalances(ctx context.Context) {
	for _, wallet := range m.wallets {
		balance, err := m.getBalance(ctx, wallet)
		if err != nil {
			log.Error().
				Err(err).
				Str("wallet", logger.TruncateAddress(wallet.String())).
				Msg("balance_monitor.fetch_error")
			continue
		}

		// Convert lamports to SOL (1 SOL = 1e9 lamports)
		balanceSOL := float64(balance) / 1e9

		log.Debug().
			Str("wallet", logger.TruncateAddress(wallet.String())).
			Float64("balance_sol", balanceSOL).
			Msg("balance_monitor.balance_checked")

		if balanceSOL < m.cfg.Monitoring.LowBalanceThreshold {
			// Check if we've already alerted recently (within last 24 hours)
			if m.shouldAlert(wallet.String()) {
				m.sendAlert(ctx, wallet.String(), balanceSOL)
			}
		} else {
			// Balance is healthy, clear alert history
			m.clearAlert(wallet.String())
		}
	}
}

// getBalance fetches the SOL balance for a wallet.
func (m *BalanceMonitor) getBalance(ctx context.Context, wallet solana.PublicKey) (uint64, error) {
	// Use confirmed commitment for faster monitoring (400ms vs 15s)
	// Monitoring doesn't require finalized commitment as we're tracking trends
	result, err := m.rpcClient.GetBalance(ctx, wallet, rpc.CommitmentConfirmed)
	if err != nil {
		return 0, fmt.Errorf("rpc get balance: %w", err)
	}
	return result.Value, nil
}

// shouldAlert returns true if we should send an alert for this wallet.
// We only alert once per 24 hours to avoid spam.
func (m *BalanceMonitor) shouldAlert(wallet string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	lastAlert, exists := m.alertedKeys[wallet]
	if !exists {
		return true
	}

	// Only alert again if 24 hours have passed
	return time.Since(lastAlert) > 24*time.Hour
}

// clearAlert removes the alert history for a wallet (when balance is restored).
func (m *BalanceMonitor) clearAlert(wallet string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.alertedKeys, wallet)
}

// sendAlert sends a webhook notification about a low balance.
func (m *BalanceMonitor) sendAlert(ctx context.Context, wallet string, balance float64) {
	alert := BalanceAlert{
		Wallet:    wallet,
		Balance:   balance,
		Threshold: m.cfg.Monitoring.LowBalanceThreshold,
		Timestamp: time.Now(),
	}

	var body []byte
	var err error

	// Use custom template if provided, otherwise use default Discord format
	if m.cfg.Monitoring.BodyTemplate != "" {
		body, err = m.renderTemplate(alert)
		if err != nil {
			log.Error().
				Err(err).
				Str("wallet", logger.TruncateAddress(wallet)).
				Msg("balance_monitor.template_error")
			return
		}
	} else {
		// Default Discord webhook format
		body, err = json.Marshal(map[string]any{
			"content": fmt.Sprintf(
				"⚠️ **Low Balance Alert**\n\n"+
					"Wallet: `%s`\n"+
					"Balance: **%.6f SOL**\n"+
					"Threshold: %.6f SOL\n\n"+
					"Please add more SOL to continue processing gasless transactions.",
				wallet, balance, m.cfg.Monitoring.LowBalanceThreshold,
			),
		})
		if err != nil {
			log.Error().
				Err(err).
				Str("wallet", logger.TruncateAddress(wallet)).
				Msg("balance_monitor.marshal_error")
			return
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.cfg.Monitoring.LowBalanceAlertURL, bytes.NewReader(body))
	if err != nil {
		log.Error().
			Err(err).
			Str("wallet", logger.TruncateAddress(wallet)).
			Msg("balance_monitor.request_error")
		return
	}

	// Set default Content-Type for Discord/Slack
	req.Header.Set("Content-Type", "application/json")

	// Apply custom headers
	for key, value := range m.cfg.Monitoring.Headers {
		req.Header.Set(key, value)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		log.Error().
			Err(err).
			Str("wallet", logger.TruncateAddress(wallet)).
			Msg("balance_monitor.send_error")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Info().
			Str("wallet", logger.TruncateAddress(wallet)).
			Float64("balance_sol", balance).
			Int("status_code", resp.StatusCode).
			Msg("balance_monitor.alert_sent")
		// Mark as alerted
		m.mu.Lock()
		m.alertedKeys[wallet] = time.Now()
		m.mu.Unlock()
	} else {
		log.Warn().
			Str("wallet", logger.TruncateAddress(wallet)).
			Int("status_code", resp.StatusCode).
			Msg("balance_monitor.alert_failed")
	}
}

// renderTemplate renders the custom body template with alert data.
func (m *BalanceMonitor) renderTemplate(alert BalanceAlert) ([]byte, error) {
	tmpl, err := template.New("alert").Parse(m.cfg.Monitoring.BodyTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, alert); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	return buf.Bytes(), nil
}
