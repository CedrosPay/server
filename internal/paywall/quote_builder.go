package paywall

import (
	"fmt"
	"strconv"
	"time"
)

// x402QuoteOptions contains the varying parameters for building x402 quotes.
type x402QuoteOptions struct {
	ResourceID            string
	AtomicAmount          uint64 // Amount in atomic units (e.g., lamports, micro-USDC)
	Token                 string
	PayToAddress          string // Wallet address for payTo field
	RecipientTokenAccount string // Actual token account for transaction building
	Description           string
	ExpiresAt             time.Time
	IncludeFeePayer       bool // Whether to include feePayer for gasless transactions
}

// buildX402Quote creates a CryptoQuote with common logic consolidated.
// This eliminates ~120 lines of duplication across cart, refund, and resource quote building.
// IMPORTANT: Pass atomic units directly (Money.Atomic) to avoid float64 precision loss.
func (s *Service) buildX402Quote(opts x402QuoteOptions) (*CryptoQuote, error) {
	atomicAmount := opts.AtomicAmount

	// Build extra field with Solana-specific metadata
	extra := map[string]any{
		"recipientTokenAccount": opts.RecipientTokenAccount,
		"decimals":              s.cfg.X402.TokenDecimals,
		"tokenSymbol":           opts.Token,
		"memo":                  fmt.Sprintf("%s:%s", s.cfg.X402.MemoPrefix, opts.ResourceID),
	}

	// Add feePayer for gasless transactions (if requested)
	if opts.IncludeFeePayer {
		if feePayerPubKey := s.getFeePayerPublicKey(); feePayerPubKey != "" {
			extra["feePayer"] = feePayerPubKey
		}
	}

	// Build crypto quote
	return &CryptoQuote{
		Scheme:            "solana-spl-transfer",
		Network:           s.cfg.X402.Network,
		MaxAmountRequired: strconv.FormatUint(atomicAmount, 10),
		Resource:          opts.ResourceID,
		Description:       opts.Description,
		MimeType:          "application/json",
		PayTo:             opts.PayToAddress,
		MaxTimeoutSeconds: int(time.Until(opts.ExpiresAt).Seconds()),
		Asset:             s.cfg.X402.TokenMint,
		Extra:             extra,
	}, nil
}
