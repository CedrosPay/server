#!/usr/bin/env bash

# populate-production-data.sh
# Populates production PostgreSQL database with products and coupons from production.yaml
#
# Usage:
#   POSTGRES_URL="postgresql://user:pass@host:port/db" ./scripts/populate-production-data.sh
#
# Environment variables:
#   POSTGRES_URL - PostgreSQL connection string (REQUIRED)

set -euo pipefail

# Check for required PostgreSQL URL
if [ -z "${POSTGRES_URL:-}" ]; then
    echo "❌ ERROR: POSTGRES_URL environment variable is required"
    echo ""
    echo "Usage:"
    echo "  POSTGRES_URL='postgresql://user:pass@host:port/db' ./scripts/populate-production-data.sh"
    exit 1
fi

# Production table names (from production.yaml schema_mapping)
POSTGRES_PRODUCTS_TABLE="cedros_pay_demo_products"
POSTGRES_COUPONS_TABLE="cedros_pay_demo_coupons"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if psql is installed
check_postgres_cli() {
    if ! command -v psql &> /dev/null; then
        log_error "psql CLI not found. Please install PostgreSQL client."
        log_info "Install: brew install postgresql"
        return 1
    fi
    return 0
}

# Populate PostgreSQL with production data
populate_postgres() {
    log_info "Populating PostgreSQL production database..."
    log_info "Connection: ${POSTGRES_URL%%@*}@****" # Hide credentials
    log_info "Tables: ${POSTGRES_PRODUCTS_TABLE}, ${POSTGRES_COUPONS_TABLE}"

    if ! check_postgres_cli; then
        return 1
    fi

    # Create temp file for PostgreSQL commands
    POSTGRES_SCRIPT=$(mktemp)
    trap "rm -f ${POSTGRES_SCRIPT}" EXIT

    cat > "${POSTGRES_SCRIPT}" <<'POSTGRES_EOF'
-- Drop existing tables (fresh start)
DROP TABLE IF EXISTS cedros_pay_demo_products CASCADE;
DROP TABLE IF EXISTS cedros_pay_demo_coupons CASCADE;

-- Create products table
-- IMPORTANT: Uses ATOMIC units (BIGINT) for all money amounts
--   - fiat_amount: cents for USD (100 = $1.00)
--   - crypto_amount: micro-units (1000000 = 1.00 USDC with 6 decimals)
CREATE TABLE cedros_pay_demo_products (
    id TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    fiat_amount BIGINT NOT NULL DEFAULT 0,
    fiat_currency TEXT NOT NULL DEFAULT 'USD',
    stripe_price_id TEXT NOT NULL DEFAULT '',
    crypto_amount BIGINT DEFAULT NULL,
    crypto_token TEXT DEFAULT NULL,
    crypto_account TEXT NOT NULL DEFAULT '',
    memo_template TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}',
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create coupons table
CREATE TABLE cedros_pay_demo_coupons (
    code TEXT PRIMARY KEY,
    discount_type TEXT NOT NULL,
    discount_value DOUBLE PRECISION NOT NULL,
    currency TEXT NOT NULL DEFAULT '',
    scope TEXT NOT NULL DEFAULT 'all',
    product_ids JSONB NOT NULL DEFAULT '[]',
    payment_method TEXT NOT NULL DEFAULT '',
    auto_apply BOOLEAN NOT NULL DEFAULT false,
    applies_at TEXT NOT NULL DEFAULT 'checkout',
    usage_limit INTEGER,
    usage_count INTEGER NOT NULL DEFAULT 0,
    starts_at TIMESTAMP,
    expires_at TIMESTAMP,
    active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Insert products from production.yaml + local.yaml (for Storybook)
INSERT INTO cedros_pay_demo_products (id, description, fiat_amount, fiat_currency, stripe_price_id, crypto_amount, crypto_token, memo_template, metadata, active, created_at, updated_at)
VALUES
  -- From production.yaml
  ('ebook-1', 'The Cedros Guide to Generational Wealth', 100, 'USD', 'price_1SSHBAJb9VbrK3umvSLT7T7T', 1000000, 'USDC', '{{resource}}:{{nonce}}', '{"product":"ebook demo"}', true, NOW(), NOW()),
  ('tip-1', 'Buy me a Coffee', 69, 'USD', 'price_1SSKBfJb9VbrK3um2qy603DZ', 690000, 'USDC', '{{resource}}:{{nonce}}', '{"product":"tip"}', true, NOW(), NOW()),

  -- From local.yaml (Storybook expects these)
  ('demo-item-id-1', 'Demo protected content', 100, 'USD', 'price_1SSOvAJb9VbrK3um0adeI14Y', 1000000, 'USDC', '{{resource}}:{{nonce}}', '{"plan":"demo"}', true, NOW(), NOW()),
  ('demo-item-id-2', 'Test product 2', 222, 'USD', 'price_1SQCuhR4HtkFbUJKDUQpCA6D', 2220000, 'USDC', '{{resource}}:{{nonce}}', '{"product":"test-2"}', true, NOW(), NOW()),
  ('demo-item-id-3', 'Test product 3', 150, 'USD', '', 1500000, 'USDC', '{{resource}}:{{nonce}}', '{"product":"test-3"}', true, NOW(), NOW()),
  ('demo-item-id-4', 'Test product 4', 69, 'USD', '', 690000, 'USDC', '{{resource}}:{{nonce}}', '{"product":"test-4"}', true, NOW(), NOW()),
  ('demo-item-id-5', 'Premium Article Access', 100, 'USD', '', 1000000, 'USDC', '{{resource}}:{{nonce}}', '{"product":"test-5"}', true, NOW(), NOW());

-- Insert coupon from production.yaml
-- SAVE20: 20% off, manual entry, expires end of 2025
INSERT INTO cedros_pay_demo_coupons (code, discount_type, discount_value, currency, scope, product_ids, payment_method, auto_apply, applies_at, usage_limit, usage_count, starts_at, expires_at, active, metadata, created_at, updated_at)
VALUES
  ('SAVE20', 'percentage', 20.0, '', 'all', '[]', '', false, 'checkout', 100000, 0, '2025-01-01 00:00:00', '2025-12-31 23:59:59', true, '{"campaign":"demo-sale","cart_eligible":"true"}', NOW(), NOW());

-- Create indexes for products
CREATE INDEX idx_cedros_pay_demo_products_active ON cedros_pay_demo_products(active);
CREATE INDEX idx_cedros_pay_demo_products_created_at ON cedros_pay_demo_products(created_at DESC);

-- Create indexes for coupons
CREATE INDEX idx_cedros_pay_demo_coupons_active ON cedros_pay_demo_coupons(active);
CREATE INDEX idx_cedros_pay_demo_coupons_expires_at ON cedros_pay_demo_coupons(expires_at);
CREATE INDEX idx_cedros_pay_demo_coupons_created_at ON cedros_pay_demo_coupons(created_at DESC);
CREATE INDEX idx_cedros_pay_demo_coupons_auto_apply ON cedros_pay_demo_coupons(auto_apply) WHERE auto_apply = true;

-- Show results
SELECT '✅ PostgreSQL Production Data Population Complete!' as result;
SELECT 'Products inserted: ' || COUNT(*)::text as products FROM cedros_pay_demo_products;
SELECT 'Coupons inserted: ' || COUNT(*)::text as coupons FROM cedros_pay_demo_coupons;
SELECT '' as separator;
SELECT 'Tables created:' as info;
SELECT '  - cedros_pay_demo_products' as table_name
UNION ALL SELECT '  - cedros_pay_demo_coupons';
POSTGRES_EOF

    # Execute PostgreSQL script
    if psql "${POSTGRES_URL}" < "${POSTGRES_SCRIPT}"; then
        log_info "✅ PostgreSQL population successful!"
        return 0
    else
        log_error "Failed to populate PostgreSQL"
        return 1
    fi
}

# Main script
main() {
    echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║  Cedros Pay - Production Data Population              ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
    echo ""
    log_warn "This will REPLACE existing data in production tables!"
    log_warn "Tables: ${POSTGRES_PRODUCTS_TABLE}, ${POSTGRES_COUPONS_TABLE}"
    echo ""
    read -p "Continue? (y/N): " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Aborted by user"
        exit 0
    fi
    echo ""

    populate_postgres

    echo ""
    log_info "🎉 Production data population complete!"
    echo ""
    echo -e "${YELLOW}Data loaded:${NC}"
    echo "  ✓ 7 products (ebook-1, tip-1, demo-item-id-1 through demo-item-id-5)"
    echo "  ✓ 1 coupon (SAVE20)"
    echo ""
    echo -e "${YELLOW}Next steps:${NC}"
    echo "1. Verify data: psql \"\$POSTGRES_URL\" -c 'SELECT * FROM ${POSTGRES_PRODUCTS_TABLE};'"
    echo "2. Deploy server with production.yaml config"
    echo "3. Test endpoints:"
    echo "   - GET /paywall/v1/products"
    echo "   - GET /paywall/v1/coupons"
}

# Run main function
main "$@"
