package solana

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/programs/memo"
	"github.com/gagliardetto/solana-go/programs/token"
)

// GaslessTxRequest contains the parameters needed to build a gasless transaction.
type GaslessTxRequest struct {
	PayerWallet           solana.PublicKey  // User's wallet (signs transfer, not fees)
	FeePayer              *solana.PublicKey // Optional: specific server wallet to use as fee payer
	RecipientTokenAccount solana.PublicKey  // Destination token account
	TokenMint             solana.PublicKey  // Token mint address (e.g., USDC)
	Amount                uint64            // Amount in atomic units (e.g., lamports for SOL, smallest unit for SPL tokens)
	Decimals              uint8             // Token decimals (e.g., 6 for USDC)
	Memo                  string            // Payment memo
	ComputeUnitLimit      uint32            // Maximum compute units (e.g., 200000)
	ComputeUnitPrice      uint64            // Priority fee in microlamports (e.g., 1)
	Blockhash             solana.Hash       // Recent blockhash (should be from cache)
}

// GaslessTxResponse contains the unsigned transaction to be partially signed by the user.
type GaslessTxResponse struct {
	Transaction string `json:"transaction"` // Base64-encoded unsigned transaction
	Blockhash   string `json:"blockhash"`   // Recent blockhash used
	FeePayer    string `json:"feePayer"`    // Server wallet that will pay fees
}

// BuildGaslessTransaction constructs a complete transaction for gasless payments.
// The transaction includes:
// 1. Compute budget instructions (unit limit + priority fee)
// 2. SPL token transfer instruction
// 3. Memo instruction
//
// The transaction is NOT signed. The frontend should:
// 1. Deserialize the transaction
// 2. Have the user sign it (partial signature - transfer authority only)
// 3. Send the partially signed transaction back to the backend
// 4. Backend co-signs as fee payer and submits
func (s *SolanaVerifier) BuildGaslessTransaction(ctx context.Context, req GaslessTxRequest) (GaslessTxResponse, error) {
	if !s.gaslessEnabled {
		return GaslessTxResponse{}, errors.New("gasless transactions not enabled")
	}

	// Get server wallet to act as fee payer
	var wallet *solana.PrivateKey
	if req.FeePayer != nil {
		// Use specific fee payer if provided
		wallet = s.findWalletByPublicKey(*req.FeePayer)
		if wallet == nil {
			return GaslessTxResponse{}, fmt.Errorf("specified fee payer not found in server wallets: %s", req.FeePayer.String())
		}
	} else {
		// Round-robin if not specified
		wallet = s.getNextWallet()
		if wallet == nil {
			return GaslessTxResponse{}, errors.New("no server wallets configured for gasless")
		}
	}

	// Use provided blockhash (should be from cache)
	// Caller should fetch from /recent-blockhash endpoint to benefit from caching
	blockhash := req.Blockhash

	// Derive the user's token account (source)
	fromTokenAccount, _, err := solana.FindAssociatedTokenAddress(req.PayerWallet, req.TokenMint)
	if err != nil {
		return GaslessTxResponse{}, fmt.Errorf("derive user token account: %w", err)
	}

	// Build instructions in order:
	// 1. Compute unit limit
	// 2. Compute unit price (priority fee)
	// 3. SPL token transfer
	// 4. Memo

	instructions := make([]solana.Instruction, 0, 4)

	// 1. Set compute unit limit
	if req.ComputeUnitLimit > 0 {
		instructions = append(instructions,
			computebudget.NewSetComputeUnitLimitInstruction(req.ComputeUnitLimit).Build(),
		)
	}

	// 2. Set compute unit price (priority fee)
	if req.ComputeUnitPrice > 0 {
		instructions = append(instructions,
			computebudget.NewSetComputeUnitPriceInstruction(req.ComputeUnitPrice).Build(),
		)
	}

	// 3. SPL token transfer (TransferChecked for safety)
	instructions = append(instructions,
		token.NewTransferCheckedInstruction(
			req.Amount,
			req.Decimals,
			fromTokenAccount,
			req.TokenMint,
			req.RecipientTokenAccount,
			req.PayerWallet,      // User is the transfer authority
			[]solana.PublicKey{}, // No multisig
		).Build(),
	)

	// 4. Memo
	if req.Memo != "" {
		instructions = append(instructions,
			memo.NewMemoInstruction(
				[]byte(req.Memo),
				req.PayerWallet, // Signer
			).Build(),
		)
	}

	// Build transaction with server wallet as fee payer
	tx, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(wallet.PublicKey()),
	)
	if err != nil {
		return GaslessTxResponse{}, fmt.Errorf("build transaction: %w", err)
	}

	// Serialize the UNSIGNED transaction
	txBytes, err := tx.MarshalBinary()
	if err != nil {
		return GaslessTxResponse{}, fmt.Errorf("serialize transaction: %w", err)
	}

	return GaslessTxResponse{
		Transaction: base64.StdEncoding.EncodeToString(txBytes),
		Blockhash:   blockhash.String(),
		FeePayer:    wallet.PublicKey().String(),
	}, nil
}
