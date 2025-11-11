# Cedros Pay Deployment Guide

Complete guide for deploying Cedros Pay to production.

## Table of Contents

- [Quick Start](#quick-start)
- [Docker Deployment](#docker-deployment)
- [Environment Variables](#environment-variables)
- [Production Checklist](#production-checklist)
- [Configuration Examples](#configuration-examples)
- [Scaling](#scaling)
- [Monitoring](#monitoring)
- [Security](#security)
- [Troubleshooting](#troubleshooting)

---

## Quick Start

### Prerequisites

- **Go 1.23+** (for building from source)
- **Docker** (optional, for containerized deployment)
- **Stripe Account** with live API keys
- **Solana Wallet** with USDC and SOL balance
- **Solana RPC Endpoint** (Helius, QuickNode, or self-hosted)

### Local Development

```bash
# Clone repository
git clone https://github.com/CedrosPay/server
cd cedrospay-server

# Copy example config
cp configs/example.yaml configs/local.yaml

# Edit config with your keys
nano configs/local.yaml

# Build and run
go build -o bin/server ./cmd/server
./bin/server --config configs/local.yaml
```

Server will start on `http://localhost:8080`

---

## Docker Deployment

We provide a production-ready multi-stage Dockerfile and multiple docker-compose configurations.

### Quick Start with Makefile

```bash
# Simple server-only deployment
make docker-simple-up

# Full stack (server + postgres + redis)
make docker-up

# View logs
make docker-logs

# Stop services
make docker-down
```

### Build Image

```bash
# Using Makefile (recommended)
make docker-build

# Or manually
docker build \
  --build-arg VERSION=v1.0.0 \
  --build-arg BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S') \
  -t cedrospay-server:latest \
  .
```

**Docker Features:**

- Multi-stage build (final image ~20MB)
- Non-root user for security
- Built-in health checks
- Version and build time injection
- Alpine-based for minimal size

### Run Container

```bash
# Using Makefile
make docker-run

# Or manually (auto-detects configs/config.yaml)
docker run -d \
  --name cedrospay-server \
  -p 8080:8080 \
  --env-file .env \
  --restart unless-stopped \
  cedrospay-server:latest

# With custom config file
docker run -d \
  --name cedrospay-server \
  -p 8080:8080 \
  --env-file .env \
  -v $(pwd)/configs:/app/configs \
  cedrospay-server:latest \
  -config configs/local.yaml

# Check version
docker run --rm cedrospay-server:latest -version
```

**Passing Arguments:**

The server binary accepts command-line arguments. Pass them after the image name:

```bash
# Use custom config
docker run cedrospay-server:latest -config /path/to/config.yaml

# Show version
docker run cedrospay-server:latest -version

# Multiple arguments
docker run cedrospay-server:latest -config custom.yaml -version
```

### Docker Compose Options

#### Option 1: Simple Server-Only (Recommended)

Use `docker-compose.simple.yml` when using external/managed databases:

```bash
# Start
make docker-simple-up

# Or manually
docker-compose -f docker-compose.simple.yml up -d

# Check status
docker ps

# Stop
make docker-simple-down
```

#### Option 2: Full Stack (Development)

Use `docker-compose.yml` for complete local environment with databases:

```bash
# Start all services (server + postgres + redis)
make docker-up

# View logs
make docker-logs

# Check status
make docker-ps

# Stop all
make docker-down

# Clean up (removes volumes)
make docker-clean
```

**Full Stack Includes:**

- Cedros Pay Server
- PostgreSQL 16
- Redis 7
- Data persistence with volumes
- Health checks for all services

### Manual Docker Compose

Create `.env` file:

```bash
# Required
X402_SERVER_WALLET_1=your_wallet_key
STRIPE_SECRET_KEY=sk_live_...
SOLANA_RPC_URL=https://api.mainnet-beta.solana.com

# Optional (for full stack)
POSTGRES_PASSWORD=changeme
REDIS_PASSWORD=changeme
```

Run:

```bash
docker-compose up -d
```

**Using Custom Config with Docker Compose:**

To use a custom config file, override the `command` in your docker-compose.yml:

```yaml
services:
  server:
    image: cedrospay-server:latest
    command: ["-config", "configs/production.yaml"]
    # ... rest of config
```

Or create a `docker-compose.override.yml`:

```yaml
services:
  server:
    command: ["-config", "configs/local.yaml"]
```

---

## Makefile Commands Reference

### Development Commands

```bash
# Run server (auto-detects configs/config.yaml)
make run

# Run with custom config
make run ARGS="-config configs/local.yaml"

# Run with version flag
make run ARGS="-version"

# Build binary
make build

# Run tests
make test

# Install dependencies
make install
```

### Docker Commands

**Docker - Simple (server only):**

- `make docker-build` - Build image with version tags
- `make docker-run` - Run standalone container
- `make docker-simple-up` - Start with simple compose
- `make docker-simple-down` - Stop simple compose

**Docker - Full Stack:**

- `make docker-up` - Start all services
- `make docker-down` - Stop all services
- `make docker-logs` - View service logs
- `make docker-ps` - Show running containers
- `make docker-clean` - Remove containers and volumes

**Production:**

- `make prod-build` - Build optimized binary
- `make docker-push` - Push to registry (set DOCKER_REGISTRY env var)

**View All Commands:**

```bash
make help
```

---

## Environment Variables

### Required

| Variable               | Description                      | Example                               |
| ---------------------- | -------------------------------- | ------------------------------------- |
| `STRIPE_SECRET_KEY`    | Stripe API secret key            | `sk_live_...`                         |
| `X402_PAYMENT_ADDRESS` | Solana wallet receiving payments | `9xQeWvG816...`                       |
| `X402_TOKEN_MINT`      | SPL token mint address           | `EPjFWdd5Auf...` (USDC)               |
| `SOLANA_RPC_URL`       | Solana RPC endpoint              | `https://api.mainnet-beta.solana.com` |
| `SOLANA_WS_URL`        | Solana WebSocket endpoint        | `wss://api.mainnet-beta.solana.com`   |

### Server Configuration

| Variable               | Description             | Default                 |
| ---------------------- | ----------------------- | ----------------------- |
| `SERVER_ADDRESS`       | Host and port to bind   | `:8080`                 |
| `SERVER_READ_TIMEOUT`  | Read timeout            | `10s`                   |
| `SERVER_WRITE_TIMEOUT` | Write timeout           | `10s`                   |
| `SERVER_IDLE_TIMEOUT`  | Idle connection timeout | `60s`                   |
| `ROUTE_PREFIX`         | API route prefix        | `/api`                  |
| `CORS_ALLOWED_ORIGINS` | Comma-separated origins | `http://localhost:3000` |

### Stripe Configuration

| Variable                 | Description                   | Required        |
| ------------------------ | ----------------------------- | --------------- |
| `STRIPE_WEBHOOK_SECRET`  | Stripe webhook signing secret | ✅ For webhooks |
| `STRIPE_PUBLISHABLE_KEY` | Stripe publishable key        | Optional        |
| `STRIPE_MODE`            | `live` or `test`              | Optional        |
| `STRIPE_SUCCESS_URL`     | Redirect after success        | Optional        |
| `STRIPE_CANCEL_URL`      | Redirect after cancel         | Optional        |

### Solana/x402 Configuration

| Variable              | Description             | Default                               |
| --------------------- | ----------------------- | ------------------------------------- |
| `X402_NETWORK`        | Solana network          | `mainnet-beta`                        |
| `X402_COMMITMENT`     | Confirmation level      | `confirmed` (use `finalized` in prod) |
| `X402_SKIP_PREFLIGHT` | Skip preflight checks   | `false`                               |
| `X402_MEMO_PREFIX`    | Transaction memo prefix | `cedros`                              |
| `X402_TOKEN_DECIMALS` | Token decimal places    | `6` (USDC)                            |
| `X402_ALLOWED_TOKENS` | Comma-separated tokens  | `USDC`                                |

### Server Wallets (Gasless/Auto-create)

| Variable                         | Description                                     |
| -------------------------------- | ----------------------------------------------- | ------- |
| `X402_SERVER_WALLET_1`           | First server wallet private key (64-byte array) |
| `X402_SERVER_WALLET_2`           | Second server wallet private key                |
| `X402_SERVER_WALLET_3`           | Third server wallet private key                 |
| `X402_GASLESS_ENABLED`           | Enable gasless transactions                     | `false` |
| `X402_AUTO_CREATE_TOKEN_ACCOUNT` | Auto-create ATAs                                | `false` |

**Server Wallet Format:**

```bash
X402_SERVER_WALLET_1="[1,2,3,4,...]"  # 64 comma-separated bytes
```

Generate from base58:

```bash
solana-keygen pubkey keypair.json  # View public key
cat keypair.json                    # Copy byte array
```

### Transaction Queue

| Variable                         | Description                     | Default |
| -------------------------------- | ------------------------------- | ------- |
| `X402_TX_QUEUE_MAX_IN_FLIGHT`    | Max concurrent transactions     | `10`    |
| `X402_TX_QUEUE_MIN_TIME_BETWEEN` | Min time between tx submissions | `100ms` |

### Monitoring

| Variable                           | Description               |
| ---------------------------------- | ------------------------- | ------ |
| `MONITORING_LOW_BALANCE_ALERT_URL` | Discord/Slack webhook URL |
| `MONITORING_LOW_BALANCE_THRESHOLD` | Alert threshold (SOL)     | `0.01` |
| `MONITORING_CHECK_INTERVAL`        | Check frequency           | `15m`  |

### Callbacks

| Variable                        | Description                    |
| ------------------------------- | ------------------------------ | ----- |
| `CALLBACK_PAYMENT_SUCCESS_URL`  | Webhook URL for payment events |
| `CALLBACK_HEADER_AUTHORIZATION` | Auth header for callbacks      |
| `CALLBACK_TIMEOUT`              | Callback HTTP timeout          | `10s` |

### Product Source

| Variable            | Description                      | Default                 |
| ------------------- | -------------------------------- | ----------------------- |
| `PRODUCT_SOURCE`    | `yaml`, `postgres`, or `mongodb` | `yaml`                  |
| `PRODUCT_CACHE_TTL` | Product cache duration           | `5m`                    |
| `DATABASE_URL`      | PostgreSQL connection string     | Required for `postgres` |
| `MONGODB_URI`       | MongoDB connection string        | Required for `mongodb`  |

---

## Production Checklist

### ✅ Required

- [ ] **Live Stripe Keys** - Use `sk_live_` and `pk_live_` keys
- [ ] **Finalized Commitment** - Set `X402_COMMITMENT=finalized` for Solana
- [ ] **Dedicated RPC** - Use paid RPC endpoint (Helius, QuickNode)
- [ ] **HTTPS Enabled** - Use reverse proxy with TLS (nginx, Caddy)
- [ ] **CORS Configured** - Set `CORS_ALLOWED_ORIGINS` to your domain only
- [ ] **Webhook HTTPS** - Stripe webhooks require HTTPS endpoint
- [ ] **Webhook Validation** - Set `STRIPE_WEBHOOK_SECRET` for signature validation
- [ ] **Balance Monitoring** - Configure `MONITORING_LOW_BALANCE_ALERT_URL`

### ✅ Recommended

- [ ] **Multiple Server Wallets** - Use 2-3 wallets for load balancing
- [ ] **Rate Limiting** - Configure `TX_QUEUE_MAX_IN_FLIGHT` and `MIN_TIME_BETWEEN`
- [ ] **Discord/Slack Alerts** - Set up low balance notifications
- [ ] **Auto-create ATAs** - Enable if targeting non-crypto users
- [ ] **Gasless Enabled** - Subsidize user fees for better UX
- [ ] **Structured Logging** - Configure log level and format
- [ ] **Metrics Monitoring** - Track CPU, memory, requests/sec
- [ ] **Health Checks** - Configure load balancer to use `/cedros-health`

### ✅ Security

- [ ] **Rotate Keys** - Change server wallet keys every 3-6 months
- [ ] **Environment Secrets** - Never commit secrets to git
- [ ] **Limited Funds** - Keep minimal SOL/USDC in server wallets
- [ ] **Stripe Radar** - Enable fraud detection
- [ ] **Review Security** - Read [SECURITY.md](../SECURITY.md)
- [ ] **Firewall Rules** - Restrict access to admin endpoints
- [ ] **DDoS Protection** - Use Cloudflare or similar
- [ ] **Audit Logs** - Log all payment/refund operations

---

## Configuration Examples

### Minimal Production (config.yaml)

```yaml
server:
  address: ":8080"
  route_prefix: "/api"
  cors_allowed_origins:
    - "https://yourdomain.com"

stripe:
  secret_key: "${STRIPE_SECRET_KEY}"
  webhook_secret: "${STRIPE_WEBHOOK_SECRET}"
  mode: "live"

x402:
  payment_address: "${X402_PAYMENT_ADDRESS}"
  token_mint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
  network: "mainnet-beta"
  rpc_url: "${SOLANA_RPC_URL}"
  ws_url: "${SOLANA_WS_URL}"
  commitment: "finalized"
  gasless_enabled: true
  server_wallet_keys:
    - "${X402_SERVER_WALLET_1}"

monitoring:
  low_balance_alert_url: "${DISCORD_WEBHOOK_URL}"
  low_balance_threshold: 0.05
  check_interval: "10m"

paywall:
  quote_ttl: "5m"
  product_source: "yaml"
  resources:
    premium-content:
      resource_id: "premium-content"
      description: "Premium Article"
      fiat_amount: 5.0
      fiat_currency: "usd"
      crypto_amount: 5.0
      crypto_token: "USDC"
```

### High-Volume Production

```yaml
server:
  address: ":8080"
  route_prefix: "/api"
  read_timeout: "15s"
  write_timeout: "15s"
  idle_timeout: "120s"

x402:
  # ... basic config ...
  server_wallet_keys:
    - "${X402_SERVER_WALLET_1}"
    - "${X402_SERVER_WALLET_2}"
    - "${X402_SERVER_WALLET_3}"
  tx_queue_max_in_flight: 20
  tx_queue_min_time_between: "50ms"

monitoring:
  low_balance_threshold: 0.1 # Higher threshold for high volume
  check_interval: "5m" # Check more frequently

paywall:
  product_source: "postgres"
  product_cache_ttl: "2m" # Fresh product data
```

### Database-Backed Products

```yaml
paywall:
  product_source: "postgres"
  product_cache_ttl: "5m"

# PostgreSQL
database:
  url: "postgresql://user:pass@host:5432/cedros?sslmode=require"
  max_connections: 25
  max_idle_connections: 5
# Or MongoDB
# mongodb:
#   uri: "mongodb+srv://user:pass@cluster.mongodb.net/cedros"
#   database: "cedros"
#   collection: "products"
```

---

## Scaling

### Horizontal Scaling

Cedros Pay is **stateless** and scales horizontally:

```
┌─────────────┐
│Load Balancer│
└──────┬──────┘
       │
   ┌───┴───┬───────┬───────┐
   │       │       │       │
┌──▼──┐ ┌──▼──┐ ┌──▼──┐ ┌──▼──┐
│ Pod │ │ Pod │ │ Pod │ │ Pod │
│  1  │ │  2  │ │  3  │ │  4  │
└──┬──┘ └──┬──┘ └──┬──┘ └──┬──┘
   │       │       │       │
   └───────┴───┬───┴───────┘
               │
        ┌──────▼──────┐
        │  Database   │
        │ (if used)   │
        └─────────────┘
```

**Configuration:**

- Use shared database for product source (PostgreSQL/MongoDB)
- Each pod can have its own server wallets
- Health check: `GET /cedros-health`
- Session affinity: Not required

### Kubernetes Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cedros-pay
spec:
  replicas: 3
  selector:
    matchLabels:
      app: cedros-pay
  template:
    metadata:
      labels:
        app: cedros-pay
    spec:
      containers:
        - name: cedros-pay
          image: cedros-pay:latest
          ports:
            - containerPort: 8080
          env:
            - name: STRIPE_SECRET_KEY
              valueFrom:
                secretKeyRef:
                  name: cedros-secrets
                  key: stripe-secret-key
            - name: X402_SERVER_WALLET_1
              valueFrom:
                secretKeyRef:
                  name: cedros-secrets
                  key: server-wallet-1
          livenessProbe:
            httpGet:
              path: /cedros-health
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /cedros-health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              memory: "256Mi"
              cpu: "250m"
            limits:
              memory: "512Mi"
              cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: cedros-pay
spec:
  selector:
    app: cedros-pay
  ports:
    - port: 80
      targetPort: 8080
  type: LoadBalancer
```

### Performance Tuning

**Bottlenecks:**

- Solana RPC latency (~1-3 seconds per confirmation)
- Stripe API rate limits
- Database queries (if using persistent storage)

**Optimizations:**

1. **Multiple Server Wallets** - Parallelize transaction submissions
2. **RPC Optimization** - Use paid RPC with high rate limits
3. **Database Caching** - Cache product data (5-15 minutes)
4. **Connection Pooling** - Reuse HTTP connections to Stripe/Solana
5. **Queue Configuration** - Tune `max_in_flight` based on RPC limits

**Expected Throughput:**

- ~100-500 requests/sec per instance
- ~5-10 transactions/sec per server wallet
- Use 3-5 server wallets for 15-50 tx/sec total

---

## Monitoring

### Metrics to Track

**Server Health:**

- Request rate (requests/second)
- Response time (p50, p95, p99)
- Error rate (5xx responses)
- Memory usage
- CPU usage

**Payment Processing:**

- Payment success rate
- Payment verification time
- Failed verifications (by reason)
- Stripe webhook delivery rate

**Wallet Health:**

- SOL balance (all server wallets)
- USDC balance (payment wallet)
- Transaction queue depth
- Failed transactions

### Discord/Slack Alerts

Configure webhook URL for real-time alerts:

```yaml
monitoring:
  low_balance_alert_url: "https://discord.com/api/webhooks/..."
  low_balance_threshold: 0.05
  check_interval: "10m"
```

**Alert Example:**

```
🚨 Low Balance Alert
Wallet: 9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin
Balance: 0.03 SOL
Threshold: 0.05 SOL
Action Required: Refill server wallet
```

### Health Check Endpoint

```bash
curl http://localhost:8080/cedros-health
```

**Response:**

```json
{
  "status": "ok",
  "routePrefix": "/api",
  "version": "1.0.0",
  "uptime": "24h30m15s"
}
```

**Use for:**

- Load balancer health checks
- Kubernetes liveness/readiness probes
- Uptime monitoring (Pingdom, UptimeRobot)

### Prometheus Metrics

If using Prometheus, expose metrics endpoint:

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

router.Handle("/metrics", promhttp.Handler())
```

---

## Security

### Secrets Management

**Never commit secrets to git:**

```bash
# .gitignore
*.key
*.json
configs/production.yaml
.env
```

**Use environment variables:**

```bash
export STRIPE_SECRET_KEY=sk_live_...
export X402_SERVER_WALLET_1="[1,2,3,...]"
```

**Or secret management service:**

- AWS Secrets Manager
- Google Secret Manager
- HashiCorp Vault
- Kubernetes Secrets

### Wallet Security

**Best Practices:**

1. **Limited Funds** - Keep only operational balance in server wallets
2. **Hot Wallet** - Server wallets are hot wallets (always online)
3. **Cold Storage** - Move excess funds to cold storage regularly
4. **Key Rotation** - Rotate server wallet keys every 3-6 months
5. **Separate Wallets** - Use different wallets for payments vs refunds

**Refill Strategy:**

```bash
# Monitor balance
solana balance <SERVER_WALLET>

# Refill when low (manual or automated)
solana transfer <SERVER_WALLET> 0.1 --from <COLD_WALLET>
```

### Network Security

**Firewall Rules:**

```bash
# Allow only necessary ports
ufw allow 80/tcp    # HTTP (redirect to HTTPS)
ufw allow 443/tcp   # HTTPS
ufw deny 8080/tcp   # Block direct access to app port
```

**Reverse Proxy (nginx):**

```nginx
server {
    listen 80;
    server_name api.yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name api.yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/api.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Audit Logging

Log all critical operations:

```go
log.Info("payment_verified",
    "resource", resourceID,
    "wallet", wallet,
    "amount", amount,
    "signature", signature,
    "timestamp", time.Now())

log.Info("refund_processed",
    "refund_id", refundID,
    "amount", amount,
    "recipient", recipient,
    "signature", signature,
    "timestamp", time.Now())
```

Ship logs to:

- CloudWatch (AWS)
- Stackdriver (GCP)
- Datadog
- ELK Stack

---

## Troubleshooting

### Common Issues

**1. Webhook Signature Validation Fails**

```
Error: stripe: webhook signature invalid
```

**Solution:**

- Verify `STRIPE_WEBHOOK_SECRET` matches Stripe dashboard
- Check webhook endpoint URL is HTTPS
- Ensure raw request body is used (not parsed JSON)

**2. Transaction Confirmation Timeout**

```
Error: transaction confirmation timeout after 60s
```

**Solution:**

- Increase `commitment` from `confirmed` to `finalized`
- Check Solana network status (mainnet-status.solana.com)
- Verify RPC endpoint is healthy
- Increase timeout in config if using custom RPC

**3. Insufficient SOL for Transaction**

```
Error: insufficient SOL for transaction fee
```

**Solution:**

- Refill server wallet: `solana transfer <WALLET> 0.1`
- Monitor balance alerts
- Increase `low_balance_threshold`

**4. RPC Rate Limit Exceeded**

```
Error: RPC rate limit exceeded
```

**Solution:**

- Reduce `tx_queue_max_in_flight`
- Increase `tx_queue_min_time_between`
- Upgrade to paid RPC endpoint
- Use multiple server wallets to distribute load

**5. Database Connection Pool Exhausted**

```
Error: database: connection pool exhausted
```

**Solution:**

- Increase `max_connections` in database config
- Check for connection leaks
- Add connection timeout
- Scale database instance

### Debug Mode

Enable detailed logging:

```yaml
logging:
  level: "debug"
  format: "json"
  output: "stdout"
```

```bash
export LOG_LEVEL=debug
./bin/server --config configs/production.yaml
```

### Support Resources

- **GitHub Issues:** https://github.com/CedrosPay/server/issues
- **Security Issues:** security@cedros.dev
- **Documentation:** https://github.com/CedrosPay/server/docs
- **Solana Status:** https://status.solana.com
- **Stripe Status:** https://status.stripe.com

---

## Backup and Recovery

### Backup Checklist

- [ ] **Config Files** - Store encrypted backups of production configs
- [ ] **Database Backups** - Daily automated backups (if using persistent storage)
- [ ] **Server Wallet Keys** - Encrypted offline backups
- [ ] **Stripe Webhook Logs** - 30-day retention
- [ ] **Payment Records** - Long-term archival

### Disaster Recovery

**Scenario: Server Wallet Compromised**

1. Stop all servers immediately
2. Generate new wallet keypairs
3. Transfer remaining funds to new wallets
4. Update server configuration with new keys
5. Deploy updated configuration
6. Monitor for unauthorized transactions
7. File incident report

**Scenario: Database Failure**

1. Switch to read-replica (if available)
2. Restore from latest backup
3. Verify data integrity
4. Resume normal operations
5. Post-mortem analysis

**Recovery Time Objective (RTO):** < 15 minutes
**Recovery Point Objective (RPO):** < 5 minutes

---

## Next Steps

1. Review [Security Best Practices](../SECURITY.md)
2. Set up [Wallet Monitoring](../WALLET_MONITORING.md)
3. Configure [Database Schema](DATABASE_SCHEMA.md) (if using persistent storage)
4. Test with [Cart Checkout Examples](../CART_CHECKOUT_TESTING.md)
5. Integrate [Frontend SDK](../FRONTEND_INTEGRATION.md)

---

**Questions or Issues?**
Open an issue: https://github.com/CedrosPay/server/issues
