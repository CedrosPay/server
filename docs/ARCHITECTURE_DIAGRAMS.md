# Cedros Pay Server - Architecture Diagrams

## System Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         CLIENT APPLICATIONS                              │
│  (Web, Mobile, Agents - Stripe Checkout & Solana x402 Support)         │
└───────────────────┬──────────────────────────────────────┬──────────────┘
                    │                                      │
         Stripe Checkout                          x402 Payment Proof
         /stripe/success                          (X-PAYMENT header)
                    │                                      │
        ┌───────────▼──────────────────────────────────────▼──────────────┐
        │                      CHI HTTP ROUTER                             │
        │  ┌────────────────────────────────────────────────────────────┐  │
        │  │ MIDDLEWARE STACK (in order)                               │  │
        │  │ 1. CORS Handler                                           │  │
        │  │ 2. Security Headers (OWASP recommended)                  │  │
        │  │ 3. Structured Logging (zerolog)                          │  │
        │  │ 4. Request ID Generation                                 │  │
        │  │ 5. Real IP Extraction                                    │  │
        │  │ 6. Panic Recovery                                        │  │
        │  │ 7. API Version Negotiation                               │  │
        │  │ 8. API Key Authentication                                │  │
        │  │ 9. Rate Limiting (Global/Per-Wallet/Per-IP)             │  │
        │  │ 10. Idempotency (24h cache for payment requests)         │  │
        │  │ 11. Selective Timeout (5s or 60s)                        │  │
        │  └────────────────────────────────────────────────────────────┘  │
        └────────┬──────────────────────────────────────┬───────────────┘
                 │                                      │
        ┌────────▼────────────────┐          ┌──────────▼──────────────┐
        │   STRIPE HANDLERS       │          │   PAYWALL HANDLERS      │
        │ ┌──────────────────────┐│          │ ┌────────────────────┐  │
        │ │ POST /stripe-session ││          │ │ POST /quote        │  │
        │ │ POST /webhook/stripe ││          │ │ POST /verify       │  │
        │ │ GET /stripe/success  ││          │ │ POST /cart/quote   │  │
        │ │ GET /stripe/cancel   ││          │ │ POST /cart/verify  │  │
        │ └──────────────────────┘│          │ │ POST /gasless      │  │
        │                          │          │ │ POST /refunds/*    │  │
        │   Stripe Client          │          │ │ POST /coupons/*    │  │
        └────────┬─────────────────┘          │ │ GET  /products     │  │
                 │                            │ │ POST /nonce        │  │
                 │                            │ └────────────────────┘  │
                 │                            │                         │
                 │                            │   Paywall Service       │
                 │                            │   (Quote Builder)       │
                 │                            │   (Authorizer)          │
                 │                            │   (Coupon Handler)      │
                 │                            └────────┬────────────────┘
                 │                                     │
        ┌────────▼─────────────────────────────────────▼─────────────┐
        │                   BUSINESS LOGIC LAYER                       │
        │ ┌───────────────┐  ┌───────────────┐  ┌──────────────────┐ │
        │ │ Stripe Client │  │ Paywall Svc   │  │ x402 Verifier    │ │
        │ │ - Sessions    │  │ - Quotes      │  │ (Solana)         │ │
        │ │ - Webhooks    │  │ - Cart Logic  │  │ - TX Validation  │ │
        │ │ - Metadata    │  │ - Refunds     │  │ - Confirmation   │ │
        │ └───────────────┘  │ - Coupons     │  │ - Gasless Txs    │ │
        │                    └───────────────┘  └──────────────────┘ │
        │ ┌──────────────────────────────────────────────────────────┐ │
        │ │ Supporting Services                                      │ │
        │ │ - Callback Notifier (Webhook Delivery with Retry)      │ │
        │ │ - Money Types (Unified monetary handling)              │ │
        │ │ - Products Repository (YAML/Postgres/MongoDB)          │ │
        │ │ - Coupons Repository (YAML/Postgres/MongoDB)           │ │
        │ └──────────────────────────────────────────────────────────┘ │
        └────────┬──────────────────────────────────────┬───────────────┘
                 │                                      │
        ┌────────▼──────────────────┐      ┌───────────▼──────────────┐
        │  EXTERNAL SERVICES        │      │  PERSISTENCE LAYER       │
        │ ┌─────────────────────────┐     │ ┌──────────────────────┐  │
        │ │ Stripe API              │     │ │ Storage Abstraction  │  │
        │ │ - Create Checkout       │     │ │ (Store interface)    │  │
        │ │ - Handle Webhooks       │     │ │ ~100 methods         │  │
        │ │ - Query Transactions    │     │ │                      │  │
        │ └─────────────────────────┘     │ │ Operations:          │  │
        │ ┌─────────────────────────┐     │ │ - Cart Management    │  │
        │ │ Solana RPC              │     │ │ - Refund Processing  │  │
        │ │ - Get Transaction       │     │ │ - Payment Tracking   │  │
        │ │ - Get Confirmation      │     │ │ - Admin Nonces       │  │
        │ │ - Get Account Info      │     │ │ - Webhook Queue      │  │
        │ └─────────────────────────┘     │ └──────────────────────┘  │
        │ ┌─────────────────────────┐     │                            │
        │ │ Solana WebSocket        │     │ Backend Implementations:   │
        │ │ - TX Subscriptions      │     │ ┌──────────────────────┐  │
        │ │ - Real-time Confirms    │     │ │ PostgreSQL Store     │  │
        │ └─────────────────────────┘     │ │ (Primary Production) │  │
        │                                 │ └──────────────────────┘  │
        │                                 │ ┌──────────────────────┐  │
        │                                 │ │ MongoDB Store        │  │
        │                                 │ │ (Alternative)        │  │
        │                                 │ └──────────────────────┘  │
        │                                 │ ┌──────────────────────┐  │
        │                                 │ │ File Store           │  │
        │                                 │ │ (Dev/Testing)        │  │
        │                                 │ └──────────────────────┘  │
        └────────┬──────────────────────────┴───────────────┬──────────┘
                 │                                          │
        ┌────────▼──────────────────┐            ┌──────────▼──────────┐
        │  STRIPE BACKEND           │            │  SOLANA BLOCKCHAIN  │
        │  - Process Payments       │            │  - Mainnet/Devnet   │
        │  - Charge Cards           │            │  - SPL Token Prog   │
        │  - Generate Webhooks      │            │  - Account Model    │
        └───────────────────────────┘            └─────────────────────┘
```

---

## Payment Processing Flows

### 1. Stripe Fiat Payment Flow

```
Client                          Server                        Stripe
  │                              │                              │
  ├──POST /paywall/v1/quote──────>│                              │
  │                              ├─ Get resource pricing        │
  │                              │                              │
  │<───Quote with Stripe PriceID──┤                              │
  │                              │                              │
  ├─POST /paywall/v1/stripe-session──>│                          │
  │  (with PriceID, email, etc)  │                              │
  │                              ├──POST /v1/checkout/sessions──>│
  │                              │                              │
  │                              │<──Session Created────────────┤
  │<───Checkout Session URL───────┤                              │
  │                              │                              │
  ├─User completes checkout───────┼──────────────────────────────>│
  │ (out of band)               │                              │
  │                              │<──Webhook (charge.succeeded)─┤
  │                              │                              │
  │                              ├─ Verify webhook signature    │
  │                              ├─ Mark payment as processed   │
  │                              ├─ Enqueue callback webhook    │
  │                              │                              │
  │<───Notification callback───────┤                              │
```

### 2. Solana x402 Payment Flow

```
Client                          Server                    Solana RPC/WS
  │                              │                              │
  ├──POST /paywall/v1/quote──────>│                              │
  │                              ├─ Get resource pricing        │
  │                              │                              │
  │<──Quote with x402 Requirement──┤                             │
  │  (recipient, amount, token)    │                             │
  │                              │                              │
  ├─Build Solana Transaction─────>│                              │
  │  (SPL Transfer to recipient)   │                             │
  │                              │                              │
  ├─Sign with wallet (off-device)─>│                              │
  │                              │                              │
  ├──POST /paywall/v1/verify──────>│                              │
  │  (with X-PAYMENT header)       │                              │
  │                              ├─ Parse PaymentProof          │
  │                              ├─ Decode transaction          │
  │                              ├─ Validate SPL transfer       │
  │                              ├──RPC: getTransaction────────>│
  │                              │                              │
  │                              │<──TX confirmation────────────┤
  │                              │                              │
  │                              ├─ Check replay protection     │
  │                              │  (HasPaymentBeenProcessed)   │
  │                              │                              │
  │                              ├─ Record payment signature    │
  │                              │                              │
  │                              ├─ Enqueue callback webhook    │
  │<──Settlement Response──────────┤                              │
  │  (success, txHash)             │                              │
```

### 3. Multi-Item Cart Flow

```
Client                          Server                    Stripe
  │                              │                              │
  ├──POST /paywall/v1/cart/quote─>│                              │
  │  (items: [{id, qty}...])     │                              │
  │                              ├─ Fetch product details      │
  │                              ├─ Apply coupon discounts    │
  │                              ├─ Calculate totals           │
  │                              │                              │
  │<───CartQuote (cartID, expires)┤                              │
  │                              │                              │
  ├──POST /paywall/v1/cart/checkout─>│                           │
  │  (cartID)                     │                              │
  │                              ├─ Retrieve cart quote        │
  │                              ├─ Aggregate line items       │
  │                              ├──POST /v1/checkout/sessions─>│
  │                              │  (line_items, metadata)    │
  │                              │                              │
  │                              │<──Checkout Session────────────┤
  │<───Checkout URL────────────────┤                              │
  │                              │                              │
  ├─User completes checkout───────┼─────────────────────────────>│
  │                              │                              │
  │                              │<──Webhook (charge.succeeded)─┤
  │                              │                              │
  │                              ├─ HasCartAccess(cartID, wallet)
  │                              │  (verify wallet ownership)   │
  │                              ├─ Enqueue callback           │
  │<───Notification callback───────┤                              │
```

### 4. Refund Workflow

```
Client/Admin                    Server                  Solana
  │                              │                        │
  ├──POST /refunds/request───────>│                        │
  │  (original_tx, reason)       │                        │
  │                              ├─ Query payment stored  │
  │                              ├─ Create refund quote  │
  │                              │  (pending approval)   │
  │<───RefundQuote─────────────────┤                       │
  │                              │                        │
  ├──POST /refunds/approve───────>│                        │
  │  (refundID, signedNonce)     │                        │
  │                              ├─ ConsumeNonce (replay) │
  │                              │  (one-time use check) │
  │                              │                        │
  │                              ├─Build refund tx──────>│
  │                              │  (reverse payment)   │
  │                              │                        │
  │                              │<─TX confirmation──────┤
  │                              │                        │
  │                              ├─ Mark refund processed
  │<───RefundResponse──────────────┤                       │
```

---

## Handler Organization

```
┌────────────────────────────────────────────────────────────────────┐
│                         HTTP HANDLERS                               │
│ (internal/httpserver/handlers_*.go)                                │
├────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  PAYWALL CORE                    STRIPE INTEGRATION                │
│  ├─ handlers_paywall.go          ├─ handlers_stripe.go             │
│  │  └─ POST /quote               │  ├─ POST /stripe-session        │
│  │  └─ POST /verify              │  ├─ POST /webhook/stripe        │
│  │  └─ GET  /cedros-health       │  ├─ GET  /stripe/success        │
│  │  └─ POST /nonce               │  └─ GET  /stripe/cancel         │
│  └─ GET  /cedros-health          └─ POST /stripe/v1/*              │
│                                                                     │
│  CART MANAGEMENT                 GASLESS SUPPORT                   │
│  ├─ handlers_cart.go             ├─ handlers_gasless.go            │
│  ├─ handlers_cart_quote.go        │  └─ POST /gasless-transaction  │
│  └─ handlers_cart_verify.go       └─ Built TX with server fee payer│
│     ├─ POST /cart/quote                                            │
│     └─ POST /cart/checkout       REFUND PROCESSING                 │
│                                  ├─ handlers_refund.go             │
│  DISCOVERY & ADMIN               ├─ handlers_refund_verify.go      │
│  ├─ handlers_wellknown.go        │  ├─ POST /refunds/request       │
│  │  ├─ GET  /.well-known/*       │  ├─ POST /refunds/approve       │
│  ├─ handlers_openapi.go          │  ├─ POST /refunds/deny          │
│  │  └─ GET  /openapi.json        │  └─ POST /refunds/pending       │
│  ├─ handlers_mcp.go              └─ Admin approval workflow        │
│  │  └─ POST /resources/list                                        │
│  ├─ handlers_products.go         COUPON MANAGEMENT                 │
│  │  └─ GET  /products            ├─ handlers_coupons.go            │
│  └─ handlers_a2a.go              │  └─ POST /coupons/validate      │
│     └─ Agent-to-Agent payment    └─ Discount tracking              │
│                                                                     │
│  METRICS & UTILITIES                                               │
│  ├─ middleware_metrics.go (Per-route instrumentation)             │
│  ├─ response_helpers.go (JSON serialization)                      │
│  ├─ rpc_proxy.go (Solana RPC passthrough)                         │
│  └─ handlers_test.go (Integration tests)                          │
│                                                                     │
└────────────────────────────────────────────────────────────────────┘
```

---

## Storage Layer Architecture

```
┌───────────────────────────────────────────────────────────────────┐
│                    STORAGE INTERFACE (Store)                       │
│ interface.go (~100 methods across 5 operation categories)         │
├───────────────────────────────────────────────────────────────────┤
│                                                                    │
│  CART OPERATIONS               REFUND OPERATIONS                  │
│  ├─ SaveCartQuote              ├─ SaveRefundQuote                 │
│  ├─ GetCartQuote               ├─ GetRefundQuote                  │
│  ├─ MarkCartPaid               ├─ MarkRefundProcessed             │
│  ├─ HasCartAccess              ├─ ListPendingRefunds              │
│  ├─ SaveCartQuotes (batch)     ├─ DeleteRefundQuote               │
│  └─ GetCartQuotes (batch)      └─ SaveRefundQuotes (batch)       │
│                                                                    │
│  PAYMENT TRANSACTION           ADMIN NONCE MANAGEMENT             │
│  (Replay Protection)           ├─ CreateNonce                     │
│  ├─ RecordPayment              ├─ ConsumeNonce                    │
│  ├─ HasPaymentBeenProcessed    └─ CleanupExpiredNonces            │
│  ├─ GetPayment                                                     │
│  ├─ RecordPayments (batch)     WEBHOOK QUEUE                      │
│  └─ ArchiveOldPayments         ├─ EnqueueWebhook                  │
│                                ├─ DequeueWebhooks                 │
│                                ├─ MarkWebhookProcessing           │
│                                ├─ MarkWebhookSuccess              │
│                                ├─ MarkWebhookFailed               │
│                                ├─ GetWebhook                      │
│                                ├─ ListWebhooks                    │
│                                ├─ RetryWebhook                    │
│                                └─ DeleteWebhook                   │
│                                                                    │
├───────────────────────────────────────────────────────────────────┤
│                    BACKEND IMPLEMENTATIONS                         │
├───────────────────────────────────────────────────────────────────┤
│                                                                    │
│  PostgreSQL Store              MongoDB Store       File Store      │
│  (postgres_store.go)           (mongodb_store.go)  (file_store.go)│
│  ├─ 957 lines                  ├─ 521 lines        ├─ 650 lines    │
│  ├─ Prepared statements        ├─ Document ops     ├─ JSON ops     │
│  ├─ Connection pooling         ├─ Aggregation ops  ├─ In-memory    │
│  ├─ Batch operations (fast)    ├─ Batch (loops)    ├─ Disk sync    │
│  ├─ Transactions (ACID)        ├─ Transactions     └─ Locking      │
│  └─ Indexes (optimized)        └─ Indexes                         │
│                                                                    │
│  Webhook Queue Backends                                           │
│  ├─ webhook_queue_postgres.go (358 lines)                         │
│  ├─ webhook_queue_mongodb.go                                      │
│  ├─ webhook_queue_file.go                                         │
│  └─ webhook_queue_memory.go (transient)                           │
│                                                                    │
└───────────────────────────────────────────────────────────────────┘
```

---

## Configuration System

```
┌──────────────────────────────────────────────────────────────────┐
│                     CONFIGURATION LOADING                         │
│ (internal/config/config.go + env.go + types.go + validation.go)  │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  Step 1: Load .env (via godotenv)                               │
│  │                                                               │
│  Step 2: Load YAML (configs/config.yaml or custom path)         │
│  │                                                               │
│  Step 3: Environment Variable Overrides                         │
│  │  STRIPE_SECRET_KEY, X402_RPC_URL, etc.                      │
│  │                                                               │
│  Step 4: Validation & Defaults                                  │
│  │                                                               │
│  ▼                                                               │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ Config Struct (~10 major sections)                       │   │
│  │ ├─ ServerConfig       (HTTP server, timeouts, CORS)      │   │
│  │ ├─ StripeConfig       (API keys, webhooks)               │   │
│  │ ├─ X402Config         (RPC, token, wallets, queuing)     │   │
│  │ ├─ PaywallConfig      (TTLs, product source)             │   │
│  │ ├─ StorageConfig      (backend, pool settings)           │   │
│  │ ├─ CouponConfig       (coupon source, caching)           │   │
│  │ ├─ CallbacksConfig    (webhook delivery, retry, DLQ)     │   │
│  │ ├─ MonitoringConfig   (balance alerts, check interval)   │   │
│  │ ├─ RateLimitConfig    (multi-tier: global/wallet/IP)     │   │
│  │ ├─ APIKeyConfig       (authentication, tier management)  │   │
│  │ └─ CircuitBreakerConfig (fault tolerance)               │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                   │
│  Environment Variables                                           │
│  ├─ STRIPE_SECRET_KEY (required)                                │
│  ├─ X402_RPC_URL (required)                                     │
│  ├─ X402_SERVER_WALLET_1, X402_SERVER_WALLET_2, ... (optional) │
│  ├─ POSTGRES_URL (if postgres storage)                          │
│  ├─ MONGO_URL (if mongodb storage)                              │
│  └─ Various feature flags and overrides                         │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

---

## Dependency Injection Pattern

```
┌────────────────────────────────────────────────────────────────┐
│ cmd/server/main.go                                             │
│ (Application bootstrap)                                         │
├────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─ Load Config                                               │
│  ├─ Create Logger → logger.Logger                             │
│  ├─ Create Resource Manager → lifecycle.Manager               │
│  │                                                             │
│  ├─ Database Setup                                            │
│  │  ├─ Shared Connection Pool (if multi-component postgres)   │
│  │  └─ Storage Backend (postgres/mongodb/file)                │
│  │                                                             │
│  ├─ External Services                                         │
│  │  ├─ Solana Verifier (RPC + WebSocket)                      │
│  │  ├─ Stripe Client                                          │
│  │  └─ Cart Service                                           │
│  │                                                             │
│  ├─ Repositories                                              │
│  │  ├─ Product Repository (with cache)                        │
│  │  └─ Coupon Repository                                      │
│  │                                                             │
│  ├─ Callback System                                           │
│  │  ├─ Retry Configuration                                    │
│  │  ├─ DLQ Store                                              │
│  │  └─ Callback Notifier                                      │
│  │                                                             │
│  ├─ Business Logic                                            │
│  │  ├─ Paywall Service (uses: store, verifier, repos, notify)│
│  │  ├─ Stripe Client (uses: store, notify, repos, metrics)   │
│  │  └─ Cart Service (uses: store, notify, repos, metrics)    │
│  │                                                             │
│  ├─ Support Services                                          │
│  │  ├─ Metrics Collector (Prometheus)                         │
│  │  ├─ Idempotency Store                                      │
│  │  └─ Balance Monitor                                        │
│  │                                                             │
│  └─ HTTP Server                                               │
│     └─ HTTP Server (uses: all above)                          │
│        └─ Router Configuration                                │
│           └─ All Handlers (can access all services via closure)
│                                                                 │
└────────────────────────────────────────────────────────────────┘
```

---

## Middleware Pipeline

```
HTTP Request
  │
  ▼
┌──────────────────────────────────────────┐
│ CORS Handler                             │ Allow cross-origin requests
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│ Security Headers (OWASP)                 │ X-Content-Type-Options, X-Frame-Options, etc.
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│ Structured Logging (zerolog)             │ Log all requests with context
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│ Request ID Generation                    │ X-Request-ID header
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│ Real IP Extraction                       │ Handle proxies
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│ Panic Recovery                           │ 500 errors instead of crashes
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│ API Version Negotiation                  │ Accept: application/vnd.api+json;version=1
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│ API Key Authentication (X-API-Key)       │ Optional tier-based auth
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│ Rate Limiting                            │ Global, Per-Wallet, Per-IP
│ - Token bucket algorithm                 │
│ - API Key tier exemptions                │
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│ Selective Timeout                        │ 5s (light), 60s (payments)
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│ Idempotency Middleware (for payments)    │ 24-hour dedup cache
└──────────────┬───────────────────────────┘
               │
               ▼
        HANDLER FUNCTION
               │
        (Accesses: paywall, stripe,
         verifier, storage, metrics)
               │
               ▼
        JSON Response
```

---

## Coupon System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         COUPON SYSTEM                                │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  COUPON SOURCES                    COUPON TYPES                     │
│  ├─ YAML (inline config)           ├─ Percentage (5% off)           │
│  ├─ PostgreSQL (dynamic)           └─ Fixed ($5 off)                │
│  ├─ MongoDB (dynamic)                                               │
│  └─ Disabled (no coupons)          DISCOUNT SCOPES                  │
│                                    ├─ All products (site-wide)      │
│  CACHING LAYER                     └─ Specific products only        │
│  └─ CachedRepository                                                │
│     └─ TTL: 1 minute (default)     WHEN COUPONS APPLY               │
│                                    ├─ catalog: Product page display │
│                                    └─ checkout: Cart/checkout only  │
│  CURRENCY HANDLING                                                  │
│  ├─ Currency field: OPTIONAL       PAYMENT METHOD FILTERING         │
│  ├─ USD-pegged equivalence:        ├─ "" (any): Stripe + x402      │
│  │   USD, USDC, USDT, PYUSD, CASH  ├─ "stripe": Fiat only          │
│  └─ Fixed discounts work across    └─ "x402": Crypto only          │
│      all USD-equivalent assets                                      │
│                                                                      │
├─────────────────────────────────────────────────────────────────────┤
│                      COUPON APPLICATION FLOW                         │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Step 1: Coupon Selection                                          │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ SelectCouponsForPayment(productID, paymentMethod, scope)     │  │
│  │ ├─ Get auto-apply coupons from repository                    │  │
│  │ ├─ Filter by payment_method (stripe/x402/any)                │  │
│  │ ├─ Filter by applies_at (catalog/checkout)                   │  │
│  │ ├─ Filter by scope (all/specific products)                   │  │
│  │ ├─ Validate coupon (active, not expired, usage limit)        │  │
│  │ └─ Add manual coupon if provided                             │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  Step 2: Coupon Stacking (StackCouponsOnMoney)                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Apply coupons in optimal order:                              │  │
│  │ 1. All percentage discounts (multiplicative)                 │  │
│  │    Price × (1 - discount1/100) × (1 - discount2/100)...     │  │
│  │ 2. Sum all fixed discounts                                   │  │
│  │    Total = discount1 + discount2 + ...                       │  │
│  │ 3. Subtract fixed total from price                           │  │
│  │    FinalPrice = Price - TotalFixedDiscount                   │  │
│  │                                                               │  │
│  │ USD-Pegged Logic (isUSDPegged):                              │  │
│  │ └─ Fixed discounts only apply if asset is USD-pegged        │  │
│  │    (USD, USDC, USDT, PYUSD, CASH)                           │  │
│  │ └─ Package-level map (zero allocations)                     │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  Step 3: Price Rounding                                            │
│  └─ Round to 2 decimal cents ($2.7661 → $2.77)                    │
│                                                                      │
│  EXAMPLE: Multiple Coupon Stacking                                 │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Original Price: $10.00 USDC                                  │  │
│  │ Coupon 1: CRYPTO5 (5% off, auto-apply, catalog)             │  │
│  │ Coupon 2: SAVE20 (20% off, manual, checkout)                │  │
│  │ Coupon 3: FIXED5 ($5 off, auto-apply, checkout)             │  │
│  │                                                               │  │
│  │ Calculation:                                                 │  │
│  │ 1. Apply CRYPTO5: $10.00 × 0.95 = $9.50                     │  │
│  │ 2. Apply SAVE20:  $9.50 × 0.80 = $7.60                      │  │
│  │ 3. Apply FIXED5:  $7.60 - $5.00 = $2.60                     │  │
│  │                                                               │  │
│  │ Final Price: $2.60 USDC (260 cents → 2,600,000 atomic)     │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## x402 Payment Verification Pipeline

```
X-PAYMENT Header (base64 JSON)
  │
  ▼
┌────────────────────────────────────┐
│ ParsePaymentProof()                │
│ - Decode base64                    │
│ - Validate x402Version             │
│ - Extract scheme-specific fields   │
└────────────┬──────────────────────┘
             │
             ▼
┌────────────────────────────────────┐
│ SolanaVerifier.Verify()            │
│ - Parse transaction from base64    │
│ - Decode Solana instruction format │
└────────────┬──────────────────────┘
             │
             ▼
┌────────────────────────────────────┐
│ validateTransferInstructionAndExtr │
│ ActAuthority()                     │
│ - Find SPL transfer instruction    │
│ - Verify destination (recipient)   │
│ - Extract payer wallet             │
│ - Validate amount                  │
└────────────┬──────────────────────┘
             │
             ▼
┌────────────────────────────────────┐
│ RPC Call: getTransaction()         │
│ - Confirm TX exists on-chain       │
│ - Check commitment level           │
│ - Retrieve full TX details         │
└────────────┬──────────────────────┘
             │
             ▼
┌────────────────────────────────────┐
│ Replay Protection Check            │
│ - HasPaymentBeenProcessed()        │
│ - Compare signature to database    │
└────────────┬──────────────────────┘
             │
             ▼
┌────────────────────────────────────┐
│ Record Payment                     │
│ - Store signature in database      │
│ - Mark as processed                │
└────────────┬──────────────────────┘
             │
             ▼
┌────────────────────────────────────┐
│ Return VerificationResult          │
│ - Wallet address                   │
│ - Amount verified                  │
│ - Signature hash                   │
│ - Expiration time                  │
└────────────┬──────────────────────┘
             │
             ▼
        Enqueue Callback Webhook
```

---

## Webhook Delivery With Retry

```
Payment Settled
  │
  ▼
┌─────────────────────────────────┐
│ Enqueue Webhook                 │
│ - Create PendingWebhook         │
│ - Store in queue (storage)      │
│ - Status: PENDING               │
└────────┬────────────────────────┘
         │
         ▼
┌─────────────────────────────────┐
│ Background Queue Worker         │
│ (runs every 30s)                │
├─────────────────────────────────┤
│ Loop: DequeueWebhooks()         │
│       (ORDER BY next_attempt_at)│
└────────┬────────────────────────┘
         │
         ▼
┌─────────────────────────────────┐
│ MarkWebhookProcessing()         │
│ - Prevent duplicate processing  │
│ - Set status: PROCESSING        │
└────────┬────────────────────────┘
         │
         ▼
┌─────────────────────────────────┐
│ Send HTTP POST                  │
│ (with Timeout middleware)       │
│                                 │
│ Success (200-299)?              │
└────┬───────────────────┬────────┘
     │ YES               │ NO (timeout/error)
     │                   │
     ▼                   ▼
┌──────────────┐  ┌──────────────────────┐
│ Mark Success │  │ MarkWebhookFailed()  │
│ - Remove     │  │ - Increment attempt  │
│   from queue │  │ - Calculate backoff: │
│ - Status: OK │  │   nextAttempt =      │
│              │  │   now + interval *   │
└──────────────┘  │   (multiplier ^      │
                  │    attempts)         │
                  │ - Reschedule         │
                  │                      │
                  │ Max attempts reached?│
                  │        │             │
                  │   YES  │ NO          │
                  │        │             │
                  ▼        ▼             │
              ┌────────┐┌──────────────┐ │
              │Move to ││Reschedule    │ │
              │DLQ     ││for retry     │ │
              │(Admin) │└──────────────┘ │
              │inspect)└────────┬────────┘
              │                 │
              │            Loop back
              │            (wait for
              │             next window)
              │
              ▼
         Admin Actions:
         ├─ Inspect DLQ
         ├─ RetryWebhook()
         └─ DeleteWebhook()
```

