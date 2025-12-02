# Environment Variable Reference

## Overview

Cedros Pay Server supports environment variable overrides for all configuration fields. This allows:

- **Kubernetes/Docker deployments**: Per-environment configuration without rebuilding images
- **Secrets management**: Inject sensitive values from Vault, Kubernetes Secrets, or Docker Secrets
- **12-Factor App compliance**: Configuration via environment follows cloud-native best practices
- **Dynamic reconfiguration**: Change settings without modifying YAML files

## Precedence Order

Configuration values are resolved in this order (highest precedence first):

1. **Environment variables** (highest precedence)
2. **YAML configuration file** (`configs/local.yaml`)
3. **Default values** (built-in defaults)

## Naming Convention

All configuration fields support environment variable overrides using two naming schemes:

### CEDROS-Prefixed (Recommended for Production)

```bash
CEDROS_<SECTION>_<FIELD>
```

Examples:
- `CEDROS_SERVER_ADDRESS`
- `CEDROS_STRIPE_SECRET_KEY`
- `CEDROS_X402_RPC_URL`
- `CEDROS_PAYWALL_QUOTE_TTL`

### Standard Names (Backwards Compatible)

```bash
<SECTION>_<FIELD>
```

Examples:
- `SERVER_ADDRESS`
- `STRIPE_SECRET_KEY`
- `SOLANA_RPC_URL`
- `PAYWALL_QUOTE_TTL`

**Priority**: CEDROS-prefixed names are checked first. If not found, standard names are checked.

## Server Configuration

| Environment Variable | CEDROS Variant | Type | Default | Description |
|---------------------|----------------|------|---------|-------------|
| `SERVER_ADDRESS` | `CEDROS_SERVER_ADDRESS` | string | `:8080` | HTTP server listen address |
| `ROUTE_PREFIX` | `CEDROS_ROUTE_PREFIX` | string | `""` | Optional route prefix (e.g., `/api`) |
| `ADMIN_METRICS_API_KEY` | `CEDROS_ADMIN_METRICS_API_KEY` | string | `""` | Bearer token for `/metrics` endpoint |

### Examples

```bash
# Change server port to 3000
export CEDROS_SERVER_ADDRESS=":3000"

# Add /api prefix to all routes
export CEDROS_ROUTE_PREFIX="/api"

# Secure metrics endpoint
export CEDROS_ADMIN_METRICS_API_KEY="your-secret-key"
```

## Stripe Configuration

| Environment Variable | CEDROS Variant | Type | Description |
|---------------------|----------------|------|-------------|
| `STRIPE_SECRET_KEY` | `CEDROS_STRIPE_SECRET_KEY` | string | Stripe API secret key |
| `STRIPE_WEBHOOK_SECRET` | `CEDROS_STRIPE_WEBHOOK_SECRET` | string | Webhook signing secret |
| `STRIPE_PUBLISHABLE_KEY` | `CEDROS_STRIPE_PUBLISHABLE_KEY` | string | Frontend publishable key |
| `STRIPE_SUCCESS_URL` | `CEDROS_STRIPE_SUCCESS_URL` | string | Checkout success redirect URL |
| `STRIPE_CANCEL_URL` | `CEDROS_STRIPE_CANCEL_URL` | string | Checkout cancel redirect URL |
| `STRIPE_TAX_RATE_ID` | `CEDROS_STRIPE_TAX_RATE_ID` | string | Optional tax rate ID |
| `STRIPE_MODE` | `CEDROS_STRIPE_MODE` | string | `test` or `live` |

### Examples

```bash
# Production Stripe keys from secrets manager
export CEDROS_STRIPE_SECRET_KEY="sk_live_..."
export CEDROS_STRIPE_WEBHOOK_SECRET="whsec_..."
export CEDROS_STRIPE_MODE="live"

# Custom success URL for production
export CEDROS_STRIPE_SUCCESS_URL="https://yourapp.com/payment/success?session_id={CHECKOUT_SESSION_ID}"
```

## X402 / Solana Configuration

