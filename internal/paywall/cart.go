package paywall

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CedrosPay/server/internal/callbacks"
	"github.com/CedrosPay/server/internal/coupons"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/internal/money"
	"github.com/CedrosPay/server/internal/storage"
	"github.com/CedrosPay/server/pkg/x402"
)

// CartQuoteRequest represents a request to generate a quote for multiple items.
type CartQuoteRequest struct {
	Items      []CartQuoteItem   `json:"items"`
	Metadata   map[string]string `json:"metadata,omitempty"`   // Cart-level metadata (user_id, campaign, etc.)
	CouponCode string            `json:"couponCode,omitempty"` // Optional coupon code to apply discount
}

// CartQuoteItem represents a single item in a cart quote request.
type CartQuoteItem struct {
	ResourceID string            `json:"resource"`           // Resource ID from paywall config
	Quantity   int64             `json:"quantity"`           // Number of this item
	Metadata   map[string]string `json:"metadata,omitempty"` // Per-item custom metadata
}

// CartQuoteResponse contains the generated quote for a cart.
type CartQuoteResponse struct {
	CartID      string            `json:"cartId"`              // Unique cart identifier
	Quote       *CryptoQuote      `json:"quote"`               // x402 requirement for the cart total (unwrapped)
	Items       []CartItem        `json:"items"`               // Itemized breakdown
	TotalAmount float64           `json:"totalAmount"`         // Final total after all discounts
	Metadata    map[string]string `json:"metadata,omitempty"`  // Cart metadata including coupon info
	ExpiresAt   time.Time         `json:"expiresAt"`           // When this cart quote expires
}

// CartItem represents an item in the quote response.
type CartItem struct {
	ResourceID      string   `json:"resource"`
	Quantity        int64    `json:"quantity"`
	PriceAmount     float64  `json:"priceAmount"`               // Price per unit (after catalog coupons)
	OriginalPrice   float64  `json:"originalPrice"`             // Original price before any discounts
	Token           string   `json:"token"`                     // Token symbol
	Description     string   `json:"description,omitempty"`
	AppliedCoupons  []string `json:"appliedCoupons,omitempty"`  // Catalog coupons applied to this item
}

// GetCartQuote retrieves an existing cart quote by ID.
func (s *Service) GetCartQuote(ctx context.Context, cartID string) (storage.CartQuote, error) {
	return s.store.GetCartQuote(ctx, cartID)
}

