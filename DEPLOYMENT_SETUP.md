# Cedros Pay Server - Deployment Setup Guide

## Overview

This guide walks you through setting up automated deployment for Cedros Pay Server to your production server using GitHub Actions.

## Architecture

- **Server Port**: `8082` (internal Docker port 8080, exposed as 127.0.0.1:8082)
- **Domain**: `pay.cedros.io` (Nginx reverse proxy handles routing)
- **Docker Containers**:
  - `cedros-pay-server` - Main application server
  - `cedros-pay-redis` - Redis for rate limiting and idempotency cache
- **PostgreSQL**: Dedicated database server (accessed via WireGuard)
- **Network**: `cedros-pay-network` (internal Docker network for container communication)

## Required GitHub Secrets

Add these secrets to your GitHub repository at: `Settings → Secrets and variables → Actions → New repository secret`

### Docker Hub Credentials

| Secret Name | Description | Example |
|------------|-------------|---------|
| `DOCKER_USERNAME` | Your Docker Hub username | `hdbdocker` |
| `DOCKER_PASSWORD` | Your Docker Hub password/token | `dckr_pat_xxx...` |

### Server Access

| Secret Name | Description | Example |
|------------|-------------|---------|
| `SERVER_HOST` | Your server's IP or hostname | `10.0.0.1` or `api.cedros.io` |
| `SERVER_USERNAME` | SSH username (usually `root` or `ubuntu`) | `root` |
| `SERVER_SSH_KEY` | Private SSH key for server access | `-----BEGIN OPENSSH PRIVATE KEY-----\n...` |

**To get your SSH key:**
```bash
cat ~/.ssh/id_rsa
# Copy the entire output including header/footer
```

### Database Configuration

| Secret Name | Description | Example |
|------------|-------------|---------|
| `POSTGRES_URL` | Full PostgreSQL connection string | `postgresql://user:password@10.0.0.5:5432/cedros_pay?sslmode=require` |

**Format:**
```
postgresql://username:password@host:port/database?sslmode=require
```

**Note:** Your dedicated PostgreSQL server should be accessible from the application server (e.g., via WireGuard VPN).

### Stripe Configuration

| Secret Name | Description | Where to Find |
|------------|-------------|---------------|
| `STRIPE_SECRET_KEY` | Stripe API secret key | Dashboard → Developers → API keys → Secret key (starts with `sk_live_` or `sk_test_`) |
| `STRIPE_PUBLISHABLE_KEY` | Stripe publishable key | Dashboard → Developers → API keys → Publishable key (starts with `pk_live_` or `pk_test_`) |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signing secret | Dashboard → Developers → Webhooks → Select endpoint → Signing secret (starts with `whsec_`) |

### Solana/x402 Configuration

| Secret Name | Description | Example |
|------------|-------------|---------|
| `X402_WALLET_PRIVATE_KEY` | Solana wallet private key (base58 encoded) | `5J7Zm...` (88 characters) |
| `X402_PAYMENT_ADDRESS` | Solana wallet public address | `tri1Vi4RkdSLu4C2YeofJedi3ZmvCPvtygNuKmiNqCa` |
| `SOLANA_RPC_URL` | Solana RPC endpoint (with auth token) | `https://rpc.quicknode.pro/abc123...` |

**Note:** `X402_TOKEN_MINT` (USDC), `X402_NETWORK` (mainnet-beta), and `CORS_ALLOWED_ORIGINS` are configured in `production.yaml.example` (public values, not secret).

**To export your Solana wallet private key:**
```bash
# If using Solana CLI:
solana-keygen recover -o ~/my-wallet.json 'prompt://keypair'
cat ~/my-wallet.json
# Copy the base58 private key

# Or use:
solana-keygen pubkey ~/my-wallet.json  # Get public address
```

### API Security

| Secret Name | Description | Example |
|------------|-------------|---------|
| `API_KEY` | API key for protected endpoints | `ck_live_abc123...` |

**Generate a secure API key:**
```bash
echo "ck_live_$(openssl rand -hex 32)"
```

## Server Preparation

Before the first deployment, prepare your server:

### 1. Install Docker

```bash
# Update system
apt update && apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh

# Enable Docker service
systemctl enable docker
systemctl start docker
```

### 2. Create Application Directory

```bash
# Create directory structure
mkdir -p /app/cedros-pay/{configs,data}
chmod 755 /app/cedros-pay

# Create a production config file (optional)
# If not provided, the server will use environment variables
nano /app/cedros-pay/configs/production.yaml
```

### 3. Configure Nginx (Reverse Proxy)

Create `/etc/nginx/sites-available/pay.cedros.io`:

```nginx
upstream cedros_pay {
    server 127.0.0.1:8082;
    keepalive 32;
}

server {
    listen 80;
    listen [::]:80;
    server_name pay.cedros.io;

    # Redirect HTTP to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name pay.cedros.io;

    # SSL certificates (configure with Certbot)
    ssl_certificate /etc/letsencrypt/live/pay.cedros.io/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/pay.cedros.io/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Proxy settings
    location / {
        proxy_pass http://cedros_pay;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;

        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # Health check endpoint (bypass auth)
    location /cedros-health {
        proxy_pass http://cedros_pay;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        access_log off;
    }

    # Increase body size for file uploads
    client_max_body_size 10M;
}
```