| Environment Variable | CEDROS Variant | Aliases | Type | Description |
|---------------------|----------------|---------|------|-------------|
| `X402_PAYMENT_ADDRESS` | `CEDROS_X402_PAYMENT_ADDRESS` | - | string | Solana wallet receiving payments |
| `X402_TOKEN_MINT` | `CEDROS_X402_TOKEN_MINT` | - | string | SPL token mint address (USDC, USDT, etc.) |
| `X402_NETWORK` | `CEDROS_X402_NETWORK` | - | string | `mainnet-beta`, `devnet`, `testnet` |
| `SOLANA_RPC_URL` | `CEDROS_X402_RPC_URL` | `CEDROS_SOLANA_RPC_URL`, `X402_RPC_URL` | string | Solana RPC endpoint URL |
| `SOLANA_WS_URL` | `CEDROS_X402_WS_URL` | `CEDROS_SOLANA_WS_URL`, `X402_WS_URL` | string | Solana WebSocket endpoint URL |
| `X402_MEMO_PREFIX` | `CEDROS_X402_MEMO_PREFIX` | - | string | Memo prefix for transactions |
| `X402_SKIP_PREFLIGHT` | `CEDROS_X402_SKIP_PREFLIGHT` | - | boolean | Skip preflight checks |
| `X402_COMMITMENT` | `CEDROS_X402_COMMITMENT` | - | string | `confirmed`, `finalized`, `processed` |
| `X402_GASLESS_ENABLED` | `CEDROS_X402_GASLESS_ENABLED` | - | boolean | Enable gasless transactions |
| `X402_AUTO_CREATE_TOKEN_ACCOUNT` | `CEDROS_X402_AUTO_CREATE_TOKEN_ACCOUNT` | - | boolean | Auto-create token accounts |
| `X402_SERVER_WALLET_1` | - | - | string | Server wallet private key (JSON array format) |
| `X402_SERVER_WALLET_2` | - | - | string | Server wallet private key (optional, for load balancing) |

### Examples

```bash
# Use custom RPC provider
export CEDROS_X402_RPC_URL="https://api.mainnet-beta.solana.com"
export CEDROS_X402_WS_URL="wss://api.mainnet-beta.solana.com"

# Enable gasless transactions with server wallet
export CEDROS_X402_GASLESS_ENABLED="true"
export X402_SERVER_WALLET_1="[1,2,3,...,64]"  # 64-byte array format

# Use finalized commitment for production
export CEDROS_X402_COMMITMENT="finalized"

# Payment address (inject from Kubernetes secret)
export CEDROS_X402_PAYMENT_ADDRESS="your-solana-wallet-pubkey"
```

## Paywall Configuration

| Environment Variable | CEDROS Variant | Type | Default | Description |
|---------------------|----------------|------|---------|-------------|
| `PAYWALL_QUOTE_TTL` | `CEDROS_PAYWALL_QUOTE_TTL` | duration | `5m` | Quote validity duration |
| `PAYWALL_PRODUCT_SOURCE` | `CEDROS_PAYWALL_PRODUCT_SOURCE` | string | `yaml` | `yaml`, `postgres`, `mongodb` |
| `PAYWALL_POSTGRES_URL` | `CEDROS_PAYWALL_POSTGRES_URL` | string | - | PostgreSQL connection URL |
| `PAYWALL_MONGODB_URL` | `CEDROS_PAYWALL_MONGODB_URL` | string | - | MongoDB connection URL |
| `PAYWALL_MONGODB_DATABASE` | `CEDROS_PAYWALL_MONGODB_DATABASE` | string | - | MongoDB database name |
| `PAYWALL_MONGODB_COLLECTION` | `CEDROS_PAYWALL_MONGODB_COLLECTION` | string | - | MongoDB collection name |
| `PAYWALL_PRODUCT_CACHE_TTL` | `CEDROS_PAYWALL_PRODUCT_CACHE_TTL` | duration | `5m` | Product list cache TTL |

### Examples

