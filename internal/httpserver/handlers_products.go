package httpserver

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"github.com/CedrosPay/server/internal/coupons"
	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
)

// ProductResponse represents the JSON structure returned to the frontend.
type ProductResponse struct {
	ID                    string            `json:"id"`
	Description           string            `json:"description"`
	FiatAmount            float64           `json:"fiatAmount"`
	EffectiveFiatAmount   float64           `json:"effectiveFiatAmount"` // Price after catalog-level auto-apply coupon for Stripe
	FiatCurrency          string            `json:"fiatCurrency"`
	StripePriceID         string            `json:"stripePriceId,omitempty"`
	CryptoAmount          float64           `json:"cryptoAmount"`
	EffectiveCryptoAmount float64           `json:"effectiveCryptoAmount"` // Price after catalog-level auto-apply coupon for x402
	CryptoToken           string            `json:"cryptoToken"`
	HasStripeCoupon       bool              `json:"hasStripeCoupon"`            // True if Stripe catalog-level auto-apply coupon exists
	HasCryptoCoupon       bool              `json:"hasCryptoCoupon"`            // True if x402 catalog-level auto-apply coupon exists
	StripeCouponCode      string            `json:"stripeCouponCode,omitempty"` // Stripe catalog-level auto-apply coupon code
	CryptoCouponCode      string            `json:"cryptoCouponCode,omitempty"` // x402 catalog-level auto-apply coupon code
	StripeDiscountPercent float64           `json:"stripeDiscountPercent"`      // Percentage off for Stripe (catalog-level)
	CryptoDiscountPercent float64           `json:"cryptoDiscountPercent"`      // Percentage off for x402 (catalog-level)
	Metadata              map[string]string `json:"metadata,omitempty"`
}

// ProductsListResponse wraps the product list with checkout-level coupons.
type ProductsListResponse struct {
	Products              []ProductResponse `json:"products"`
	CheckoutStripeCoupons []CouponSummary   `json:"checkoutStripeCoupons"` // Auto-apply coupons for Stripe at checkout
	CheckoutCryptoCoupons []CouponSummary   `json:"checkoutCryptoCoupons"` // Auto-apply coupons for x402 at checkout
}

// CouponSummary represents a checkout-level coupon.
type CouponSummary struct {
	Code          string  `json:"code"`
	DiscountType  string  `json:"discountType"`  // "percentage", "fixed", "free"
	DiscountValue float64 `json:"discountValue"` // Percentage (e.g., 20.0 for 20%) or fixed amount
	Currency      string  `json:"currency,omitempty"`
	Description   string  `json:"description,omitempty"`
}

// listProducts returns all active products (uses cache if configured).
func (h *handlers) listProducts(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	products, err := h.paywall.ListProducts(r.Context())
	if err != nil {
		log.Error().
			Err(err).
			Msg("products.list.fetch_failed")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "failed to fetch products")
		return
	}

	// Batch fetch auto-apply coupons for both payment methods (2 queries instead of 2*N)
	var stripeCouponsMap, cryptoCouponsMap map[string][]coupons.Coupon
	var stripeCheckoutCoupons, cryptoCheckoutCoupons []coupons.Coupon
	if h.couponRepo != nil {
		allStripeCoupons, _ := h.couponRepo.GetAllAutoApplyCouponsForPayment(r.Context(), coupons.PaymentMethodStripe)
		allCryptoCoupons, _ := h.couponRepo.GetAllAutoApplyCouponsForPayment(r.Context(), coupons.PaymentMethodX402)

		// Separate catalog-level (product page) and checkout-level (cart) coupons
		stripeCouponsMap, stripeCheckoutCoupons = splitCouponsByAppliesAt(allStripeCoupons)
		cryptoCouponsMap, cryptoCheckoutCoupons = splitCouponsByAppliesAt(allCryptoCoupons)
	}

	// Convert to response format with auto-apply coupons
	response := make([]ProductResponse, 0, len(products))
	for _, p := range products {
		// Extract pricing from Money types
		var fiatAmount, cryptoAmount float64
		var fiatCurrency, cryptoToken string

		if p.FiatPrice != nil {
			fiatAmountStr := p.FiatPrice.ToMajor()
			fiatAmount, _ = strconv.ParseFloat(fiatAmountStr, 64)
			fiatCurrency = p.FiatPrice.Asset.Code
		}

		if p.CryptoPrice != nil {
			cryptoAmountStr := p.CryptoPrice.ToMajor()
			cryptoAmount, _ = strconv.ParseFloat(cryptoAmountStr, 64)
			cryptoToken = p.CryptoPrice.Asset.Code
		}

		pr := ProductResponse{
			ID:                    p.ID,
			Description:           p.Description,
			FiatAmount:            fiatAmount,
			EffectiveFiatAmount:   fiatAmount,
			FiatCurrency:          fiatCurrency,
			StripePriceID:         p.StripePriceID,
			CryptoAmount:          cryptoAmount,
			EffectiveCryptoAmount: cryptoAmount,
			CryptoToken:           cryptoToken,
			HasStripeCoupon:       false,
			HasCryptoCoupon:       false,
			StripeDiscountPercent: 0,
			CryptoDiscountPercent: 0,
			Metadata:              p.Metadata,
		}

		// Apply Stripe coupons
		if stripeCouponsMap != nil {
			applyCouponsToProduct(&pr, p.ID, stripeCouponsMap, fiatAmount, true)
		}

		// Apply x402 coupons
		if cryptoCouponsMap != nil {
			applyCouponsToProduct(&pr, p.ID, cryptoCouponsMap, cryptoAmount, false)
		}

		response = append(response, pr)
	}

	// Build final response with checkout-level coupons
	finalResponse := ProductsListResponse{
		Products:              response,
		CheckoutStripeCoupons: buildCouponSummaries(stripeCheckoutCoupons),
		CheckoutCryptoCoupons: buildCouponSummaries(cryptoCheckoutCoupons),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300") // 5-minute browser cache
	if err := json.NewEncoder(w).Encode(finalResponse); err != nil {
		log.Error().
			Err(err).
			Int("product_count", len(response)).
			Msg("products.list.encode_failed")
	} else {
		log.Debug().
			Int("product_count", len(response)).
			Int("stripe_checkout_coupons", len(stripeCheckoutCoupons)).
			Int("crypto_checkout_coupons", len(cryptoCheckoutCoupons)).
			Msg("products.list.success")
	}
}

