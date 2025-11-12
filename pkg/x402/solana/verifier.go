package solana

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"

	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/metrics"
	solanaHelpers "github.com/CedrosPay/server/internal/solana"
	"github.com/CedrosPay/server/pkg/x402"
)

// SolanaVerifier confirms x402 payments against the Solana blockchain.
type SolanaVerifier struct {
	rpcClient               *rpc.Client
	wsClient                *ws.Client
	clock                   func() time.Time
	serverWallets           []solana.PrivateKey // Server wallets for gasless and token account creation
	walletIndex             atomic.Uint64       // Round-robin counter for wallet selection
	gaslessEnabled          bool
	autoCreateTokenAccounts bool
	txQueue                 *TransactionQueue    // Transaction queue for rate limiting
	healthChecker           *WalletHealthChecker // Health checker for wallet balance monitoring
	metrics                 *metrics.Metrics     // Optional: Prometheus metrics collector
	network                 string               // Network identifier for metrics (mainnet-beta, devnet, etc.)
}

// NewSolanaVerifier creates a verifier backed by RPC + WebSocket endpoints.
func NewSolanaVerifier(rpcURL, wsURL string) (*SolanaVerifier, error) {
	if rpcURL == "" {
		return nil, errors.New("x402 solana: rpc url required")
	}
	if wsURL == "" {
		derived, err := deriveWebsocketURL(rpcURL)
		if err != nil {
			return nil, fmt.Errorf("x402 solana: derive websocket url: %w", err)
		}
		wsURL = derived
	}

	wsClient, err := ws.Connect(context.Background(), wsURL)
	if err != nil {
		return nil, fmt.Errorf("x402 solana: connect websocket: %w", err)
	}

	verifier := &SolanaVerifier{
		rpcClient: rpc.New(rpcURL),
		wsClient:  wsClient,
		clock:     time.Now,
	}

	return verifier, nil
}

// Close releases underlying websocket resources and stops health checker.
func (s *SolanaVerifier) Close() {
	if s.healthChecker != nil {
		s.healthChecker.Stop()
	}
	if s.wsClient != nil {
		s.wsClient.Close()
	}
}

// RPCClient returns the underlying RPC client for direct access.
func (s *SolanaVerifier) RPCClient() *rpc.Client {
	return s.rpcClient
}

// GetHealthChecker returns the wallet health checker for monitoring.
func (s *SolanaVerifier) GetHealthChecker() *WalletHealthChecker {
	return s.healthChecker
}

// SetServerWallets configures the server wallets for gasless transactions and token account creation.
// Wallets are used in round-robin fashion to distribute load and avoid rate limits.
// This also initializes and starts the wallet health checker.
func (s *SolanaVerifier) SetServerWallets(wallets []solana.PrivateKey) {
	s.serverWallets = wallets

	// Initialize health checker if wallets are provided
	if len(wallets) > 0 {
		s.healthChecker = NewWalletHealthChecker(s.rpcClient, wallets)
		s.healthChecker.Start()
	}
}

// WithMetrics adds metrics collection to the verifier.
func (s *SolanaVerifier) WithMetrics(m *metrics.Metrics, network string) *SolanaVerifier {
	s.metrics = m
	s.network = network
	return s
}

// EnableGasless enables gasless transaction support.
// When enabled, the verifier will co-sign partially signed transactions with a server wallet.
func (s *SolanaVerifier) EnableGasless() {
	s.gaslessEnabled = true
}

// EnableAutoCreateTokenAccounts enables automatic token account creation.
// When enabled, if a payment fails due to a missing token account, the verifier will create it and retry.
func (s *SolanaVerifier) EnableAutoCreateTokenAccounts() {
	s.autoCreateTokenAccounts = true
}

// SetupTxQueue initializes the transaction queue with the given rate limiting settings.
func (s *SolanaVerifier) SetupTxQueue(minTimeBetween time.Duration, maxInFlight int) {
	s.txQueue = NewTransactionQueue(s.rpcClient, s, minTimeBetween, maxInFlight)
	s.txQueue.Start()
}

// ShutdownTxQueue stops the transaction queue gracefully.
func (s *SolanaVerifier) ShutdownTxQueue() {
	if s.txQueue != nil {
		s.txQueue.Shutdown()
	}
}

// getNextWallet returns the next healthy server wallet using round-robin selection.
// Returns nil if no wallets are configured or all wallets are unhealthy.
func (s *SolanaVerifier) getNextWallet() *solana.PrivateKey {
	if len(s.serverWallets) == 0 {
		return nil
	}

	// If health checker is active, use it to get a healthy wallet
	if s.healthChecker != nil {
		idx := s.walletIndex.Load()
		wallet := s.healthChecker.GetHealthyWallet(&idx)
		s.walletIndex.Store(idx)
		return wallet
	}

	// Fallback: no health checker, use simple round-robin
	idx := s.walletIndex.Add(1) % uint64(len(s.serverWallets))
	return &s.serverWallets[idx]
}