// GenerateCartQuote creates a quote for multiple items with locked prices.
func (s *Service) GenerateCartQuote(ctx context.Context, req CartQuoteRequest) (CartQuoteResponse, error) {
	if len(req.Items) == 0 {
		return CartQuoteResponse{}, errors.New("paywall: at least one item required")
	}

	// Generate unique cart ID
	cartID, err := storage.GenerateCartID()
	if err != nil {
		return CartQuoteResponse{}, fmt.Errorf("paywall: generate cart id: %w", err)
	}

	// Lookup all resources and validate they exist
	// Apply catalog-level coupons to each item's price
	var storageItems []storage.CartItem
	var responseItems []CartItem
	var totalMoney money.Money // Use Money for precise arithmetic
	var cryptoAsset money.Asset // Asset for all items (must be consistent)
	var token string // All items must use same token
	var allAppliedCatalogCoupons []string // Track all catalog coupons applied across items
	seenCouponCodes := make(map[string]bool) // O(1) deduplication instead of O(n) linear search

	for i, item := range req.Items {
		if item.ResourceID == "" {
			return CartQuoteResponse{}, fmt.Errorf("paywall: item %d missing resource id", i)
		}
		if item.Quantity <= 0 {
			item.Quantity = 1 // Default to 1
		}

		resource, err := s.ResourceDefinition(ctx, item.ResourceID)
		if err != nil {
			return CartQuoteResponse{}, fmt.Errorf("paywall: item %d (%s): %w", i, item.ResourceID, err)
		}

		// Verify crypto amount is configured
		if resource.CryptoAtomicAmount <= 0 {
			return CartQuoteResponse{}, fmt.Errorf("paywall: resource %s has no crypto price configured", item.ResourceID)
		}

		// Get asset for Money conversion (only once for first item)
		if token == "" {
			var err error
			cryptoAsset, err = money.GetAsset(resource.CryptoToken)
			if err != nil {
				return CartQuoteResponse{}, fmt.Errorf("get asset for token %s: %w", resource.CryptoToken, err)
			}
			token = resource.CryptoToken
			totalMoney = money.Zero(cryptoAsset) // Initialize total
		} else if token != resource.CryptoToken {
			return CartQuoteResponse{}, fmt.Errorf("paywall: mixed tokens in cart (got %s and %s)", token, resource.CryptoToken)
		}

		// Use atomic amount directly (Money type)
		originalPriceMoney := money.Money{Asset: cryptoAsset, Atomic: resource.CryptoAtomicAmount}

		// IMPORTANT: Apply catalog-level coupons to each item's unit price using Money arithmetic
		// This ensures product-specific discounts are shown at item level
		itemPriceMoney := originalPriceMoney
		var itemCouponCodes []string

		if s.coupons != nil {
			// Get catalog-level auto-apply coupons for this product (no manual coupons at item level)
			catalogCoupons := SelectCouponsForPayment(ctx, s.coupons, item.ResourceID, coupons.PaymentMethodX402, nil, ScopeCatalog)
			if len(catalogCoupons) > 0 {
				discounted, err := StackCouponsOnMoney(itemPriceMoney, catalogCoupons)
				if err != nil {
					return CartQuoteResponse{}, fmt.Errorf("apply catalog coupons: %w", err)
				}
				itemPriceMoney = discounted

				// Track coupon codes for this specific item
				for _, c := range catalogCoupons {
					itemCouponCodes = append(itemCouponCodes, c.Code)
					// Also track globally for cart metadata using O(1) map lookup
					if !seenCouponCodes[c.Code] {
						allAppliedCatalogCoupons = append(allAppliedCatalogCoupons, c.Code)
						seenCouponCodes[c.Code] = true
					}
				}
			}
		}

		// Calculate item total with discounted price using integer arithmetic
		itemTotalMoney, err := itemPriceMoney.Mul(int64(item.Quantity))
		if err != nil {
			return CartQuoteResponse{}, fmt.Errorf("multiply item price by quantity: %w", err)
		}

		// Add to cart total using integer addition
		totalMoney, err = totalMoney.Add(itemTotalMoney)
		if err != nil {
			return CartQuoteResponse{}, fmt.Errorf("add item to cart total: %w", err)
		}

		// Store item with locked discounted price (already Money)
		storageItems = append(storageItems, storage.CartItem{
			ResourceID:  item.ResourceID,
			Quantity:    item.Quantity,
			Price:       itemPriceMoney, // Already Money with coupons applied
			Metadata:    item.Metadata,
		})

		// Build response item with original price, discounted price, and applied coupons
		// Convert Money to float64 for legacy response format
		itemPriceFloat, _ := strconv.ParseFloat(itemPriceMoney.ToMajor(), 64)
		originalPriceFloat, _ := strconv.ParseFloat(originalPriceMoney.ToMajor(), 64)

		responseItems = append(responseItems, CartItem{
			ResourceID:     item.ResourceID,
			Quantity:       item.Quantity,
			PriceAmount:    itemPriceFloat,     // Discounted price (float64 for response)
			OriginalPrice:  originalPriceFloat, // Original price before discounts
			Token:          resource.CryptoToken,
			Description:    resource.Description,
			AppliedCoupons: itemCouponCodes, // Coupons applied to this specific item
		})
	}

	// PHASE 2: Apply checkout-level (site-wide) coupons to cart total using Money arithmetic
	// Total amount at this point already includes catalog-level discounts from items
	subtotalAfterCatalogCoupons := totalMoney

	// Validate manual coupon if provided
	// For cart checkout: Only allow site-wide coupons (scope="all")
	// Product-specific coupons are already applied at item level
	manualCoupon := s.validateManualCoupon(ctx, req.CouponCode, "", coupons.PaymentMethodX402)

	// Get checkout-level coupons (site-wide auto-apply + optional manual)
	var checkoutCoupons []coupons.Coupon
	if s.coupons != nil {
		checkoutCoupons = SelectCouponsForPayment(ctx, s.coupons, "", coupons.PaymentMethodX402, manualCoupon, ScopeCheckout)
	}

	// Apply stacked checkout coupons to cart total using Money arithmetic
	if len(checkoutCoupons) > 0 {
		discounted, err := StackCouponsOnMoney(totalMoney, checkoutCoupons)
		if err != nil {
			return CartQuoteResponse{}, fmt.Errorf("apply checkout coupons: %w", err)
		}
		totalMoney = discounted
	}

	// IMPORTANT: Round to cents precision (2 decimals) using integer arithmetic
	// This ensures $2.7661 becomes $2.77, not $2.7661
	// Uses Money.RoundUpToCents() for precise integer-based rounding
	totalMoney = totalMoney.RoundUpToCents()

	// Build cart metadata including coupon info from both levels
	cartMetadata := req.Metadata
	if cartMetadata == nil {
		cartMetadata = make(map[string]string)
	}

	// Track all coupons applied (catalog + checkout)
	var allAppliedCouponCodes []string
	allAppliedCouponCodes = append(allAppliedCouponCodes, allAppliedCatalogCoupons...)
	for _, c := range checkoutCoupons {
		allAppliedCouponCodes = append(allAppliedCouponCodes, c.Code)
	}

	if len(allAppliedCouponCodes) > 0 {
		cartMetadata["coupon_codes"] = formatCouponCodes(allAppliedCouponCodes)
		cartMetadata["subtotal_after_catalog"] = subtotalAfterCatalogCoupons.ToMajor()
		cartMetadata["discounted_amount"] = totalMoney.ToMajor()
	}

	// Store breakdown for frontend display
	if catalogCodesStr := formatCouponCodes(allAppliedCatalogCoupons); catalogCodesStr != "" {
		cartMetadata["catalog_coupons"] = catalogCodesStr
	}
	if len(checkoutCoupons) > 0 {
		var checkoutCouponCodes []string
		for _, c := range checkoutCoupons {
			checkoutCouponCodes = append(checkoutCouponCodes, c.Code)
		}
		if checkoutCodesStr := formatCouponCodes(checkoutCouponCodes); checkoutCodesStr != "" {
			cartMetadata["checkout_coupons"] = checkoutCodesStr
		}
	}

	// Save cart quote to storage
	now := time.Now()
	cartTTL := s.cfg.Storage.CartQuoteTTL.Duration
	if cartTTL == 0 {
		cartTTL = 15 * time.Minute // Fallback default
	}
	expiresAt := now.Add(cartTTL)

	// totalMoney is already calculated using Money arithmetic - no conversion needed!
	cartQuote := storage.CartQuote{
		ID:        cartID,
		Items:     storageItems,
		Total:     totalMoney, // Already Money type with precise integer arithmetic
		Metadata:  cartMetadata, // Use updated metadata with coupon info
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	if err := s.store.SaveCartQuote(ctx, cartQuote); err != nil {
		return CartQuoteResponse{}, fmt.Errorf("paywall: save cart quote: %w", err)
	}

	// Build x402 quote for cart total
	// Pass atomic units directly from Money type (no float64 conversion)
	quote, err := s.buildCartX402Quote(cartID, uint64(totalMoney.Atomic), token, expiresAt)
	if err != nil {
		return CartQuoteResponse{}, fmt.Errorf("paywall: build x402 quote: %w", err)
	}

	// Convert Money to float64 for JSON response (external API boundary)
	totalAmountFloat, _ := strconv.ParseFloat(totalMoney.ToMajor(), 64)

	return CartQuoteResponse{
		CartID:      cartID,
		Quote:       quote,
		Items:       responseItems,
		TotalAmount: totalAmountFloat, // Convert to float64 for JSON response
		Metadata:    cartMetadata,
		ExpiresAt:   expiresAt,
	}, nil
}

// buildCartX402Quote creates an x402 quote for a cart's total amount.
func (s *Service) buildCartX402Quote(cartID string, atomicAmount uint64, token string, expiresAt time.Time) (*CryptoQuote, error) {
	// Convert atomic units to major units for display only
	asset, _ := money.GetAsset(token)
	displayAmount := money.New(asset, int64(atomicAmount)).ToMajor()

	return s.buildX402Quote(x402QuoteOptions{
		ResourceID:            fmt.Sprintf("cart:%s", cartID),
		AtomicAmount:          atomicAmount,
		Token:                 token,
		PayToAddress:          s.cfg.X402.PaymentAddress,
		RecipientTokenAccount: deriveTokenAccountSafe(s.cfg.X402.PaymentAddress, s.cfg.X402.TokenMint),
		Description:           fmt.Sprintf("Cart purchase (%s %s)", displayAmount, token),
		ExpiresAt:             expiresAt,
		IncludeFeePayer:       true, // Carts support gasless
	})
}

// formatFloat formats a float with appropriate precision (removes trailing zeros).
func formatFloat(f float64) string {
	s := fmt.Sprintf("%.6f", f)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// authorizeCart handles x402 payment verification for cart purchases.
func (s *Service) authorizeCart(ctx context.Context, cartID, paymentHeader, couponCode string) (AuthorizationResult, error) {
	// Payment header is required for cart payments
	if paymentHeader == "" {
		return AuthorizationResult{}, fmt.Errorf("payment proof required for cart %s", cartID)
	}

	// Parse payment proof
	proof, err := x402.ParsePaymentProof(paymentHeader)
	if err != nil {
		return AuthorizationResult{}, fmt.Errorf("parse payment header: %w", err)
	}

	// Validate scheme and network
	if proof.Scheme != "solana-spl-transfer" && proof.Scheme != "solana" {
		return AuthorizationResult{}, fmt.Errorf("unsupported scheme: %s", proof.Scheme)
	}
	if proof.Network != s.cfg.X402.Network {
		return AuthorizationResult{}, fmt.Errorf("network mismatch: expected %s, got %s", s.cfg.X402.Network, proof.Network)
	}

	// Lookup cart quote
	cart, err := s.store.GetCartQuote(ctx, cartID)
	if err != nil {
		if err == storage.ErrCartExpired {
			return AuthorizationResult{}, fmt.Errorf("cart quote expired, please request a new quote")
		}
		if err == storage.ErrNotFound {
			return AuthorizationResult{}, fmt.Errorf("cart not found: %s", cartID)
		}
		return AuthorizationResult{}, fmt.Errorf("get cart quote: %w", err)
	}

	// Get cart TTL from config with fallback
	cartTTL := s.cfg.Storage.CartQuoteTTL.Duration
	if cartTTL == 0 {
		cartTTL = 15 * time.Minute // Fallback default
	}

	// Note: With coupon stacking, we don't validate coupon codes match during verification
	// The cart total already includes all discounts from quote generation
	// Auto-apply coupons are applied server-side, so we just verify the total amount

	// Verify payment amount matches cart total EXACTLY
	// Convert cart.Total (Money) to float64 for x402 verification
	cartTotalFloat, err := strconv.ParseFloat(cart.Total.ToMajor(), 64)
	if err != nil {
		return AuthorizationResult{}, fmt.Errorf("parse cart total: %w", err)
	}

	requirement := x402.Requirement{
		ResourceID:            cartID,
		RecipientOwner:        s.cfg.X402.PaymentAddress,
		RecipientTokenAccount: deriveTokenAccountSafe(s.cfg.X402.PaymentAddress, s.cfg.X402.TokenMint),
		TokenMint:             s.cfg.X402.TokenMint,
		Amount:                cartTotalFloat, // Use cart's locked total
		Network:               s.cfg.X402.Network,
		TokenDecimals:         s.cfg.X402.TokenDecimals,
		AllowedTokens:         []string{cart.Total.Asset.Code}, // Only cart's token allowed
		QuoteTTL:              cartTTL,
		SkipPreflight:         s.cfg.X402.SkipPreflight,
		Commitment:            s.cfg.X402.Commitment,
	}

	// CRITICAL: Atomically claim this signature BEFORE verification to prevent TOCTOU race
	// For non-gasless transactions, the signature is already known from the X-PAYMENT header
	// For gasless transactions, the signature is determined after the backend co-signs and submits
	// This prevents concurrent requests from both passing the check and wasting RPC calls
	log := logger.FromContext(ctx)
	now := time.Now()

	// For gasless transactions, skip optimistic recording since signature doesn't exist yet
	// The actual signature is only known after the backend co-signs and Solana accepts the transaction
	isGasless := proof.FeePayer != ""
	if !isGasless {
		// Optimistically record the payment with placeholder data
		// If verification fails, we don't clean this up (intentional - prevents replay)
		placeholderTx := storage.PaymentTransaction{
			Signature:  proof.Signature,
			ResourceID: cartID,
			Wallet:     "", // Will be updated after verification
			Amount:     money.Zero(cart.Total.Asset), // Will be updated after verification
			CreatedAt:  now,
			Metadata:   map[string]string{"status": "verifying"},
		}

		if err := s.store.RecordPayment(ctx, placeholderTx); err != nil {
			// Signature already exists - either replay attack or concurrent request
			originalTx, _ := s.store.GetPayment(ctx, proof.Signature)
			log.Warn().
				Err(err).
				Str("signature", logger.TruncateAddress(proof.Signature)).
				Str("original_resource_hash", hashResourceID(originalTx.ResourceID)).
				Str("attempted_cart_hash", hashResourceID(cartID)).
				Msg("cart.replay_attack_detected")

			if s.metrics != nil {
				s.metrics.ObservePaymentFailure("x402", cartID, "replay_attack")
			}

			return AuthorizationResult{}, fmt.Errorf("payment proof has already been used (originally for: %s)", originalTx.ResourceID)
		}
	}

	// Track cart payment timing
	paymentStart := time.Now()

	result, err := s.verifier.Verify(ctx, proof, requirement)
	paymentDuration := time.Since(paymentStart)

	if err != nil {
		// Record failed cart payment metric
		if s.metrics != nil {
			reason := "verification_failed"
			if vErr, ok := err.(x402.VerificationError); ok {
				reason = string(vErr.Code)
			}
			s.metrics.ObservePaymentFailure("x402", cartID, reason)
		}

		if vErr, ok := err.(x402.VerificationError); ok {
			log.Error().
				Err(vErr.Err).
				Str("cart_id", cartID).
				Str("error_code", string(vErr.Code)).
				Msg("cart.x402_verification_failed")
			return AuthorizationResult{}, fmt.Errorf("%s", vErr.Message)
		}
		return AuthorizationResult{}, err
	}

	// SECURITY: Enforce exact amount matching to prevent frontend bugs and user error
	// The Solana verifier allows overpayment (for tips), but we require exact match
	// Use tolerance of 1 smallest unit (0.000001 for 6 decimals) to handle floating-point precision
	const tolerance = 0.000001
	amountDiff := result.Amount - cartTotalFloat
	if amountDiff < -tolerance || amountDiff > tolerance {
		// Record amount mismatch failure
		if s.metrics != nil {
			s.metrics.ObservePaymentFailure("x402", cartID, "amount_mismatch")
		}

		log.Error().
			Str("cart_hash", hashResourceID(cartID)).
			Float64("quoted_amount", cartTotalFloat).
			Float64("paid_amount", result.Amount).
			Float64("difference", amountDiff).
			Str("wallet", logger.TruncateAddress(result.Wallet)).
			Msg("cart.payment_amount_mismatch")
		return AuthorizationResult{}, fmt.Errorf("payment amount (%.6f %s) does not match required cart total (%.6f %s). Please ensure you're paying the exact quoted amount.",
			result.Amount, cart.Total.Asset.Code, cartTotalFloat, cart.Total.Asset.Code)
	}

	// For gasless transactions, the actual signature comes from the verifier (after co-signing + submission)
	// For non-gasless transactions, use the signature from the proof (pre-submitted by user)
	actualSignature := result.Signature
	if !isGasless && proof.Signature != "" {
		actualSignature = proof.Signature
	}

	// Payment signature was already recorded before verification (atomic claim) for non-gasless
	// For gasless, this is the first time we're recording since we didn't know the signature before
	// Now update/create the record with verified payment details
	finalPaymentTx := storage.PaymentTransaction{
		Signature:  actualSignature,
		ResourceID: cartID,
		Wallet:     result.Wallet,
		Amount:     cart.Total,
		CreatedAt:  now,
		Metadata: map[string]string{
			"status":  "verified",
			"network": s.cfg.X402.Network,
			"type":    "cart",
		},
	}
	if err := s.store.RecordPayment(ctx, finalPaymentTx); err != nil {
		// For gasless: might be a race where same tx was submitted twice
		// For non-gasless: should not happen since we claimed the signature earlier
		if isGasless {
			// Check if this signature was already used
			originalTx, _ := s.store.GetPayment(ctx, actualSignature)
			log.Warn().
				Err(err).
				Str("signature", logger.TruncateAddress(actualSignature)).
				Str("original_resource_hash", hashResourceID(originalTx.ResourceID)).
				Str("attempted_cart_hash", hashResourceID(cartID)).
				Msg("cart.gasless_replay_detected")
			return AuthorizationResult{}, fmt.Errorf("payment proof has already been used (originally for: %s)", originalTx.ResourceID)
		}
		// Log error but don't fail - payment was already verified on-chain
		log.Error().
			Err(err).
			Str("signature", logger.TruncateAddress(actualSignature)).
			Str("cart_hash", hashResourceID(cartID)).
			Msg("cart.failed_to_finalize_payment_record")
	}

	// Convert amount to cents for metrics (stored as float64 in USD)
	amountCents := int64(result.Amount * 100)

	// Record successful cart payment metrics
	if s.metrics != nil {
		s.metrics.ObservePayment("x402", cartID, true, paymentDuration, amountCents, cart.Total.Asset.Code)
		s.metrics.ObserveSettlement(s.cfg.X402.Network, paymentDuration)
		// Record cart checkout success with item count
		s.metrics.ObserveCartCheckout("success", len(cart.Items))
	}

	// Mark cart as paid
	if err := s.store.MarkCartPaid(ctx, cartID, result.Wallet); err != nil {
		return AuthorizationResult{}, fmt.Errorf("mark cart paid: %w", err)
	}

	// Increment usage for all coupons applied to the cart
	storedCouponCodes := cart.Metadata["coupon_codes"]
	if storedCouponCodes != "" && s.coupons != nil {
		codes := strings.Split(storedCouponCodes, ",")
		for _, code := range codes {
			if err := s.coupons.IncrementUsage(ctx, code); err != nil {
				// Log error but don't fail - payment was successful
				log.Warn().
					Err(err).
					Str("coupon_code", code).
					Msg("cart.coupon_increment_failed")
			}
		}
	}

	// Build callback event with cart item details
	metadata := mergeMetadata(cart.Metadata, proof.Metadata)

	// Enhance metadata with coupon info (if coupons were applied)
	// Note: With stacking, metadata now contains aggregated discount info
	// original_amount and discounted_amount already in cart.Metadata from quote generation

	// Add cart item details to metadata for callback processing
	metadata["cart_items"] = fmt.Sprintf("%d", len(cart.Items))
	var totalQuantity int64
	for i, item := range cart.Items {
		prefix := fmt.Sprintf("item_%d_", i)
		metadata[prefix+"resource"] = item.ResourceID
		metadata[prefix+"quantity"] = fmt.Sprintf("%d", item.Quantity)
		metadata[prefix+"price_amount"] = item.Price.ToMajor() // Convert Money to string
		metadata[prefix+"token"] = item.Price.Asset.Code
		totalQuantity += item.Quantity

		// Merge per-item metadata
		for k, v := range item.Metadata {
			metadata[prefix+k] = v
		}
	}
	metadata["total_quantity"] = fmt.Sprintf("%d", totalQuantity)

	// Fire payment succeeded callback
	s.notifier.PaymentSucceeded(ctx, callbacks.PaymentEvent{
		ResourceID:         cartID,
		Method:             "x402-cart",
		CryptoAtomicAmount: cart.Total.Atomic,
		CryptoToken:        cart.Total.Asset.Code,
		Wallet:             result.Wallet,
		ProofSignature:     actualSignature,
		Metadata:           metadata,
		PaidAt:             now.UTC(),
	})

	// Build settlement response
	networkID := s.cfg.X402.Network
	settlement := &SettlementResponse{
		Success:   true,
		Error:     nil,
		TxHash:    &result.Signature, // Use actual signature from verification result
		NetworkID: &networkID,
	}

	return AuthorizationResult{
		Granted:    true,
		Method:     "x402-cart",
		Wallet:     result.Wallet,
		Settlement: settlement,
	}, nil
}