Enable the site:
```bash
ln -s /etc/nginx/sites-available/pay.cedros.io /etc/nginx/sites-enabled/
nginx -t
systemctl reload nginx
```

### 4. Set Up SSL Certificate

```bash
# Install Certbot
apt install certbot python3-certbot-nginx -y

# Obtain certificate
certbot --nginx -d pay.cedros.io

# Auto-renewal is configured by default
```

### 5. Configure Firewall

```bash
# Allow SSH, HTTP, HTTPS
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw enable
```

## Deployment Workflow

The GitHub Actions workflow will:

1. ✅ **Lint & Test**: Run `go fmt`, `go vet`, and `go test`
2. ✅ **Build Docker Image**: Create optimized multi-stage Docker image
3. ✅ **Push to Docker Hub**: Upload image to your Docker Hub repository
4. ✅ **Deploy to Server**: SSH into server and:
   - Stop old containers
   - Start Redis (rate limiting and idempotency cache)
   - Pull latest image
   - Start Cedros Pay Server on port 8082
   - Connect to dedicated PostgreSQL server via WireGuard
5. ✅ **Health Check**: Verify deployment success

## Manual Deployment Trigger

You can manually trigger a deployment from GitHub:

1. Go to your repository on GitHub
2. Click **Actions** tab
3. Click **Production Deploy** workflow
4. Click **Run workflow** dropdown
5. Select branch (usually `main`)
6. Optionally check **Skip tests** (not recommended)
7. Click **Run workflow**

## Monitoring Deployment

### Check GitHub Actions

Watch the deployment progress in the **Actions** tab of your repository.

### SSH into Server

```bash
ssh root@your-server-ip

# Check running containers
docker ps | grep cedros-pay

# Check logs
docker logs -f cedros-pay-server
docker logs -f cedros-pay-redis

# Check health
curl http://localhost:8082/cedros-health
```

### Check Public Endpoint

```bash
# From your local machine
curl https://pay.cedros.io/cedros-health
```

## Troubleshooting

### Deployment Failed - "Cannot connect to Docker daemon"

**Solution**: Docker service not running on server
```bash
systemctl start docker
systemctl enable docker
```

### Container Exits Immediately

**Solution**: Check logs for errors
```bash
docker logs cedros-pay-server
```

Common issues:
- Missing required environment variables
- Database connection failed
- Invalid Stripe/Solana credentials

### Database Connection Issues

If the server can't connect to PostgreSQL:
```bash
# Check if WireGuard VPN is active
wg show

# Test database connection from application server
psql "$POSTGRES_URL"

# Verify PostgreSQL is listening on the correct interface
# (Run on database server)
sudo netstat -tulpn | grep 5432
```

### Port Already in Use

**Solution**: Check what's using port 8082
```bash
netstat -tulpn | grep 8082
# Kill the process or change the port in deploy.yml
```

## Post-Deployment Checklist

- [ ] Health endpoint responds: `https://pay.cedros.io/cedros-health`
- [ ] Products endpoint works: `https://pay.cedros.io/paywall/v1/products`
- [ ] Stripe webhooks configured to: `https://pay.cedros.io/webhook/stripe`
- [ ] x402 payments are being verified correctly
- [ ] Database is persisting data (check volumes)
- [ ] Redis is caching rate limits
- [ ] Logs are being written properly
- [ ] SSL certificate is valid and auto-renewing

## Backup & Recovery

### Database Backup

```bash
# Backup PostgreSQL (from your dedicated database server)
pg_dump -U cedros_pay_user -h your-db-server cedros_pay > backup.sql

# Or if using WireGuard IP:
pg_dump "postgresql://user:password@10.0.0.5:5432/cedros_pay" > backup.sql

# Restore
psql "postgresql://user:password@10.0.0.5:5432/cedros_pay" < backup.sql
```

### Redis Backup

Redis is configured with AOF (append-only file) persistence. Data is automatically saved to the `cedros_redis_data` Docker volume on the application server.

## Scaling Considerations

For production at scale, consider:

1. **Load Balancer**: Run multiple instances behind a load balancer
2. **Database High Availability**: Configure PostgreSQL replication on dedicated server
3. **External Redis**: Use managed Redis (AWS ElastiCache, Redis Cloud, etc.) instead of containerized Redis
4. **Monitoring**: Add Prometheus/Grafana for metrics
5. **Alerting**: Configure alerts for downtime/errors

## Support

For issues with deployment:
- Check GitHub Actions logs
- Check server logs: `docker logs cedros-pay-server`
- Review CHANGELOG.md for breaking changes
- Open an issue on GitHub

---

**Last Updated**: 2025-11-11
**Version**: 1.0.0