// findWalletByPublicKey returns the wallet matching the given public key, or nil if not found.
func (s *SolanaVerifier) findWalletByPublicKey(pubkey solana.PublicKey) *solana.PrivateKey {
	for i := range s.serverWallets {
		if s.serverWallets[i].PublicKey().Equals(pubkey) {
			return &s.serverWallets[i]
		}
	}
	return nil
}

// Verify inspects the signed transaction, submits it, and waits for finalised confirmation.
func (s *SolanaVerifier) Verify(ctx context.Context, proof x402.PaymentProof, requirement x402.Requirement) (x402.VerificationResult, error) {
	if requirement.RecipientOwner == "" {
		return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInvalidRecipient, errors.New("recipient owner not configured"))
	}
	if requirement.TokenMint == "" {
		return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInvalidTokenMint, errors.New("token mint required"))
	}
	if proof.Transaction == "" {
		return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, errors.New("transaction payload missing"))
	}

	tx, err := solana.TransactionFromBase64(proof.Transaction)
	if err != nil {
		return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, err)
	}
	// Note: We don't validate tx.Signatures here because the actual signature
	// is returned by SendTransactionWithOpts after the transaction is broadcast

	if len(tx.Message.AccountKeys) == 0 {
		return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, errors.New("transaction missing account keys"))
	}
	txFeePayer := tx.Message.AccountKeys[0]

	// In gasless mode, validate that the fee payer matches what was provided
	if s.gaslessEnabled && proof.FeePayer != "" {
		expectedFeePayer, err := solana.PublicKeyFromBase58(proof.FeePayer)
		if err != nil {
			return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, fmt.Errorf("invalid fee payer address: %w", err))
		}
		if !txFeePayer.Equals(expectedFeePayer) {
			return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, fmt.Errorf("transaction fee payer %s does not match expected %s", txFeePayer.String(), proof.FeePayer))
		}
	}

	// Extract user wallet (transfer authority) from the transaction by validating the transfer instruction
	// This returns the amount AND validates that the transfer is properly structured
	amount, userWallet, err := validateTransferInstructionAndExtractAuthority(tx, requirement)
	if err != nil {
		return x402.VerificationResult{}, err
	}
	if amount+x402.AmountTolerance < requirement.Amount {
		return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeAmountBelowMinimum, fmt.Errorf("amount %.8f < %.8f", amount, requirement.Amount))
	}

	// If this is a gasless transaction (feePayer provided in proof), co-sign with the server wallet
	// IMPORTANT: Only co-sign if proof.FeePayer is set, even if gasless is globally enabled
	// This allows non-gasless transactions (like refunds) to work when gasless mode is configured
	if s.gaslessEnabled && proof.FeePayer != "" {
		// Extract the fee payer from the transaction (first signer)
		// The fee payer was set when we built the transaction, so we need to use the SAME wallet
		if len(tx.Message.AccountKeys) == 0 {
			return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, errors.New("transaction has no account keys"))
		}
		feePayer := tx.Message.AccountKeys[0] // Fee payer is always the first account

		// Find the matching server wallet
		matchingWallet := s.findWalletByPublicKey(feePayer)
		if matchingWallet == nil {
			return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, fmt.Errorf("transaction fee payer %s does not match any configured server wallet", feePayer.String()))
		}

		// Partial sign with the server wallet (transaction already has user's signature)
		_, err := tx.PartialSign(func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(matchingWallet.PublicKey()) {
				return matchingWallet
			}
			return nil
		})
		if err != nil {
			return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInternalError, fmt.Errorf("failed to co-sign transaction: %w", err))
		}
	}

	commitment := commitmentFromString(requirement.Commitment)

	sendOpts := rpc.TransactionOpts{
		SkipPreflight:       requirement.SkipPreflight,
		PreflightCommitment: commitment,
	}

	// Track RPC call metrics
	rpcStart := time.Now()
	actualSignature, sendErr := s.rpcClient.SendTransactionWithOpts(ctx, tx, sendOpts)
	if s.metrics != nil {
		s.metrics.ObserveRPCCall("SendTransaction", s.network, time.Since(rpcStart), sendErr)
	}
	if sendErr != nil {
		if !isAlreadyProcessedError(sendErr) {
			// Check for specific error types and return user-friendly messages
			if isInsufficientFundsTokenError(sendErr) {
				return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInsufficientFundsToken, sendErr)
			}
			if isInsufficientFundsSOLError(sendErr) {
				// If this is a gasless transaction (feePayer provided), this means the SERVER wallet is out of SOL
				// Return a server error instead of telling user to add SOL
				// For non-gasless transactions (like refunds), return insufficient funds error
				if s.gaslessEnabled && proof.FeePayer != "" {
					return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInternalError, sendErr)
				}
				return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInsufficientFunds, sendErr)
			}

			// Check if this is an account-not-found error and we can auto-create
			if isAccountNotFoundError(sendErr) {
				if s.autoCreateTokenAccounts {
					wallet := s.getNextWallet()
					if wallet == nil {
						return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeTransactionFailed, fmt.Errorf("auto-create enabled but no server wallets configured (original error: %w)", sendErr))
					}
					// Try to create the missing token account
					if err := s.handleMissingTokenAccount(ctx, requirement, *wallet); err != nil {
						return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeTransactionFailed, fmt.Errorf("failed to create token account: %w (original error: %w)", err, sendErr))
					}
					// Poll for account existence with exponential backoff instead of fixed sleep
					if err := s.waitForTokenAccountPropagation(ctx, requirement.RecipientTokenAccount); err != nil {
						return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeTransactionFailed, fmt.Errorf("token account creation timeout: %w", err))
					}
					// Retry the transaction after creating the account, skipping preflight to avoid RPC staleness issues
					retryOpts := rpc.TransactionOpts{
						SkipPreflight:       true, // Skip preflight since we just created the account
						PreflightCommitment: commitment,
					}

					// Track retry RPC call metrics
					retryStart := time.Now()
					actualSignature, sendErr = s.rpcClient.SendTransactionWithOpts(ctx, tx, retryOpts)
					if s.metrics != nil {
						s.metrics.ObserveRPCCall("SendTransaction", s.network, time.Since(retryStart), sendErr)
					}
					if sendErr != nil && !isAlreadyProcessedError(sendErr) {
						return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeTransactionFailed, sendErr)
					}
				} else {
					// Account not found and auto-create is disabled - this is a server configuration issue
					return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeInternalError, fmt.Errorf("service temporarily unavailable, please try again later or contact support"))
				}
			} else {
				return x402.VerificationResult{}, newVerificationError(apierrors.ErrCodeTransactionFailed, sendErr)
			}
		}
	}

	waitCtx, cancel := context.WithTimeout(ctx, maxDuration(requirement.QuoteTTL, x402.DefaultConfirmationTimeout))
	defer cancel()

	log := logger.FromContext(ctx)
	log.Debug().
		Str("signature", logger.TruncateAddress(actualSignature.String())).
		Str("commitment", string(commitment)).
		Msg("payment.awaiting_confirmation")

	confirmStart := time.Now()
	if err := s.awaitConfirmation(waitCtx, actualSignature, commitment); err != nil {
		log.Error().
			Err(err).
			Str("signature", logger.TruncateAddress(actualSignature.String())).
			Dur("wait_time_ms", time.Since(confirmStart)).
			Msg("payment.confirmation_failed")
		return x402.VerificationResult{}, err
	}

	confirmDuration := time.Since(confirmStart)
	log.Info().
		Str("wallet", logger.TruncateAddress(userWallet.String())).
		Str("signature", logger.TruncateAddress(actualSignature.String())).
		Float64("amount", amount).
		Str("token_mint", requirement.TokenMint).
		Dur("confirmation_time_ms", confirmDuration).
		Msg("payment.confirmed")

	expiry := s.clock().Add(maxDuration(requirement.QuoteTTL, x402.DefaultAccessTTL))
	return x402.VerificationResult{
		Wallet:    userWallet.String(),
		Amount:    amount,
		Signature: actualSignature.String(),
		ExpiresAt: expiry,
	}, nil
}