```bash
# Increase quote TTL to 2 minutes (user's requirement from issue)
export CEDROS_PAYWALL_QUOTE_TTL="120s"

# Use PostgreSQL for products
export CEDROS_PAYWALL_PRODUCT_SOURCE="postgres"
export CEDROS_PAYWALL_POSTGRES_URL="postgresql://user:pass@db:5432/products?sslmode=require"

# Disable product caching for always-fresh data
export CEDROS_PAYWALL_PRODUCT_CACHE_TTL="0s"
```

## Callback/Webhook Configuration

| Environment Variable | CEDROS Variant | Type | Default | Description |
|---------------------|----------------|------|---------|-------------|
| `CALLBACK_PAYMENT_SUCCESS_URL` | - | string | `""` | Webhook URL for payment events |
| `CALLBACK_TIMEOUT` | - | duration | `3s` | HTTP timeout for webhooks |
| `CALLBACK_HEADER_*` | - | string | - | Custom headers (e.g., `CALLBACK_HEADER_AUTHORIZATION`) |

### Examples

```bash
# Set webhook URL from Kubernetes secret
export CALLBACK_PAYMENT_SUCCESS_URL="https://yourapp.com/webhooks/cedros"

# Add Authorization header to webhooks
export CALLBACK_HEADER_AUTHORIZATION="Bearer your-webhook-secret"

# Add custom API key header
export CALLBACK_HEADER_X_API_KEY="your-api-key"

# Increase webhook timeout for slow endpoints
export CALLBACK_TIMEOUT="10s"
```

## Monitoring Configuration

| Environment Variable | Type | Default | Description |
|---------------------|------|---------|-------------|
| `MONITORING_LOW_BALANCE_ALERT_URL` | string | `""` | Webhook URL for low balance alerts |
| `MONITORING_LOW_BALANCE_THRESHOLD` | float | `0.01` | SOL balance threshold |
| `MONITORING_CHECK_INTERVAL` | duration | `15m` | Balance check frequency |
| `MONITORING_TIMEOUT` | duration | `5s` | HTTP timeout for alerts |
| `MONITORING_HEADER_*` | string | - | Custom headers for alerts |

### Examples

```bash
# Discord webhook for balance alerts
export MONITORING_LOW_BALANCE_ALERT_URL="https://discord.com/api/webhooks/..."
export MONITORING_LOW_BALANCE_THRESHOLD="0.005"
export MONITORING_CHECK_INTERVAL="5m"
```

## Subscriptions Configuration

| Environment Variable | CEDROS Variant | Type | Default | Description |
|---------------------|----------------|------|---------|-------------|
| `SUBSCRIPTIONS_ENABLED` | `CEDROS_SUBSCRIPTIONS_ENABLED` | boolean | `false` | Enable subscription support |
| `SUBSCRIPTIONS_BACKEND` | `CEDROS_SUBSCRIPTIONS_BACKEND` | string | `memory` | `memory` or `postgres` |
| `SUBSCRIPTIONS_POSTGRES_URL` | `CEDROS_SUBSCRIPTIONS_POSTGRES_URL` | string | - | PostgreSQL connection URL |
| `SUBSCRIPTIONS_GRACE_PERIOD_HOURS` | `CEDROS_SUBSCRIPTIONS_GRACE_PERIOD_HOURS` | int | `24` | Grace period after payment failure |

### Examples

```bash
# Enable subscriptions with PostgreSQL storage
export CEDROS_SUBSCRIPTIONS_ENABLED="true"
export CEDROS_SUBSCRIPTIONS_BACKEND="postgres"
export CEDROS_SUBSCRIPTIONS_POSTGRES_URL="postgresql://user:pass@db:5432/subscriptions?sslmode=require"

# Increase grace period to 48 hours
export CEDROS_SUBSCRIPTIONS_GRACE_PERIOD_HOURS="48"
```

## Storage Configuration

Environment variables for storage backends are defined in YAML but can be overridden via:

```bash
# Not yet implemented as env vars (future enhancement)
# Coming soon: STORAGE_BACKEND, STORAGE_FILE_PATH, STORAGE_POSTGRES_URL, etc.
```

