package httpserver

import (
	"net/http"

	"github.com/CedrosPay/server/internal/coupons"
	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/pkg/responders"
)

// ValidateCouponRequest represents the request body for coupon validation.
type ValidateCouponRequest struct {
	Code          string   `json:"code"`
	ProductIDs    []string `json:"productIds,omitempty"`    // Optional: products to validate against
	PaymentMethod string   `json:"paymentMethod,omitempty"` // Optional: "stripe", "x402", or empty for any
}

// ValidateCouponResponse represents the response for coupon validation.
type ValidateCouponResponse struct {
	Valid              bool     `json:"valid"`
	Code               string   `json:"code,omitempty"`
	DiscountType       string   `json:"discountType,omitempty"`
	DiscountValue      float64  `json:"discountValue,omitempty"`
	Scope              string   `json:"scope,omitempty"`              // "all" or "specific"
	ApplicableProducts []string `json:"applicableProducts,omitempty"` // Only relevant when scope="specific"
	PaymentMethod      string   `json:"paymentMethod,omitempty"`      // Payment method restriction: "", "stripe", or "x402"
	ExpiresAt          *string  `json:"expiresAt,omitempty"`          // RFC3339 timestamp
	RemainingUses      *int     `json:"remainingUses,omitempty"`
	ErrorMessage       string   `json:"error,omitempty"`
}

// validateCoupon validates a coupon code and returns discount information.
func (h *handlers) validateCoupon(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	var req ValidateCouponRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		log.Warn().
			Err(err).
			Msg("coupons.validate.invalid_body")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInvalidField, "invalid request body")
		return
	}

	// Get coupon from repository
	coupon, err := h.couponRepo.GetCoupon(r.Context(), req.Code)
	if err == coupons.ErrCouponNotFound {
		log.Debug().
			Str("coupon_code", req.Code).
			Msg("coupons.validate.not_found")
		responders.JSON(w, http.StatusOK, ValidateCouponResponse{
			Valid:        false,
			ErrorMessage: "Coupon not found",
		})
		return
	}
	if err != nil {
		log.Error().
			Err(err).
			Str("coupon_code", req.Code).
			Msg("coupons.validate.fetch_failed")
		apierrors.WriteSimpleError(w, apierrors.ErrCodeInternalError, "failed to validate coupon")
		return
	}

	// Validate coupon
	if err := coupon.IsValid(); err != nil {
		responders.JSON(w, http.StatusOK, ValidateCouponResponse{
			Valid:        false,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Check payment method restriction if provided
	if req.PaymentMethod != "" {
		paymentMethod := coupons.PaymentMethod(req.PaymentMethod)
		if !coupon.AppliesToPaymentMethod(paymentMethod) {
			responders.JSON(w, http.StatusOK, ValidateCouponResponse{
				Valid:        false,
				ErrorMessage: "Coupon does not apply to the selected payment method",
			})
			return
		}
	}

	// Check if coupon applies to requested products (if product IDs provided)
	applicableProducts := []string{}
	if len(req.ProductIDs) > 0 {
		// Product IDs provided - validate against them
		for _, productID := range req.ProductIDs {
			if coupon.AppliesToProduct(productID) {
				applicableProducts = append(applicableProducts, productID)
			}
		}

		if len(applicableProducts) == 0 {
			responders.JSON(w, http.StatusOK, ValidateCouponResponse{
				Valid:        false,
				ErrorMessage: "Coupon does not apply to the selected products",
			})
			return
		}
	} else {
		// No product IDs provided - scope="all" coupons are valid, scope="specific" need context
		if coupon.Scope == coupons.ScopeAll {
			applicableProducts = nil // Applies to all products
		} else {
			// For scope="specific", return the list of applicable products from coupon config
			applicableProducts = coupon.ProductIDs
		}
	}

	// Calculate remaining uses
	// IMPORTANT: YAML coupons cannot track usage counts (read-only), so we don't return
	// remainingUses for YAML sources to avoid misleading the frontend with stale data.
	// Only database-backed coupons (Postgres/MongoDB) have accurate usage tracking.
	var remainingUses *int
	isYAMLSource := h.cfg.Coupons.CouponSource == "yaml" || h.cfg.Coupons.CouponSource == ""

	if !isYAMLSource && coupon.UsageLimit != nil {
		remaining := *coupon.UsageLimit - coupon.UsageCount
		if remaining >= 0 {
			remainingUses = &remaining
		}
	}

	// Format expiration date
	var expiresAt *string
	if coupon.ExpiresAt != nil {
		formatted := coupon.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
		expiresAt = &formatted
	}

	log.Info().
		Str("coupon_code", req.Code).
		Str("discount_type", string(coupon.DiscountType)).
		Float64("discount_value", coupon.DiscountValue).
		Msg("coupons.validate.success")

	// Return valid coupon response
	responders.JSON(w, http.StatusOK, ValidateCouponResponse{
		Valid:              true,
		Code:               coupon.Code,
		DiscountType:       string(coupon.DiscountType),
		DiscountValue:      coupon.DiscountValue,
		Scope:              string(coupon.Scope),
		ApplicableProducts: applicableProducts,
		PaymentMethod:      string(coupon.PaymentMethod), // Return payment method restriction
		ExpiresAt:          expiresAt,
		RemainingUses:      remainingUses,
	})
}