// handleMissingTokenAccount creates the associated token account for the recipient.
// This is called when a transaction fails due to a missing token account.
func (s *SolanaVerifier) handleMissingTokenAccount(ctx context.Context, requirement x402.Requirement, wallet solana.PrivateKey) error {
	// Parse the owner and mint
	owner, err := solana.PublicKeyFromBase58(requirement.RecipientOwner)
	if err != nil {
		return fmt.Errorf("invalid recipient owner: %w", err)
	}

	mint, err := solana.PublicKeyFromBase58(requirement.TokenMint)
	if err != nil {
		return fmt.Errorf("invalid token mint: %w", err)
	}

	// Create the associated token account using the provided wallet
	_, err = solanaHelpers.CreateAssociatedTokenAccount(ctx, s.rpcClient, s.wsClient, wallet, owner, mint)
	if err != nil {
		return fmt.Errorf("create ATA: %w", err)
	}

	return nil
}

// waitForTokenAccountPropagation polls for token account existence with exponential backoff.
// This replaces the hardcoded 15s sleep to reduce latency and adapt to network conditions.
func (s *SolanaVerifier) waitForTokenAccountPropagation(ctx context.Context, tokenAccountAddr string) error {
	accountPubkey, err := solana.PublicKeyFromBase58(tokenAccountAddr)
	if err != nil {
		return fmt.Errorf("invalid token account address: %w", err)
	}

	const maxAttempts = 30
	backoff := 500 * time.Millisecond
	const maxBackoff = 2 * time.Second

	timer := time.NewTimer(backoff)
	defer timer.Stop()

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Check if account exists
		getAccountStart := time.Now()
		accountInfo, err := s.rpcClient.GetAccountInfo(ctx, accountPubkey)
		if s.metrics != nil {
			s.metrics.ObserveRPCCall("GetAccountInfo", s.network, time.Since(getAccountStart), err)
		}
		if err == nil && accountInfo != nil && accountInfo.Value != nil {
			// Account exists and has data
			return nil
		}

		// Wait before retrying
		timer.Reset(backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			// Exponential backoff with cap
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return fmt.Errorf("token account not found after %d attempts", maxAttempts)
}
