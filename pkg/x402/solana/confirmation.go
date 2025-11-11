package solana

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/CedrosPay/server/pkg/x402"
)

// awaitConfirmation waits for transaction confirmation using WebSocket (fast) or RPC polling (fallback).
func (s *SolanaVerifier) awaitConfirmation(ctx context.Context, signature solana.Signature, commitment rpc.CommitmentType) error {
	// Try WebSocket first (faster)
	err := s.awaitConfirmationViaWebSocket(ctx, signature, commitment)
	if err == nil {
		return nil
	}

	// WebSocket failed - fall back to RPC polling to check if transaction actually succeeded
	// This is critical: if WS connection breaks, we MUST verify the transaction status via RPC
	// to ensure we don't miss payments
	return s.awaitConfirmationViaRPC(ctx, signature, commitment)
}

// awaitConfirmationViaWebSocket uses WebSocket subscription for fast confirmation.
func (s *SolanaVerifier) awaitConfirmationViaWebSocket(ctx context.Context, signature solana.Signature, commitment rpc.CommitmentType) error {
	sub, err := s.wsClient.SignatureSubscribe(signature, commitment)
	if err != nil {
		return fmt.Errorf("x402 solana: subscribe signature: %w", err)
	}
	defer sub.Unsubscribe()

	res, err := sub.Recv(ctx)
	if err != nil {
		return fmt.Errorf("x402 solana: wait confirmation: %w", err)
	}
	if res == nil {
		return errors.New("x402 solana: empty confirmation result")
	}
	if res.Value.Err != nil {
		return fmt.Errorf("x402 solana: transaction error: %v", res.Value.Err)
	}
	return nil
}

// awaitConfirmationViaRPC polls the RPC to check transaction status.
// This is a fallback when WebSocket fails - critical for payment reliability.
func (s *SolanaVerifier) awaitConfirmationViaRPC(ctx context.Context, signature solana.Signature, commitment rpc.CommitmentType) error {
	ticker := time.NewTicker(x402.RPCPollInterval)
	defer ticker.Stop()

	// Get blockhash validity to know when to stop polling
	// Solana blockhashes are valid for ~150 slots (~60 seconds on mainnet)
	// After that, if the transaction hasn't been seen, it never will be
	maxValidTime := time.Now().Add(x402.BlockhashValidityWindow)

	for {
		select {
		case <-ctx.Done():
			// Context timeout - but still check one last time
			return s.checkTransactionStatus(ctx, signature, commitment)
		case <-ticker.C:
			// Check if blockhash has expired
			if time.Now().After(maxValidTime) {
				// Blockhash expired - do one final check
				err := s.checkTransactionStatus(ctx, signature, commitment)
				if err == nil {
					return nil
				}
				// Transaction was never seen on-chain within blockhash validity period
				return errors.New("x402 solana: transaction not found within blockhash validity period (likely dropped)")
			}

			// Check transaction status
			err := s.checkTransactionStatus(ctx, signature, commitment)
			if err == nil {
				// Transaction confirmed!
				return nil
			}

			// Check if error is "not found" (still pending) or actual failure
			if isTransactionNotFoundError(err) {
				// Still pending, continue polling
				continue
			}

			// Actual error (transaction failed)
			return err
		}
	}
}

// checkTransactionStatus checks if a transaction is confirmed via RPC.
func (s *SolanaVerifier) checkTransactionStatus(ctx context.Context, signature solana.Signature, commitment rpc.CommitmentType) error {
	getStatusStart := time.Now()
	result, err := s.rpcClient.GetSignatureStatuses(ctx, true, signature)
	if s.metrics != nil {
		s.metrics.ObserveRPCCall("GetSignatureStatuses", s.network, time.Since(getStatusStart), err)
	}
	if err != nil {
		return fmt.Errorf("x402 solana: get signature status: %w", err)
	}

	if result == nil || result.Value == nil || len(result.Value) == 0 {
		return errors.New("x402 solana: transaction not found")
	}

	status := result.Value[0]
	if status == nil {
		return errors.New("x402 solana: transaction not found")
	}

	// Check if transaction has enough confirmations
	confirmedStatus := status.ConfirmationStatus
	if confirmedStatus == "" {
		return errors.New("x402 solana: transaction not confirmed yet")
	}

	switch commitment {
	case rpc.CommitmentFinalized:
		if confirmedStatus != rpc.ConfirmationStatusFinalized {
			return errors.New("x402 solana: transaction not finalized yet")
		}
	case rpc.CommitmentConfirmed:
		if confirmedStatus != rpc.ConfirmationStatusConfirmed && confirmedStatus != rpc.ConfirmationStatusFinalized {
			return errors.New("x402 solana: transaction not confirmed yet")
		}
	case rpc.CommitmentProcessed:
		if confirmedStatus != rpc.ConfirmationStatusProcessed && confirmedStatus != rpc.ConfirmationStatusConfirmed && confirmedStatus != rpc.ConfirmationStatusFinalized {
			return errors.New("x402 solana: transaction not processed yet")
		}
	}

	// Check if transaction succeeded
	if status.Err != nil {
		return fmt.Errorf("x402 solana: transaction error: %v", status.Err)
	}

	return nil
}

// isTransactionNotFoundError checks if error indicates transaction is still pending.
func isTransactionNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "not confirmed yet") || strings.Contains(msg, "not processed yet") || strings.Contains(msg, "not finalized yet")
}
