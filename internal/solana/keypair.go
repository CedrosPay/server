package solana

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/rpcutil"
	"github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
)

// ParsePrivateKey parses a Solana private key from either base58 or JSON array format.
// Supported formats:
//   - Base58: "5Kd7..." (standard format from solana-keygen)
//   - JSON array: "[1,2,3,...,64]" (64 bytes, Phantom wallet export format)
func ParsePrivateKey(keyStr string) (solana.PrivateKey, error) {
	if keyStr == "" {
		return solana.PrivateKey{}, fmt.Errorf("private key string is empty")
	}

	// Trim whitespace
	keyStr = strings.TrimSpace(keyStr)

	// Try base58 format first (most common)
	if !strings.HasPrefix(keyStr, "[") {
		privateKey, err := solana.PrivateKeyFromBase58(keyStr)
		if err != nil {
			return solana.PrivateKey{}, fmt.Errorf("invalid base58 private key: %w", err)
		}
		return privateKey, nil
	}

	// Fall back to JSON array format
	return parsePrivateKeyArray(keyStr)
}

// parsePrivateKeyArray parses a private key from JSON array format: [1,2,3,...,64]
func parsePrivateKeyArray(keyStr string) (solana.PrivateKey, error) {
	// Validate JSON array format
	if !strings.HasPrefix(keyStr, "[") || !strings.HasSuffix(keyStr, "]") {
		return solana.PrivateKey{}, fmt.Errorf("private key array must be in JSON format: [1,2,3,...]")
	}

	// Remove brackets and split by comma
	arrayContent := keyStr[1 : len(keyStr)-1]
	parts := strings.Split(arrayContent, ",")

	if len(parts) != 64 {
		return solana.PrivateKey{}, fmt.Errorf("private key must be a 64-byte array, got %d bytes", len(parts))
	}

	// Convert string numbers to bytes
	var keyBytes [64]byte
	for i, part := range parts {
		part = strings.TrimSpace(part)
		val, err := strconv.Atoi(part)
		if err != nil {
			return solana.PrivateKey{}, fmt.Errorf("invalid byte value at position %d: %s (%w)", i, part, err)
		}
		if val < 0 || val > 255 {
			return solana.PrivateKey{}, fmt.Errorf("byte value at position %d out of range (0-255): %d", i, val)
		}
		keyBytes[i] = byte(val)
	}

	privateKey := solana.PrivateKey(keyBytes[:])
	return privateKey, nil
}

// CreateAssociatedTokenAccount creates an associated token account for the given owner and mint.
// This is useful when a merchant's wallet doesn't have a token account initialized yet.
// It waits for the transaction to be confirmed before returning.
func CreateAssociatedTokenAccount(ctx context.Context, rpcClient *rpc.Client, wsClient *ws.Client, payer solana.PrivateKey, owner solana.PublicKey, mint solana.PublicKey) (solana.PublicKey, error) {
	// Derive the associated token account address
	ata, _, err := solana.FindAssociatedTokenAddress(owner, mint)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("derive ATA: %w", err)
	}

	log := logger.FromContext(ctx)
	log.Info().
		Str("ata", logger.TruncateAddress(ata.String())).
		Str("owner", logger.TruncateAddress(owner.String())).
		Str("mint", logger.TruncateAddress(mint.String())).
		Msg("token_account.creating")

	// Get latest blockhash with retry logic
	latestBlockhash, err := rpcutil.WithRetry(ctx, func() (*rpc.GetLatestBlockhashResult, error) {
		return rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	})
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("get latest blockhash: %w", err)
	}

	// Build create ATA instruction
	createATAInstruction := associatedtokenaccount.NewCreateInstruction(
		payer.PublicKey(),
		owner,
		mint,
	).Build()

	// Build transaction
	tx, err := solana.NewTransaction(
		[]solana.Instruction{createATAInstruction},
		latestBlockhash.Value.Blockhash,
		solana.TransactionPayer(payer.PublicKey()),
	)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("create transaction: %w", err)
	}

	// Sign transaction
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(payer.PublicKey()) {
			return &payer
		}
		return nil
	})
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("sign transaction: %w", err)
	}

	// Send transaction
	sig, err := rpcClient.SendTransaction(ctx, tx)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("send transaction: %w", err)
	}

	// Wait for confirmation before returning
	// This is critical - the user's payment transaction will fail if the account isn't confirmed yet
	log.Info().
		Str("signature", logger.TruncateAddress(sig.String())).
		Msg("token_account.waiting_for_confirmation")

	// Subscribe to transaction confirmation via WebSocket
	sub, err := wsClient.SignatureSubscribe(sig, rpc.CommitmentConfirmed)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("subscribe to confirmation: %w", err)
	}
	defer sub.Unsubscribe()

	// Wait for confirmation with timeout
	confirmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for {
		select {
		case <-confirmCtx.Done():
			return solana.PublicKey{}, fmt.Errorf("confirmation timeout: %w", confirmCtx.Err())
		case result := <-sub.Response():
			if result.Value.Err != nil {
				return solana.PublicKey{}, fmt.Errorf("transaction failed: %v", result.Value.Err)
			}
			// Transaction confirmed successfully
			goto confirmed
		case err := <-sub.Err():
			return solana.PublicKey{}, fmt.Errorf("subscription error: %w", err)
		}
	}

confirmed:

	log.Info().
		Str("ata", logger.TruncateAddress(ata.String())).
		Str("signature", logger.TruncateAddress(sig.String())).
		Msg("token_account.created")

	return ata, nil
}
