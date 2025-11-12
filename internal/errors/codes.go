package errors

// ErrorCode represents a machine-readable error identifier for frontend error handling.
type ErrorCode string

// Payment Verification Errors (x402 spec + Solana-specific)
const (
	// Invalid payment proof format or structure
	ErrCodeInvalidPaymentProof ErrorCode = "invalid_payment_proof"
	ErrCodeInvalidSignature    ErrorCode = "invalid_signature"
	ErrCodeInvalidTransaction  ErrorCode = "invalid_transaction"

	// Solana transaction verification failures
	ErrCodeTransactionNotFound     ErrorCode = "transaction_not_found"
	ErrCodeTransactionNotConfirmed ErrorCode = "transaction_not_confirmed"
	ErrCodeTransactionFailed       ErrorCode = "transaction_failed"

	// Recipient/sender validation failures
	ErrCodeInvalidRecipient         ErrorCode = "invalid_recipient"
	ErrCodeInvalidSender            ErrorCode = "invalid_sender"
	ErrCodeUnauthorizedRefundIssuer ErrorCode = "unauthorized_refund_issuer"

	// Amount/token validation failures
	ErrCodeAmountBelowMinimum     ErrorCode = "amount_below_minimum"
	ErrCodeAmountMismatch         ErrorCode = "amount_mismatch"
	ErrCodeInsufficientFunds      ErrorCode = "insufficient_funds_sol"
	ErrCodeInsufficientFundsToken ErrorCode = "insufficient_funds_token"
	ErrCodeInvalidTokenMint       ErrorCode = "invalid_token_mint"

	// SPL transfer validation failures
	ErrCodeNotSPLTransfer      ErrorCode = "not_spl_transfer"
	ErrCodeMissingTokenAccount ErrorCode = "missing_token_account"
	ErrCodeInvalidTokenProgram ErrorCode = "invalid_token_program"

	// Memo/metadata validation failures
	ErrCodeMissingMemo ErrorCode = "missing_memo"
	ErrCodeInvalidMemo ErrorCode = "invalid_memo"

	// Replay protection
	ErrCodePaymentAlreadyUsed ErrorCode = "payment_already_used"
	ErrCodeSignatureReused    ErrorCode = "signature_reused"

	// Timeout/expiration errors
	ErrCodeQuoteExpired       ErrorCode = "quote_expired"
	ErrCodeTransactionExpired ErrorCode = "transaction_expired"
)

// Validation Errors (Request input validation)
const (
	ErrCodeMissingField    ErrorCode = "missing_field"
	ErrCodeInvalidField    ErrorCode = "invalid_field"
	ErrCodeInvalidAmount   ErrorCode = "invalid_amount"
	ErrCodeInvalidWallet   ErrorCode = "invalid_wallet"
	ErrCodeInvalidResource ErrorCode = "invalid_resource"
	ErrCodeInvalidCoupon   ErrorCode = "invalid_coupon"
	ErrCodeInvalidCartItem ErrorCode = "invalid_cart_item"
	ErrCodeEmptyCart       ErrorCode = "empty_cart"
)

// Resource/State Errors (Resource not found or in wrong state)
const (
	ErrCodeResourceNotFound ErrorCode = "resource_not_found"
	ErrCodeCartNotFound     ErrorCode = "cart_not_found"
	ErrCodeRefundNotFound   ErrorCode = "refund_not_found"
	ErrCodeProductNotFound  ErrorCode = "product_not_found"
	ErrCodeCouponNotFound   ErrorCode = "coupon_not_found"
	ErrCodeSessionNotFound  ErrorCode = "session_not_found"

	ErrCodeCartAlreadyPaid        ErrorCode = "cart_already_paid"
	ErrCodeRefundAlreadyProcessed ErrorCode = "refund_already_processed"
)

// Coupon-Specific Errors
const (
	ErrCodeCouponExpired            ErrorCode = "coupon_expired"
	ErrCodeCouponUsageLimitReached  ErrorCode = "coupon_usage_limit_reached"
	ErrCodeCouponNotApplicable      ErrorCode = "coupon_not_applicable"
	ErrCodeCouponWrongPaymentMethod ErrorCode = "coupon_wrong_payment_method"
)

// External Service Errors (Stripe, RPC, etc.)
const (
	ErrCodeStripeError  ErrorCode = "stripe_error"
	ErrCodeRPCError     ErrorCode = "rpc_error"
	ErrCodeNetworkError ErrorCode = "network_error"
)

// Internal/System Errors
const (
	ErrCodeInternalError ErrorCode = "internal_error"
	ErrCodeDatabaseError ErrorCode = "database_error"
	ErrCodeConfigError   ErrorCode = "config_error"
)

// IsRetryable returns whether an error code represents a retryable error.
// Retryable errors are typically transient network/service issues, not validation failures.
func (e ErrorCode) IsRetryable() bool {
	switch e {
	// Network and service errors are retryable
	case ErrCodeRPCError,
		ErrCodeNetworkError,
		ErrCodeStripeError,
		ErrCodeTransactionNotConfirmed:
		return true

	// Validation, authorization, and permanent failures are NOT retryable
	default:
		return false
	}
}

// HTTPStatus returns the appropriate HTTP status code for this error.
func (e ErrorCode) HTTPStatus() int {
	switch e {
	// 400 Bad Request - Client validation errors
	case ErrCodeInvalidPaymentProof,
		ErrCodeInvalidSignature,
		ErrCodeInvalidTransaction,
		ErrCodeMissingField,
		ErrCodeInvalidField,
		ErrCodeInvalidAmount,
		ErrCodeInvalidWallet,
		ErrCodeInvalidResource,
		ErrCodeInvalidCoupon,
		ErrCodeInvalidCartItem,
		ErrCodeEmptyCart,
		ErrCodeInvalidRecipient,
		ErrCodeInvalidSender,
		ErrCodeInvalidTokenMint,
		ErrCodeNotSPLTransfer,
		ErrCodeInvalidTokenProgram,
		ErrCodeMissingMemo,
		ErrCodeInvalidMemo,
		ErrCodeCartAlreadyPaid,
		ErrCodeRefundAlreadyProcessed:
		return 400

	// 402 Payment Required - Payment verification failures
	case ErrCodeTransactionNotFound,
		ErrCodeTransactionNotConfirmed,
		ErrCodeTransactionFailed,
		ErrCodeAmountBelowMinimum,
		ErrCodeAmountMismatch,
		ErrCodeInsufficientFunds,
		ErrCodeInsufficientFundsToken,
		ErrCodeMissingTokenAccount,
		ErrCodePaymentAlreadyUsed,
		ErrCodeSignatureReused,
		ErrCodeQuoteExpired,
		ErrCodeTransactionExpired:
		return 402

	// 403 Forbidden - Authorization failures
	case ErrCodeUnauthorizedRefundIssuer:
		return 403

	// 404 Not Found - Resource not found
	case ErrCodeResourceNotFound,
		ErrCodeCartNotFound,
		ErrCodeRefundNotFound,
		ErrCodeProductNotFound,
		ErrCodeCouponNotFound,
		ErrCodeSessionNotFound:
		return 404

	// 409 Conflict - Coupon validation failures (business rule conflicts)
	case ErrCodeCouponExpired,
		ErrCodeCouponUsageLimitReached,
		ErrCodeCouponNotApplicable,
		ErrCodeCouponWrongPaymentMethod:
		return 409

	// 502 Bad Gateway - External service errors
	case ErrCodeStripeError,
		ErrCodeRPCError,
		ErrCodeNetworkError:
		return 502

	// 500 Internal Server Error - System/internal errors
	default:
		return 500
	}
}
