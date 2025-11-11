package x402

import "time"

// Transaction confirmation timeouts and intervals
const (
	// BlockhashValidityWindow is the conservative window for Solana blockhash validity.
	// Solana blockhashes are valid for ~150 slots (~60 seconds on mainnet).
	// We use 90 seconds as a conservative estimate.
	BlockhashValidityWindow = 90 * time.Second

	// RPCPollInterval is how frequently we poll RPC for transaction status when WebSocket fails.
	RPCPollInterval = 2 * time.Second

	// DefaultConfirmationTimeout is the maximum time to wait for transaction confirmation.
	DefaultConfirmationTimeout = 2 * time.Minute

	// DefaultAccessTTL is how long verified payments remain cached.
	DefaultAccessTTL = 45 * time.Minute
)

// Floating point tolerance for amount comparisons
const (
	// AmountTolerance is the epsilon used when comparing cryptocurrency amounts.
	// This accounts for floating point precision issues.
	AmountTolerance = 1e-9
)
