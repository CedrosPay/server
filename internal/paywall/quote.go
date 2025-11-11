package paywall

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CedrosPay/server/internal/coupons"
	"github.com/CedrosPay/server/internal/money"
)

// GenerateQuote builds a paywall quote for the resource with optional coupon.
func (s *Service) GenerateQuote(ctx context.Context, resourceID, couponCode string) (Quote, error) {
	resource, err := s.ResourceDefinition(ctx, resourceID)
	if err != nil {
		return Quote{}, err
	}

	generatedAt := time.Now()
	expiry := generatedAt.Add(s.cfg.Paywall.QuoteTTL.Duration)
	memo := InterpolateMemo(resource.MemoTemplate, resourceID)

	// Validate and apply manually provided coupon if specified
	// Note: We silently ignore invalid coupons (don't error out)
	// This matches the Stripe behavior - invalid coupons are just not applied
	manualCoupon := s.validateManualCoupon(ctx, couponCode, resourceID, "")

	quote := Quote{
		ResourceID: resourceID,
		ExpiresAt:  expiry,
	}

	if resource.FiatAmountCents > 0 || resource.StripePriceID != "" {
		// Get all applicable coupons for Stripe payment (auto-apply + manual)
		// For Stripe, we apply all coupons (catalog + checkout) since Stripe checkout is single-step
		stripeCoupons := SelectCouponsForPayment(ctx, s.coupons, resourceID, coupons.PaymentMethodStripe, manualCoupon, ScopeAll)

		quote.Stripe = &StripeOption{
			PriceID:     resource.StripePriceID,
			AmountCents: resource.FiatAmountCents,
			Currency:    strings.ToLower(resource.FiatCurrency),
			Description: resource.Description,
			Metadata:    cloneMap(resource.Metadata),
		}

		// Apply stacked coupons if available
		if len(stripeCoupons) > 0 {
			quote.Stripe.AmountCents = stackFiatCoupons(resource.FiatAmountCents, stripeCoupons)
		}
	}

	if resource.CryptoAtomicAmount > 0 {
		// IMPORTANT: For single product quotes, apply ALL coupons (catalog + checkout)
		// Since there's no separate cart step, the single product IS the cart
		// This ensures users see the full discounted price immediately
		catalogCoupons := SelectCouponsForPayment(ctx, s.coupons, resourceID, coupons.PaymentMethodX402, manualCoupon, ScopeCatalog)
		checkoutCoupons := SelectCouponsForPayment(ctx, s.coupons, "", coupons.PaymentMethodX402, nil, ScopeCheckout)

		// Combine catalog + checkout coupons for single product quotes
		allApplicableCoupons := append([]coupons.Coupon{}, catalogCoupons...)
		allApplicableCoupons = append(allApplicableCoupons, checkoutCoupons...)

		// Use atomic amount directly (Money type)
		cryptoAsset, err := money.GetAsset(resource.CryptoToken)
		if err != nil {
			return Quote{}, fmt.Errorf("get crypto asset: %w", err)
		}
		cryptoMoney := money.Money{Asset: cryptoAsset, Atomic: resource.CryptoAtomicAmount}

		// Apply stacked coupons using precise Money arithmetic (catalog first, then checkout)
		if len(allApplicableCoupons) > 0 {
			cryptoMoney, err = StackCouponsOnMoney(cryptoMoney, allApplicableCoupons)
			if err != nil {
				return Quote{}, fmt.Errorf("apply coupons to crypto price: %w", err)
			}
		}

		// IMPORTANT: Round to cents precision (2 decimals) using precise integer arithmetic
		// This ensures $2.7661 becomes $2.77, not $2.7661
		cryptoMoney = cryptoMoney.RoundUpToCents()

		// Use atomic units directly from Money type (no float64 conversion needed)
		atomicAmount := uint64(cryptoMoney.Atomic)

		// Convert to float64 only for display/logging purposes (external API boundary)
		cryptoAmount, _ := strconv.ParseFloat(cryptoMoney.ToMajor(), 64)

		// Use custom crypto account if specified, otherwise use configured payment address
		// Note: We send the wallet address in payTo for better developer experience.
		// The actual token account is used in extra.recipientTokenAccount for transaction building.
		payToAddress := resource.CryptoAccount
		if payToAddress == "" {
			payToAddress = s.cfg.X402.PaymentAddress
		}

		// For extra.recipientTokenAccount, derive the token account (needed for transaction building)
		recipientTokenAccount := resource.CryptoAccount
		if recipientTokenAccount == "" {
			recipientTokenAccount = deriveTokenAccountSafe(s.cfg.X402.PaymentAddress, s.cfg.X402.TokenMint)
		}

		// Build extra field with Solana-specific metadata
		extra := map[string]any{
			"recipientTokenAccount": recipientTokenAccount, // Token account for transaction building
			"decimals":              s.cfg.X402.TokenDecimals,
			"tokenSymbol":           resource.CryptoToken,
			"memo":                  memo,
		}

		// Add feePayer field when gasless is enabled
		// Frontend can detect this and send a partially signed transaction
		// The server will co-sign and submit it automatically
		if feePayerPubKey := s.getFeePayerPublicKey(); feePayerPubKey != "" {
			extra["feePayer"] = feePayerPubKey
		}

		// IMPORTANT: Add coupon metadata to extra so frontend knows original price
		// Without this, frontend has no way to display discount information
		if len(allApplicableCoupons) > 0 {
			// Build code maps for O(1) lookup
			catalogSet := make(map[string]bool, len(catalogCoupons))
			for _, c := range catalogCoupons {
				catalogSet[c.Code] = true
			}

			checkoutSet := make(map[string]bool, len(checkoutCoupons))
			for _, c := range checkoutCoupons {
				checkoutSet[c.Code] = true
			}

			// Single pass: collect all codes and categorize
			allCodes := make([]string, 0, len(allApplicableCoupons))
			catalogCodes := make([]string, 0, len(catalogCoupons))
			checkoutCodes := make([]string, 0, len(checkoutCoupons))

			for _, c := range allApplicableCoupons {
				allCodes = append(allCodes, c.Code)
				if catalogSet[c.Code] {
					catalogCodes = append(catalogCodes, c.Code)
				} else if checkoutSet[c.Code] {
					checkoutCodes = append(checkoutCodes, c.Code)
				}
			}

			// Add pricing metadata
			extra["original_amount"] = money.Money{Asset: cryptoAsset, Atomic: resource.CryptoAtomicAmount}.ToMajor()
			extra["discounted_amount"] = fmt.Sprintf("%.6f", cryptoAmount)
			extra["applied_coupons"] = formatCouponCodes(allCodes)

			// Separate catalog vs checkout for frontend display
			if catalogCodesStr := formatCouponCodes(catalogCodes); catalogCodesStr != "" {
				extra["catalog_coupons"] = catalogCodesStr
			}
			if checkoutCodesStr := formatCouponCodes(checkoutCodes); checkoutCodesStr != "" {
				extra["checkout_coupons"] = checkoutCodesStr
			}
		}

		quote.Crypto = &CryptoQuote{
			Scheme:            "solana-spl-transfer",
			Network:           s.cfg.X402.Network,
			MaxAmountRequired: strconv.FormatUint(atomicAmount, 10),
			Resource:          resourceID,
			Description:       resource.Description,
			MimeType:          "application/json",
			PayTo:             payToAddress, // Wallet address for developer readability
			MaxTimeoutSeconds: int(s.cfg.Paywall.QuoteTTL.Duration.Seconds()),
			Asset:             s.cfg.X402.TokenMint,
			Extra:             extra, // Contains recipientTokenAccount for transaction building
		}
	}
	return quote, nil
}

