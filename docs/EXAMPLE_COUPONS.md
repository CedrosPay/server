# Coupon Examples

This document provides YAML examples for every type of coupon you can create in Cedros Pay. Copy and customize these examples for your promotions.

---

## Table of Contents

1. [Basic Discounts](#basic-discounts)
2. [Auto-Apply Coupons](#auto-apply-coupons)
3. [Time-Based Promotions](#time-based-promotions)
4. [Product-Specific Coupons](#product-specific-coupons)
5. [Payment Method Restrictions](#payment-method-restrictions)
6. [Usage Limits](#usage-limits)
7. [Two-Phase Coupon System](#two-phase-coupon-system)
8. [Advanced Stacking Examples](#advanced-stacking-examples)

---

## Basic Discounts

### Percentage Off (20% off everything)

```yaml
SAVE20:
  code: "SAVE20"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 20.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "spring-sale"
```

### Fixed Dollar Amount ($10 off)

```yaml
NEWUSER10:
  code: "NEWUSER10"
  auto_apply: false
  discount_type: "fixed"
  discount_value: 10.0
  currency: "usd"
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "new-user-promotion"
```

### Small Fixed Discount ($0.50 off)

```yaml
HALFOFF:
  code: "HALFOFF"
  auto_apply: false
  discount_type: "fixed"
  discount_value: 0.50
  currency: "usd"
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "micro-discount"
```

---

## Auto-Apply Coupons

### Auto-Applied 5% Discount (no code needed)

```yaml
WELCOME5:
  code: "WELCOME5"
  auto_apply: true  # Automatically applies to all eligible purchases
  discount_type: "percentage"
  discount_value: 5.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "checkout"
  metadata:
    campaign: "auto-welcome"
    description: "Automatic welcome discount for all new visitors"
```

### Auto-Applied Crypto Incentive (3% off crypto payments)

```yaml
CRYPTO3OFF:
  code: "CRYPTO3OFF"
  auto_apply: true
  discount_type: "percentage"
  discount_value: 3.0  # Pass Stripe fee savings to crypto users
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: "x402"
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "checkout"
  metadata:
    campaign: "crypto-incentive"
    description: "Automatic discount for crypto payments"
```

---

## Time-Based Promotions

### Limited Time Sale (Black Friday)

```yaml
BLACKFRIDAY50:
  code: "BLACKFRIDAY50"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 50.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: "2025-11-28T00:00:00Z"  # Black Friday start
  expires_at: "2025-11-29T23:59:59Z"  # 24-hour sale
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "black-friday-2025"
```

### Flash Sale (2-hour window)

```yaml
FLASH2HR:
  code: "FLASH2HR"
  auto_apply: true  # Auto-apply during flash sale window
  discount_type: "percentage"
  discount_value: 30.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: "2025-12-15T18:00:00Z"
  expires_at: "2025-12-15T20:00:00Z"
  active: true
  applies_at: "checkout"
  metadata:
    campaign: "flash-sale-evening"
```

### Monthly Subscription Discount (recurring)

```yaml
JANUARY2025:
  code: "JANUARY2025"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 15.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: 1000
  usage_count: 0
  starts_at: "2025-01-01T00:00:00Z"
  expires_at: "2025-01-31T23:59:59Z"
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "monthly-promo"
    month: "january"
```

### Early Access Discount (pre-launch)

```yaml
EARLYBIRD:
  code: "EARLYBIRD"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 25.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: 100  # First 100 customers only
  usage_count: 0
  starts_at: "2025-11-01T00:00:00Z"
  expires_at: "2025-11-15T23:59:59Z"  # Expires at official launch
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "early-access"
    tier: "founder"
```

---

## Product-Specific Coupons

### Single Product Discount (specific item)

```yaml
PREMIUM50:
  code: "PREMIUM50"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 50.0
  currency: ""
  scope: "specific"
  product_ids:
    - "premium-course"
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "premium-launch"
```

### Multi-Product Bundle Discount

```yaml
BUNDLE3PACK:
  code: "BUNDLE3PACK"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 20.0
  currency: ""
  scope: "specific"
  product_ids:
    - "course-intro"
    - "course-advanced"
    - "course-masterclass"
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "course-bundle"
    bundle_name: "complete-learning-path"
```

### Category-Wide Discount (digital goods)

```yaml
DIGITAL25:
  code: "DIGITAL25"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 25.0
  currency: ""
  scope: "specific"
  product_ids:
    - "ebook-javascript"
    - "ebook-python"
    - "ebook-golang"
    - "video-tutorial-react"
    - "video-tutorial-vue"
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "digital-content-sale"
    category: "education"
```

---

## Payment Method Restrictions

### Stripe-Only Coupon (bank card payments)

```yaml
CARD10OFF:
  code: "CARD10OFF"
  auto_apply: false
  discount_type: "fixed"
  discount_value: 10.0
  currency: "usd"
  scope: "all"
  product_ids: []
  payment_method: "stripe"  # Only applies to Stripe payments
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "credit-card-promo"
```

### Crypto-Only Coupon (x402 payments)

```yaml
CRYPTO15:
  code: "CRYPTO15"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 15.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: "x402"  # Only applies to crypto payments
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "crypto-exclusive"
    description: "Reward users who pay with crypto"
```

### Universal Coupon (any payment method)

```yaml
UNIVERSAL20:
  code: "UNIVERSAL20"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 20.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""  # Empty = applies to both Stripe and x402
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "universal-sale"
```

---

## Usage Limits

### One-Time Use Coupon (single redemption)

```yaml
ONETIME50:
  code: "ONETIME50"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 50.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: 1  # Can only be used once globally
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "exclusive-offer"
```

### Limited Redemptions (first 500 customers)

```yaml
FIRST500:
  code: "FIRST500"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 30.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: 500  # First 500 redemptions only
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "limited-availability"
```

### Unlimited Usage Coupon

```yaml
ALWAYS10:
  code: "ALWAYS10"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 10.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null  # Unlimited redemptions
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "permanent-discount"
```

---

## Two-Phase Coupon System

### Catalog Coupon (product page discount)

Shows discounted price immediately on product pages. Users see the discount before adding to cart.

```yaml
PRODUCT20:
  code: "PRODUCT20"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 20.0
  currency: ""
  scope: "specific"
  product_ids:
    - "premium-article"
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"  # Applied at product page level
  metadata:
    campaign: "catalog-discount"
```

### Checkout Coupon (cart-level discount)

Applied at checkout after items are added to cart. Site-wide discounts.

```yaml
CHECKOUT10:
  code: "CHECKOUT10"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 10.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "checkout"  # Applied at checkout level
  metadata:
    campaign: "checkout-discount"
```

### Auto-Apply Checkout Coupon (site-wide)

Automatically applies to entire cart at checkout, regardless of products.

```yaml
SITE10:
  code: "SITE10"
  auto_apply: true
  discount_type: "percentage"
  discount_value: 10.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "checkout"  # Site-wide at checkout
  metadata:
    campaign: "auto-checkout"
```

---

## Advanced Stacking Examples

### Stack Multiple Percentage Discounts

These coupons stack multiplicatively (20% + 10% = 28% total discount).

```yaml
# Catalog coupon (20% off)
CATALOG20:
  code: "CATALOG20"
  auto_apply: true
  discount_type: "percentage"
  discount_value: 20.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "catalog-auto"

# Checkout coupon (10% off)
CHECKOUT10:
  code: "CHECKOUT10"
  auto_apply: true
  discount_type: "percentage"
  discount_value: 10.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "checkout"
  metadata:
    campaign: "checkout-auto"

# Result: $100 → $80 (20% off) → $72 (10% off remaining) = 28% total
```

### Stack Percentage + Fixed Discounts

Percentage applies first, then fixed amount is subtracted.

```yaml
# Percentage discount (15% off)
PERCENT15:
  code: "PERCENT15"
  auto_apply: true
  discount_type: "percentage"
  discount_value: 15.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "percentage-first"

# Fixed discount ($5 off)
FIXED5:
  code: "FIXED5"
  auto_apply: true
  discount_type: "fixed"
  discount_value: 5.0
  currency: "usd"
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "checkout"
  metadata:
    campaign: "fixed-second"

# Result: $100 → $85 (15% off) → $80 (subtract $5)
```

### Crypto Payment Incentive Stack

Combine product discount with crypto payment discount.

```yaml
# Product-specific catalog discount
PREMIUMITEM40:
  code: "PREMIUMITEM40"
  auto_apply: true
  discount_type: "percentage"
  discount_value: 40.0
  currency: ""
  scope: "specific"
  product_ids:
    - "premium-course"
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "premium-discount"

# Crypto-only checkout discount
CRYPTO5AUTO:
  code: "CRYPTO5AUTO"
  auto_apply: true
  discount_type: "percentage"
  discount_value: 5.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: "x402"
  usage_limit: null
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "checkout"
  metadata:
    campaign: "crypto-incentive"

# Result (crypto payment): $100 → $60 (40% off) → $57 (5% crypto discount)
# Result (Stripe payment): $100 → $60 (40% off, no crypto discount)
```

### Seasonal Sale with Bundle Discount

```yaml
# Time-limited site-wide sale
SUMMER2025:
  code: "SUMMER2025"
  auto_apply: true
  discount_type: "percentage"
  discount_value: 20.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: "2025-06-01T00:00:00Z"
  expires_at: "2025-08-31T23:59:59Z"
  active: true
  applies_at: "checkout"
  metadata:
    campaign: "summer-sale"

# Manual bundle code for extra savings
BUNDLE5MORE:
  code: "BUNDLE5MORE"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 5.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: null
  usage_count: 0
  starts_at: "2025-06-01T00:00:00Z"
  expires_at: "2025-08-31T23:59:59Z"
  active: true
  applies_at: "checkout"
  metadata:
    campaign: "bundle-bonus"

# Result with manual code: $100 → $80 (20% auto) → $76 (5% manual)
# Result without manual code: $100 → $80 (20% auto only)
```

---

## Special Use Cases

### Referral Program Coupon

```yaml
REFERRAL25:
  code: "REFERRAL25"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 25.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: 1  # One-time use per referral link
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "referral-program"
    referrer: "generate-dynamically"
```

### Loyalty Reward (existing customers)

```yaml
LOYAL30:
  code: "LOYAL30"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 30.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: 1
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "loyalty-reward"
    tier: "platinum"
```

### Cart Abandonment Recovery

```yaml
COMEBACK20:
  code: "COMEBACK20"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 20.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: 1
  usage_count: 0
  starts_at: null
  expires_at: "2025-12-31T23:59:59Z"
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "cart-recovery"
    trigger: "abandoned-cart-email"
```

### Influencer Collaboration Code

```yaml
INFLUENCER15:
  code: "INFLUENCER15"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 15.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: 1000
  usage_count: 0
  starts_at: "2025-11-01T00:00:00Z"
  expires_at: "2025-11-30T23:59:59Z"
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "influencer-collab"
    influencer: "@techreviewpro"
```

### Student Discount (verification required)

```yaml
STUDENT40:
  code: "STUDENT40"
  auto_apply: false
  discount_type: "percentage"
  discount_value: 40.0
  currency: ""
  scope: "all"
  product_ids: []
  payment_method: ""
  usage_limit: 1  # One-time use per verified student
  usage_count: 0
  starts_at: null
  expires_at: null
  active: true
  applies_at: "catalog"
  metadata:
    campaign: "student-program"
    verification_required: "true"
```

---

## Tips and Best Practices

### Naming Conventions

- **Descriptive codes**: `BLACKFRIDAY50` is clearer than `BF2025`
- **Include value**: `SAVE20` immediately shows 20% discount
- **Campaign tags**: Use metadata to track coupon families

### Stacking Strategy

- **Percentage first, fixed second**: System applies percentage discounts first, then fixed
- **Catalog + Checkout**: Users can combine catalog (product) and checkout (site-wide) coupons
- **Auto-apply + Manual**: Auto-apply coupons stack with manually entered codes

### Expiration Dates

- **Use UTC timezone**: Always specify times in UTC (`2025-12-31T23:59:59Z`)
- **End of day**: Set expiration to `23:59:59Z` for full last day validity
- **Flash sales**: Use precise timestamps for hour-based promotions

### Usage Limits

- **Scarcity marketing**: Limited usage creates urgency (`usage_limit: 100`)
- **Per-user limits**: Set `usage_limit: 1` for one-time codes
- **Unlimited**: Set `usage_limit: null` for ongoing promotions

### Testing Coupons

Always test your coupons with:
- Single product quotes
- Multi-item carts
- Both payment methods (Stripe + x402)
- Edge cases (expired, used up, wrong product)

---

## Need Help?

- **API Reference**: See `docs/API_REFERENCE.md` for coupon endpoints
- **Migration Guide**: See `docs/FRONTEND_MIGRATION_COUPON_PHASES.md` for frontend integration
- **Database Setup**: See `docs/DATABASE_SCHEMA.md` for PostgreSQL/MongoDB storage
