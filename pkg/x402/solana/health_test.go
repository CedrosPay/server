package solana

import (
	"testing"

	"github.com/gagliardetto/solana-go"
)

func TestWalletHealthChecker_HealthyWalletSelection(t *testing.T) {
	// Create test wallets
	wallet1 := solana.NewWallet()
	wallet2 := solana.NewWallet()
	wallet3 := solana.NewWallet()

	wallets := []solana.PrivateKey{
		wallet1.PrivateKey,
		wallet2.PrivateKey,
		wallet3.PrivateKey,
	}

	// Create checker (without RPC, we'll manually set health)
	checker := &WalletHealthChecker{
		wallets: wallets,
		health:  make(map[string]*WalletHealth),
	}

	// Manually set health status to simulate different balance states
	checker.health[wallet1.PublicKey().String()] = &WalletHealth{
		PublicKey:  wallet1.PublicKey(),
		Balance:    0.1, // 0.1 SOL (healthy)
		IsHealthy:  true,
		IsCritical: false,
	}
	checker.health[wallet2.PublicKey().String()] = &WalletHealth{
		PublicKey:  wallet2.PublicKey(),
		Balance:    0.0005, // 0.0005 SOL (critical)
		IsHealthy:  false,
		IsCritical: true,
	}
	checker.health[wallet3.PublicKey().String()] = &WalletHealth{
		PublicKey:  wallet3.PublicKey(),
		Balance:    0.01, // 0.01 SOL (healthy)
		IsHealthy:  true,
		IsCritical: false,
	}

	// Verify health status
	h1, ok := checker.GetWalletHealth(wallet1.PublicKey())
	if !ok || !h1.IsHealthy {
		t.Errorf("Wallet 1 should be healthy (balance: %.6f SOL)", h1.Balance)
	}

	h2, ok := checker.GetWalletHealth(wallet2.PublicKey())
	if !ok || !h2.IsCritical {
		t.Errorf("Wallet 2 should be critical (balance: %.6f SOL)", h2.Balance)
	}

	h3, ok := checker.GetWalletHealth(wallet3.PublicKey())
	if !ok || !h3.IsHealthy {
		t.Errorf("Wallet 3 should be healthy (balance: %.6f SOL)", h3.Balance)
	}

	// Test round-robin selection (should skip unhealthy wallet 2)
	var idx uint64 = 0

	// First call should return wallet1 (index 0, healthy)
	selected := checker.GetHealthyWallet(&idx)
	if selected == nil {
		t.Fatal("Expected a healthy wallet, got nil")
	}
	if selected.PublicKey().Equals(wallet2.PublicKey()) {
		t.Error("Should not select critical wallet2")
	}
	if !selected.PublicKey().Equals(wallet1.PublicKey()) {
		t.Errorf("Expected wallet1, got %s", selected.PublicKey().String()[:8])
	}

	// Second call should skip wallet2 (unhealthy) and return wallet3
	selected2 := checker.GetHealthyWallet(&idx)
	if selected2 == nil {
		t.Fatal("Expected a second healthy wallet, got nil")
	}
	if selected2.PublicKey().Equals(wallet2.PublicKey()) {
		t.Error("Should not select critical wallet2")
	}
	if !selected2.PublicKey().Equals(wallet3.PublicKey()) {
		t.Errorf("Expected wallet3 after skipping wallet2, got %s", selected2.PublicKey().String()[:8])
	}

	// Third call should wrap around to wallet1 again
	selected3 := checker.GetHealthyWallet(&idx)
	if selected3 == nil {
		t.Fatal("Expected wallet to wrap around, got nil")
	}
	if !selected3.PublicKey().Equals(wallet1.PublicKey()) {
		t.Errorf("Expected wallet1 on wrap-around, got %s", selected3.PublicKey().String()[:8])
	}
}

func TestWalletHealthChecker_AllUnhealthy(t *testing.T) {
	// Create test wallets
	wallet1 := solana.NewWallet()
	wallet2 := solana.NewWallet()

	wallets := []solana.PrivateKey{
		wallet1.PrivateKey,
		wallet2.PrivateKey,
	}

	checker := &WalletHealthChecker{
		wallets: wallets,
		health:  make(map[string]*WalletHealth),
	}

	// Set all wallets as critical
	checker.health[wallet1.PublicKey().String()] = &WalletHealth{
		PublicKey:  wallet1.PublicKey(),
		Balance:    0.0005, // 0.0005 SOL (critical)
		IsHealthy:  false,
		IsCritical: true,
	}
	checker.health[wallet2.PublicKey().String()] = &WalletHealth{
		PublicKey:  wallet2.PublicKey(),
		Balance:    0.0008, // 0.0008 SOL (critical)
		IsHealthy:  false,
		IsCritical: true,
	}

	// Try to get a healthy wallet (should return nil)
	var idx uint64 = 0
	selected := checker.GetHealthyWallet(&idx)
	if selected != nil {
		t.Errorf("Expected nil when all wallets are unhealthy, got wallet: %s", selected.PublicKey().String())
	}
}

func TestWalletHealthChecker_Summary(t *testing.T) {
	// Create test wallets with different states
	wallet1 := solana.NewWallet()
	wallet2 := solana.NewWallet()
	wallet3 := solana.NewWallet()
	wallet4 := solana.NewWallet()

	wallets := []solana.PrivateKey{
		wallet1.PrivateKey,
		wallet2.PrivateKey,
		wallet3.PrivateKey,
		wallet4.PrivateKey,
	}

	checker := &WalletHealthChecker{
		wallets: wallets,
		health:  make(map[string]*WalletHealth),
	}

	// Set different health states
	checker.health[wallet1.PublicKey().String()] = &WalletHealth{
		PublicKey:  wallet1.PublicKey(),
		Balance:    0.1, // 0.1 SOL (healthy)
		IsHealthy:  true,
		IsCritical: false,
	}
	checker.health[wallet2.PublicKey().String()] = &WalletHealth{
		PublicKey:  wallet2.PublicKey(),
		Balance:    0.003, // 0.003 SOL (unhealthy but not critical)
		IsHealthy:  false,
		IsCritical: false,
	}
	checker.health[wallet3.PublicKey().String()] = &WalletHealth{
		PublicKey:  wallet3.PublicKey(),
		Balance:    0.0005, // 0.0005 SOL (critical)
		IsHealthy:  false,
		IsCritical: true,
	}
	checker.health[wallet4.PublicKey().String()] = &WalletHealth{
		PublicKey:  wallet4.PublicKey(),
		Balance:    0.01, // 0.01 SOL (healthy)
		IsHealthy:  true,
		IsCritical: false,
	}

	// Check summary
	healthy, unhealthy, critical := checker.HealthySummary()
	if healthy != 2 {
		t.Errorf("Expected 2 healthy wallets, got %d", healthy)
	}
	if unhealthy != 1 {
		t.Errorf("Expected 1 unhealthy wallet, got %d", unhealthy)
	}
	if critical != 1 {
		t.Errorf("Expected 1 critical wallet, got %d", critical)
	}
}