// applyCouponsToProduct applies coupons from the map to a product response.
// isStripe determines which fields to update (fiat vs crypto).
func applyCouponsToProduct(pr *ProductResponse, productID string, couponMap map[string][]coupons.Coupon, originalPrice float64, isStripe bool) {
	// Collect applicable coupons (product-specific + "all" scope)
	var applicableCoupons []coupons.Coupon
	if productCoupons, ok := couponMap[productID]; ok {
		applicableCoupons = append(applicableCoupons, productCoupons...)
	}
	if allCoupons, ok := couponMap["*"]; ok {
		applicableCoupons = append(applicableCoupons, allCoupons...)
	}

	if len(applicableCoupons) == 0 {
		return
	}

	bestCoupon := findBestCoupon(applicableCoupons, originalPrice)
	if bestCoupon == nil {
		return
	}

	effectivePrice := bestCoupon.ApplyDiscount(originalPrice)

	// IMPORTANT: Round to cents precision (2 decimals) using ceiling
	// This ensures $0.345 becomes $0.35, not $0.345
	// Matches behavior in paywall quote generation
	effectivePrice = math.Ceil(effectivePrice*100) / 100

	discountPercent := calculateDiscountPercent(originalPrice, effectivePrice)

	if isStripe {
		pr.EffectiveFiatAmount = effectivePrice
		pr.HasStripeCoupon = true
		pr.StripeCouponCode = bestCoupon.Code
		pr.StripeDiscountPercent = discountPercent
	} else {
		pr.EffectiveCryptoAmount = effectivePrice
		pr.HasCryptoCoupon = true
		pr.CryptoCouponCode = bestCoupon.Code
		pr.CryptoDiscountPercent = discountPercent
	}
}

// calculateDiscountPercent computes the discount percentage.
func calculateDiscountPercent(original, discounted float64) float64 {
	if original > 0 {
		return ((original - discounted) / original) * 100
	}
	return 0
}

// findBestCoupon returns the coupon that provides the maximum discount.
func findBestCoupon(couponList []coupons.Coupon, originalPrice float64) *coupons.Coupon {
	if len(couponList) == 0 {
		return nil
	}

	var best *coupons.Coupon
	maxDiscount := 0.0

	for i := range couponList {
		discounted := couponList[i].ApplyDiscount(originalPrice)
		discount := originalPrice - discounted

		if discount > maxDiscount {
			maxDiscount = discount
			best = &couponList[i]
		}
	}

	return best
}

// splitCouponsByAppliesAt separates catalog-level and checkout-level coupons.
// Returns (catalogMap, checkoutList) where:
// - catalogMap: product-specific coupons for product pages (appliesAt: "catalog")
// - checkoutList: cart-wide coupons for checkout (appliesAt: "checkout")
func splitCouponsByAppliesAt(couponMap map[string][]coupons.Coupon) (map[string][]coupons.Coupon, []coupons.Coupon) {
	if couponMap == nil {
		return nil, nil
	}

	catalogMap := make(map[string][]coupons.Coupon)
	var checkoutList []coupons.Coupon

	for productID, couponList := range couponMap {
		for _, c := range couponList {
			if c.AppliesAt == coupons.AppliesAtCheckout {
				// Checkout-level: add to list (only once, not per product)
				checkoutList = append(checkoutList, c)
			} else {
				// Catalog-level (including empty for backward compatibility)
				catalogMap[productID] = append(catalogMap[productID], c)
			}
		}
	}

	// Deduplicate checkout coupons (since they were collected from all product IDs)
	checkoutList = deduplicateCoupons(checkoutList)

	return catalogMap, checkoutList
}

// deduplicateCoupons removes duplicate coupons by code.
func deduplicateCoupons(couponList []coupons.Coupon) []coupons.Coupon {
	seen := make(map[string]bool)
	var result []coupons.Coupon
	for _, c := range couponList {
		if !seen[c.Code] {
			seen[c.Code] = true
			result = append(result, c)
		}
	}
	return result
}

// buildCouponSummaries converts coupons to summary format for API response.
func buildCouponSummaries(couponList []coupons.Coupon) []CouponSummary {
	summaries := make([]CouponSummary, 0, len(couponList))
	for _, c := range couponList {
		summary := CouponSummary{
			Code:          c.Code,
			DiscountType:  string(c.DiscountType),
			DiscountValue: c.DiscountValue,
			Currency:      c.Currency,
		}
		// Use metadata description if available
		if desc, ok := c.Metadata["description"]; ok {
			summary.Description = desc
		}
		summaries = append(summaries, summary)
	}
	return summaries
}