// CouponScope defines the scope of coupon selection for different payment phases.
type CouponScope int

const (
	// ScopeAll selects all coupons regardless of AppliesAt (used for Stripe payments).
	ScopeAll CouponScope = iota
	// ScopeCatalog selects only catalog-level coupons (product-specific).
	ScopeCatalog
	// ScopeCheckout selects only checkout-level coupons (site-wide).
	ScopeCheckout
)

// selectCouponsWithFilter is a generic coupon selector that applies a filter predicate.
// This consolidates common logic across all coupon selection functions.
func selectCouponsWithFilter(
	ctx context.Context,
	couponRepo coupons.Repository,
	productID string,
	paymentMethod coupons.PaymentMethod,
	manualCoupon *coupons.Coupon,
	filter func(coupons.Coupon) bool,
	manualCouponFilter func(*coupons.Coupon) bool,
) []coupons.Coupon {
	if couponRepo == nil {
		return nil
	}

	var result []coupons.Coupon
	seenCodes := make(map[string]bool) // O(1) duplicate detection

	// Get all auto-apply coupons for this payment method
	autoApplyCoupons, err := couponRepo.GetAutoApplyCouponsForPayment(ctx, productID, paymentMethod)
	if err == nil && len(autoApplyCoupons) > 0 {
		// Apply filter to auto-apply coupons and track seen codes
		for _, c := range autoApplyCoupons {
			if filter == nil || filter(c) {
				result = append(result, c)
				seenCodes[c.Code] = true
			}
		}
	}

	// Add manual coupon if provided, applies to payment method, passes filter, and not duplicate
	if manualCoupon != nil && manualCoupon.AppliesToPaymentMethod(paymentMethod) {
		if manualCouponFilter == nil || manualCouponFilter(manualCoupon) {
			// O(1) duplicate check using map
			if !seenCodes[manualCoupon.Code] {
				result = append(result, *manualCoupon)
			}
		}
	}

	return result
}