## Boolean Values

Boolean environment variables accept these values:

- **True**: `1`, `true`, `TRUE`, `True`
- **False**: `0`, `false`, `FALSE`, `False`, or empty/unset

Examples:
```bash
export CEDROS_X402_GASLESS_ENABLED="true"   # Enabled
export CEDROS_X402_GASLESS_ENABLED="1"      # Enabled
export CEDROS_X402_GASLESS_ENABLED="false"  # Disabled
export CEDROS_X402_GASLESS_ENABLED="0"      # Disabled
```

## Duration Values

Duration environment variables use Go's `time.ParseDuration` format:

- **Seconds**: `30s`, `120s`
- **Minutes**: `5m`, `15m`
- **Hours**: `1h`, `2h30m`
- **Mixed**: `1h30m45s`

Examples:
```bash
export CEDROS_PAYWALL_QUOTE_TTL="120s"     # 2 minutes
export CEDROS_PAYWALL_QUOTE_TTL="2m"       # Same as above
export CALLBACK_TIMEOUT="10s"              # 10 seconds
export MONITORING_CHECK_INTERVAL="30m"     # 30 minutes
```

## Docker Compose Example

```yaml
version: '3.8'
services:
  cedros-pay:
    image: cedros-pay-server:latest
    environment:
      # Server config
      CEDROS_SERVER_ADDRESS: ":8080"
      CEDROS_ROUTE_PREFIX: "/api"

      # Stripe (from secrets)
      CEDROS_STRIPE_SECRET_KEY: ${STRIPE_SECRET_KEY}
      CEDROS_STRIPE_WEBHOOK_SECRET: ${STRIPE_WEBHOOK_SECRET}
      CEDROS_STRIPE_MODE: "live"

      # Solana/X402
      CEDROS_X402_RPC_URL: "https://api.mainnet-beta.solana.com"
      CEDROS_X402_PAYMENT_ADDRESS: ${SOLANA_PAYMENT_WALLET}
      CEDROS_X402_TOKEN_MINT: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
      CEDROS_X402_COMMITMENT: "finalized"

      # Paywall
      CEDROS_PAYWALL_QUOTE_TTL: "120s"
      CEDROS_PAYWALL_PRODUCT_SOURCE: "postgres"
      CEDROS_PAYWALL_POSTGRES_URL: "postgresql://user:pass@db:5432/products"

      # Callbacks
      CALLBACK_PAYMENT_SUCCESS_URL: "https://yourapp.com/webhooks/cedros"
      CALLBACK_HEADER_AUTHORIZATION: "Bearer ${WEBHOOK_SECRET}"
    ports:
      - "8080:8080"
```

## Kubernetes Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cedros-pay
spec:
  template:
    spec:
      containers:
      - name: cedros-pay
        image: cedros-pay-server:latest
        env:
          # Server config
          - name: CEDROS_SERVER_ADDRESS
            value: ":8080"

          # Stripe secrets from Kubernetes Secret
          - name: CEDROS_STRIPE_SECRET_KEY
            valueFrom:
              secretKeyRef:
                name: stripe-credentials
                key: secret-key

          - name: CEDROS_STRIPE_WEBHOOK_SECRET
            valueFrom:
              secretKeyRef:
                name: stripe-credentials
                key: webhook-secret

          # Solana config
          - name: CEDROS_X402_RPC_URL
            value: "https://api.mainnet-beta.solana.com"

          - name: CEDROS_X402_PAYMENT_ADDRESS
            valueFrom:
              secretKeyRef:
                name: solana-wallet
                key: payment-address

          # Paywall config
          - name: CEDROS_PAYWALL_QUOTE_TTL
            value: "120s"

          - name: CEDROS_PAYWALL_PRODUCT_SOURCE
            value: "postgres"

          - name: CEDROS_PAYWALL_POSTGRES_URL
            valueFrom:
              secretKeyRef:
                name: postgres-credentials
                key: connection-url
        ports:
          - containerPort: 8080
```

## Vault Integration Example

Using HashiCorp Vault to inject secrets:

```bash
#!/bin/bash

