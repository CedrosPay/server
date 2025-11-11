package x402

import (
	"fmt"
	"strings"

	"github.com/CedrosPay/server/internal/errors"
)

// VerificationError classifies failures encountered during transaction validation.
type VerificationError struct {
	Code    errors.ErrorCode // Machine-readable error code
	Message string           // User-friendly message
	Err     error            // Technical error for logging
}

func (e VerificationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err == nil {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e VerificationError) Unwrap() error {
	return e.Err
}

// NewVerificationError creates a new verification error with a user-friendly message.
func NewVerificationError(code errors.ErrorCode, err error) VerificationError {
	return VerificationError{
		Code:    code,
		Message: GetUserFriendlyMessage(code, err),
		Err:     err,
	}
}

// GetUserFriendlyMessage converts error codes to user-friendly messages.
func GetUserFriendlyMessage(code errors.ErrorCode, err error) string {
	switch code {
	case errors.ErrCodeInsufficientFundsToken:
		return "Insufficient token balance. Please add more tokens to your wallet and try again."
	case errors.ErrCodeInsufficientFunds:
		return "Insufficient SOL for transaction fees. Please add some SOL to your wallet and try again."
	case "server_insufficient_funds":
		return "Service temporarily unavailable due to insufficient server funds. Please try again later or contact support."
	case errors.ErrCodeAmountBelowMinimum:
		return "Payment amount is less than required. Please check the payment amount and try again."
	case errors.ErrCodeInvalidSignature:
		return "Invalid transaction signature. Please try again."
	case errors.ErrCodeInvalidMemo:
		return "Invalid payment memo. Please use the payment details provided by the quote."
	case errors.ErrCodeInvalidTokenMint:
		return "Wrong token used for payment. Please use the correct token specified in the quote."
	case errors.ErrCodeInvalidRecipient:
		return "Payment sent to wrong address. Please check the recipient address and try again."
	case errors.ErrCodeMissingTokenAccount:
		return "Token account not found. Please create a token account for this token first."
	case "send_failed":
		// Check for specific Solana errors
		if err != nil {
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "insufficient funds") || strings.Contains(errMsg, "insufficient lamports") {
				if strings.Contains(errMsg, "custom program error: 0x1") {
					return "Insufficient token balance. Please add more tokens to your wallet and try again."
				}
				return "Insufficient SOL for transaction fees. Please add some SOL to your wallet and try again."
			}
			if strings.Contains(errMsg, "account not found") || strings.Contains(errMsg, "could not find account") {
				return "Token account not found. Please create a token account for this token first."
			}
		}
		return "Transaction failed to send. Please check your wallet balance and try again."
	case "send_failed_after_account_creation":
		return "Transaction failed after creating token account. Please try again."
	case errors.ErrCodeTransactionNotFound:
		return "Transaction not found on the blockchain. It may have been dropped. Please try again."
	case errors.ErrCodeTransactionExpired:
		return "Transaction timed out. Please check the blockchain explorer and try again if needed."
	case errors.ErrCodeTransactionFailed:
		// Check for specific Solana errors in the underlying error
		if err != nil {
			errMsg := strings.ToLower(err.Error())
			// Token insufficient funds
			if strings.Contains(errMsg, "custom program error: 0x1") ||
				(strings.Contains(errMsg, "insufficient") && !strings.Contains(errMsg, "lamports")) {
				return "Insufficient token balance. Please add more tokens to your wallet and try again."
			}
			// SOL insufficient funds (for fees)
			if strings.Contains(errMsg, "insufficient lamports") {
				return "Insufficient SOL for transaction fees. Please add some SOL to your wallet and try again."
			}
			// Account not found
			if strings.Contains(errMsg, "account not found") || strings.Contains(errMsg, "could not find account") {
				return "Token account not found. Please create a token account for this token first."
			}
		}
		return "Transaction failed on the blockchain. Check your wallet for details. You may need to adjust your transaction settings or add more SOL for fees."
	case errors.ErrCodePaymentAlreadyUsed:
		return "This payment has already been processed. Each payment can only be used once."
	case errors.ErrCodeAmountMismatch:
		return "Payment amount does not match the required amount. Please pay the exact amount shown."
	default:
		return fmt.Sprintf("Payment verification failed: %s", code)
	}
}
