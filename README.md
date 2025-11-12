# Cedros Pay Server

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](./CONTRIBUTING.md)

> **Unified payments for humans and agents.**

Cedros Pay is an open-source integration framework that bundles **Stripe (fiat)** and **x402 (Solana crypto)** into one seamless developer experience.
This is the repository for the server application.

**What it does:**
- React SDK for "Pay with Card" and "Pay with Crypto" buttons
- Go backend for Stripe sessions, x402 verification, and route protection
- Unified API for humans, apps, and AI agents

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Why Cedros Pay?](#-why-cedros-pay)
- [Common Use Cases & Integration Patterns](#-common-use-cases--integration-patterns)
- [Architecture](#-architecture)
- [Backend API Endpoints](#-backend-api-endpoints)
- [Key Features](#-key-features)
- [Quick Start](#-quick-start)
  - [Option 1: Try It (5 Minutes)](#option-1-try-it-5-minutes)
  - [Option 2: Deploy as Standalone Microservice](#option-2-deploy-as-standalone-microservice)
  - [Option 3: Integrate into Go API](#option-3-integrate-into-go-api)
- [Local Development Setup](#️-local-development-setup)
- [Deployment Patterns](#️-deployment-patterns)
  - [Pattern 1: Standalone Microservice](#pattern-1-standalone-microservice)
  - [Pattern 2: Integrated Library](#pattern-2-integrated-library)
  - [Pattern 3: Reverse Proxy](#pattern-3-reverse-proxy)
- [Docker Deployment](#-docker-deployment)
- [Advanced Configuration](#-advanced-configuration)
- [Performance & Cost](#-performance--cost)
- [Frontend Integration](#-frontend-integration)
- [Migration Guide](#-migration-guide)
  - [From Stripe-Only Implementation](#migrating-from-stripe-only-implementation)
  - [From Manual Crypto Payments](#migrating-from-manual-crypto-payments)
  - [To Database-Backed Products](#migrating-to-database-backed-products)
- [Integration FAQ](#integration-faq)
- [Deployment](#deployment)
- [Troubleshooting](#troubleshooting)
- [Related Documentation](#related-documentation)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

---

## Prerequisites

- **Go 1.23+** — Server runtime
- **Stripe Account** — For fiat payments (test mode works)
- **Solana Wallet** — For receiving crypto payments
- **Solana RPC Access** — Mainnet or devnet (public or private endpoint)

**Optional:**
- **Discord/Slack Webhook** — For balance monitoring alerts
- **Node.js 18+** — If using the React SDK locally

---

## 🌅 Why Cedros Pay?

Quickly and easily set up stripe and crypto checkout options to accept payments from both credit cards and USDC on Solana.  
Manage your products in a single YAML file and utilize the Cedros Pay UI library to add payment buttons anywhere.

- Stripe-hosted checkout for credit/debit cards.
- x402-verified USDC payments for instant settlement on Solana. Support for any SPL (Solana based) token.
- Usable as a standalone microservice or as a first class integration into your server.

---

## 🎯 Common Use Cases & Integration Patterns

### Is Cedros Pay Right for Your Application?

Cedros Pay excels at combining Stripe and crypto payments, but understanding your use case helps determine the best integration approach.

#### ✅ Ideal Use Cases

**1. Adding Crypto Payments to Existing Stripe Integration**
- You already accept Stripe payments and want to add USDC/Solana as an option
- Users should be able to choose between card or crypto at checkout
- You want instant settlement for crypto payments

**2. Pay-Per-API-Request / Microtransactions**
- AI services charging per API call
- Content access fees (articles, videos, courses)
- Agent-to-agent automated payments via x402 protocol

**3. Credit/Token Systems (with adaptation)**
- Users purchase credits upfront, use later for services
- Credit packages defined dynamically (database-backed)
- See [Credit System Integration](#credit-system-integration) below

#### 🔧 Integration Approaches

**Approach 1: Stripe-Only (Compatibility Mode)** ⚡ *Easiest*

If you're not ready for crypto payments, you can use Cedros as a Stripe wrapper:

```go
// Use Cedros's Stripe client directly
import "github.com/CedrosPay/server/pkg/cedros"

app, _ := cedros.NewApp(cfg)
session, _ := app.Stripe.CreateCheckoutSession(ctx, stripesvc.CreateSessionRequest{
    ResourceID:  "credit-package-100",
    AmountCents: 1000,
    Currency:    "usd",
    // ... other params
})
// Redirect user to session.URL
```

**Benefits:**
- Minimal changes to existing code
- Keep your current payment flow
- Add crypto later when ready

**Tradeoffs:**
- Doesn't use paywall features
- Not leveraging x402 protocol

**Approach 2: Full Integration (Paywall + Crypto)** 🚀 *Most Features*

Embrace the paywall model for dual payment support:

```javascript
// Frontend requests resource
const quote = await fetch('/paywall/credit-package-100')
// Returns 402 with Stripe + crypto options

// User chooses payment method
if (userWantsCrypto) {
    // Pay with Solana/USDC
} else {
    // Pay with Stripe (same as before)
}
```

**Benefits:**
- Full crypto payment support
- Future-proof for AI agents
- Marketing differentiation

**Tradeoffs:**
- Requires frontend changes (handle 402 responses)
- More complex integration

**Approach 3: Hybrid (Backend Paywall, Frontend Abstraction)** 🎨 *Balanced*

Use paywall backend, hide complexity from frontend:

```go
// Compatibility wrapper endpoint
func PurchaseCredits(c *gin.Context) {
    // Get quote from Cedros paywall
    quote := cedrosApp.Paywall.GenerateQuote("credits-100")

    // Create Stripe session (hide crypto option from user for now)
    session := cedrosApp.Stripe.CreateCheckoutSession(...)

    // Return old API format
    c.JSON(200, gin.H{"checkoutUrl": session.URL})
}
```

**Benefits:**
- No frontend changes needed
- Add crypto option later via feature flag
- Gradual migration path

### Credit System Integration

**Question:** *"We sell credits that users spend on AI jobs. How does this fit the paywall model?"*

**Answer:** Cedros can absolutely support credit systems with a small conceptual shift:

**Traditional Flow:**
```
User → "Buy 100 Credits" → Stripe Checkout → Credits Added to Balance
```

**With Cedros (treat credit packages as resources):**
```
User → Request "credit-package-100" resource → Quote (Stripe + Crypto options)
     → Pay → Webhook grants credits to user balance
```

**Implementation:**

1. **Define credit packages as resources** (YAML or database-backed):
```yaml
paywall:
  resources:
    credit-package-100:
      description: "100 AI Generation Credits"
      fiat_amount: 10.00
      crypto_amount: 10.0
      stripe_price_id: "price_..."
      metadata:
        credits: "100"
```

2. **Implement custom fulfillment** via callback:
```go
// Custom notifier grants credits instead of "content access"
type CreditGranter struct{}

func (c *CreditGranter) PaymentSucceeded(ctx context.Context, event PaymentEvent) error {
    credits, _ := strconv.Atoi(event.Metadata["credits"])
    userID := event.Metadata["user_id"]

    // Add credits to user balance in your database
    db.Exec("UPDATE users SET credits = credits + ? WHERE id = ?", credits, userID)

    // Record transaction
    db.Create(&Transaction{
        UserID:     userID,
        Type:       "purchase",
        Credits:    credits,
        Amount:     event.Amount,
        ExternalID: event.StripeSessionID,
    })

    return nil
}

// Inject into Cedros
app, _ := cedros.NewApp(cfg, cedros.WithNotifier(&CreditGranter{}))
```

3. **Database-backed packages** (dynamic pricing):

If you manage packages via admin API (not static YAML), implement a custom resource loader:

```go
// Load packages from PostgreSQL on demand
type DatabaseResourceLoader struct {
    db *sql.DB
}

func (l *DatabaseResourceLoader) LoadResources(ctx context.Context) (map[string]PaywallResource, error) {
    rows, _ := l.db.Query("SELECT id, name, price, credits FROM credit_packages WHERE active = true")

    resources := make(map[string]PaywallResource)
    for rows.Next() {
        var pkg CreditPackage
        rows.Scan(&pkg.ID, &pkg.Name, &pkg.Price, &pkg.Credits)

        resources[pkg.ID] = PaywallResource{
            ResourceID:         pkg.ID,
            Description:        pkg.Name,
            FiatAmountCents:    int64(pkg.Price * 100), // Convert dollars to cents
            CryptoAtomicAmount: int64(pkg.Price * 1000000), // Convert to USDC atomic units (6 decimals)
            CryptoToken:        "USDC",
            Metadata: map[string]string{
                "credits": fmt.Sprint(pkg.Credits),
            },
        }
    }
    return resources, nil
}
```

**Result:** Your credit system works with Cedros while preserving:
- Dynamic package management
- Admin API for pricing changes
- Transaction history and analytics
- Referral bonuses (in your custom notifier)

### Database vs. YAML Configuration

**Concern:** *"Our credit packages are managed dynamically via API, but Cedros uses YAML config."*

**Solutions:**

**Option 1: Dynamic Resource Loading** (Recommended)

Extend Cedros to load resources from your database:

```go
// Query database on each quote generation
app, _ := cedros.NewApp(cfg, cedros.WithResourceLoader(NewDatabaseLoader(db)))
```

This allows admin users to update pricing via your existing API without server restarts.

**Option 2: YAML + Database Sync**

Keep packages in database, sync to config file on updates:

```go
// After admin updates package
func UpdatePackage(pkg CreditPackage) {
    db.Save(&pkg)
    RegenerateCedrosConfig() // Write updated YAML
    SendReloadSignal()       // Trigger config reload
}
```

**Option 3: Stripe-Only Mode**

Use Cedros just for Stripe handling, manage packages entirely in your DB:

```go
// Bypass Cedros resource config, query DB directly
pkg := db.GetCreditPackage(packageID)
session, _ := cedrosApp.Stripe.CreateCheckoutSession(ctx, stripesvc.CreateSessionRequest{
    AmountCents: pkg.PriceCents,
    PriceID:     pkg.StripePriceID,
    // ...
})
```

### When NOT to Use Cedros Pay

Be honest with yourself about whether Cedros adds value:

**❌ Skip Cedros if:**
- You only need Stripe (no crypto plans) → Use `stripe-go` directly
- You need instant package updates without server restarts → Requires extension
- You have complex promotional pricing logic → May require custom implementation
- Frontend changes are off-limits → Use Approach 3 (Hybrid) or skip

**✅ Use Cedros if:**
- You want crypto payments alongside Stripe
- You're building pay-per-API-request features
- You value unified payment abstraction
- You're open to minor architectural adjustments

### Migration Checklist

If integrating Cedros into an existing Stripe app:

- [ ] **Decide:** Stripe-only, hybrid, or full integration?
- [ ] **Audit:** Can your payment flow adapt to quote-based model?
- [ ] **Database:** Will you use YAML or extend for database-backed resources?
- [ ] **Fulfillment:** Can you implement credit granting in a webhook callback?
- [ ] **Frontend:** Can you handle 402 responses, or will you abstract them away?
- [ ] **Testing:** Can you test Stripe webhooks + (optionally) Solana transactions?

**Estimated Integration Time:**
- Stripe-only wrapper: 1-2 days
- Hybrid (backend paywall, frontend unchanged): 3-5 days
- Full integration (Stripe + crypto): 2-3 weeks

---

## 🧩 Architecture

### High-Level Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                           Frontend                                │
│  ┌────────────┐  ┌────────────┐  ┌─────────────┐                │
│  │   React    │  │  Vue.js    │  │   Mobile    │                │
│  │    App     │  │    App     │  │     App     │                │
│  └─────┬──────┘  └──────┬─────┘  └──────┬──────┘                │
│        │                 │                │                        │
│        └─────────────────┼────────────────┘                        │
│                          │                                         │
└──────────────────────────┼─────────────────────────────────────────┘
                           │ HTTP/HTTPS
                           │
┌──────────────────────────▼─────────────────────────────────────────┐
│                    Cedros Pay Server                                │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │                     HTTP Handlers                            │  │
│  │  /paywall/stripe-session  /paywall/{id}  /webhook/stripe    │  │
│  └────────┬──────────────────┬──────────────────┬──────────────┘  │
│           │                  │                   │                 │
│  ┌────────▼────────┐  ┌──────▼────────┐  ┌──────▼────────┐       │
│  │  Stripe Service │  │ Paywall Svc   │  │ Webhook Svc   │       │
│  └────────┬────────┘  └──────┬────────┘  └──────┬────────┘       │
│           │                  │                   │                 │
│  ┌────────▼──────────────────▼───────────────────▼────────┐       │
│  │              Storage Layer (Memory/PostgreSQL)          │       │
│  └────────────────────────────┬────────────────────────────┘       │
│                               │                                     │
│  ┌────────────────────────────▼────────────────────────────┐       │
│  │           Callback/Notification System                   │       │
│  └─────────────────────────────────────────────────────────┘       │
│                                                                     │
└──────────────┬─────────────────────────────┬───────────────────────┘
               │                             │
       ┌───────▼────────┐          ┌────────▼─────────┐
       │  Stripe API    │          │  Solana Network  │
       │  (Checkout)    │          │  (RPC + WS)      │
       └────────────────┘          └──────────────────┘
```

**Frontend**

- React SDK (`@cedros/pay-react`) for drop-in payment buttons.
- Uses wallet adapters for Solana and Stripe JS SDK for fiat.

**Backend**

- Go service handling session creation, webhooks, x402 verification, and route protection.
- Deploy standalone or embed into existing backend.

---

## 🏗️ Deployment Patterns

Cedros Pay supports three deployment patterns:

### Pattern Comparison

| Pattern | Best For | Pros | Cons |
|---------|----------|------|------|
| **Standalone Microservice** | Non-Go backends, multi-service | Language agnostic, isolated | Extra network hop |
| **Integrated Library** | Go backends, monoliths | Low latency, tight integration | Go-only |
| **Reverse Proxy** | Adding payments to existing APIs | No code changes needed | Additional proxy layer |

### Pattern 1: Standalone Microservice

Run Cedros as an independent service that any backend can call:

```
┌───────────┐      ┌──────────┐      ┌────────────────┐
│  Frontend │─────▶│ Your API │─────▶│ Cedros Service │
│           │      │ (Any Lang)│      │   (Port 8080)  │
└───────────┘      └──────────┘      └────────┬───────┘
                                              │
                                      ├───────▶ Stripe
                                      └───────▶ Solana
```

**When to use:**
- Backend in Python, Node, Ruby, Java, etc.
- Need complete isolation of payment processing
- Multiple services need payment capabilities

**Example (Python):**

```python
import requests

response = requests.post('http://cedros-service:8080/paywall/stripe-session', json={
    'resource': 'premium-plan',
    'customerEmail': user.email,
    'metadata': {'user_id': str(user.id)}
})

checkout_url = response.json()['url']
```

See [Quick Start - Option 2](#option-2-deploy-as-standalone-microservice) for Docker setup.

### Pattern 2: Integrated Library

Embed Cedros directly into your Go codebase:

```
┌───────────┐      ┌────────────────────────────┐
│  Frontend │─────▶│    Your Go API             │
│           │      │  ┌──────────────────────┐  │
└───────────┘      │  │ Cedros (Embedded)    │  │
                   │  └───────┬──────────────┘  │
                   │          ├─────▶ Stripe    │
                   │          └─────▶ Solana    │
                   └──────────────────────────────┘
```

**When to use:**
- Backend written in Go
- Want tight integration with existing middleware
- Prefer single binary deployment

**Example:**

```go
import "github.com/CedrosPay/server/pkg/cedros"

app, _ := cedros.NewApp(cfg)
router.Mount("/payments", app.Router())
```

See [Quick Start - Option 3](#option-3-integrate-into-go-api) for full integration example.

### Pattern 3: Reverse Proxy

Run Cedros in front of existing API as payment gateway:

```
┌───────────┐      ┌──────────────┐      ┌──────────┐
│  Frontend │─────▶│ Cedros Proxy │─────▶│ Your API │
└───────────┘      └──────┬───────┘      └──────────┘
                          │
                   ├──────▶ Stripe
                   └──────▶ Solana
```

**When to use:**
- Want to add payments without code changes
- Legacy API that's hard to modify
- Need payment gateway functionality

**Example (Nginx):**

```nginx
location /api/premium {
    auth_request /payments/verify;
    proxy_pass http://your_api;
}
```

---

## 🔌 Backend API Endpoints

Your backend must implement these endpoints (all routes support custom prefixes):

### Core Endpoints

**GET /cedros-health**
- Health check and route prefix discovery
- Always unprefixed (responds at root path)
- Returns server status and configured route prefix

### AI Agent Discovery Endpoints

**POST /resources/list** 🆕
- Model Context Protocol (MCP) endpoint for AI agent resource discovery
- JSON-RPC 2.0 format for standardized agent communication
- Returns list of all available paid resources with URIs
- Example request:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "method": "resources/list"
  }
  ```
- Example response:
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
- **Use case:** AI agents (like Claude Desktop) can discover available resources via MCP standard
- **Spec:** [Model Context Protocol](https://modelcontextprotocol.io/docs/concepts/resources/)
- **Dynamic updates:** Automatically reflects product catalog changes from configured data source (YAML/PostgreSQL/MongoDB)

**GET /.well-known/payment-options** 🆕
- RFC 8615 well-known URI for web-based service discovery
- Returns comprehensive payment configuration and available resources
- Includes supported payment methods (Stripe + x402) with network details
- CORS-enabled for cross-origin discovery
- 5-minute cache for performance
- Example response:
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
        "paymentAddress": "...",
        "tokenMint": "EPjFWdd..."
      }
    }
  }
  ```
- **Use case:** AI agents browsing the web can discover paid services and payment options
- **Spec:** [RFC 8615 - Well-Known URIs](https://tools.ietf.org/html/rfc8615)
- **Dynamic updates:** Automatically reflects product catalog changes from configured data source (YAML/PostgreSQL/MongoDB)

#### Dynamic Updates for Discovery Endpoints

Both MCP and well-known endpoints automatically reflect product catalog changes:

**Data Source Behavior:**
- **YAML** (`product_source: yaml`): Changes require server restart (loaded at startup)
- **PostgreSQL** (`product_source: postgres`): Updates visible within cache TTL (default: 5 minutes)
- **MongoDB** (`product_source: mongodb`): Updates visible within cache TTL (default: 5 minutes)

**Cache Configuration:**
```yaml
paywall:
  product_source: postgres     # or mongodb
  product_cache_ttl: 5m        # 5 minutes (recommended for database sources)
  # product_cache_ttl: 0s      # Real-time updates (no cache, higher DB load)
  # product_cache_ttl: 1h      # 1 hour (lower DB load, slower updates)
```

**Update Timeline:**
- Database changes → Visible within `product_cache_ttl`
- Set `product_cache_ttl: 0s` for instant updates (queries DB on every request)
- Recommended: Keep default `5m` for balance of freshness and performance

**GET {prefix}/paywall/v1/products** 🆕
- Returns list of all active products with pricing information
- Cached with configurable TTL (`product_cache_ttl` in config)
- Perfect for displaying product catalogs in your frontend
- Example response:
  ```json
  [
    {
      "id": "demo-content",
      "description": "Demo protected content",
      "fiatAmount": 1.0,
      "fiatCurrency": "usd",
      "stripePriceId": "price_123",
      "cryptoAmount": 1.0,
      "cryptoToken": "USDC",
      "metadata": {
        "plan": "demo"
      }
    }
  ]
  ```
- **Caching:** Set `product_cache_ttl: 5m` to cache for 5 minutes (recommended for database sources)
- **No caching:** Set `product_cache_ttl: 0s` to always fetch fresh data

**POST {prefix}/paywall/v1/quote**
- Generates x402 payment quote for a resource
- Resource ID passed in request body to prevent URL leakage
- Returns payment details for crypto transactions
- Optional: Include `couponCode` in request body for discounts

### Stripe Payment (Optional)

**POST {prefix}/paywall/v1/stripe-session** *(Single-item checkout)*
- Create Stripe checkout session for a single product
- Optional: only needed if supporting Stripe payments
- Returns checkout session URL for redirect
- Example request:
  ```json
  {
    "resource": "demo-content",
    "customerEmail": "user@example.com"
  }
  ```
- Example response:
  ```json
  {
    "sessionId": "cs_test_...",
    "url": "https://checkout.stripe.com/..."
  }
  ```

**GET {prefix}/paywall/v1/stripe-session/verify** 🆕 *(Stripe payment verification)*
- Verify that a Stripe checkout session was completed and paid
- **Security Critical:** Prevents payment bypass attacks where users manually enter success URLs
- Frontend MUST call this endpoint before granting access to purchased content
- Query parameter: `session_id` (from Stripe redirect URL)
- Returns 200 with payment details if verified, 404 if session not found
- Example request:
  ```bash
  GET /paywall/v1/stripe-session/verify?session_id=cs_test_abc123
  ```
- Example response (200 - verified):
  ```json
  {
    "verified": true,
    "resource_id": "demo-content",
    "paid_at": "2025-11-11T13:45:00Z",
    "amount": "$1.00 USD",
    "customer": "cus_abc123",
    "metadata": {
      "userId": "12345"
    }
  }
  ```
- Example response (404 - not verified):
  ```json
  {
    "error": {
      "code": "session_not_found",
      "message": "Payment not completed or session invalid"
    }
  }
  ```
- **Integration:** See `STRIPE_VERIFICATION_INTEGRATION.md` for complete frontend implementation guide

**POST {prefix}/paywall/v1/cart/checkout** *(Multi-item cart checkout)* 🆕
- Create Stripe checkout session for multiple products in a single transaction
- Supports quantity > 1 per item
- Ideal for credit bundles, packages, or bulk purchases
- Example request:
  ```json
  {
    "items": [
      {
        "priceId": "price_1SPqRpR4HtkFbUJKUciKecmZ",
        "resource": "demo-content",
        "quantity": 1
      },
      {
        "priceId": "price_1SQCuhR4HtkFbUJKDUQpCA6D",
        "resource": "test-product-2",
        "quantity": 2
      }
    ],
    "customerEmail": "user@example.com",
    "metadata": {
      "order_type": "bundle"
    }
  }
  ```
- Example response:
  ```json
  {
    "sessionId": "cs_test_...",
    "url": "https://checkout.stripe.com/...",
    "totalItems": 2
  }
  ```
- **When to use:**
  - Purchasing multiple credit packages at once
  - Product bundles (course + workbook + certificate)
  - Bulk purchases (10x API calls, 5x reports)
  - Any scenario where user adds multiple items to cart
- **Note:** Uses Stripe Price IDs directly (not resource IDs from paywall config)
- See [CART_CHECKOUT_TESTING.md](./CART_CHECKOUT_TESTING.md) for detailed examples

**Cart Webhooks & Callbacks - IMPORTANT:**

When a cart checkout completes, your webhook/callback receives **all item details** in metadata:

```json
{
  "cart_items": "2",
  "total_quantity": "3",
  "item_0_price_id": "price_1SPqRpR4HtkFbUJKUciKecmZ",
  "item_0_resource": "demo-content",      // Backend resource ID
  "item_0_quantity": "1",
  "item_0_description": "Product A",
  "item_0_credits": "100",                // Your custom metadata
  "item_1_price_id": "price_1SQCuhR4HtkFbUJKDUQpCA6D",
  "item_1_resource": "test-product-2",    // Backend resource ID
  "item_1_quantity": "2",
  "user_id": "12345"                      // Cart-level metadata
}
```

**Processing cart purchases in your callback:**

```go
func (n *YourNotifier) PaymentSucceeded(ctx context.Context, event callbacks.PaymentEvent) error {
    // Check if this is a cart purchase
    numItems, _ := strconv.Atoi(event.Metadata["cart_items"])
    if numItems == 0 {
        return n.handleSingleItem(ctx, event)  // Regular purchase
    }

    // Process each item in the cart
    for i := 0; i < numItems; i++ {
        prefix := fmt.Sprintf("item_%d_", i)
        priceID := event.Metadata[prefix+"price_id"]
        quantity, _ := strconv.ParseInt(event.Metadata[prefix+"quantity"], 10, 64)

        // Update inventory/grant credits based on price ID
        if credits := event.Metadata[prefix+"credits"]; credits != "" {
            creditAmount, _ := strconv.Atoi(credits)
            n.grantCredits(event.StripeCustomer, creditAmount * int(quantity))
        }
    }

    return nil
}
```

**Why metadata instead of line items?**
- ✅ No extra API calls needed (Stripe includes metadata in webhook automatically)
- ✅ Custom per-item data (credits, SKU, warehouse, product type)
- ✅ Cart-level data (user ID, referral code, campaign)
- ✅ Works perfectly for typical carts (<15 items)

**Adding custom metadata:**

```bash
POST /paywall/v1/cart/checkout
{
  "items": [
    {
      "priceId": "price_xxx",
      "quantity": 1,
      "metadata": {
        "credits": "100",        // Accessible as item_0_credits
        "product_type": "digital"
      }
    }
  ],
  "metadata": {
    "user_id": "12345",          // Accessible as user_id
    "referral_code": "FRIEND20"
  }
}
```

See [CART_WEBHOOKS.md](./CART_WEBHOOKS.md) for complete examples including credit fulfillment, inventory management, and helper functions.

**POST {prefix}/paywall/v1/refunds/request** *(Request crypto refund)* 🆕
- Creates pending refund request in database (never auto-expires)
- Returns refund ID and initial x402 quote
- Requires cryptographic signature from original payer OR admin
- Refund must be approved (executed) or denied (deleted) by admin
- See [REFUND_FLOW.md](./REFUND_FLOW.md) for complete workflow

**POST {prefix}/paywall/v1/nonce** *(Generate admin nonce)* 🆕
- Generates one-time nonce for admin signature replay protection
- Public endpoint (no auth required)
- Returns nonce + expiry (5 minutes)
- See [REFUND_ADMIN.md](./REFUND_ADMIN.md) for usage

**POST {prefix}/paywall/v1/refunds/pending** *(List pending refunds)* 🆕
- Admin-only endpoint to view all pending refund requests
- Requires nonce-based signature authentication
- Each nonce can only be used ONCE (replay protection)
- See [REFUND_ADMIN.md](./REFUND_ADMIN.md) for complete security model

**POST {prefix}/paywall/v1/refunds/approve** *(Get fresh refund quote)* 🆕
- Regenerates x402 quote for existing refund request
- Used when original quote expires (blockhash stale after 15 min)
- Returns fresh quote with new 15-min execution window
- Refund request persists - only quote is regenerated
- Accepts `refundId` in request body

**POST {prefix}/paywall/v1/refunds/deny** *(Deny refund request)* 🆕
- Admin endpoint to deny/cancel pending refund requests
- Requires wallet signature from payTo wallet (crypto-native auth)
- Only unprocessed refunds can be denied
- Returns 404 if refund not found, 409 if already processed
- Permanently deletes refund from storage
- Accepts `refundId` in request body

**POST {prefix}/paywall/v1/verify** *(Verify refund execution)* 🆕
- Verifies refund transaction with X-PAYMENT header (resourceType: "refund")
- Admin calls after building/signing/submitting Solana transaction
- Backend confirms transaction on-chain
- Marks refund as processed and fires RefundSucceeded callback

**POST {prefix}/paywall/v1/cart/quote** *(Multi-item x402 cart checkout)* 🆕
- Generate x402 quote for multiple items with price locking
- Returns unique cart ID and crypto payment quote
- Cart expires after 15 minutes (frontend should request fresh quote)
- Example request:
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
    "metadata": {
      "user_id": "12345",
      "campaign": "summer_sale"
    }
  }
  ```
- Example response:
  ```json
  {
    "cartId": "cart_a20910dd8fc1c15b1e99125dd0810065",
    "quote": {
      "ResourceID": "cart_a20910dd8fc1c15b1e99125dd0810065",
      "ExpiresAt": "2025-11-05T13:15:00Z",
      "Crypto": {
        "scheme": "solana-spl-transfer",
        "network": "mainnet-beta",
        "maxAmountRequired": "3000000",
        "resource": "cart_a20910dd8fc1c15b1e99125dd0810065",
        "description": "Cart purchase (3 USDC)",
        "payTo": "TokenAccountAddress...",
        "asset": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
      }
    },
    "items": [...],
    "expiresAt": "2025-11-05T13:15:00Z"
  }
  ```
- **Payment flow:**
  1. Request cart quote → Get cart ID + x402 quote
  2. User pays exact cart total via Solana
  3. Verify payment via `POST /paywall/v1/verify` with `X-PAYMENT` header
  4. Server verifies payment EXACTLY matches cart total
  5. Callback receives all item details (resource, quantity, price, metadata)
- **Price locking:** Prices frozen at quote time (no mid-checkout changes)
- **Strict verification:** Payment must match cart total exactly (no partial payments)
- **Cart callbacks:** Same as Stripe cart - all item details in metadata
- See [X402_CART_FLOW.md](./X402_CART_FLOW.md) for complete guide

**Cart Checkout Comparison:**

| Feature | Stripe Cart | x402 Cart |
|---------|-------------|-----------|
| **Endpoint** | POST /paywall/v1/cart/checkout | POST /paywall/v1/cart/quote |
| **Payment Method** | Credit card | Solana crypto (USDC, SOL, etc.) |
| **Settlement** | 2-7 days | Instant (on-chain) |
| **Gasless Support** | N/A | ✅ Server can pay network fees |
| **Price Source** | Stripe Price IDs | Resource config (locked at quote) |
| **Expiration** | Session-based | 15 minutes |
| **Callback Metadata** | ✅ All item details | ✅ All item details |
| **Use Case** | Traditional fiat payments | Instant crypto, agent-to-agent |

Both cart systems provide identical callback metadata structure for inventory/fulfillment processing.

### Coupon Validation

**POST {prefix}/paywall/v1/coupons/validate** 🆕
- Validate coupon code and return discount information
- Check if coupon applies to specific products
- Returns scope ("all" for site-wide, "specific" for product-specific)
- Example request:
  ```json
  {
    "code": "SAVE20",
    "productIds": ["demo-item-id-1", "demo-item-id-2"]
  }
  ```
- Example response (site-wide coupon):
  ```json
  {
    "valid": true,
    "code": "SAVE20",
    "discountType": "percentage",
    "discountValue": 20.0,
    "scope": "all",
    "applicableProducts": ["demo-item-id-1", "demo-item-id-2"],
    "expiresAt": "2025-12-31T23:59:59Z"
  }
  ```
- Example response (product-specific coupon):
  ```json
  {
    "valid": true,
    "code": "DEMO50",
    "discountType": "percentage",
    "discountValue": 50.0,
    "scope": "specific",
    "applicableProducts": ["demo-item-id-1"],
    "expiresAt": "2025-12-31T23:59:59Z"
  }
  ```
- **Important:** For cart purchases, individual product restrictions are ignored and discount applies to entire cart total
- **Frontend usage:** Check `scope` field to determine if coupon is site-wide (`"all"`) or product-specific (`"specific"`)
- **Coupon Stacking:** All matching auto-apply coupons + 1 manual coupon stack automatically (percentage discounts first, then fixed)
- **Two-Phase System:** Catalog coupons show on product pages, checkout coupons apply at cart
- See [docs/FRONTEND_MIGRATION_COUPON_PHASES.md](./docs/FRONTEND_MIGRATION_COUPON_PHASES.md) for frontend migration guide
- See [API_REFERENCE.md](./docs/API_REFERENCE.md#validate-coupon) for complete documentation

#### Coupon Configuration (Two-Phase System)

Coupons now support two display phases to improve user experience:

**Catalog-level coupons** - Product-specific discounts shown on product pages:
```yaml
coupons:
  PRODUCT20:
    discount_type: percentage
    discount_value: 20
    scope: specific                # Must be "specific" for catalog
    product_ids: ["demo-item-1"]   # Required for catalog coupons
    payment_method: x402
    auto_apply: true
    applies_at: catalog              # Show on product page
    active: true
```

**Checkout-level coupons** - Site-wide discounts applied at cart/checkout:
```yaml
coupons:
  SITE10:
    discount_type: percentage
    discount_value: 10
    scope: all                     # Must be "all" for checkout
    payment_method: x402
    auto_apply: true
    applies_at: checkout             # Show only at cart
    active: true
```

**How it works:**

1. **Product Pages:** Quote includes catalog coupons in `extra` field
   ```json
   {
     "crypto": {
       "maxAmountRequired": "800000",
       "extra": {
         "original_amount": "1.000000",
         "discounted_amount": "0.800000",
         "applied_coupons": "PRODUCT20"
       }
     }
   }
   ```

2. **Cart/Checkout:** Both catalog AND checkout coupons stack
   ```
   Item A: $10 → $8 (catalog coupon)
   Item B: $5 → $5 (no catalog coupon)
   Subtotal: $13
   SITE10 (checkout): $13 → $11.70
   Final Total: $11.70
   ```

**Migration:** Existing coupons without `applies_at` continue to work (backward compatible). See [docs/FRONTEND_MIGRATION_COUPON_PHASES.md](./docs/FRONTEND_MIGRATION_COUPON_PHASES.md) for frontend integration guide.

### Solana Payment - Regular Mode (User Pays All Fees)

**POST {prefix}/paywall/v1/verify** (with `X-PAYMENT` header containing fully-signed transaction)
- Verify fully-signed transaction and return success confirmation
- User pays all fees (network fees + token transfer)
- Transaction must be complete and signed by user
- Server verifies transaction on-chain before granting access
- **Note:** Each signature can only be verified once (replay protection)

**GET {prefix}/paywall/v1/x402-transaction/verify?signature={signature}** 🆕 *(Re-access verification)*
- Verify that an x402 transaction was previously completed and paid
- Query parameter: `signature` (Solana transaction signature)
- Returns payment details: resource_id, wallet, amount, metadata, paid_at
- **Use case:** User paid with crypto, stored signature, wants to re-access without paying again
- Example request:
  ```bash
  GET /paywall/v1/x402-transaction/verify?signature=5Kn8...
  ```
- Example response (200 - verified):
  ```json
  {
    "verified": true,
    "resource_id": "demo-content",
    "wallet": "2TRi...",
    "paid_at": "2025-11-11T13:45:00Z",
    "amount": "$1.00 USDC",
    "metadata": {
      "userId": "12345"
    }
  }
  ```
- **Frontend should verify:** Check that `wallet` matches currently connected wallet before granting access
- **Security:** Prevents sharing transaction signatures between different users

### Solana Payment - Gasless Mode (Server Pays Network Fees)

**POST {prefix}/paywall/v1/gasless-transaction**
- Build complete unsigned transaction with compute budget instructions
- Returns base64-encoded transaction for user to partially sign
- Server designates fee payer but doesn't sign yet
- Transaction includes: compute budget, transfer, and memo instructions

**POST {prefix}/derive-token-account**
- Derive associated token account address for a wallet and mint
- Used by frontend to determine user's token account before building transaction
- Returns the derived ATA address

**POST {prefix}/paywall/v1/verify** (with `X-PAYMENT` header containing partially-signed transaction)
- Co-sign partially-signed transaction as fee payer and submit to network
- Server pays network fees (~0.000005 SOL), user only pays token transfer
- Returns protected content after transaction confirmation

---

## 💳 Key Features

- 🪙 **Dual payment support** — Card + Crypto
- ⚡ **Instant agentic payments** — Pay per request via x402
- 🏷️ **Two-phase coupon system** — Catalog-level discounts shown on product pages, checkout-level coupons at cart
- 🎁 **Coupon stacking** — Auto-apply + manual coupons stack automatically with percentage-first strategy
- 🔐 **Stateless & secure** — No need for user accounts or deposit addresses
- 🌍 **Open source** — MIT-licensed and extensible
- 🧱 **Minimal integration** — Middleware or proxy for Go APIs
- 🚦 **Multi-tier rate limiting** — Per-wallet, per-IP, and global rate limiting to prevent spam
- 🚦 **RPC rate limiting** — Transaction queue with configurable send delays and concurrency limits
- 🔄 **Retry logic** — Automatic retries for transient RPC failures
- 💰 **Gasless transactions** — Server can pay network fees for user transactions
- 🔁 **Multi-wallet support** — Load balance across multiple server wallets
- 🏦 **Auto-create token accounts** — Automatically create missing token accounts for users
- 📊 **User-friendly errors** — Clear, actionable error messages for payment failures
- 🔔 **Wallet monitoring** — Low balance alerts via Discord/Slack webhooks
- 📈 **Prometheus metrics** — Comprehensive observability for payments, webhooks, rate limits, and performance

---

## 🚀 Quick Start

Choose your path based on your needs:

| Path | Time | Best For |
|------|------|----------|
| **[Option 1: Try It](#option-1-try-it-5-minutes)** | 5 min | Testing locally, exploring features |
| **[Option 2: Standalone Microservice](#option-2-deploy-as-standalone-microservice)** | 10 min | Non-Go backends, production deployment |
| **[Option 3: Integrate into Go API](#option-3-integrate-into-go-api)** | 15 min | Existing Go codebases, tight integration |

For comprehensive step-by-step setup instructions, see [Local Development Setup](#️-local-development-setup).

---

### Option 1: Try It (5 Minutes)

Get Cedros running locally with minimal configuration:

```bash
# 1. Clone and install
git clone https://github.com/CedrosPay/server.git
cd server
go mod download

# 2. Copy example config to local.yaml (gitignored)
cp configs/config.example.yaml configs/local.yaml

# 3. Add your Stripe test key (get from https://dashboard.stripe.com/test/apikeys)
# Edit configs/local.yaml and set stripe.secret_key

# 4. Run server with local config
make run ARGS="--config configs/local.yaml"

# OR use go run directly
go run ./cmd/server --config configs/local.yaml
```

**Verify it's working:**

```bash
# Health check
curl http://localhost:8080/cedros-health

# Try creating a payment session
curl -X POST http://localhost:8080/paywall/v1/stripe-session \
  -H "Content-Type: application/json" \
  -d '{"resource": "demo-content", "customerEmail": "test@example.com"}'
```

**Next steps:**
- Add Solana wallet for crypto payments (see [Local Development Setup](#️-local-development-setup))
- Set up Stripe webhooks with [Stripe CLI](#stripe-cli-webhook-forwarding)
- Configure products in `configs/local.yaml`

#### Running the Server - Complete Guide

**For Local Development:**

```bash
# 1. Copy example to local config (gitignored)
cp configs/config.example.yaml configs/local.yaml

# 2. Edit local.yaml with your settings
vim configs/local.yaml

# 3. Run with local config
make run ARGS="--config configs/local.yaml"

# OR for live reload during development
make dev
```

**For Testing with Real Stripe:**

```bash
# Set Stripe keys in .env (gitignored)
cat > .env << EOF
CEDROS_STRIPE_SECRET_KEY="sk_test_YOUR_KEY"
CEDROS_STRIPE_WEBHOOK_SECRET="whsec_YOUR_SECRET"
EOF

# Run with local.yaml + env overrides
go run ./cmd/server --config configs/local.yaml
```

**For Production:**

```bash
# Create production config (DO NOT commit this)
cp configs/config.example.yaml configs/production.yaml
vim configs/production.yaml  # Set production values

# Build optimized binary
make prod-build

# Run on server
./bin/server-linux-amd64 --config configs/production.yaml
```

**Common Commands:**

```bash
# Show version
go run ./cmd/server --version

# Run with default config (config.example.yaml)
make run

# Run with custom config
go run ./cmd/server --config path/to/config.yaml

# Build and run binary
make build
./bin/server --config configs/local.yaml
```

---

### Option 2: Deploy as Standalone Microservice

Run Cedros as an independent service that any backend (Python, Node, Ruby, etc.) can call:

> **⚠️ PRODUCTION REQUIREMENT**: PostgreSQL or MongoDB is **required** for production deployments.
> The default file-based storage is **NOT safe** for concurrent requests or horizontal scaling.
> See [Production Deployment Guide](docs/PRODUCTION.md) for setup instructions.

**With Docker + PostgreSQL:**

```bash
# Create config file
cat > production.yaml <<EOF
storage:
  backend: "postgres"  # REQUIRED for production
  postgres_url: "${DATABASE_URL}"

stripe:
  secret_key: "${STRIPE_SECRET_KEY}"

x402:
  payment_address: "${X402_PAYMENT_ADDRESS}"
  token_mint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
  network: "mainnet-beta"
  rpc_url: "${SOLANA_RPC_URL}"
EOF

# Run container
docker run -d \
  --name cedros-pay \
  -p 8080:8080 \
  -v $(pwd)/production.yaml:/app/config.yaml \
  -e STRIPE_SECRET_KEY="sk_live_..." \
  cedros-pay/server:latest
```

**From source:**

```bash
# Build binary
go build -o cedros-server ./cmd/server

# Run as service
./cedros-server --config production.yaml
```

**Call from your API:**

```python
# Python example
import requests

response = requests.post('http://cedros-service:8080/paywall/stripe-session', json={
    'resource': 'premium-plan',
    'customerEmail': user.email,
    'metadata': {'user_id': str(user.id)}
})

checkout_url = response.json()['url']
```

See [Deployment Patterns](#️-deployment-patterns) for architecture diagrams and language-specific examples.

---

### Option 3: Integrate into Go API

Embed Cedros directly into your existing Go codebase:

```go
package main

import (
    "log"
    "net/http"

    "github.com/CedrosPay/server/pkg/cedros"
    "github.com/go-chi/chi/v5"
)

func main() {
    // Load Cedros configuration
    cfg, err := cedros.LoadConfig("configs/production.yaml")
    if err != nil {
        log.Fatal(err)
    }

    // Create Cedros app with custom payment notifier
    app, err := cedros.NewApp(cfg,
        cedros.WithNotifier(&MyPaymentHandler{}),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Create your main router
    router := chi.NewRouter()

    // Your existing routes
    router.Get("/", homeHandler)
    router.Get("/api/users", listUsersHandler)

    // Mount Cedros payment routes
    router.Mount("/payments", app.Router())

    // Start server
    log.Println("Server starting on :8080")
    http.ListenAndServe(":8080", router)
}

// Handle successful payments
type MyPaymentHandler struct{}

func (h *MyPaymentHandler) PaymentSucceeded(ctx context.Context, event cedros.PaymentEvent) error {
    // Your business logic here
    log.Printf("Payment received: %+v", event)

    // Grant credits, unlock content, etc.
    userID := event.Metadata["user_id"]
    grantAccess(userID, event.ResourceID)

    return nil
}
```

**Install package:**

```bash
go get github.com/CedrosPay/server
```

See [Deployment Patterns](#️-deployment-patterns) for more integration examples with Gin, Echo, and custom verifiers.

---

### Stripe CLI Webhook Forwarding

For local development, forward Stripe webhooks to your localhost:

```bash
# Install Stripe CLI
brew install stripe/stripe-cli/stripe   # macOS
# or download from https://stripe.com/docs/stripe-cli

# Login to Stripe
stripe login

# Forward webhooks to local server
stripe listen --forward-to localhost:8080/webhook/stripe
```

The CLI prints a signing secret:

```
> Ready! Your webhook signing secret is whsec_...
```

**Update `configs/local.yaml`:**

```yaml
stripe:
  webhook_secret: "whsec_YOUR_SECRET_HERE"
```

**Test webhooks:**

```bash
# Trigger test event
stripe trigger checkout.session.completed

# Check server logs for webhook processing
```

For production, configure webhooks in Stripe Dashboard pointing to your HTTPS endpoint.

---

### Quick API Tests (curl)

Run these in a terminal to confirm the backend is reachable even without the frontend UI (replace `item-id-1` or session IDs with your own values):

```bash
# 1. Health check (always at /cedros-health, returns route prefix if configured)
curl -s http://localhost:8080/cedros-health | jq

# 2. CORS preflight from Storybook (adjust the origin if needed)
curl -i -X OPTIONS http://localhost:8080/paywall/v1/stripe-session \
  -H "Origin: http://localhost:6006" \
  -H "Access-Control-Request-Method: POST" \
  -H "Access-Control-Request-Headers: content-type"

# 3. Create a Stripe checkout session (requires the resource to have stripe_price_id)
curl -s http://localhost:8080/paywall/v1/stripe-session \
  -H "Origin: http://localhost:6006" \
  -H "Content-Type: application/json" \
  -d '{
        "resourceId": "item-id-1",
        "metadata": { "user_id": "demo-user" }
      }'

# 4. Inspect the Stripe webhook helper page (GET)
curl http://localhost:8080/webhook/stripe

# 5. Request a paywall-protected resource to fetch an x402 quote (expect HTTP 402 with quote JSON)
curl -i http://localhost:8080/paywall/item-id-1

# 6. Inspect the quote payload (matching the published 402 spec)
curl -s http://localhost:8080/paywall/item-id-1 | jq '{status, quote, stripe, message}'

# 7. Verify access with a completed Stripe session (replace SESSION_ID after checkout)
curl -i http://localhost:8080/paywall/item-id-1 \
  -H "X-Stripe-Session: cs_test_a1b2c3"

# 8. Hit the dev success page directly (optional sanity check)
curl http://localhost:8080/stripe/success?session_id=cs_test_a1b2c3

# 9. Generate an x402 payment proof (signs the transfer with your Solana keypair but does NOT call the paywall)
export SOLANA_KEYPAIR=~/.config/solana/id.json
eval "$(go run ./cmd/tests/x402pay --config configs/local.yaml --resource item-id-1 --keypair $SOLANA_KEYPAIR --server http://localhost:8080)"

# 10. Authorize access with the generated proof (expect HTTP 200 after the transaction settles)
curl -i http://localhost:8080/paywall/item-id-1 \
  -H "X-PAYMENT: $X_PAYMENT_HEADER"

# 11. Example failure: tamper with the memo to watch Cedros reject the payment
export BAD_X_PAYMENT_HEADER=$(python - <<'PY'
import base64, json, os
payload = json.loads(base64.b64decode(os.environ["X_PAYMENT_HEADER"]))
payload["memo"] = payload.get("memo", "") + "-tampered"
print(base64.b64encode(json.dumps(payload).encode()).decode())
PY
)
curl -i http://localhost:8080/paywall/item-id-1 \
  -H "X-PAYMENT: $BAD_X_PAYMENT_HEADER"
```

Protected endpoints expect either:

- `X-Stripe-Session: <session_id>` where the session completed via webhook, or
- `X-PAYMENT: <base64 payment proof>` generated by an x402 client.

For x402, the `X-PAYMENT` payload must include the base64-encoded signed transaction and signature; the server parses the transaction, confirms it sends the expected SPL token transfer, submits it to your Solana RPC/WS endpoints, and waits for a finalized confirmation before continuing.

Tweak Solana verification behaviour with environment overrides such as `X402_SKIP_PREFLIGHT=true` or `X402_COMMITMENT=confirmed` if you need different settlement semantics.

When a payment is verified (Stripe or x402), the server forwards a JSON payload to `callbacks.payment_success_url` if set. Use that hook to mark users as paid or trigger downstream workflows; the payload includes the resource id, method (`stripe` or `x402`), amounts, wallet/session info, and metadata.
By default Cedros POSTs this structured event. Adding a literal `callbacks.body` value overrides the payload entirely—remove the field or set it to `""` if you want the full JSON delivered downstream. Define the endpoint in `configs/local.yaml` or via `CALLBACK_PAYMENT_SUCCESS_URL` and add custom headers with `callbacks.headers` or environment variables like `CALLBACK_HEADER_AUTHORIZATION=Bearer <token>`.
Example callback JSON:

```json
{
  "resourceId": "demo-item-id-1",
  "method": "x402",
  "fiatAmountCents": 100,
  "fiatCurrency": "usd",
  "cryptoAtomicAmount": 1000000,
  "cryptoToken": "USDC",
  "wallet": "payer-wallet",
  "metadata": {
    "plan": "demo",
    "user_id": "example-user"
  },
  "paidAt": "2025-11-04T15:17:35Z"
}
```

Have the frontend pass request-specific metadata (e.g. `user_id`, `email`) when it calls `/paywall/stripe-session` or embeds the x402 proof, and they will be merged into `metadata` for your webhook.
Need Discord-friendly text? Use `callbacks.body_template` to render the event into whatever JSON your webhook expects, for example:

```yaml
callbacks:
  payment_success_url: "https://discord.com/api/webhooks/..."
  headers:
    Content-Type: application/json
  body_template: |
    {"content":"✅ {{.Method}} payment for {{.ResourceID}} ({{.CryptoAtomicAmount}} atomic {{.CryptoToken}})"}
  timeout: 5s
```

Templates receive the `PaymentEvent` fields (`ResourceID`, `Method`, `FiatAmountCents`, `CryptoAtomicAmount`, `Metadata`, etc.), so you can include dynamic context in Discord, Slack, or any custom webhook format.
Send a synthetic callback for smoke testing:

```bash
go run ./cmd/tests/callbacktest --config configs/local.yaml --resource test-item --method test --amount 1.23 --wallet AgentWallet
```

---

## 🛠️ Local Development Setup

**For comprehensive local setup**, expand this section. For quick start, see [Quick Start](#-quick-start) above.

<details>
<summary><strong>Click to expand full local development guide</strong></summary>

### Step 1: Prerequisites Check

Verify you have the required tools installed:

```bash
# Check Go version (need 1.21+)
go version

# Check Git
git --version

# Optional: Check Solana CLI (for wallet management)
solana --version

# Optional: Check Docker (if using containerized approach)
docker --version
```

**If missing:**
- **Go:** Download from https://go.dev/dl/
- **Solana CLI:** `sh -c "$(curl -sSfL https://release.solana.com/stable/install)"`
- **Docker:** Download from https://docker.com/get-started

### Step 2: Clone and Install Dependencies

```bash
# Clone repository
git clone https://github.com/CedrosPay/server.git
cd server

# Download Go dependencies
go mod download

# Verify build works
go build ./cmd/server
```

**Expected output:** Binary created at `./server` (or `./server.exe` on Windows)

### Step 3: Configure Stripe (Test Mode)

1. **Get Stripe Test Keys:**
   - Go to https://dashboard.stripe.com/test/apikeys
   - Copy **Secret key** (starts with `sk_test_`)

2. **Update `configs/local.yaml`:**
   ```yaml
   stripe:
     secret_key: "sk_test_YOUR_KEY_HERE"
     mode: "test"
   ```

3. **Create Test Products:** Create products in Stripe Dashboard and copy Price IDs

### Step 4: Configure Solana (Devnet)

```bash
# Create new wallet for devnet
solana-keygen new --outfile ~/.config/solana/devnet.json

# Set to devnet
solana config set --url devnet

# Get free devnet SOL
solana airdrop 2
```

**Update `configs/local.yaml`:**

```yaml
x402:
  payment_address: "YOUR_WALLET_ADDRESS"  # From 'solana address'
  token_mint: "Gh9ZwEmdLJ8DscKNTkTqPbNwLNNBjuSzaG9Vp2KGtKJr"  # Devnet USDC
  network: "devnet"
  rpc_url: "https://api.devnet.solana.com"
```

### Step 5: Start Server

```bash
go run ./cmd/server --config configs/local.yaml
```

### Step 6: Verify Setup

```bash
# Health check
curl http://localhost:8080/cedros-health

# Get products
curl http://localhost:8080/paywall/v1/products
```

**For complete troubleshooting guide**, see full instructions in [docs/DEPLOYMENT.md](./docs/DEPLOYMENT.md#local-development).

</details>

---

### Embedding in Go

Cedros Pay is designed to be used both as a standalone server and as an embeddable Go library. The codebase follows Go's package visibility conventions:

- **`pkg/`** — Public packages that external developers can import
- **`internal/`** — Private implementation details (cannot be imported by external code)

#### Quick Start: Full Integration

Add Cedros Paywall routes to your existing Go service:

```go
import (
    "github.com/CedrosPay/server/pkg/cedros"
    "github.com/go-chi/chi/v5"
)

cfg, _ := cedros.LoadConfig("configs/local.yaml")
app, _ := cedros.NewApp(cfg)
router := chi.NewRouter()
router.Mount("/cedros", app.Router())
// add your own routes...
```

`cedros.NewApp` provides a batteries-included integration with all features enabled. Customize with options:

```go
import (
    "github.com/CedrosPay/server/internal/metrics"
    "github.com/CedrosPay/server/pkg/cedros"
)

recorder := &metrics.SimpleRecorder{}
app, _ := cedros.NewApp(cfg,
    cedros.WithMetrics(recorder),
    cedros.WithStore(customStore),
    cedros.WithNotifier(customNotifier),
    cedros.WithRouter(existingRouter),
)
```

Available options:
- `cedros.WithStore` — Custom storage backend (default: in-memory)
- `cedros.WithNotifier` — Custom payment notification handler
- `cedros.WithVerifier` — Custom x402 payment verifier
- `cedros.WithMetrics` — Custom metrics recorder
- `cedros.WithRouter` — Use existing chi.Router instead of creating new one

#### Custom Integration: Using Core Packages

For custom integrations, import the core x402 packages directly:

```go
import (
    "github.com/CedrosPay/server/pkg/x402"
    "github.com/CedrosPay/server/pkg/x402/solana"
)

// Create Solana verifier
verifier, err := solana.NewSolanaVerifier(
    "https://api.mainnet-beta.solana.com",
    "wss://api.mainnet-beta.solana.com",
)
if err != nil {
    return err
}
defer verifier.Close()

// Verify payment
result, err := verifier.Verify(ctx, paymentProof, requirement)
if err != nil {
    // Handle verification error
    return err
}

// Access verification result
fmt.Printf("Payment verified from %s for %f tokens\n",
    result.Wallet, result.Amount)
```

#### Package Structure

**Public Packages (importable):**

```
pkg/
├── cedros/          # High-level integration (NewApp, LoadConfig)
├── responders/      # HTTP response helpers
├── x402/            # Core x402 types and interfaces
│   ├── types.go         # PaymentProof, Requirement, VerificationResult
│   ├── errors.go        # VerificationError, error codes
│   ├── constants.go     # Timeouts, tolerances
│   └── solana/      # Solana implementation
│       ├── verifier.go      # SolanaVerifier (implements x402.Verifier)
│       ├── health.go        # WalletHealthChecker
│       ├── builder.go       # Transaction builder
│       ├── queue.go         # Transaction queue (rate limiting)
│       └── ...
```

**Core Types:**

```go
// x402.Verifier interface - implement this for custom payment networks
type Verifier interface {
    Verify(ctx context.Context, proof PaymentProof, requirement Requirement) (VerificationResult, error)
}

// PaymentProof - payment details from client
type PaymentProof struct {
    X402Version           int
    Scheme                string // "solana-spl-transfer"
    Network               string // "mainnet-beta", "devnet"
    Signature             string
    Payer                 string
    Transaction           string
    Memo                  string
    Metadata              map[string]string
    RecipientTokenAccount string
}

// Requirement - verification constraints
type Requirement struct {
    ResourceID            string
    RecipientOwner        string
    RecipientTokenAccount string
    TokenMint             string
    Amount                float64
    Network               string
    TokenDecimals         uint8
    AllowedTokens         []string
    QuoteTTL              time.Duration
    SkipPreflight         bool
    Commitment            string
}

// VerificationResult - successful verification details
type VerificationResult struct {
    Wallet      string
    Amount      float64
    Signature   string
    ExpiresAt   time.Time
    Metadata    map[string]string
}
```

#### Implementing Custom Verifiers

Create custom payment verifiers for other blockchains:

```go
type MyCustomVerifier struct {
    // your fields
}

func (v *MyCustomVerifier) Verify(ctx context.Context, proof x402.PaymentProof, req x402.Requirement) (x402.VerificationResult, error) {
    // 1. Parse proof.Transaction
    // 2. Verify signature matches proof.Signature
    // 3. Check amount >= req.Amount
    // 4. Check recipient matches req.RecipientTokenAccount
    // 5. Submit transaction to blockchain
    // 6. Wait for confirmation

    return x402.VerificationResult{
        Wallet:    proof.Payer,
        Amount:    req.Amount,
        Signature: proof.Signature,
        ExpiresAt: time.Now().Add(1 * time.Hour),
    }, nil
}
```

Then use it with `cedros.NewApp`:

```go
app, _ := cedros.NewApp(cfg, cedros.WithVerifier(customVerifier))
```

#### Why This Structure?

- **Flexibility**: Use full integration or cherry-pick components
- **Extensibility**: Implement `x402.Verifier` for new payment networks
- **Go Conventions**: Follows Go's `internal/` vs `pkg/` visibility rules
- **Backward Compatibility**: Existing code continues to work unchanged

---

## 🔧 Advanced Configuration

### Transaction Queue (RPC Rate Limiting)

Control transaction submission rate to match your RPC provider's limits:

```yaml
x402:
  # Minimum time between transaction sends (e.g., "100ms", "1s")
  tx_queue_min_time_between: "0s"

  # Maximum concurrent "in-flight" (sent but pending) transactions
  # Set to 0 for unlimited
  tx_queue_max_in_flight: 0
```

**How it works:**
- Transactions queue for sending, respecting the minimum time between sends
- Rate-limited transactions are automatically re-queued to the TOP with exponential backoff
- Concurrent transaction limit prevents overwhelming the RPC with pending confirmations
- Perfect for managed RPC plans with strict rate limits

**Environment overrides:**
```bash
X402_TX_QUEUE_MIN_TIME_BETWEEN=100ms
X402_TX_QUEUE_MAX_IN_FLIGHT=5
```

### Rate Limiting

Protect your server from spam and abuse with multi-tier rate limiting:

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

**How it works:**
- **Three-tier protection:** Global → Per-Wallet → Per-IP
- **Wallet identification:** Extracts wallet address from `X-Wallet` header, `X-Signer` header, or `wallet` query parameter
- **Graceful fallback:** If no wallet header is present, uses IP-based limiting
- **Token bucket algorithm:** Allows bursts while maintaining average rate
- **HTTP 429 response:** Returns `Too Many Requests` with `Retry-After` header when limit exceeded

**Default limits (generous to prevent spam without restricting legitimate use):**
- Global: 1000 req/min (handles ~16 concurrent users at 1 req/sec)
- Per-Wallet: 60 req/min (1 request/second average)
- Per-IP: 120 req/min (2 requests/second average)

**Rate limit response example:**
```json
{
  "error": "rate_limit_exceeded",
  "message": "Rate limit exceeded for wallet 8xW..abc. Please try again later."
}
```

**Disabling rate limits (not recommended for production):**
```yaml
rate_limit:
  global_enabled: false
  per_wallet_enabled: false
  per_ip_enabled: false
```

### Multi-Wallet Support

Load balance transaction submissions across multiple server wallets:

```yaml
x402:
  server_wallet_keys:
    - "base58_private_key_1"
    - "base58_private_key_2"
    - "base58_private_key_3"
```

Wallets are selected in round-robin fashion. Each wallet can be rate-limited independently by the transaction queue.

### Gasless Transactions

Allow users to pay without holding SOL for transaction fees:

```yaml
x402:
  gasless_enabled: true
  server_wallet_keys:
    - "base58_private_key"  # Server pays fees from this wallet
```

**How it works:**
1. Frontend detects `feePayer` field in the quote (see FRONTEND_INTEGRATION.md)
2. User partially signs transaction (transfer authority only)
3. Backend co-signs as fee payer and submits to Solana
4. User's wallet only needs tokens (USDC), not SOL

### Auto-Create Token Accounts

Automatically create missing token accounts for users during payment:

```yaml
x402:
  auto_create_token_account: true
  server_wallet_keys:
    - "base58_private_key"  # Server pays rent for account creation
```

**How it works:**
1. Payment fails with "token account not found" error
2. Server automatically creates the Associated Token Account (ATA) for the user
3. Server retries the payment transaction
4. User receives seamless experience without manual account setup

**Cost:** ~0.002 SOL per account creation (one-time rent)

**Note:** Requires server wallets with SOL for rent payment. Works with both gasless and regular payment modes.

### Stripe Products & Pricing

Configure your Stripe products in the paywall config:

```yaml
paywall:
  resources:
    demo-content:
      description: "Demo protected content"
      fiat_amount: 1.0
      fiat_currency: usd
      stripe_price_id: "price_1SPqRpR4HtkFbUJKUciKecmZ"  # From Stripe Dashboard
      crypto_amount: 1.0
      crypto_token: "USDC"

    test-product-2:
      description: "Test product 2"
      fiat_amount: 2.22
      fiat_currency: usd
      stripe_price_id: "price_1SQCuhR4HtkFbUJKDUQpCA6D"  # From Stripe Dashboard
      crypto_amount: 2.22
      crypto_token: "USDC"
```

**How to get Stripe Price IDs:**

1. Go to [Stripe Dashboard → Products](https://dashboard.stripe.com/products)
2. Create a product (e.g., "100 Credits Package")
3. Add a price (e.g., $9.99 one-time payment)
4. Copy the Price ID (starts with `price_`)
5. Add to your `configs/local.yaml` under `paywall.resources`

**Single-item vs Cart checkout:**

- **Single-item:** Uses `resource` ID (e.g., "demo-content") → looks up price from config
  ```bash
  POST /paywall/v1/stripe-session
  {"resource": "demo-content"}
  ```

- **Cart checkout:** Uses `priceId` directly → no config lookup needed
  ```bash
  POST /paywall/v1/cart/checkout
  {"items": [{"priceId": "price_1SPqRpR4HtkFbUJKUciKecmZ", "quantity": 1}]}
  ```

**Tax Rates (Optional):**

Configure a default tax rate for all Stripe checkouts:

```yaml
stripe:
  tax_rate_id: "txr_1234567890"  # From Stripe Dashboard → Tax Rates
```

This tax rate applies to:
- ✅ Single-item checkouts (when using `fiat_amount` fallback)
- ✅ Cart checkouts (all line items)
- ❌ Checkouts using `stripe_price_id` directly (tax configured in product)

### Database-Backed Product Catalogs

Instead of managing products in YAML config files, you can store them in PostgreSQL or MongoDB for dynamic updates without server restarts.

**Choose your product source:**

```yaml
paywall:
  # product_source: "yaml"      # Default: YAML config (restart required for updates)
  product_source: "postgres"    # PostgreSQL database (dynamic updates)
  # product_source: "mongodb"   # MongoDB database (dynamic updates)

  # Cache product list to reduce database load (recommended: 5m for DB sources)
  product_cache_ttl: 5m          # Set to 0s to disable caching

  # PostgreSQL connection (only used if product_source = "postgres")
  postgres_url: "postgresql://user:password@localhost:5432/cedros_products?sslmode=disable"

  # MongoDB connection (only used if product_source = "mongodb")
  mongodb_url: "mongodb://localhost:27017"
  mongodb_database: "cedros_products"
  mongodb_collection: "products"  # Optional, defaults to "products"
```

**Benefits:**

| Feature | YAML | PostgreSQL | MongoDB |
|---------|------|------------|---------|
| Dynamic updates (no restart) | ❌ | ✅ | ✅ |
| Version controlled with code | ✅ | ❌ | ❌ |
| Relational queries | ❌ | ✅ | ❌ |
| Horizontal scaling | ❌ | ⚠️ | ✅ |
| Setup complexity | Easy | Medium | Medium |

**Database Schema:**

See [DATABASE_SCHEMA.md](DATABASE_SCHEMA.md) for complete details including:
- PostgreSQL and MongoDB table/collection structures
- Migration scripts from YAML to database
- Example queries and operations
- Admin API endpoint suggestions

**Quick PostgreSQL Setup:**

```sql
CREATE TABLE products (
    id VARCHAR(255) PRIMARY KEY,
    description TEXT NOT NULL,
    fiat_amount DECIMAL(10, 2),
    fiat_currency VARCHAR(3) DEFAULT 'usd',
    stripe_price_id VARCHAR(255),
    crypto_amount DECIMAL(18, 6),
    crypto_token VARCHAR(50),
    memo_template TEXT DEFAULT '{{resource}}:{{nonce}}',
    metadata JSONB DEFAULT '{}',
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insert demo product
INSERT INTO products (id, description, fiat_amount, crypto_amount, crypto_token, metadata)
VALUES ('demo-content', 'Demo protected content', 1.00, 1.00, 'USDC', '{"plan": "demo"}');
```

**Quick MongoDB Setup:**

```javascript
db.products.insertOne({
  _id: "demo-content",
  description: "Demo protected content",
  fiatAmount: 1.00,
  cryptoAmount: 1.00,
  cryptoToken: "USDC",
  metadata: { plan: "demo" },
  active: true,
  createdAt: new Date(),
  updatedAt: new Date()
});
```

**Environment variable overrides:**

```bash
# PostgreSQL
PAYWALL_PRODUCT_SOURCE=postgres
PAYWALL_POSTGRES_URL="postgresql://user:password@localhost:5432/cedros_products"

# MongoDB
PAYWALL_PRODUCT_SOURCE=mongodb
PAYWALL_MONGODB_URL="mongodb://localhost:27017"
PAYWALL_MONGODB_DATABASE="cedros_products"
PAYWALL_MONGODB_COLLECTION="products"
```

**Custom Metadata for Fulfillment:**

When creating products, you can add custom metadata that will be available in webhooks for order fulfillment:

```yaml
paywall:
  resources:
    credits-100:
      description: "100 Credits Package"
      fiat_amount: 9.99
      fiat_currency: usd
      stripe_price_id: "price_100_credits"
      crypto_amount: 9.99
      crypto_token: "USDC"
      metadata:
        credits: "100"           # Will be in webhook metadata
        grant_immediately: "true"
        product_type: "digital"
```

For cart checkouts, pass custom metadata per item:

```bash
POST /paywall/v1/cart/checkout
{
  "items": [
    {
      "priceId": "price_100_credits",
      "quantity": 2,
      "metadata": {
        "credits": "100",
        "bonus_credits": "10"
      }
    }
  ],
  "metadata": {
    "user_id": "12345",
    "source": "mobile_app"
  }
}
```

Your webhook receives:
```json
{
  "cart_items": "1",
  "item_0_price_id": "price_100_credits",
  "item_0_quantity": "2",
  "item_0_credits": "100",
  "item_0_bonus_credits": "10",
  "user_id": "12345",
  "source": "mobile_app"
}
```

This allows your callback to:
- Grant credits to user accounts
- Update inventory quantities
- Track referrals and campaigns
- Send personalized confirmation emails
- Trigger fulfillment workflows

See [Cart Webhooks & Callbacks](#cart-webhooks--callbacks---important) section above for processing examples.

### Wallet Balance Monitoring

Monitor server wallet balances and receive alerts when running low:

```yaml
monitoring:
  low_balance_alert_url: "https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_WEBHOOK_TOKEN"
  low_balance_threshold: 0.01  # SOL
  check_interval: 15m
  timeout: 5s
```

**Features:**
- Checks all server wallet balances periodically (default: every 15 minutes)
- Sends webhook alerts when balance falls below threshold (default: 0.01 SOL)
- 24-hour cooldown per wallet prevents alert spam
- Auto-clears alerts when balance is restored
- Discord, Slack, PagerDuty, or any webhook-compatible service

**Default alert format (Discord/Slack):**
```
⚠️ Low Balance Alert

Wallet: `<public_key>`
Balance: 0.005000 SOL
Threshold: 0.010000 SOL

Please add more SOL to continue processing gasless transactions.
```

**Custom templates:** Use Go templates to customize alert format (see [WALLET_MONITORING.md](WALLET_MONITORING.md))

**Environment overrides:**
```bash
MONITORING_LOW_BALANCE_ALERT_URL="https://discord.com/api/webhooks/..."
MONITORING_LOW_BALANCE_THRESHOLD="0.01"
MONITORING_CHECK_INTERVAL="15m"
```

**Why use it:** Prevents service disruption by alerting operators before wallets run out of SOL for network fees. Critical for production gasless deployments.

For detailed setup and examples, see **[WALLET_MONITORING.md](WALLET_MONITORING.md)**.

### RPC Retry Logic

Automatic retries for transient RPC failures:

- **Blockhash fetching**: Retries with exponential backoff (100ms → 200ms → 400ms)
- **Token account creation**: Retries on network errors, rate limits, 5xx errors
- **Transaction confirmation**: WebSocket-first with RPC polling fallback

**Configuration:**

```yaml
x402:
  # Enable preflight checks (disable for faster submission with retries)
  skip_preflight: false

  # Commitment level for transaction confirmation
  commitment: "finalized"  # or "confirmed"
```

**Confirmation reliability:**
- Tries WebSocket confirmation first (fastest)
- Falls back to RPC polling if WebSocket fails
- Polls every 2 seconds until transaction confirms or blockhash expires (~90 seconds)
- Never misses payments, even if WebSocket connection dies mid-confirmation

### Webhook Retry + Dead Letter Queue

Automatically retry failed webhook deliveries with exponential backoff to handle transient failures:

```yaml
callbacks:
  payment_success_url: "https://your-app.com/webhook"

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

**How it works:**

1. **Initial attempt**: Webhook fires immediately after successful payment
2. **Retry on failure**: If webhook fails (HTTP error, timeout, network error):
   - Retry 1: after 1 second
   - Retry 2: after 2 seconds (1s × 2.0)
   - Retry 3: after 4 seconds (2s × 2.0)
   - Retry 4: after 8 seconds (4s × 2.0)
   - Retry 5: after 16 seconds (8s × 2.0)
3. **Dead Letter Queue**: After all retries exhausted, save to DLQ file for manual processing

**Retry Strategy:**
- ✅ Retries on: HTTP 4xx/5xx errors, timeouts, network errors
- ✅ Exponential backoff prevents overwhelming failing endpoints
- ✅ Max interval cap prevents unbounded delays
- ✅ Async retry doesn't block payment processing

**DLQ File Format:**
```json
{
  "webhook_1730000000000": {
    "id": "webhook_1730000000000",
    "url": "https://your-app.com/webhook",
    "payload": {"resource": "demo-content", "method": "stripe", ...},
    "eventType": "payment",
    "attempts": 5,
    "lastError": "received status 503 from https://your-app.com/webhook",
    "lastAttempt": "2025-11-09T10:30:00Z",
    "createdAt": "2025-11-09T10:29:00Z"
  }
}
```

**Manual DLQ Processing:**
```bash
# View failed webhooks
cat ./data/webhook-dlq.json | jq .

# Retry a specific webhook manually (after fixing your endpoint)
curl -X POST https://your-app.com/webhook \
  -H "Content-Type: application/json" \
  -d '{"resource": "demo-content", ...}'  # Copy from DLQ payload

# Clear DLQ after processing
> ./data/webhook-dlq.json
echo "{}" > ./data/webhook-dlq.json
```

**Disabling retries (not recommended):**
```yaml
callbacks:
  retry:
    enabled: false  # Webhooks fire once with no retries
```

**Best practices:**
- Enable DLQ in production to prevent webhook loss
- Monitor DLQ file size - growing file indicates endpoint problems
- Set up alerting when DLQ has items (indicates delivery failures)
- Keep `max_attempts` at 5-10 for good balance of reliability and delay

### User-Friendly Error Messages

All payment errors return clear, actionable messages:

**Insufficient funds errors:**
- "Insufficient token balance. Please add more tokens to your wallet and try again."
- "Insufficient SOL for transaction fees. Please add some SOL to your wallet and try again."

**Account errors:**
- "Token account not found. Please create a token account for this token first."

**Transaction errors:**
- "Invalid transaction signature. Please try again."
- "Invalid payment memo. Please use the payment details provided by the quote."
- "Wrong token used for payment. Please use the correct token specified in the quote."
- "Payment amount is less than required. Please check the payment amount and try again."

**Blockchain errors:**
- "Transaction not found on the blockchain. It may have been dropped. Please try again."
- "Transaction timed out. Please check the blockchain explorer and try again if needed."

These messages can be displayed directly to users in your frontend UI. Technical details are logged server-side for debugging.

---

## 📊 Performance & Cost

### Performance Benchmarks

**Single Instance Throughput:**

| Operation | Requests/sec | Latency (p50) |
|-----------|-------------|---------------|
| Health check | 5,000+ | <1ms |
| Generate quote | 500-1,000 | 5-10ms |
| Stripe session | 100-200 | 50-100ms |
| Crypto verification | 50-100 | 1-3s |

**Bottlenecks:** Solana RPC latency (1-3s confirmation time) and Stripe API calls (100-200ms)

**Scaling:** Cedros is stateless and scales horizontally:
```bash
# Docker Compose
docker-compose up -d --scale cedros-pay=5

# Kubernetes
kubectl scale deployment cedros-pay --replicas=10
```

### Cost Estimation

**Solana Transaction Costs (mainnet):**

| Operation | SOL Cost | USD Cost (SOL=$20) |
|-----------|----------|-------------------|
| Token transfer | ~0.000005 | $0.0001 |
| Create token account | ~0.00203 | $0.04 |

**Monthly costs (1,000 payments/day):**
- Without auto-create: $3/month
- With auto-create (20% new users): $243/month

**RPC Provider Costs:**
- Public (free): ~10 req/s - development only
- Helius ($50-500/mo): 100-10,000 req/s
- QuickNode ($49-999/mo): 100-25,000 req/s

**Total cost per 10,000 crypto payments:**
- Solana fees: $1
- RPC provider: $50
- Infrastructure (AWS EC2 t3.small): $22
- **Total: ~$73/month** ($0.0073 per payment)

**Compare to Stripe:** For $10 payment, Stripe charges $0.59 (60× more expensive)

For detailed cost analysis and optimization strategies, see [docs/DEPLOYMENT.md#cost-estimation](./docs/DEPLOYMENT.md).

---

## 🔄 Migration Guide

### Migrating from Stripe-Only

**Option A: Add Crypto Alongside Existing Stripe**

Keep existing code, add crypto payments as new option:

```python
def create_checkout(product_id, user, payment_method='card'):
    if payment_method == 'card':
        return create_stripe_checkout(product_id, user)  # Existing code
    elif payment_method == 'crypto':
        return cedros.request_quote(product_id)  # New crypto path
```

**Option B: Migrate to Cedros for All Payments**

Replace Stripe SDK with Cedros API:

```python
# Before (direct Stripe SDK)
session = stripe.checkout.Session.create(...)

# After (Cedros API handles Stripe internally)
session = requests.post(f"{CEDROS_URL}/paywall/stripe-session", json={
    'resource': product_id,
    'stripe_price_id': 'price_existing_id'  # Keep existing Price IDs!
}).json()
```

**No need to recreate Stripe products** - reference existing `price_id` in Cedros config.

### Migrating from Manual Crypto

Replace polling loops with x402 verification:

```python
# Before: Manual transaction monitoring
while True:
    transactions = solana_rpc.get_signatures_for_address(wallet)
    # Check each transaction...
    time.sleep(5)

# After: x402 verification
response = requests.get(f"{CEDROS_URL}/paywall/{resource_id}",
    headers={'X-PAYMENT': payment_proof})
# Payment verified instantly!
```

### Migrating to Database-Backed Products

Move from YAML to PostgreSQL for dynamic pricing:

```yaml
paywall:
  product_source: postgres  # Changed from 'yaml'
  postgres_url: "postgresql://..."
  product_cache_ttl: 5m
```

Products now managed via database - update via admin API without restarts!

For complete migration guides, see comprehensive documentation in planning docs.

---

## 📚 Frontend Integration

For comprehensive frontend integration documentation, see **[FRONTEND_INTEGRATION.md](FRONTEND_INTEGRATION.md)**.

The guide covers:
- Route discovery via `/cedros-health`
- Payment quote structure
- Stripe and crypto payment flows (regular + gasless)
- Helper endpoints (`/recent-blockhash`, `/derive-token-account`)
- Complete error handling examples
- Full TypeScript integration examples

**Quick example:**

```typescript
// 1. Discover routes
const health = await fetch('http://localhost:8080/cedros-health').then(r => r.json());
const prefix = health.routePrefix || '';

// 2. Get quote
const quote = await fetch(`${prefix}/paywall/item-id-1`).then(r => r.json());

// 3. Check for gasless support
const isGasless = quote.crypto?.extra?.feePayer !== undefined;

// 4. Pay with crypto (regular or gasless)
// See FRONTEND_INTEGRATION.md for complete examples
```

---

## Integration FAQ

- **How do users choose between Stripe and crypto?**
  Frontend can request quotes from both `/paywall/stripe-session` (for Stripe) and `/paywall/quote` (for x402 crypto). Present both options in the UI; the frontend then either redirects to Stripe Checkout or signs the quoted x402 payment and verifies via `/paywall/verify` with `X-PAYMENT` header.

- **How do we identify the paying user in callbacks?**
  Include user-specific fields (e.g. `user_id`, `email`) in the `metadata` object when you call `/paywall/stripe-session`, or embed them in the x402 proof metadata. Cedros merges request metadata with the static `paywall.resources[].metadata` map and echoes everything back in the callback payload. That same metadata also lives on the Stripe Checkout session for cross-referencing.

- **What happens on failed payments?**
  Stripe failures surface through standard Checkout errors and do not invoke the Cedros callback until the webhook confirms completion. Crypto verification failures return user-friendly error messages immediately (e.g. "Insufficient token balance" or "Insufficient SOL for transaction fees"). Display these messages directly to users—no callback fires until payment succeeds.

- **How do refunds work?**
  Cedros supports crypto refunds via the `/paywall/refund-request` API. Anyone can submit a refund request. Admin reviews and either approves or denies. For approval: admin gets a quote via `/paywall/refund-approve`, builds/signs the transaction with the payTo wallet (paying both refund amount AND network fees), submits to Solana, then verifies via `/paywall/verify`. Gasless mode is NOT used for refunds. For Stripe refunds, use Stripe's dashboard or API directly. See [docs/API_REFERENCE.md](docs/API_REFERENCE.md#refunds) for details.

- **Do we need to handle currency conversion?**
  Prices are locked at quote time. When you charge in USDC (or another stable SPL token) the fiat and crypto amounts typically match 1:1. If you want dynamic exchange rates, regenerate the quote before presenting it or update `paywall.resources[].crypto_amount` via your own process.

- **Can we apply tax rates?**
  Set `stripe.tax_rate_id` in your config to attach a Stripe Tax Rate when Cedros creates ad-hoc prices (i.e. when you omit `stripe_price_id`). If you rely on saved Prices, configure tax behaviour inside Stripe instead.

- **How does the frontend discover the route prefix?**
  Call `GET /cedros-health` which is always available at a fixed location (not prefixed). The response includes `routePrefix` if one is configured, so your frontend can construct full URLs dynamically (e.g., `${routePrefix}/paywall/stripe-session`). This allows deploying Cedros behind any path without hardcoding routes.

- **Should I enable gasless transactions?**
  Gasless removes friction for users who only have tokens (USDC) but not SOL for fees. Enable it if you want to subsidize transaction costs. The server pays ~0.000005 SOL per transaction (~$0.0005 at $100/SOL). Disable it to require users to hold SOL for their own fees.

- **How do I handle RPC rate limits?**
  Configure `tx_queue_min_time_between` (e.g., "100ms") to pace transaction submissions. Set `tx_queue_max_in_flight` (e.g., 5) to limit concurrent pending transactions. Rate-limited transactions automatically retry at the front of the queue. This prevents hitting your RPC provider's rate limits.

- **What if transaction confirmation fails?**
  Cedros uses WebSocket-first with RPC polling fallback. If the WebSocket connection dies, the system automatically polls the RPC every 2 seconds until the transaction confirms or the blockhash expires (~90 seconds). Payments are never missed. Failed transactions (invalid memo, insufficient balance, etc.) return clear error messages immediately.

- **Can I use multiple server wallets?**
  Yes. List multiple `server_wallet_keys` in your config. Cedros will round-robin transactions across wallets for load balancing. Useful if you have multiple funded wallets or want to distribute transaction load.

- **What if a user doesn't have a token account?**
  Enable `auto_create_token_account: true` to automatically create missing Associated Token Accounts (ATAs) during payment. The server pays ~0.002 SOL rent per account (one-time). Without this feature, users must manually create their token accounts before paying.

- **How do I monitor server wallet balances?**
  Configure `monitoring.low_balance_alert_url` with a Discord or Slack webhook URL. The server checks wallet balances periodically (default: 15 minutes) and sends alerts when below threshold (default: 0.01 SOL). Critical for preventing service disruption in production. See [WALLET_MONITORING.md](WALLET_MONITORING.md) for details.

---

## 🪄 Example Use Cases

- Paywalled blog or API monetization
- Agent-to-agent microtransactions
- Subscription and one-time digital content unlocks
- AI service pay-per-call endpoints
- Per-request API pricing
- Digital content sales (ebooks, courses, videos)

---

## Deployment

### Quick Deployment

**Docker (Simplest):**
```bash
# 1. Copy environment template
cp .env.example .env

# 2. Edit with your credentials
nano .env

# 3. Start server
make docker-simple-up

# 4. Verify
curl http://localhost:8080/health
```

**Full Stack (with databases):**
```bash
make docker-up        # Start server + postgres + redis
make docker-logs      # View logs
make docker-down      # Stop all
```

**Standard Go Build:**
```bash
make install
make build
./bin/server
```

**Available Commands:**
- `make help` - See all available commands
- `make run` - Run server (auto-detects config.yaml)
- `make run ARGS="-config path.yaml"` - Run with custom config
- `make docker-build` - Build Docker image
- `make test` - Run tests
- `make prod-build` - Build production binary

**Passing Arguments:**
```bash
# Makefile - use ARGS
make run ARGS="-config configs/local.yaml"

# Docker - pass after image name
docker run CedrosPay/server:latest -config configs/custom.yaml

# Make with Docker
docker run CedrosPay/server:latest -version
```

For comprehensive deployment guides including Kubernetes, cloud platforms (AWS, GCP, Azure), and production configuration, see **[docs/DEPLOYMENT.md](./docs/DEPLOYMENT.md)**.

### Production Checklist

✅ **Required:**
- [ ] Use live Stripe keys (`sk_live_`, `pk_live_`)
- [ ] Set `commitment: finalized` for Solana
- [ ] Use dedicated RPC endpoint (not public)
- [ ] Enable HTTPS (reverse proxy with TLS)
- [ ] Configure CORS to your domain only
- [ ] Set up webhook endpoints with HTTPS
- [ ] Enable Stripe webhook signature validation
- [ ] Configure wallet balance monitoring

✅ **Recommended:**
- [ ] Use multiple server wallets for load balancing
- [ ] Enable transaction queue rate limiting
- [ ] Set up Discord/Slack alerts for low balances
- [ ] Configure auto-create token accounts
- [ ] Enable gasless if subsidizing user fees
- [ ] Set up structured logging
- [ ] Monitor server metrics (CPU, memory, requests/sec)
- [ ] Configure health check endpoint for load balancer

✅ **Security:**
- [ ] Rotate server wallet keys regularly
- [ ] Use environment variables for all secrets
- [ ] Restrict server wallet funds (refill as needed)
- [ ] Enable Stripe Radar for fraud detection
- [ ] Review [SECURITY.md](./SECURITY.md)

### Environment Variables Reference

```bash
# Server
SERVER_ADDRESS=":8080"
ROUTE_PREFIX="/api"

# Stripe
STRIPE_SECRET_KEY="sk_live_..."
STRIPE_WEBHOOK_SECRET="whsec_..."
STRIPE_PUBLISHABLE_KEY="pk_live_..."
STRIPE_MODE="live"

# Solana
X402_PAYMENT_ADDRESS="YourWallet..."
X402_TOKEN_MINT="EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
X402_NETWORK="mainnet-beta"
SOLANA_RPC_URL="https://your-rpc.com"
SOLANA_WS_URL="wss://your-rpc.com"
X402_COMMITMENT="finalized"

# Server Wallets (for gasless/auto-create)
X402_SERVER_WALLET_1="[1,2,3,...]"  # 64-byte array
X402_SERVER_WALLET_2="[1,2,3,...]"
X402_GASLESS_ENABLED="true"
X402_AUTO_CREATE_TOKEN_ACCOUNT="true"

# Monitoring
MONITORING_LOW_BALANCE_ALERT_URL="https://discord.com/api/webhooks/..."
MONITORING_LOW_BALANCE_THRESHOLD="0.05"
MONITORING_CHECK_INTERVAL="10m"

# Callbacks
CALLBACK_PAYMENT_SUCCESS_URL="https://your-api.com/webhooks/payment"
CALLBACK_HEADER_AUTHORIZATION="Bearer your_token"
```

### Scaling Considerations

**Horizontal Scaling:**
- Stateless design allows multiple instances
- Use load balancer with health checks (`/cedros-health`)
- Share database/storage across instances (if using persistent store)
- Each instance can have its own server wallets

**Performance:**
- ~100-500 requests/sec per instance (depending on RPC latency)
- Bottleneck is typically Solana RPC confirmation time (~1-3 seconds)
- Use multiple server wallets to parallelize transactions
- Configure `tx_queue_max_in_flight` based on RPC limits

**Database:**
- Default in-memory store (not production-ready)
- Implement `storage.Store` interface for persistent storage
- PostgreSQL or Redis recommended for production

### Monitoring

#### Prometheus Metrics

Cedros Pay exposes comprehensive Prometheus metrics at `/metrics` (or `{route_prefix}/metrics`).

**Endpoint Security (Production Recommended):**

```yaml
# configs/config.yaml
server:
  admin_metrics_api_key: "your-secure-random-key-here"
```

Or via environment variable:
```bash
export ADMIN_METRICS_API_KEY="your-secure-random-key-here"
```

**Access Metrics:**
```bash
# Without authentication (development)
curl http://localhost:8080/metrics

# With admin API key (production)
curl -H "Authorization: Bearer your-admin-api-key" \
  http://localhost:8080/metrics
```

**Available Metrics:**

| Metric | Type | Description |
|--------|------|-------------|
| `cedros_payments_total` | Counter | Payment attempts by method, resource, status, currency |
| `cedros_payment_amount_cents` | Histogram | Payment amounts in cents |
| `cedros_payment_duration_seconds` | Histogram | Payment processing time |
| `cedros_payment_failures_total` | Counter | Failed payments by reason |
| `cedros_settlements_total` | Counter | On-chain settlement confirmations |
| `cedros_settlement_time_seconds` | Histogram | Time to blockchain confirmation |
| `cedros_cart_checkouts_total` | Counter | Cart checkout events by status |
| `cedros_refunds_total` | Counter | Refund requests by status |
| `cedros_webhooks_total` | Counter | Webhook delivery attempts |
| `cedros_webhook_retries` | Histogram | Webhook retry attempts |
| `cedros_webhook_dlq_total` | Counter | Webhooks saved to dead letter queue |
| `cedros_rate_limit_hits_total` | Counter | Rate limit hits by tier |
| `cedros_db_queries_total` | Counter | Database queries by operation |
| `cedros_archival_records_deleted_total` | Counter | Records deleted by archival |

**Prometheus Scrape Configuration:**

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'cedros-pay'
    scrape_interval: 15s
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'

    # If using admin API key:
    authorization:
      type: Bearer
      credentials: 'your-admin-api-key'
```

**Example Queries:**

```promql
# Payment success rate
sum(rate(cedros_payments_total{status="success"}[5m])) by (method) /
sum(rate(cedros_payments_total[5m])) by (method)

# P95 payment latency
histogram_quantile(0.95,
  rate(cedros_payment_duration_seconds_bucket[5m])
)

# Revenue by payment method (last hour)
sum(cedros_payment_amount_cents{status="success"} / 100) by (method, currency)

# Webhook failure rate
sum(rate(cedros_webhooks_total{status="failed"}[5m])) /
sum(rate(cedros_webhooks_total[5m]))
```

**Grafana Dashboard:**

Create panels for:
1. Payment Volume (graph by method)
2. Payment Success Rate (gauge)
3. Average Transaction Time (P50/P95/P99)
4. Failed Payments Table (by reason)
5. Webhook Health (success rate + retry counts)
6. Rate Limit Activity (hits by tier)
7. Settlement Times (histogram)

See [API Reference - Metrics](./docs/API_REFERENCE.md#metrics--observability) for complete documentation.

#### Health Checks

**Server Health:**
```bash
curl http://your-server/cedros-health
```

Returns server status and route prefix configuration.

#### Logging

**Configuration:**
- Structured JSON logging (production) or console (development)
- Log levels: INFO for payments, ERROR for failures
- Set via `logging.level` and `logging.format` in config

**Security:**
- Don't log: private keys, full credit card numbers, transaction signatures
- Email addresses are automatically redacted in logs
- Wallet addresses are truncated to first/last 4 characters

---

## Troubleshooting

### Server Won't Start

**Error:** `config validation failed`
- Check required fields: `x402.payment_address`, `x402.token_mint`, `x402.rpc_url`
- Ensure valid wallet addresses (base58)

**Error:** `listen tcp :8080: bind: address already in use`
- Change port: `SERVER_ADDRESS=":8081"`
- Or kill existing process: `lsof -ti:8080 | xargs kill`

### Payment Failures

**Error:** `"Insufficient SOL for transaction fees"`
- User needs SOL in wallet for gas
- Or enable `gasless_enabled: true` to pay fees for them

**Error:** `"Token account not found"`
- Enable `auto_create_token_account: true`
- Or user must create ATA manually first

**Error:** `"Transaction not found on the blockchain"`
- Transaction may have been dropped (rare)
- Check `skip_preflight` setting
- Verify RPC endpoint is healthy
- User should retry payment

**Error:** `"Invalid payment memo"`
- Memo mismatch (tampering or stale quote)
- Frontend should fetch fresh quote
- Check memo template configuration

### Stripe Issues

**Error:** `stripe: webhook signature verification failed`
- Wrong `webhook_secret` configured
- Use secret from Stripe CLI or dashboard
- Ensure webhook endpoint is receiving correct header

**Error:** `stripe session not found`
- Session expired (24 hours)
- User needs to create new session

### RPC Issues

**Error:** `429 Too Many Requests`
- Configure `tx_queue_min_time_between: "100ms"`
- Set `tx_queue_max_in_flight: 5`
- Use paid RPC tier

**Error:** `connection refused / timeout`
- Check `rpc_url` is correct
- Verify network connectivity
- RPC endpoint may be down

**Slow transaction confirmations:**
- Increase `compute_unit_price_micro_lamports` for priority
- Check Solana network congestion
- Use `commitment: confirmed` instead of `finalized` (less safe)

### Balance Monitoring

**Alerts not sent:**
- Check `low_balance_alert_url` is correct
- Verify webhook URL is reachable
- Check server logs for "Error sending alert"
- Verify balance is actually below threshold

**Duplicate alerts:**
- Expected after server restart (cooldown is in-memory)
- Otherwise check logs for issues

### Debug Mode

Enable verbose logging:
```bash
# Set log level (implementation-dependent)
LOG_LEVEL=debug go run ./cmd/server
```

Check recent transactions:
```bash
# View Solana transaction
solana confirm <signature> -u mainnet-beta

# View Stripe session
stripe sessions retrieve cs_test_...
```

### Common Mistakes

❌ **Using test Stripe key with live mode**
- Set `stripe.mode: "test"` when using `sk_test_` keys

❌ **Wrong network configuration**
- `x402.network` must match `rpc_url` cluster
- Mainnet = `mainnet-beta`, devnet = `devnet`

❌ **Missing server wallet for gasless**
- Set `X402_SERVER_WALLET_1` env var when `gasless_enabled: true`

❌ **Server wallet has no SOL**
- Fund wallet with SOL for gas fees
- Enable monitoring alerts

❌ **CORS blocking requests**
- Add frontend origin to `server.cors_allowed_origins`

### Getting Help

- **Documentation:** Check linked docs (FRONTEND_INTEGRATION.md, WALLET_MONITORING.md, etc.)
- **Issues:** Search existing issues or open new one with:
  - Go version: `go version`
  - Config (sanitized)
  - Full error logs
  - Steps to reproduce

---

## Related Documentation

### Core Documentation

- **[docs/API_REFERENCE.md](./docs/API_REFERENCE.md)** 📖 — Complete API reference for all endpoints (payments, carts, refunds)
- **[docs/DEPLOYMENT.md](./docs/DEPLOYMENT.md)** 🚀 — Production deployment guide (Docker, Kubernetes, scaling)
- **[docs/DATABASE_SCHEMA.md](./docs/DATABASE_SCHEMA.md)** — Database schema for PostgreSQL/MongoDB product sources

### Payment Integration Guides

- **[FRONTEND_CART_INTEGRATION.md](./FRONTEND_CART_INTEGRATION.md)** 🆕 — Complete cart checkout guide for frontend developers (Stripe + x402)
- **[docs/FRONTEND_REFUND_INTEGRATION.md](./docs/FRONTEND_REFUND_INTEGRATION.md)** 🆕 — Frontend refund integration guide with x402 verification
- **[CART_CHECKOUT_TESTING.md](./CART_CHECKOUT_TESTING.md)** 🆕 — Stripe cart/bundle checkout testing guide with curl examples
- **[CART_WEBHOOKS.md](./CART_WEBHOOKS.md)** 🆕 — Backend cart webhook/callback processing guide
- **[X402_CART_FLOW.md](./X402_CART_FLOW.md)** 🆕 — Detailed x402 cart implementation guide
- **[FRONTEND_INTEGRATION.md](./FRONTEND_INTEGRATION.md)** — Complete frontend guide with TypeScript examples
- **[WALLET_MONITORING.md](./WALLET_MONITORING.md)** — Detailed balance monitoring setup
- **[GASLESS_TRANSACTIONS.md](./GASLESS_TRANSACTIONS.md)** — Deep dive on gasless payment flow
- **[X402_COMPLIANCE_COMPLETE.md](./X402_COMPLIANCE_COMPLETE.md)** — x402 specification compliance details

### Project Guidelines

- **[AGENTS.md](./AGENTS.md)** — Repository structure and coding guidelines
- **[CONTRIBUTING.md](./CONTRIBUTING.md)** — How to contribute
- **[SECURITY.md](./SECURITY.md)** — Security policy and best practices

---

## Contributing

We welcome contributions! See [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines.

Quick start:
```bash
git clone https://github.com/CedrosPay/server.git
go mod download
go test ./...
```

### Development Workflow

This is an open-source project with a private development repository. If you're a maintainer working on the private repo:

```bash
# Make changes in the private repository
git add .
git commit -m "Your changes"

# Publish to public repository (excludes audit docs, .env, etc.)
./scripts/publish.sh

# Review and push public changes
cd /path/to/public/repo
git add .
git commit -m "Public release: your changes"
git push
```

See [scripts/README.md](./scripts/README.md) for details on the publishing workflow.

---

## Security

Report vulnerabilities to **conorholds@gmail.com** (or via GitHub Security Advisories).

See [SECURITY.md](./SECURITY.md) for details.

---

## License

MIT License - see [LICENSE](./LICENSE) for details.

Copyright (c) 2025 Cedros Pay Contributors

---

**Cedros Pay** — _Unified Payments for Humans & Agents_