# Fetch secrets from Vault and export as env vars
export CEDROS_STRIPE_SECRET_KEY=$(vault kv get -field=secret_key secret/stripe)
export CEDROS_STRIPE_WEBHOOK_SECRET=$(vault kv get -field=webhook_secret secret/stripe)
export CEDROS_X402_PAYMENT_ADDRESS=$(vault kv get -field=payment_address secret/solana)
export X402_SERVER_WALLET_1=$(vault kv get -field=server_wallet_1 secret/solana)

# Start server (env vars automatically loaded)
./cedros-pay-server -config=configs/production.yaml
```

## Environment Variable Discovery

To see which environment variables are supported, check:

1. **This document** (comprehensive reference)
2. **`internal/config/env.go`** (implementation source)
3. **`configs/config.example.yaml`** (inline documentation)

## Best Practices

### Production Deployments

✅ **DO:**
- Use `CEDROS_` prefix for clarity and namespace isolation
- Store secrets in dedicated secrets management (Vault, K8s Secrets, AWS Secrets Manager)
- Override sensitive values via env vars, not YAML
- Use duration formats for timeouts (`120s` instead of `120`)
- Document environment-specific overrides in deployment templates

❌ **DON'T:**
- Hardcode secrets in YAML files
- Mix YAML and env vars for the same field without clear precedence rules
- Use production secrets in development `.env` files committed to git
- Forget to quote special characters in shell scripts (`export VAR="value"`)

### Development Workflow

```bash
# .env.local (gitignored)
export CEDROS_SERVER_ADDRESS=":3000"
export CEDROS_STRIPE_SECRET_KEY="sk_test_..."
export CEDROS_X402_NETWORK="devnet"
export CEDROS_X402_RPC_URL="https://api.devnet.solana.com"
export CEDROS_PAYWALL_QUOTE_TTL="30s"  # Shorter for dev testing

# Load and run
source .env.local
./cedros-pay-server -config=configs/dev.yaml
```

## Troubleshooting

### Environment Variable Not Applied

**Problem**: Set env var but YAML value still used

**Solution**: Check precedence order
```bash
# Debug: Print effective config
./cedros-pay-server -config=configs/local.yaml -debug-config

# Verify env var is set
echo $CEDROS_PAYWALL_QUOTE_TTL

# Check for typos in variable name
env | grep CEDROS
```

### Boolean Not Recognized

**Problem**: `export X402_GASLESS_ENABLED="yes"` doesn't work

**Solution**: Use accepted boolean values
```bash
# Correct
export CEDROS_X402_GASLESS_ENABLED="true"  # ✅
export CEDROS_X402_GASLESS_ENABLED="1"     # ✅

# Incorrect
export CEDROS_X402_GASLESS_ENABLED="yes"   # ❌ Not recognized
```

### Duration Parse Error

**Problem**: `invalid duration` error

**Solution**: Use Go duration format
```bash
# Correct
export CEDROS_PAYWALL_QUOTE_TTL="120s"    # ✅
export CEDROS_PAYWALL_QUOTE_TTL="2m"      # ✅

# Incorrect
export CEDROS_PAYWALL_QUOTE_TTL="120"     # ❌ Missing unit
export CEDROS_PAYWALL_QUOTE_TTL="2min"    # ❌ Use 'm', not 'min'
```

## Future Enhancements

Planned env var support (not yet implemented):

- `STORAGE_BACKEND` - Override storage backend
- `STORAGE_POSTGRES_URL` - Override main storage PostgreSQL URL
- `LOGGING_LEVEL` - Override log level
- `LOGGING_FORMAT` - Override log format
- `RATE_LIMIT_GLOBAL_LIMIT` - Override global rate limit

Check the [GitHub issues](https://github.com/CedrosPay/server/issues) for implementation status.

## Related Documentation

- **Configuration Schema**: `internal/config/types.go`
- **YAML Examples**: `configs/config.example.yaml`
- **Implementation**: `internal/config/env.go`
- **12-Factor App**: https://12factor.net/config
