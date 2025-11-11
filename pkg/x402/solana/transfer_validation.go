package solana

import (
	"errors"
	"fmt"
	"math"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"

	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/pkg/x402"
)

// newVerificationError is a helper to create verification errors.
func newVerificationError(code apierrors.ErrorCode, err error) x402.VerificationError {
	return x402.NewVerificationError(code, err)
}

// resolveTokenAccount derives the associated token account for a given requirement.
func resolveTokenAccount(requirement x402.Requirement) (solana.PublicKey, error) {
	if requirement.RecipientTokenAccount != "" {
		pk, err := solana.PublicKeyFromBase58(requirement.RecipientTokenAccount)
		if err != nil {
			return solana.PublicKey{}, newVerificationError(apierrors.ErrCodeInvalidRecipient, err)
		}
		return pk, nil
	}
	owner, err := solana.PublicKeyFromBase58(requirement.RecipientOwner)
	if err != nil {
		return solana.PublicKey{}, newVerificationError(apierrors.ErrCodeInvalidRecipient, err)
	}
	mint, err := solana.PublicKeyFromBase58(requirement.TokenMint)
	if err != nil {
		return solana.PublicKey{}, newVerificationError(apierrors.ErrCodeInvalidTokenMint, err)
	}
	account, _, err := solana.FindAssociatedTokenAddress(owner, mint)
	if err != nil {
		return solana.PublicKey{}, newVerificationError(apierrors.ErrCodeInternalError, err)
	}
	return account, nil
}

// validateTransferInstructionAndExtractAuthority checks that a transaction contains a valid token transfer
// and returns both the amount and the transfer authority (user wallet).
func validateTransferInstructionAndExtractAuthority(tx *solana.Transaction, requirement x402.Requirement) (float64, solana.PublicKey, error) {
	expectedAccount, err := resolveTokenAccount(requirement)
	if err != nil {
		return 0, solana.PublicKey{}, err
	}
	mintKey, err := solana.PublicKeyFromBase58(requirement.TokenMint)
	if err != nil {
		return 0, solana.PublicKey{}, fmt.Errorf("x402 solana: invalid token mint: %w", err)
	}

	for _, inst := range tx.Message.Instructions {
		if int(inst.ProgramIDIndex) >= len(tx.Message.AccountKeys) {
			continue
		}
		programID := tx.Message.AccountKeys[inst.ProgramIDIndex]
		if !programID.Equals(solana.TokenProgramID) {
			continue
		}
		accounts, err := inst.ResolveInstructionAccounts(&tx.Message)
		if err != nil {
			return 0, solana.PublicKey{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, err)
		}
		decoded, err := token.DecodeInstruction(accounts, []byte(inst.Data))
		if err != nil {
			return 0, solana.PublicKey{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, err)
		}
		switch ins := decoded.Impl.(type) {
		case *token.Transfer:
			dest := ins.GetDestinationAccount().PublicKey
			if !dest.Equals(expectedAccount) {
				continue
			}
			owner := ins.GetOwnerAccount().PublicKey
			if ins.Amount == nil {
				return 0, solana.PublicKey{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, errors.New("transfer instruction missing amount"))
			}
			amount := float64(*ins.Amount) / math.Pow10(int(requirement.TokenDecimals))
			return amount, owner, nil
		case *token.TransferChecked:
			dest := ins.GetDestinationAccount().PublicKey
			if !dest.Equals(expectedAccount) {
				continue
			}
			if acct := ins.GetMintAccount().PublicKey; !acct.Equals(mintKey) {
				continue
			}
			owner := ins.GetOwnerAccount().PublicKey
			if ins.Decimals == nil || *ins.Decimals != requirement.TokenDecimals {
				return 0, solana.PublicKey{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, fmt.Errorf("transfer decimals mismatch %v != %d", ins.Decimals, requirement.TokenDecimals))
			}
			if ins.Amount == nil {
				return 0, solana.PublicKey{}, newVerificationError(apierrors.ErrCodeInvalidTransaction, errors.New("transferChecked amount missing"))
			}
			amount := float64(*ins.Amount) / math.Pow10(int(requirement.TokenDecimals))
			return amount, owner, nil
		default:
			continue
		}
	}

	return 0, solana.PublicKey{}, newVerificationError(apierrors.ErrCodeNotSPLTransfer, fmt.Errorf("token transfer to %s not found in transaction", expectedAccount.String()))
}

