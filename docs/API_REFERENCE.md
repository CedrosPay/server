# Cedros Pay API Reference

Complete reference for all Cedros Pay server endpoints.

## Table of Contents

- [Core Endpoints](#core-endpoints)
- [AI Agent Discovery](#ai-agent-discovery)
- [Product Management](#product-management)
- [Stripe Payments](#stripe-payments)
- [x402 Crypto Payments](#x402-crypto-payments)
- [Cart Checkout](#cart-checkout)
- [Refunds](#refunds)
- [Subscriptions](#subscriptions)
- [Webhooks](#webhooks)
- [Callbacks](#callbacks)
- [Metrics & Observability](#metrics--observability)

---

## Core Endpoints

### Health Check

**GET /cedros-health**

Health check and route prefix discovery. Always unprefixed (responds at root path).

**Response:**
```json
{
  "status": "ok",
  "routePrefix": "/api",
  "version": "1.0.0"
}
```

---

## AI Agent Discovery

### MCP Resources List

**POST /resources/list**

Model Context Protocol (MCP) endpoint for AI agent resource discovery. Uses JSON-RPC 2.0 format.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "resources/list"
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "resources": [
      {
        "uri": "cedros-pay://paywall/demo-content",
        "name": "Demo protected content",
        "description": "Demo protected content",
        "mimeType": "application/json"
      }
    ]
  }
}
```

**Use Case:** AI agents (like Claude Desktop) can discover available resources via MCP standard.

**Spec:** [Model Context Protocol](https://modelcontextprotocol.io/docs/concepts/resources/)

### Well-Known Payment Options

**GET /.well-known/payment-options**

RFC 8615 well-known URI for web-based service discovery. Returns comprehensive payment configuration.

**Response:**
```json
{
  "version": "1.0",
  "server": "cedros-pay",
  "resources": [
    {
      "id": "demo-content",
      "name": "Demo protected content",
      "endpoint": "/paywall/demo-content",
      "price": {
        "fiat": {"amount": 1.0, "currency": "usd"},
        "crypto": {"amount": 1.0, "token": "USDC"}
      }
    }
  ],
  "payment": {
    "methods": ["stripe", "x402-solana-spl-transfer"],
    "x402": {
      "network": "mainnet-beta",
      "paymentAddress": "YourWalletAddress...",
      "tokenMint": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
    }
  }
}
```

**Caching:** 5-minute cache (configurable via `product_cache_ttl`)

**Spec:** [RFC 8615 - Well-Known URIs](https://tools.ietf.org/html/rfc8615)

### Agent Card

**GET /.well-known/agent.json**

Google Agent2Agent (A2A) protocol agent card for agent discovery.

**Response:**
```json
{
  "version": "1.0.0",
  "agent": {
    "name": "Cedros Pay",
    "description": "Unified payment gateway for fiat (Stripe) and crypto (x402/Solana)",
    "url": "https://your-domain.com",
    "capabilities": ["payments", "x402", "stripe"]
  }
}
```

### OpenAPI Specification

**GET /openapi.json**

Returns the OpenAPI 3.0 specification for all Cedros Pay API endpoints. Use this for SDK generation, API testing tools, and documentation.

**Response:**
```json
{
  "openapi": "3.0.0",
  "info": {
    "title": "Cedros Pay API",
    "version": "1.0.0",
    "description": "Unified payment API for Stripe and x402 (Solana)"
  },
  "paths": {
    "/paywall/v1/quote": {
      "post": {
        "summary": "Generate payment quote",
        "operationId": "generateQuote",
        ...
      }
    },
    ...
  }
}
```

**Use Cases:**
- **SDK Generation:** Use with openapi-generator to create client SDKs
  ```bash
  npx openapi-generator-cli generate \
    -i http://localhost:8080/openapi.json \
    -g typescript-axios \
    -o ./sdk
  ```
- **API Testing:** Import into Postman, Insomnia, or Thunder Client
- **Documentation:** Power Swagger UI, Redoc, or other API doc tools
  ```bash
  # View in Swagger UI
  docker run -p 8081:8080 \
    -e SWAGGER_JSON=/openapi.json \
    -v $(pwd)/openapi.json:/openapi.json \
    swaggerapi/swagger-ui
  ```
- **Validation:** Programmatically validate request/response formats

**Example Usage:**
```bash
# Download specification
curl http://localhost:8080/openapi.json > cedros-pay-api.json

# Generate Python client
openapi-generator-cli generate \
  -i cedros-pay-api.json \
  -g python \
  -o ./python-sdk

# Generate Go client
openapi-generator-cli generate \
  -i cedros-pay-api.json \
  -g go \
  -o ./go-sdk
```

---

## Product Management

### List Products

**GET {prefix}/paywall/v1/products**

Returns list of all active products with pricing information and auto-apply coupons. Cached with configurable TTL.

**Response:**
```json
{
  "products": [
    {
      "id": "demo-content",
      "description": "Demo protected content",
      "fiatAmount": 1.0,
      "effectiveFiatAmount": 1.0,
      "fiatCurrency": "usd",
      "stripePriceId": "price_123",
      "cryptoAmount": 1.0,
      "effectiveCryptoAmount": 0.97,
      "cryptoToken": "USDC",
      "hasStripeCoupon": false,
      "hasCryptoCoupon": true,
      "stripeCouponCode": "",
      "cryptoCouponCode": "CRYPTO3OFF",
      "stripeDiscountPercent": 0,
      "cryptoDiscountPercent": 3.0,
      "metadata": {
        "plan": "demo"
      }
    }
  ],
  "checkoutStripeCoupons": [
    {
      "code": "SAVE10",
      "discountType": "percentage",
      "discountValue": 10.0,
      "description": "10% off your entire cart"
    }
  ],
  "checkoutCryptoCoupons": [
    {
      "code": "FIXED5",
      "discountType": "fixed",
      "discountValue": 0.5,
      "currency": "usd",
      "description": "50 cents off (auto-applied)"
    }
  ]
}
```

**Auto-Apply Coupons:**

The response includes two types of auto-apply coupons:

1. **Catalog-Level Coupons** (applied to individual product prices):
   - Shown in each product's `effectiveFiatAmount` / `effectiveCryptoAmount`
   - Applied when products are added to cart
   - Use `hasStripeCoupon` / `hasCryptoCoupon` to check if discount is active
   - Display with badge: "3% off with crypto!" using `cryptoDiscountPercent`

2. **Checkout-Level Coupons** (applied to cart total):
   - Listed in `checkoutStripeCoupons` / `checkoutCryptoCoupons` arrays
   - Applied after all items are in cart (at checkout)
   - Display in cart summary: "FIXED5: -$0.50" or "SAVE10: -10%"
   - Auto-applied, user doesn't need to enter code

**Use Case - Product Page:**
```javascript
const product = response.products[0];
const price = paymentMethod === 'stripe'
  ? product.effectiveFiatAmount
  : product.effectiveCryptoAmount;

if (product.hasCryptoCoupon) {
  showBadge(`${product.cryptoDiscountPercent}% off with crypto!`);
}
```

**Use Case - Checkout Page:**
```javascript
const { checkoutCryptoCoupons } = response;
let cartTotal = calculateItemsTotal();

// Show which coupons will be auto-applied
checkoutCryptoCoupons.forEach(coupon => {
  const discount = coupon.discountType === 'percentage'
    ? cartTotal * (coupon.discountValue / 100)
    : coupon.discountValue;

  showDiscount(`${coupon.code}: -${formatMoney(discount)}`);
  cartTotal -= discount;
});
```

#### Caching & Cache Invalidation

**Cache Configuration:**
```yaml
paywall:
  product_source: postgres     # or mongodb, yaml
  product_cache_ttl: 5m        # Cache duration (default: 5 minutes)
```

**How Cache Invalidation Works:**

Product data is cached using **time-based expiration only**. Database updates become visible after the configured TTL expires.

**Timeline Example:**
```
1. Developer adds new product to database
   → Product not visible in /products endpoint yet

2. Wait for product_cache_ttl to expire (e.g., 5 minutes)
   → Cache entry expires

3. Next request to /products endpoint
   → Server fetches fresh data from database
   → New product now visible to clients
```

**Cache Behavior by Data Source:**

| Data Source | Cache Behavior | Update Visibility |
|-------------|----------------|-------------------|
| `yaml` | Loaded at startup | Requires server restart |
| `postgres` | Cached with TTL | Visible after TTL expires |
| `mongodb` | Cached with TTL | Visible after TTL expires |

**Force Immediate Updates:**

**Option 1: Disable Caching** (not recommended for production)
```yaml
paywall:
  product_cache_ttl: 0s  # No cache - always fetch fresh from database
```
⚠️ **Warning:** Increases database load. Every `/products` request hits the database.

**Option 2: Restart Server**
```bash
# Kubernetes
kubectl rollout restart deployment cedros-pay

# Docker Compose
docker-compose restart server

# Systemd
systemctl restart cedros-pay
```
Cache is cleared on startup, forcing immediate reload from database.

**Option 3: Use Shorter TTL**
```yaml
paywall:
  product_cache_ttl: 30s  # Updates visible within 30 seconds
```
Balance between freshness and database load.

**Production Recommendations:**

✅ **Recommended:** `product_cache_ttl: 5m`
- Good balance of freshness and performance
- Database updates visible within 5 minutes
- Low database load (12 queries/hour per server)

⚠️ **High-Frequency Updates:** `product_cache_ttl: 1m`
- Use if products change frequently (e.g., flash sales)
- Moderate database load (60 queries/hour per server)

❌ **Not Recommended:** `product_cache_ttl: 0s`
- Real-time updates but high database load
- Only use with read replicas and connection pooling
- Consider Redis cache instead for high-traffic scenarios

**Monitoring Cache Performance:**
```yaml
# Enable metrics to track cache hit rate
metrics:
  enabled: true
```

Check Prometheus metrics:
- `cedros_product_cache_hits` - Requests served from cache
- `cedros_product_cache_misses` - Requests that fetched from database

**Example:** If hit rate is 95%+ with `5m` TTL, cache is working well.

---

## Stripe Payments

### Create Stripe Session (Single Item)

**POST {prefix}/paywall/v1/stripe-session**

Create Stripe checkout session for a single product.

**Request:**
```json
{
  "resource": "demo-content",
  "customerEmail": "user@example.com",
  "metadata": {
    "user_id": "12345"
  }
}
```

**Response:**
```json
{
  "sessionId": "cs_test_...",
  "url": "https://checkout.stripe.com/..."
}
```

**Use Case:** Single-item purchases (one article, one course, one API credit package)

### Verify Stripe Session

**GET {prefix}/paywall/v1/stripe-session/verify?session_id={session_id}**

Verify that a Stripe checkout session was completed and paid. This endpoint prevents payment bypass attacks where users manually enter success URLs without paying.

**Security:** Frontend should call this endpoint before granting access to purchased content. Simply receiving a `session_id` in the URL is NOT proof of payment.

**Query Parameters:**
- `session_id` (required): Stripe checkout session ID from redirect URL

**Success Response (200):**
```json
{
  "verified": true,
  "resource_id": "demo-content",
  "paid_at": "2025-01-15T10:30:00Z",
  "amount": "$1.00 USD",
  "customer": "cus_abc123",
  "metadata": {
    "user_id": "12345",
    "item_name": "Premium Article"
  }
}
```

**Error Response (404):**
```json
{
  "error": {
    "code": "session_not_found",
    "message": "Payment not completed or session invalid"
  }
}
```

**Integration Flow:**
```javascript
// 1. User completes Stripe payment
// 2. Stripe redirects to: your-site.com/success?session_id=cs_test_abc123
// 3. Frontend calls verification endpoint
const response = await fetch(
  `/paywall/v1/stripe-session/verify?session_id=${sessionId}`
);

if (response.ok) {
  const data = await response.json();
  if (data.verified) {
    // Payment confirmed! Grant access to content
    window.open('/protected-content.pdf', '_blank');
  }
} else {
  // Payment not verified - show error
  showError('Payment verification failed');
}
```

**Use Case:** Secure content delivery after Stripe payment. Prevents users from bypassing payment by manually entering success URLs.

### Stripe Success Page

**GET {prefix}/stripe/success**

Redirect page after successful Stripe payment. Query parameter: `session_id`

**Note:** This is a fallback page. For custom experiences, pass `successUrl` when creating the session and use the verification endpoint above to confirm payment.

### Stripe Cancel Page

**GET {prefix}/stripe/cancel**

Redirect page when user cancels Stripe checkout. Query parameter: `session_id`

---

## x402 Crypto Payments

### Get Payment Quote

**POST {prefix}/paywall/v1/quote**

Generate x402 payment quote for a resource. Resource ID is passed in body to prevent leakage in URLs.

**Request:**
```json
{
  "resource": "demo-content",
  "couponCode": "SAVE20"
}
```

**Response (HTTP 402):**
```json
{
  "scheme": "solana-spl-transfer",
  "network": "mainnet-beta",
  "maxAmountRequired": "184000",
  "resource": "demo-content",
  "description": "Demo protected content",
  "mimeType": "application/json",
  "payTo": "YourTokenAccount...",
  "maxTimeoutSeconds": 300,
  "asset": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
  "extra": {
    "recipientTokenAccount": "...",
    "decimals": 6,
    "tokenSymbol": "USDC",
    "memo": "demo-content:abc123",
    "feePayer": "ServerWallet...",
    "original_amount": "1.000000",
    "discounted_amount": "0.184000",
    "applied_coupons": "PRODUCT20,SITE10,CRYPTO5AUTO,FIXED5",
    "catalog_coupons": "PRODUCT20",
    "checkout_coupons": "SITE10,CRYPTO5AUTO,FIXED5"
  }
}
```

**Response Fields:**
- `maxAmountRequired`: Final price in atomic units after ALL coupons (catalog + checkout)
- `extra.original_amount`: Price before any discounts (only present if coupons applied)
- `extra.discounted_amount`: Final price after all discounts (only present if coupons applied)
- `extra.applied_coupons`: Comma-separated list of all applied coupon codes
- `extra.catalog_coupons`: Product-specific coupons (shown on product page)
- `extra.checkout_coupons`: Site-wide coupons (applied at cart/checkout)

**Note:** For single product quotes, both catalog AND checkout coupons are applied immediately since there's no separate cart step. The single product IS the cart.

**To Complete Payment:**
1. Frontend builds Solana transaction using quote details
2. User signs and submits transaction
3. Frontend sends proof via `X-PAYMENT` header to verify endpoint
4. Server verifies transaction and grants access

### Verify Payment

**POST {prefix}/paywall/v1/verify**

Verify payment proof for any resource type (regular, cart, or refund). Resource ID and type are extracted from the X-PAYMENT header payload.

**X-PAYMENT Header Format:**
```json
{
  "x402Version": 0,
  "scheme": "solana-spl-transfer",
  "network": "mainnet-beta",
  "payload": {
    "signature": "transaction_signature",
    "transaction": "base64_encoded_transaction",
    "payer": "wallet_address",
    "resource": "demo-content",
    "resourceType": "regular"
  }
}
```

**Response (HTTP 200):**
```json
{
  "success": true,
  "message": "Payment verified",
  "method": "x402",
  "wallet": "user_wallet_address",
  "signature": "transaction_signature",
  "settlement": {
    "success": true,
    "txHash": "signature...",
    "networkId": "mainnet-beta"
  }
}
```

**Resource Types:**
- `"regular"` - Standard single-item payments
- `"cart"` - Multi-item cart payments
- `"refund"` - Refund verification

**Note:** This endpoint consumes the transaction signature. Each signature can only be verified once to prevent replay attacks.

### Verify x402 Transaction (Re-Access)

**GET {prefix}/paywall/v1/x402-transaction/verify?signature={signature}**

Verify that an x402 transaction was previously completed and paid. This endpoint allows re-access verification for already-paid transactions.

**Use Case:** User paid with crypto, stored transaction signature, and now wants to re-access content without paying again.

**Query Parameters:**
- `signature` (required): Solana transaction signature from original payment

**Success Response (200):**
```json
{
  "verified": true,
  "resource_id": "demo-content",
  "wallet": "2TRi...",
  "paid_at": "2025-01-15T10:30:00Z",
  "amount": "$1.00 USDC",
  "metadata": {
    "userId": "12345",
    "email": "user@example.com"
  }
}
```

**Error Response (404):**
```json
{
  "error": {
    "code": "transaction_not_found",
    "message": "Transaction not found or not verified"
  }
}
```

**Integration Flow:**
```javascript
// 1. User completes payment and stores signature
const signature = txResult.signature;
localStorage.setItem('ebook_tx', signature);

// 2. User returns later, retrieve stored signature
const savedTx = localStorage.getItem('ebook_tx');
const currentWallet = await getConnectedWallet();

// 3. Verify transaction is still valid
const response = await fetch(
  `/paywall/v1/x402-transaction/verify?signature=${savedTx}`
);

if (response.ok) {
  const data = await response.json();

  // 4. Check if wallet matches (frontend logic)
  if (data.wallet === currentWallet) {
    // Same wallet that paid - grant re-access
    window.open('/ebook.pdf', '_blank');
  } else {
    // Different wallet - require new payment
    showMessage('This content was purchased by a different wallet');
  }
}
```

**Security:** Frontend should verify that `data.wallet` matches the currently connected wallet before granting access. This prevents sharing transaction signatures between users.

### Build Gasless Transaction

**POST {prefix}/paywall/v1/gasless-transaction**

Builds an unsigned Solana transaction for gasless payments. The server acts as fee payer, allowing users to make payments without SOL for gas fees.

**Request:**
```json
{
  "resourceId": "demo-content",
  "userWallet": "user_wallet_address",
  "feePayer": "server_wallet_address",
  "couponCode": "SAVE20"
}
```

**Request Fields:**
- `resourceId` (required): Resource ID or cart ID (e.g., `cart_abc123`)
- `userWallet` (required): User's wallet address (signs the transfer)
- `feePayer` (optional): Specific server wallet to use as fee payer (from quote's `extra.feePayer`)
- `couponCode` (optional): Coupon code for discount

**Response:**
```json
{
  "transaction": "base64_encoded_unsigned_transaction",
  "feePayer": "server_wallet_that_pays_fees",
  "message": "Partially sign this transaction and send it back for verification"
}
```

**Gasless Payment Flow:**
1. **Get Quote**: `POST /paywall/quote` returns quote with `feePayer` in `extra`
2. **Build Transaction**: `POST /paywall/gasless-transaction` with user wallet and feePayer
3. **User Signs**: Frontend partially signs transaction (transfer authority only)
4. **Verify**: Send signed transaction via `X-PAYMENT` header to `POST /paywall/verify`
5. **Server Co-Signs**: Server adds fee payer signature and submits transaction
6. **Confirmation**: Server waits for on-chain confirmation and returns success

**Benefits:**
- Users don't need SOL for transaction fees
- Server pays all gas costs
- Seamless UX for crypto payments

### Validate Coupon

**POST {prefix}/paywall/v1/coupons/validate**

Validates a coupon code and returns discount information.

**Request:**
```json
{
  "code": "SAVE20",
  "productIds": ["demo-item-id-1", "demo-item-id-2"],
  "paymentMethod": "stripe"
}
```

**Request Fields:**
- `code` (required): Coupon code to validate
- `productIds` (optional): Array of product IDs to check if coupon applies. If omitted:
  - `scope: "all"` coupons are considered valid for all products
  - `scope: "specific"` coupons return their configured product list in `applicableProducts`
- `paymentMethod` (optional): Payment method to validate against ("stripe", "x402", or omit for any)

**Response (Valid - Site-Wide Coupon):**
```json
{
  "valid": true,
  "code": "SAVE20",
  "discountType": "percentage",
  "discountValue": 20.0,
  "scope": "all",
  "applicableProducts": null,
  "paymentMethod": "",
  "expiresAt": "2025-12-31T23:59:59Z",
  "remainingUses": 99500
}
```

**Response (Valid - Product-Specific Coupon):**
```json
{
  "valid": true,
  "code": "DEMO50",
  "discountType": "percentage",
  "discountValue": 50.0,
  "scope": "specific",
  "applicableProducts": ["demo-item-id-1"],
  "paymentMethod": "stripe",
  "expiresAt": "2025-12-31T23:59:59Z"
}
```

**Response (Invalid):**
```json
{
  "valid": false,
  "error": "Coupon not found"
}
```

**Response Fields:**
- `valid`: Whether coupon is valid for use
- `code`: Coupon code
- `discountType`: "percentage" or "fixed"
- `discountValue`: Discount amount (0-100 for percentage, fixed amount for fixed)
- `scope`: "all" (applies to all products/carts) or "specific" (limited products)
- `applicableProducts`: List of products that coupon applies to. Behavior depends on request:
  - If `productIds` provided: products from request that matched
  - If no `productIds` + `scope: "all"`: `null` (applies to everything)
  - If no `productIds` + `scope: "specific"`: configured product list from coupon
- `paymentMethod`: Payment method restriction ("", "stripe", or "x402"). Empty string means applies to any payment method
- `expiresAt`: ISO 8601 expiration timestamp
- `remainingUses`: Number of uses remaining (only for database-backed coupons)

**Important Notes:**
- When `scope: "all"`, coupon applies to ALL products and carts
- When `scope: "specific"`, coupon only applies to products in `productIds` config
- For cart purchases, **individual product restrictions are ignored** and discount applies to entire cart total
- `remainingUses` only returned for Postgres/MongoDB coupons (not YAML)

### Coupon Stacking

**ALL matching coupons stack together** (since v1.x):

1. **Auto-apply coupons**: ALL matching auto-apply coupons are applied automatically
2. **Manual coupon**: User can add ONE additional manual coupon on top
3. **Duplicate prevention**: If manual coupon code matches an auto-apply coupon, it's only counted once
4. **Payment method filter**: Coupons only apply if they match the payment method (Stripe/x402/any)

**Stacking Order (percentage-first strategy):**
1. Apply ALL percentage discounts multiplicatively
2. Then apply ALL fixed discounts additively

**Example:**
```
Original price: $100
Auto-apply coupons: SITE10 (10% off), CRYPTO5 (5% off, x402 only)
Manual coupon: SAVE20 (20% off)

For x402 payment:
Step 1: Apply percentages: 100 * 0.9 * 0.95 * 0.8 = $68.40
Step 2: No fixed discounts
Final price: $68.40

For Stripe payment:
Step 1: Apply percentages: 100 * 0.9 * 0.8 = $72.00 (CRYPTO5 doesn't apply)
Step 2: No fixed discounts
Final price: $72.00
```

### Two-Phase Coupon System

**Auto-apply coupons are separated into two phases for better UX** (since v2.x):

**Phase 1: Catalog Coupons** (`applies_at: catalog`)
- Product-specific discounts shown on product pages
- Must have `scope: specific` and `product_ids` configured
- Applied to individual product quotes
- Frontend displays discounted price immediately

**Phase 2: Checkout Coupons** (`applies_at: checkout`)
- Site-wide discounts applied at cart/checkout only
- Must have `scope: all`
- Applied to cart total after catalog coupons
- Frontend shows additional discount at checkout

**Example Flow:**
```yaml
# Config
coupons:
  PRODUCT20:  # Catalog coupon
    applies_at: catalog
    scope: specific
    product_ids: ["item-1"]
    discount_value: 20
    auto_apply: true

  SITE10:  # Checkout coupon
    applies_at: checkout
    scope: all
    discount_value: 10
    auto_apply: true
```

```
Product Page (item-1):
  Original: $10.00
  PRODUCT20 applied: $8.00  ← User sees this immediately

Cart:
  Item 1: $8.00 (catalog discount preserved)
  Item 2: $5.00 (no catalog discount)
  Subtotal: $13.00
  SITE10 applied: -$1.30  ← Applied at checkout
  Final Total: $11.70
```

**Quote Metadata:**

Single product quotes include coupon info in `extra` field:
```json
{
  "crypto": {
    "maxAmountRequired": "800000",
    "extra": {
      "original_amount": "10.000000",
      "discounted_amount": "8.000000",
      "applied_coupons": "PRODUCT20"
    }
  }
}
```

Cart quotes include breakdown in metadata:
```json
{
  "metadata": {
    "catalog_coupons": "PRODUCT20",
    "checkout_coupons": "SITE10",
    "subtotal_after_catalog": "13.000000",
    "discounted_amount": "11.700000"
  }
}
```

**Frontend Integration:**
- Check `extra.original_amount` for product page pricing
- Display strikethrough on original price if coupon applied
- Show catalog vs checkout coupon breakdown in cart
- See [FRONTEND_MIGRATION_COUPON_PHASES.md](./FRONTEND_MIGRATION_COUPON_PHASES.md) for complete guide

**Backward Compatibility:**
Existing coupons without `applies_at` continue to work. For Stripe payments, all coupons (catalog + checkout) are applied together since Stripe checkout is single-step.

---

## Cart Checkout

### Create Cart Checkout (Stripe)

**POST {prefix}/paywall/v1/cart/checkout**

Create Stripe checkout session for multiple products in a single transaction.

**Request:**
```json
{
  "items": [
    {
      "priceId": "price_1SPqRpR4HtkFbUJKUciKecmZ",
      "resource": "demo-content",
      "quantity": 1,
      "metadata": {
        "credits": "100",
        "product_type": "digital"
      }
    },
    {
      "priceId": "price_1SQCuhR4HtkFbUJKDUQpCA6D",
      "resource": "test-product-2",
      "quantity": 2
    }
  ],
  "customerEmail": "user@example.com",
  "couponCode": "SAVE20",
  "stripeCouponId": "SAVE20",
  "metadata": {
    "user_id": "12345",
    "order_type": "bundle"
  }
}
```

**Request Fields:**
- `items` (required): Array of cart line items with `priceId`, `resource`, `quantity`, and optional `metadata`
- `customerEmail` (optional): Customer email for Stripe checkout
- `couponCode` (optional): Internal coupon code for tracking (e.g., "SAVE20")
- `stripeCouponId` (optional): Stripe promotion code ID to apply native Stripe discount
- `metadata` (optional): Custom metadata attached to the session

**Response:**
```json
{
  "sessionId": "cs_test_...",
  "url": "https://checkout.stripe.com/...",
  "totalItems": 2,
  "cartId": "cart_abc123"
}
```

**When to Use:**
- Multiple credit packages at once
- Product bundles (course + workbook)
- Bulk purchases (10x API calls)

**Note:** Uses Stripe Price IDs directly (not resource IDs from paywall config)

**Webhook Metadata:**

When cart checkout completes, webhook receives all item details:

```json
{
  "cart_items": "2",
  "total_quantity": "3",
  "item_0_price_id": "price_1SPqRpR4HtkFbUJKUciKecmZ",
  "item_0_resource": "demo-content",
  "item_0_quantity": "1",
  "item_0_credits": "100",
  "item_1_price_id": "price_1SQCuhR4HtkFbUJKDUQpCA6D",
  "item_1_resource": "test-product-2",
  "item_1_quantity": "2",
  "user_id": "12345",
  "order_type": "bundle",
  "coupon_codes": "SITE10,SAVE20",
  "original_amount": "100.00",
  "discounted_amount": "72.00"
}
```

**Note:** `coupon_codes` contains comma-separated list of all applied coupons (auto-apply + manual)

### Request Cart Quote (x402)

**POST {prefix}/paywall/v1/cart/quote**

Generate x402 quote for multiple items with price locking. Cart expires after 15 minutes.

**Request:**
```json
{
  "items": [
    {
      "resource": "demo-content",
      "quantity": 2,
      "metadata": {"credits": "100"}
    },
    {
      "resource": "premium-post",
      "quantity": 1,
      "metadata": {"credits": "500"}
    }
  ],
  "couponCode": "SAVE20",
  "metadata": {
    "user_id": "12345"
  }
}
```

**Request Fields:**
- `items` (required): Array of cart items with `resource`, `quantity`, and optional `metadata`
- `couponCode` (optional): Discount code applied to entire cart total (e.g., "SAVE20" for 20% off)
- `metadata` (optional): Custom metadata attached to the cart quote

**Coupon Behavior:**
- Applies to **entire cart total** (not per-item)
- Individual product restrictions are ignored for cart coupons
- Invalid coupons are silently ignored (no error thrown)
- Discount details stored in cart metadata for verification

**Response:**
```json
{
  "cartId": "cart_abc123",
  "quote": {
    "scheme": "solana-spl-transfer",
    "network": "mainnet-beta",
    "maxAmountRequired": "2766100",
    "resource": "cart_abc123",
    "description": "Cart purchase (2.7661 USDC)",
    "payTo": "YourTokenAccount...",
    "asset": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
    "extra": {
      "recipientTokenAccount": "...",
      "decimals": 6,
      "tokenSymbol": "USDC",
      "memo": "cart:cart_abc123"
    }
  },
  "items": [
    {
      "resource": "demo-content",
      "quantity": 2,
      "originalPrice": 1.0,
      "priceAmount": 0.8,
      "token": "USDC",
      "description": "Demo protected content",
      "appliedCoupons": ["PRODUCT20"]
    },
    {
      "resource": "premium-post",
      "quantity": 1,
      "originalPrice": 2.22,
      "priceAmount": 2.22,
      "token": "USDC",
      "description": "Premium post access",
      "appliedCoupons": []
    }
  ],
  "totalAmount": 2.7661,
  "metadata": {
    "catalog_coupons": "PRODUCT20",
    "checkout_coupons": "SITE10,CRYPTO5AUTO,FIXED5",
    "subtotal_after_catalog": "3.820000",
    "discounted_amount": "2.766100",
    "coupon_codes": "PRODUCT20,SITE10,CRYPTO5AUTO,FIXED5"
  },
  "expiresAt": "2025-11-07T12:30:00Z"
}
```

**Response Fields:**
- `cartId`: Unique cart identifier for payment verification
- `quote`: x402 payment requirement for the cart total
- `items`: Array of cart items with per-item pricing breakdown
  - `originalPrice`: Price before any discounts
  - `priceAmount`: Final price after catalog coupons applied
  - `appliedCoupons`: Array of catalog coupon codes applied to this specific item
- `totalAmount`: Final cart total after all discounts (catalog + checkout)
- `metadata`: Coupon breakdown showing which coupons were applied at each phase
  - `catalog_coupons`: Product-specific coupons applied at item level
  - `checkout_coupons`: Site-wide coupons applied to cart total
  - `subtotal_after_catalog`: Subtotal before checkout coupons
  - `discounted_amount`: Final total after all discounts
- `expiresAt`: Cart quote expiration timestamp

**Note:** Cart payment verification uses the unified `POST /paywall/verify` endpoint with `resourceType: "cart"` in the X-PAYMENT header payload.

---

## Refunds

### Request Refund

**POST {prefix}/paywall/v1/refunds/request**

Submit a refund request for admin review. Creates a pending refund request in the system.

**Request:**
```json
{
  "originalPurchaseId": "5vN7Z...transaction-sig...8xQ2",
  "recipientWallet": "CustomerWallet...",
  "amount": 10.5,
  "token": "USDC",
  "reason": "customer request",
  "metadata": {
    "support_ticket": "12345",
    "refund_type": "full"
  }
}
```

**Response:**
```json
{
  "refundId": "refund_10ef9eb18f21b12bcf7a38f8280797c4",
  "status": "pending",
  "originalPurchaseId": "5vN7Z...transaction-sig...8xQ2",
  "recipientWallet": "CustomerWallet...",
  "amount": 10.5,
  "token": "USDC",
  "reason": "customer request",
  "createdAt": "2025-11-08T08:21:06-08:00",
  "message": "Refund request submitted successfully. An admin will review and process your request."
}
```

**Important:**
- This endpoint creates a **pending refund request**, NOT an executable quote
- The x402 quote is generated when admin approves (POST /paywall/refund-approve)
- Refund requests never auto-expire - they stay pending until approved/denied by admin
- Only the configured `paymentAddress` (payTo wallet) can execute refunds
- **Gasless mode is NOT used** - payTo wallet pays both refund amount AND network fees

**Security (7 Validation Layers):**
1. Valid Solana transaction signature format
2. Payment exists in backend records (not just any Solana signature)
3. Recipient wallet matches original payer's wallet
4. Refund amount ≤ original payment amount (prevents over-refunding)
5. Refund token matches original payment token (prevents token swaps)
6. Cryptographic signature authentication (user OR admin)
7. One refund request per transaction (prevents duplicates)

**Workflow:**
1. User/Admin: POST /paywall/v1/refunds/request → Creates pending request (status: "pending")
2. Admin: POST /paywall/v1/refunds/pending → Lists all pending refund requests
3. Admin decides: Approve or Deny
4. **If Approve**: POST /paywall/v1/refunds/approve → Generates x402 quote for admin to execute
5. Admin builds Solana transaction using quote
6. Admin signs transaction with payTo wallet and submits to Solana network
7. Admin calls POST /paywall/v1/verify with transaction signature (resourceType: "refund")
8. Server confirms on-chain, marks refund as processed, fires RefundSucceeded callback
9. **If Deny**: POST /paywall/v1/refunds/deny → Deletes refund request

See [REFUND_FLOW.md](../REFUND_FLOW.md) for complete workflow documentation.

---

### Get Fresh Refund Quote

**POST {prefix}/paywall/v1/refunds/approve**

Regenerates x402 quote for existing refund request. Used when original quote expires (after 15 min).

**Request:**
```json
{
  "refundId": "refund_abc123..."
}
```

**Response:**
```json
{
  "refundId": "refund_abc123...",
  "quote": {
    "scheme": "solana-spl-transfer",
    "network": "mainnet-beta",
    "maxAmountRequired": "10500000",
    "resource": "refund_abc123...",
    "description": "Refund (10.5 USDC)",
    "payTo": "CustomerWallet...",
    "asset": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
    "extra": {
      "recipientTokenAccount": "CustomerTokenAccount...",
      "decimals": 6,
      "tokenSymbol": "USDC",
      "memo": "cedros:refund:refund_abc123..."
    }
  },
  "expiresAt": "2025-11-07T12:30:00Z"
}
```

**Error Responses:**

**404 Not Found** - Refund doesn't exist:
```json
{
  "error": "refund not found"
}
```

**409 Conflict** - Refund already processed:
```json
{
  "error": "refund already processed"
}
```

**Use Cases:**
- Original quote expired (blockhash stale after 15 min)
- Admin needs fresh quote to approve refund
- Refund request sat pending for longer than 15 min

**Important:**
- Refund request persists in database until approved/denied
- This endpoint generates fresh quote with new 15-min expiry window
- Updates ExpiresAt field in database
- Only works for unprocessed refunds

---

### Verify Refund Execution

**POST {prefix}/paywall/v1/verify**

Verify refund transaction using the unified verify endpoint with `resourceType: "refund"`.

**X-PAYMENT Header:**
```json
{
  "x402Version": 0,
  "scheme": "solana-spl-transfer",
  "network": "mainnet-beta",
  "payload": {
    "signature": "refund_transaction_signature",
    "transaction": "base64_encoded_transaction",
    "payer": "ServerWallet...",
    "resource": "refund_abc123...",
    "resourceType": "refund"
  }
}
```

**Response (HTTP 200):**
```json
{
  "success": true,
  "message": "Refund verified for refund_abc123...",
  "refundId": "refund_abc123...",
  "method": "x402-refund",
  "settlement": {
    "success": true,
    "txHash": "signature...",
    "networkId": "mainnet-beta"
  }
}
```

**Verification Checks:**
1. ✅ Refund quote exists and not expired
2. ✅ Refund not already processed
3. ✅ Payer is the configured payment address (server wallet)
4. ✅ Recipient matches original refund quote
5. ✅ Amount matches exactly
6. ✅ Transaction confirmed on-chain

**After Verification:**
- Refund marked as processed in storage
- `RefundSucceeded` callback fired with refund details

### List Pending Refunds

**POST {prefix}/paywall/v1/refunds/pending**

Retrieve all pending refund requests. Requires admin authentication.

**Response:**
```json
[
  {
    "refundId": "refund_abc123",
    "originalPurchaseId": "tx_sig_123",
    "recipientWallet": "CustomerWallet...",
    "amount": 10.5,
    "token": "USDC",
    "reason": "customer request",
    "createdAt": "2025-11-08T08:21:06Z",
    "status": "pending"
  }
]
```

---

### Generate Admin Nonce

**POST {prefix}/paywall/v1/nonce**

Generate a unique nonce for replay protection in admin operations.

**Response:**
```json
{
  "nonce": "abc123def456"
}
```

**Use Case:** Admin operations requiring one-time authentication tokens.

---

### Deny Refund Request

**POST {prefix}/paywall/v1/refunds/deny**

Deny or cancel a pending refund request. Requires wallet signature from the configured payTo wallet.

**Authentication:** Wallet signature (not traditional API keys)

**Required Headers:**
```
X-Signature: <base64-encoded Ed25519 signature>
X-Message: deny-refund:{refundId}
X-Signer: <base58 wallet address>
```

**Request:**
```json
{
  "refundId": "refund_abc123..."
}
```

**Request Example:**
```bash
POST /paywall/v1/refunds/deny
Content-Type: application/json
X-Signature: 5K9pN3...base64encoded...
X-Message: deny-refund:refund_abc123
X-Signer: YourPayToWalletAddress...

{
  "refundId": "refund_abc123..."
}
```

**Response (HTTP 200):**
```json
{
  "success": true,
  "message": "refund denied"
}
```

**Error Responses:**

**401 Unauthorized** - Missing signature headers:
```json
{
  "error": "signature required: include X-Signature, X-Message, and X-Signer headers"
}
```

**403 Forbidden** - Wrong wallet (not payTo address):
```json
{
  "error": "unauthorized: only payment address can deny refunds"
}
```

**401 Unauthorized** - Invalid signature:
```json
{
  "error": "signature verification failed"
}
```

**400 Bad Request** - Wrong message format:
```json
{
  "error": "invalid message format",
  "expectedMessage": "deny-refund:refund_abc123"
}
```

**404 Not Found** - Refund doesn't exist:
```json
{
  "error": "refund not found"
}
```

**409 Conflict** - Refund already processed (cannot be denied):
```json
{
  "error": "cannot deny already processed refund"
}
```

**Authentication Flow:**

1. Admin connects wallet (must be the configured payTo wallet)
2. Frontend creates message: `deny-refund:{refundId}`
3. User signs message with Solana wallet
4. Frontend sends signature + message + signer in headers
5. Backend verifies signature and checks signer matches payTo address

**Use Case:**
- Admin cancels invalid refund requests
- Fraudulent refund attempt prevention
- Customer withdraws refund request
- Refund request expired before processing

**Important:**
- Only payTo wallet can deny refunds (verified via signature)
- Only unprocessed refunds can be denied
- Already processed refunds cannot be deleted (returns 409)
- Refund is permanently deleted from storage
- No callback is fired for denied refunds
- No complex auth system needed - uses wallet signatures

---

## Subscriptions

Cedros Pay supports recurring subscriptions via both Stripe and x402 crypto payments. Subscriptions can be managed through these endpoints.

### Check Subscription Status

**GET {prefix}/paywall/v1/subscription/status**

Check if a user has an active subscription for a resource.

**Query Parameters:**
- `resource` (required): The plan/resource ID
- `userId` (required): User identifier (wallet address for crypto, email/customer ID for Stripe)

**Example:** `GET /paywall/v1/subscription/status?resource=plan-pro&userId=BYNhM2C7hRdqY8PAkBGvxnVkKvHPRAqCGbFYk2aQePWg`

**Success Response (200 OK):**
```json
{
  "active": true,
  "status": "active",
  "expiresAt": "2025-12-31T23:59:59Z",
  "currentPeriodEnd": "2025-01-31T23:59:59Z",
  "interval": "monthly",
  "cancelAtPeriodEnd": false
}
```

**No Subscription Response (200 OK):**
```json
{
  "active": false,
  "status": "expired"
}
```

**Status Values:**
- `active` - Subscription is current and paid
- `trialing` - User is in trial period
- `past_due` - Payment failed but within grace period
- `canceled` - User canceled, access until period end
- `unpaid` - Payment failed, beyond grace period
- `expired` - Subscription has ended

**Notes:**
- For crypto subscriptions, `userId` is the wallet's public key (base58 string)
- For Stripe subscriptions, `userId` can be email or Stripe customer ID
- Returns `active: false` if no subscription exists (don't return 404)

---

### Create Stripe Subscription Session

**POST {prefix}/paywall/v1/subscription/stripe-session**

Creates a Stripe Checkout session for subscription signup.

**Request Headers:**
```
Content-Type: application/json
Idempotency-Key: <uuid>  // For safe retries
```

**Request Body:**
```json
{
  "resource": "plan-pro",
  "interval": "monthly",
  "intervalDays": 45,
  "trialDays": 14,
  "customerEmail": "user@example.com",
  "metadata": {
    "userId": "user-123",
    "source": "pricing-page"
  },
  "couponCode": "SAVE20",
  "successUrl": "https://example.com/success",
  "cancelUrl": "https://example.com/cancel"
}
```

**Request Fields:**
- `resource` (required): Plan/resource ID from paywall config
- `interval` (required): `"weekly"` | `"monthly"` | `"yearly"` | `"custom"`
- `intervalDays` (optional): Only used when interval is `"custom"`
- `trialDays` (optional): Number of trial days (0 for no trial)
- `customerEmail` (optional): Pre-fills Stripe checkout
- `metadata` (optional): Custom metadata for tracking
- `couponCode` (optional): Coupon code for discount
- `successUrl` (optional): Redirect URL on success
- `cancelUrl` (optional): Redirect URL on cancel

**Success Response (200 OK):**
```json
{
  "sessionId": "cs_test_a1b2c3d4...",
  "url": "https://checkout.stripe.com/c/pay/cs_test_a1b2c3d4..."
}
```

**Error Response (400/500):**
```json
{
  "error": "Invalid plan",
  "code": "invalid_resource"
}
```

---

### Request Subscription Quote (x402)

**POST {prefix}/paywall/v1/subscription/quote**

Returns a payment quote for a crypto subscription. Follows x402 protocol.

**Request Body:**
```json
{
  "resource": "plan-pro",
  "interval": "monthly",
  "couponCode": "CRYPTO10",
  "intervalDays": 45
}
```

**Response (402 Payment Required):**
```json
{
  "requirement": {
    "scheme": "solana-spl-transfer",
    "network": "mainnet-beta",
    "maxAmountRequired": "10000000",
    "resource": "plan-pro",
    "description": "Pro Plan Monthly Subscription",
    "mimeType": "application/json",
    "payTo": "BYNhM2C7hRdqY8PAkBGvxnVkKvHPRAqCGbFYk2aQePWg",
    "maxTimeoutSeconds": 300,
    "asset": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
    "extra": {
      "recipientTokenAccount": "...",
      "decimals": 6,
      "tokenSymbol": "USDC",
      "memo": "sub:plan-pro:monthly",
      "feePayer": "..."
    }
  },
  "subscription": {
    "interval": "monthly",
    "intervalDays": 30,
    "durationSeconds": 2592000,
    "periodStart": "2025-01-01T00:00:00Z",
    "periodEnd": "2025-01-31T23:59:59Z"
  }
}
```

**Response Fields:**
- `requirement`: Standard x402 payment requirement
- `subscription.interval`: Billing interval
- `subscription.durationSeconds`: How long this payment covers
- `subscription.periodStart/periodEnd`: When subscription period starts/ends

**Payment Flow:**
1. UI gets quote from `/subscription/quote`
2. User signs transaction in wallet
3. UI submits payment to `/paywall/v1/verify` with `X-PAYMENT` header
4. Backend verifies payment and creates/extends subscription
5. Backend returns success response

---

### Cancel Subscription

**POST {prefix}/paywall/v1/subscription/cancel**

Cancel an active subscription. Subscription remains active until current period ends.

**Request Body:**
```json
{
  "resource": "plan-pro",
  "userId": "BYNhM2C7hRdqY8PAkBGvxnVkKvHPRAqCGbFYk2aQePWg"
}
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "message": "Subscription will cancel at period end",
  "cancelAtPeriodEnd": true,
  "currentPeriodEnd": "2025-01-31T23:59:59Z"
}
```

---

### Get Billing Portal

**POST {prefix}/paywall/v1/subscription/portal**

Get Stripe billing portal URL for managing subscription (update payment method, view invoices, cancel).

**Request Body:**
```json
{
  "customerId": "cus_abc123"
}
```

**Success Response (200 OK):**
```json
{
  "url": "https://billing.stripe.com/p/session/..."
}
```

---

### Activate x402 Subscription

**POST {prefix}/paywall/v1/subscription/x402/activate**

Activate a subscription after successful x402 crypto payment. Called internally after payment verification.

**Request Body:**
```json
{
  "resource": "plan-pro",
  "userId": "BYNhM2C7hRdqY8PAkBGvxnVkKvHPRAqCGbFYk2aQePWg",
  "interval": "monthly",
  "signature": "5vN7Z...transaction-sig..."
}
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "subscriptionId": "sub_abc123",
  "status": "active",
  "currentPeriodEnd": "2025-01-31T23:59:59Z"
}
```

---

### Change Subscription (Upgrade/Downgrade)

**POST {prefix}/paywall/v1/subscription/change**

Upgrade or downgrade a subscription to a different plan.

**Request Body:**
```json
{
  "subscriptionId": "sub_abc123",
  "newResource": "plan-enterprise",
  "prorationBehavior": "create_prorations"
}
```

**Request Fields:**
- `subscriptionId` (required): ID of the subscription to change
- `newResource` (required): New plan/resource ID to switch to
- `prorationBehavior` (optional): How to handle mid-cycle price changes
  - `"create_prorations"` (default): Prorate charges/credits for remaining time
  - `"none"`: No proration, change takes effect at next renewal
  - `"always_invoice"`: Invoice immediately for any difference

**Success Response (200 OK):**
```json
{
  "success": true,
  "subscriptionId": "sub_abc123",
  "previousResource": "plan-basic",
  "newResource": "plan-enterprise",
  "status": "active",
  "currentPeriodEnd": "2025-01-31T23:59:59Z",
  "prorationBehavior": "create_prorations"
}
```

**Notes:**
- For Stripe subscriptions, the plan change is applied via Stripe API
- For x402 subscriptions, only the local record is updated
- The `previous_product` and `changed_at` are stored in subscription metadata
- Proration calculates the difference between old and new plan prices

---

### Reactivate Subscription

**POST {prefix}/paywall/v1/subscription/reactivate**

Reactivate a subscription that was scheduled for cancellation at period end.

**Request Body:**
```json
{
  "subscriptionId": "sub_abc123"
}
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "subscriptionId": "sub_abc123",
  "status": "active",
  "cancelAtPeriodEnd": false,
  "currentPeriodEnd": "2025-01-31T23:59:59Z"
}
```

**Error Response (400 Bad Request):**
```json
{
  "error": "subscription is not scheduled for cancellation"
}
```

**Notes:**
- Only works if `cancelAtPeriodEnd` is true
- Subscription must still be within current period
- For Stripe subscriptions, also updates the Stripe subscription

---

### Subscription Configuration

**Example Configuration:**
```yaml
subscriptions:
  enabled: true
  backend: memory  # or postgres
  postgres_url: ""  # required if backend is postgres
  grace_period_hours: 24  # grace period after payment fails

paywall:
  products:
    - id: plan-pro
      description: "Pro Plan"
      fiat_amount: 10.00
      crypto_amount: 10.00
      subscription:
        enabled: true
        intervals: [monthly, yearly]
        trial_days: 14
        stripe_price_ids:
          monthly: price_1ABC123
          yearly: price_1DEF456
```

**Stripe Webhook Events:**

Configure your Stripe webhook to send these events for subscription management:
- `customer.subscription.created` - New subscription created
- `customer.subscription.updated` - Subscription status changed
- `customer.subscription.deleted` - Subscription canceled/expired
- `invoice.payment_succeeded` - Subscription payment successful
- `invoice.payment_failed` - Subscription payment failed

---

## Webhooks

### Stripe Webhook

**POST {prefix}/webhook/stripe**

Receives Stripe webhook events (checkout.session.completed, payment_intent.succeeded, etc.)

**Headers Required:**
```
Stripe-Signature: t=timestamp,v1=signature
```

**Event Types Handled:**
- `checkout.session.completed` - Single-item and cart purchases
- `payment_intent.succeeded` - Payment confirmation
- `payment_intent.payment_failed` - Payment failure

**Security:**
- Webhook signature validation required
- Set `STRIPE_WEBHOOK_SECRET` environment variable

### Stripe Webhook Info

**GET {prefix}/webhook/stripe**

Returns webhook endpoint information and recent event stats (development only).

**Response:**
```json
{
  "endpoint": "/api/webhook/stripe",
  "eventsProcessed": 42,
  "lastEvent": "2025-11-07T12:00:00Z"
}
```

---

## Callbacks

Cedros Pay fires callbacks to your application after successful payments/refunds. Configure callback URLs in your config file.

### Payment Success Callback

**Configuration:**
```yaml
callbacks:
  payment_success_url: "https://your-api.com/webhooks/payment"
  headers:
    Authorization: "Bearer your_secret_token"
  timeout: 10s

  # Webhook Retry Configuration (with Exponential Backoff)
  retry:
    enabled: true          # Enable retry with exponential backoff (default: true)
    max_attempts: 5        # Maximum retry attempts (default: 5)
    initial_interval: 1s   # Initial backoff interval (default: 1s)
    max_interval: 5m       # Maximum backoff interval (default: 5m)
    multiplier: 2.0        # Backoff multiplier (default: 2.0)

  # Dead Letter Queue (DLQ) - saves failed webhooks after all retries exhausted
  dlq_enabled: true       # Enable DLQ for failed webhooks (default: false)
  dlq_path: "./data/webhook-dlq.json"  # File path for DLQ storage
```

**Retry Behavior:**
- Webhooks are retried automatically with exponential backoff on failures (HTTP errors, timeouts, network errors)
- After all retries exhausted, failed webhooks are saved to DLQ (if enabled)
- Retries happen asynchronously and don't block payment processing
- See **[Webhook Retry + Dead Letter Queue](#webhook-retry--dead-letter-queue)** section for details

**Callback Payload (PaymentEvent):**

All payment webhooks include idempotency fields. Your webhook handler MUST use `eventId` to prevent duplicate processing.

**x402 Crypto Payment:**
```json
{
  "eventId": "evt_a1b2c3d4e5f67890abcdef12",
  "eventType": "payment.succeeded",
  "eventTimestamp": "2025-11-07T12:00:00Z",
  "resource": "demo-content",
  "method": "x402",
  "cryptoAtomicAmount": 1000000,
  "cryptoToken": "USDC",
  "wallet": "CustomerWallet...",
  "proofSignature": "transaction_signature",
  "paidAt": "2025-11-07T12:00:00Z",
  "metadata": {
    "user_id": "12345",
    "memo": "cedros:demo-content"
  }
}
```

**Stripe Payment:**
```json
{
  "eventId": "evt_x9y8z7w6v5u43210fedcba98",
  "eventType": "payment.succeeded",
  "eventTimestamp": "2025-11-07T12:00:00Z",
  "resource": "demo-content",
  "method": "stripe",
  "stripeSessionId": "cs_test_...",
  "stripeCustomer": "cus_...",
  "fiatAmountCents": 100,
  "fiatCurrency": "usd",
  "paidAt": "2025-11-07T12:00:00Z",
  "metadata": {
    "user_id": "12345"
  }
}
```

**Idempotency Fields (ALWAYS present):**
- `eventId` - Unique event identifier (e.g., "evt_a1b2c3d4e5f67890abcdef12"). Use this for deduplication.
- `eventType` - Always "payment.succeeded" for payment events
- `eventTimestamp` - ISO8601 UTC timestamp when event was created

**Payment Method Fields:**
- x402: `cryptoAtomicAmount` (int64 atomic units), `cryptoToken`, `wallet`, `proofSignature`
- Stripe: `fiatAmountCents` (int64 cents), `fiatCurrency`, `stripeSessionId`, `stripeCustomer`

**Cart Payment Metadata:**
```json
{
  "cart_items": "2",
  "total_quantity": "3",
  "item_0_price_id": "price_...",
  "item_0_resource": "demo-content",
  "item_0_quantity": "1",
  "item_0_credits": "100",
  "item_1_price_id": "price_...",
  "item_1_resource": "premium-content",
  "item_1_quantity": "2"
}
```

### Refund Success Callback

**Callback Payload (RefundEvent):**

All refund webhooks include idempotency fields. Your webhook handler MUST use `eventId` to prevent duplicate processing.

```json
{
  "eventId": "evt_refund_abc123xyz456",
  "eventType": "refund.succeeded",
  "eventTimestamp": "2025-11-07T12:00:00Z",
  "refundId": "refund_abc123...",
  "originalPurchaseId": "purchase_123",
  "recipientWallet": "CustomerWallet...",
  "atomicAmount": 10500000,
  "token": "USDC",
  "processedBy": "ServerWallet...",
  "signature": "refund_transaction_signature",
  "reason": "customer request",
  "refundedAt": "2025-11-07T12:00:00Z",
  "metadata": {
    "support_ticket": "12345",
    "original_purchase_id": "purchase_123",
    "recipient_wallet": "CustomerWallet...",
    "reason": "customer request"
  }
}
```

**Idempotency Fields (ALWAYS present):**
- `eventId` - Unique event identifier for deduplication
- `eventType` - Always "refund.succeeded" for refund events
- `eventTimestamp` - ISO8601 UTC timestamp when event was created

**Refund Fields:**
- `atomicAmount` - Refund amount in atomic units (e.g., 10500000 for 10.5 USDC with 6 decimals)
- `token` - Token symbol (e.g., "USDC")
- `processedBy` - Server wallet that executed the refund
- `signature` - On-chain transaction signature

### Custom Notifier Implementation

Implement the `Notifier` interface to receive callbacks:

```go
type Notifier interface {
    PaymentSucceeded(ctx context.Context, event PaymentEvent) error
    RefundSucceeded(ctx context.Context, event RefundEvent) error
}
```

**Example:**
```go
type MyNotifier struct {
    db *sql.DB
}

func (n *MyNotifier) PaymentSucceeded(ctx context.Context, event callbacks.PaymentEvent) error {
    // Grant access, update credits, etc.
    if event.Method == "x402" {
        return n.grantCryptoAccess(event.ResourceID, event.Wallet)
    }
    return n.grantStripeAccess(event.ResourceID, event.StripeCustomer)
}

func (n *MyNotifier) RefundSucceeded(ctx context.Context, event callbacks.RefundEvent) error {
    // Revoke access, deduct credits, etc.
    return n.processRefund(event.OriginalPurchaseID, event.Amount)
}
```

### Webhook Retry + Dead Letter Queue

Cedros Pay automatically retries failed webhook deliveries with exponential backoff.

**Retry Strategy:**

1. **Initial attempt**: Webhook fires immediately after payment success
2. **Exponential backoff**: Retries with increasing delays
   - Retry 1: after 1 second
   - Retry 2: after 2 seconds (1s × 2.0)
   - Retry 3: after 4 seconds (2s × 2.0)
   - Retry 4: after 8 seconds (4s × 2.0)
   - Retry 5: after 16 seconds (8s × 2.0)
3. **Dead Letter Queue**: After max attempts exhausted, save to DLQ file

**Triggers for retry:**
- HTTP 4xx/5xx status codes
- Network timeouts
- Connection errors
- Any HTTP client error

**DLQ File Structure:**

Location: `./data/webhook-dlq.json` (configurable via `dlq_path`)

```json
{
  "webhook_1730000000000": {
    "id": "webhook_1730000000000",
    "url": "https://your-api.com/webhooks/payment",
    "payload": {
      "resourceId": "demo-content",
      "method": "stripe",
      "stripeCustomer": "cus_...",
      "paidAt": "2025-11-09T10:29:00Z"
    },
    "headers": {
      "Authorization": "Bearer your_secret_token",
      "Content-Type": "application/json"
    },
    "eventType": "payment",
    "attempts": 5,
    "lastError": "received status 503 from https://your-api.com/webhooks/payment",
    "lastAttempt": "2025-11-09T10:30:00Z",
    "createdAt": "2025-11-09T10:29:00Z"
  }
}
```

**Manual DLQ Recovery:**

```bash
# 1. View failed webhooks
cat ./data/webhook-dlq.json | jq .

# 2. Fix your webhook endpoint
# (deploy fixes, check logs, verify endpoint is healthy)

# 3. Manually retry from DLQ payload
curl -X POST https://your-api.com/webhooks/payment \
  -H "Authorization: Bearer your_secret_token" \
  -H "Content-Type: application/json" \
  -d '{
    "resourceId": "demo-content",
    "method": "stripe",
    "stripeCustomer": "cus_...",
    "paidAt": "2025-11-09T10:29:00Z"
  }'

# 4. Clear DLQ after successful processing
echo "{}" > ./data/webhook-dlq.json
```

**Monitoring DLQ:**

```bash
# Check DLQ size (indicates delivery failures)
wc -l ./data/webhook-dlq.json

# Count failed webhooks
cat ./data/webhook-dlq.json | jq '. | length'

# List recent failures
cat ./data/webhook-dlq.json | jq -r '.[] | "\(.lastAttempt) - \(.lastError)"'
```

**Best Practices:**
- Enable DLQ in production to prevent webhook loss
- Monitor DLQ file size - growing file indicates endpoint problems
- Set up alerting when DLQ has items (indicates persistent delivery failures)
- Verify your webhook endpoint returns 2xx status codes on success
- Implement idempotency in your webhook handler (same payment might be retried)

---

## Metrics & Observability

Cedros Pay exposes comprehensive Prometheus metrics for monitoring payment flows, performance, and system health.

### Metrics Endpoint

**GET {prefix}/metrics**

Returns Prometheus-formatted metrics for scraping.

**Security:**

The metrics endpoint supports optional API key protection (recommended for production):

**Without Authentication (Development):**
```bash
curl http://localhost:8080/metrics
```

**With Admin API Key (Production):**
```bash
curl -H "Authorization: Bearer your-admin-api-key" \
  http://localhost:8080/metrics
```

**Configuration:**

Enable authentication via YAML config or environment variable:

```yaml
# configs/local.yaml
server:
  admin_metrics_api_key: "your-secure-random-key-here"
```

Or via environment variable:
```bash
export ADMIN_METRICS_API_KEY="your-secure-random-key-here"
```

**Security Recommendation:** Always enable authentication in production to prevent unauthorized access to operational metrics.

---

### Available Metrics

#### Payment Metrics

**cedros_payments_total**
- Counter tracking all payment attempts by method and status
- Labels: `method` (stripe, x402, gasless, quote, stripe_cart, cart_quote), `resource`, `status` (success, failed), `currency`

**cedros_payment_amount_cents**
- Histogram tracking payment amounts in cents
- Labels: `method`, `resource`, `currency`

**cedros_payment_duration_seconds**
- Histogram tracking payment processing time
- Labels: `method`, `resource`

**cedros_payment_failures_total**
- Counter tracking failed payment attempts by reason
- Labels: `method`, `resource`, `reason` (e.g., verification_failed, session_creation_failed, amount_mismatch)

**cedros_settlements_total**
- Counter tracking on-chain settlement confirmations
- Labels: `network` (mainnet-beta, devnet)

**cedros_settlement_time_seconds**
- Histogram tracking time from payment to on-chain confirmation
- Labels: `network`

#### Cart Metrics

**cedros_cart_checkouts_total**
- Counter tracking cart checkout events
- Labels: `status` (quote, session_created, session_creation_failed, success), `item_count`

#### Refund Metrics

**cedros_refunds_total**
- Counter tracking refund requests
- Labels: `status` (quote, success, failed), `currency`, `method` (crypto, stripe)

**cedros_refund_amount_cents**
- Histogram tracking refund amounts in cents
- Labels: `currency`

**cedros_refund_duration_seconds**
- Histogram tracking refund processing time
- Labels: `status`

#### Webhook Metrics

**cedros_webhooks_total**
- Counter tracking webhook delivery attempts
- Labels: `event_type` (stripe, payment.success, cart.payment.success, refund.processed), `status` (success, failed)

**cedros_webhook_duration_seconds**
- Histogram tracking webhook delivery time
- Labels: `event_type`

**cedros_webhook_retries**
- Histogram tracking number of retry attempts
- Labels: `event_type`

**cedros_webhook_dlq_total**
- Counter tracking webhooks saved to dead letter queue after exhausting retries
- Labels: `event_type`

#### Rate Limit Metrics

**cedros_rate_limit_hits_total**
- Counter tracking rate limit hits (requests blocked)
- Labels: `tier` (global, wallet, ip), `identifier` (all, wallet_address, ip_address)

#### Database Metrics

**cedros_db_queries_total**
- Counter tracking database queries
- Labels: `operation` (read, write, delete), `backend` (memory, postgres, mongodb, file)

**cedros_db_query_duration_seconds**
- Histogram tracking database query time
- Labels: `operation`, `backend`

#### Archival Metrics

**cedros_archival_records_deleted_total**
- Counter tracking records deleted by archival process
- Total records cleaned up (payments + nonces)

---

### Prometheus Scrape Configuration

Add this to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'cedros-pay'
    scrape_interval: 15s
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'  # Or /{route_prefix}/metrics if prefix is set

    # If using admin API key authentication:
    authorization:
      type: Bearer
      credentials: 'your-admin-api-key'
```

---

### Example Queries

**Payment Success Rate:**
```promql
sum(rate(cedros_payments_total{status="success"}[5m])) by (method) /
sum(rate(cedros_payments_total[5m])) by (method)
```

**Average Payment Processing Time:**
```promql
histogram_quantile(0.95,
  rate(cedros_payment_duration_seconds_bucket[5m])
)
```

**Webhook Failure Rate:**
```promql
sum(rate(cedros_webhooks_total{status="failed"}[5m])) /
sum(rate(cedros_webhooks_total[5m]))
```

**Rate Limit Hit Rate:**
```promql
sum(rate(cedros_rate_limit_hits_total[5m])) by (tier)
```

**Revenue by Payment Method (Last Hour):**
```promql
sum(cedros_payment_amount_cents{status="success"} / 100) by (method, currency)
```

---

### Grafana Dashboard

Example dashboard panels:

1. **Payment Volume** - Graph of `cedros_payments_total` by method
2. **Payment Success Rate** - Gauge showing success percentage
3. **Average Transaction Time** - Graph of P50/P95/P99 latency
4. **Failed Payments** - Table of `cedros_payment_failures_total` by reason
5. **Webhook Health** - Graph of webhook success rate and retry counts
6. **Rate Limit Activity** - Graph of rate limit hits by tier
7. **Settlement Times** - Histogram of blockchain confirmation times

---

## Error Responses

All endpoints return consistent error format:

```json
{
  "error": "error_code",
  "message": "Human-readable error message",
  "details": {
    "field": "Additional context"
  }
}
```

**Common Error Codes:**
- `payment_required` - Payment not verified (402)
- `invalid_request` - Missing or invalid parameters (400)
- `not_found` - Resource not found (404)
- `expired` - Quote or session expired (400)
- `already_processed` - Payment/refund already processed (409)
- `verification_failed` - Transaction verification failed (402)
- `unauthorized` - Invalid payer for refund (403)
- `rate_limit_exceeded` - Too many requests (429)

---

## Rate Limiting

Cedros Pay implements multi-tier rate limiting to prevent abuse while allowing legitimate use.

### Configuration

```yaml
rate_limit:
  # Global rate limit (across all users) - prevents DoS attacks
  global_enabled: true
  global_limit: 1000        # 1000 requests per minute (16.6 req/sec)
  global_window: 1m

  # Per-wallet rate limit - prevents spam from individual wallets
  # Wallet identified via X-Wallet, X-Signer headers or query params
  per_wallet_enabled: true
  per_wallet_limit: 60      # 60 requests per minute (1 req/sec avg)
  per_wallet_window: 1m

  # Per-IP rate limit - fallback when wallet not identified
  per_ip_enabled: true
  per_ip_limit: 120         # 120 requests per minute (2 req/sec avg)
  per_ip_window: 1m
```

### Rate Limit Tiers

1. **Global Rate Limit** - Applied to all requests across all clients
   - Default: 1000 requests/minute
   - Prevents server-wide DoS attacks

2. **Per-Wallet Rate Limit** - Applied per wallet address
   - Default: 60 requests/minute
   - Extracts wallet from `X-Wallet` header, `X-Signer` header, or `wallet` query parameter
   - Prevents spam from individual wallets

3. **Per-IP Rate Limit** - Fallback when no wallet is identified
   - Default: 120 requests/minute
   - Applied based on client IP address
   - Prevents anonymous spam

### Rate Limit Response

When rate limit is exceeded, returns **HTTP 429 Too Many Requests** with rate limit headers:

**Full Response Example:**
```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 60
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1699564800
```

```json
{
  "error": "rate_limit_exceeded",
  "message": "Rate limit exceeded for wallet 8xW...abc. Please try again in 60 seconds."
}
```

**Response Headers Explained:**
- `Retry-After: 60` - Seconds until rate limit resets (human-readable)
- `X-RateLimit-Limit: 60` - Maximum requests allowed in the current window
- `X-RateLimit-Remaining: 0` - Requests remaining in current window (0 when limit hit)
- `X-RateLimit-Reset: 1699564800` - Unix timestamp when the limit resets

**Client Handling:**
```javascript
// Example: Parse rate limit headers
const response = await fetch('/paywall/v1/quote', { ... });

if (response.status === 429) {
  const retryAfter = response.headers.get('Retry-After');
  const resetTime = response.headers.get('X-RateLimit-Reset');

  console.log(`Rate limited. Retry in ${retryAfter} seconds`);
  console.log(`Limit resets at: ${new Date(resetTime * 1000)}`);

  // Wait before retrying
  await new Promise(resolve => setTimeout(resolve, retryAfter * 1000));
}
```

### Wallet Identification

The server identifies wallets in this priority order:

1. `X-Wallet` header
2. `X-Signer` header
3. `wallet` query parameter
4. Falls back to IP-based limiting if none found

**Example request with wallet header:**
```bash
curl -X POST http://localhost:8080/paywall/v1/quote \
  -H "X-Wallet: 8xW...abc" \
  -H "Content-Type: application/json" \
  -d '{"resource":"demo-content"}'
```

### Disabling Rate Limits

Not recommended for production, but can be disabled per tier:

```yaml
rate_limit:
  global_enabled: false      # Disable global limit
  per_wallet_enabled: false  # Disable per-wallet limit
  per_ip_enabled: false      # Disable per-IP limit
```

---

## Idempotency

Payment endpoints support idempotency to prevent duplicate charges:

**Header:**
```
Idempotency-Key: unique_request_id
```

**Behavior:**
- Same key within 24 hours returns cached response
- Prevents duplicate Stripe sessions, refunds, cart quotes
- Key must be unique per logical operation

**Endpoints with Idempotency:**
- `POST /paywall/v1/stripe-session`
- `POST /paywall/v1/cart/checkout`
- `POST /paywall/v1/cart/quote`
- `POST /paywall/v1/refunds/request`

---

## OpenAPI Specification

**GET /openapi.json**

Returns OpenAPI 3.0 specification for all endpoints. Use with Swagger UI, Postman, or code generators.

---

## Support

- **GitHub Issues:** https://github.com/CedrosPay/server/issues
- **Documentation:** https://github.com/CedrosPay/server/docs
- **x402 Spec:** https://github.com/coinbase/x402