// SelectCouponsForPayment is a unified coupon selector supporting all scopes.
// Handles catalog (product-specific), checkout (site-wide), and all (Stripe) coupon selection.
func SelectCouponsForPayment(
	ctx context.Context,
	couponRepo coupons.Repository,
	productID string,
	paymentMethod coupons.PaymentMethod,
	manualCoupon *coupons.Coupon,
	scope CouponScope,
) []coupons.Coupon {
	switch scope {
	case ScopeAll:
		// No filter - accept all coupons
		return selectCouponsWithFilter(ctx, couponRepo, productID, paymentMethod, manualCoupon, nil, nil)

	case ScopeCatalog:
		// Filter to only catalog-level coupons (product-specific)
		return selectCouponsWithFilter(
			ctx, couponRepo, productID, paymentMethod, manualCoupon,
			func(c coupons.Coupon) bool {
				return c.AppliesAt == coupons.AppliesAtCatalog
			},
			nil, // No additional filter for manual coupon
		)

	case ScopeCheckout:
		// Filter to only checkout-level coupons (site-wide)
		// Pass empty productID to match site-wide coupons
		return selectCouponsWithFilter(
			ctx, couponRepo, "", paymentMethod, manualCoupon,
			func(c coupons.Coupon) bool {
				return c.AppliesAt == coupons.AppliesAtCheckout && c.Scope == coupons.ScopeAll
			},
			func(c *coupons.Coupon) bool {
				// Only allow site-wide manual coupons at checkout (reject product-specific)
				return c.Scope == coupons.ScopeAll
			},
		)

	default:
		return nil
	}
}

// stackFiatCoupons applies stacked coupons to a fiat price (in cents).
// Fixed discounts are assumed to be in dollars and converted to cents.
// Returns the final price in cents using precise Money arithmetic.
func stackFiatCoupons(originalPriceCents int64, applicableCoupons []coupons.Coupon) int64 {
	if len(applicableCoupons) == 0 {
		return originalPriceCents
	}

	// Stripe payments are always in USD (fiat)
	// The currency field is optional and ignored - all fiat is USD
	asset, err := money.GetAsset("USD")
	if err != nil {
		// If asset not found, return original price (fail safe)
		return originalPriceCents
	}

	priceMoney := money.New(asset, originalPriceCents)

	// Apply coupons using Money arithmetic
	discounted, err := StackCouponsOnMoney(priceMoney, applicableCoupons)
	if err != nil {
		// On error, return original price (fail safe)
		return originalPriceCents
	}

	return discounted.Atomic
}

// InterpolateMemo wraps the standalone InterpolateMemo function.
// Exposed as a method on Service for use by HTTP handlers.
func (s *Service) InterpolateMemo(template, resourceID string) string {
	return InterpolateMemo(template, resourceID)
}