// extractTokenTransfer extracts the transfer amount from a parsed transaction.
func extractTokenTransfer(tx *rpc.GetParsedTransactionResult, destination solana.PublicKey, mint solana.PublicKey, decimals uint8, minAmount float64) (float64, error) {
	if tx.Transaction == nil || tx.Meta == nil {
		return 0, errors.New("x402 solana: parsed transaction incomplete")
	}

	if amount, ok := scanParsedInstructions(tx.Transaction.Message.Instructions, tx.Meta, &tx.Transaction.Message, destination, mint, decimals, minAmount); ok {
		return amount, nil
	}
	for _, inner := range tx.Meta.InnerInstructions {
		if amount, ok := scanParsedInstructions(inner.Instructions, tx.Meta, &tx.Transaction.Message, destination, mint, decimals, minAmount); ok {
			return amount, nil
		}
	}
	return 0, fmt.Errorf("x402 solana: no transfer to %s found", destination.String())
}

// scanParsedInstructions scans instruction list for a matching transfer.
func scanParsedInstructions(instructions []*rpc.ParsedInstruction, meta *rpc.ParsedTransactionMeta, message *rpc.ParsedMessage, destination solana.PublicKey, mint solana.PublicKey, decimals uint8, minAmount float64) (float64, bool) {
	for _, inst := range instructions {
		amount, ok := parseTokenTransfer(inst, meta, message, destination, mint, decimals, minAmount)
		if ok {
			return amount, true
		}
	}
	return 0, false
}

// parseTokenTransfer parses a single parsed instruction for a token transfer.
func parseTokenTransfer(inst *rpc.ParsedInstruction, meta *rpc.ParsedTransactionMeta, message *rpc.ParsedMessage, destination solana.PublicKey, mint solana.PublicKey, decimals uint8, minAmount float64) (float64, bool) {
	if inst == nil || inst.Parsed == nil {
		return 0, false
	}
	if inst.Program != "spl-token" {
		return 0, false
	}

	info, instructionType, err := extractInstructionInfo(inst)
	if err != nil {
		return 0, false
	}
	if instructionType != "transfer" && instructionType != "transferChecked" {
		return 0, false
	}

	destStr := stringValue(info["destination"])
	if destStr == "" {
		return 0, false
	}
	destKey, err := solana.PublicKeyFromBase58(destStr)
	if err != nil || !destKey.Equals(destination) {
		return 0, false
	}

	if !postBalanceMatches(meta, message, destination, mint) {
		return 0, false
	}

	amount, err := parseAmount(info, decimals)
	if err != nil {
		return 0, false
	}
	if amount+x402.AmountTolerance < minAmount {
		return 0, false
	}

	mintHint := stringValue(info["mint"])
	if mintHint != "" {
		hintKey, err := solana.PublicKeyFromBase58(mintHint)
		if err != nil || !hintKey.Equals(mint) {
			return 0, false
		}
	}

	return amount, true
}

// postBalanceMatches checks if the destination account has a post-balance for the expected mint.
func postBalanceMatches(meta *rpc.ParsedTransactionMeta, message *rpc.ParsedMessage, destination solana.PublicKey, mint solana.PublicKey) bool {
	if meta == nil || message == nil {
		return false
	}
	for _, balance := range meta.PostTokenBalances {
		idx := int(balance.AccountIndex)
		if idx >= len(message.AccountKeys) {
			continue
		}
		account := message.AccountKeys[idx].PublicKey
		if account.Equals(destination) && balance.Mint.Equals(mint) {
			return true
		}
	}
	return false
}

// findParsedPayer extracts the first signer from a parsed transaction.
func findParsedPayer(tx *rpc.ParsedTransaction) string {
	if tx == nil {
		return ""
	}
	for _, account := range tx.Message.AccountKeys {
		if account.Signer {
			return account.PublicKey.String()
		}
	}
	return ""
}
