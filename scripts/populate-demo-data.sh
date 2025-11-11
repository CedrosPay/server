#!/usr/bin/env bash

# populate-demo-data.sh
# Populates MongoDB and PostgreSQL with demo data for testing
#
# This script uses PREFIXED table/collection names (cedros_pay_test_*)
# to safely test in shared development databases without conflicts.
#
# Usage:
#   ./scripts/populate-demo-data.sh [mongodb|postgres|both]
#
# Environment variables:
#   MONGO_URI     - MongoDB connection string
#   POSTGRES_URI  - PostgreSQL connection string

set -euo pipefail

# Default connection strings (from your local.yaml)
MONGO_URI="your-mongodb-uri"
POSTGRES_URI="your-postgresql-uri"

# IMPORTANT: Using prefixed names for safe testing in shared databases
MONGO_DB="cedros_pay_test"
MONGO_PRODUCTS_COLLECTION="cedros_pay_test_products"
MONGO_COUPONS_COLLECTION="cedros_pay_test_coupons"
MONGO_SESSIONS_COLLECTION="cedros_pay_test_sessions"

POSTGRES_PRODUCTS_TABLE="cedros_pay_test_products"
POSTGRES_COUPONS_TABLE="cedros_pay_test_coupons"
POSTGRES_SESSIONS_TABLE="cedros_pay_test_sessions"

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

# Check if mongosh is installed
check_mongo_cli() {
    if ! command -v mongosh &> /dev/null; then
        log_error "mongosh CLI not found. Please install MongoDB Shell."
        log_info "Install: brew install mongosh"
        return 1
    fi
    return 0
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

# Populate MongoDB
populate_mongodb() {
    log_info "Populating MongoDB at ${MONGO_URI}..."
    log_info "Database: ${MONGO_DB}"
    log_info "Collections: ${MONGO_PRODUCTS_COLLECTION}, ${MONGO_COUPONS_COLLECTION}, ${MONGO_SESSIONS_COLLECTION}"

    if ! check_mongo_cli; then
        return 1
    fi

    # Create temp file for MongoDB commands
    MONGO_SCRIPT=$(mktemp)
    trap "rm -f ${MONGO_SCRIPT}" EXIT

    cat > "${MONGO_SCRIPT}" <<MONGO_EOF
// Switch to test database
use ${MONGO_DB};

// Drop existing collections (fresh start)
db.${MONGO_PRODUCTS_COLLECTION}.drop();
db.${MONGO_COUPONS_COLLECTION}.drop();
db.${MONGO_SESSIONS_COLLECTION}.drop();

// Create products collection with demo data
// IMPORTANT: Uses ATOMIC units (int64) for all money amounts
//   - fiatAtomic: cents for USD (100 = \$1.00)
//   - cryptoAtomic: micro-USDC for USDC (1000000 = 1.00 USDC)
db.${MONGO_PRODUCTS_COLLECTION}.insertMany([
  {
    "_id": "demo-item-id-1",
    "description": "Demo protected content (MongoDB test)",
    "fiatAtomic": NumberLong(100),        // \$1.00 = 100 cents
    "fiatAsset": "USD",
    "stripePriceId": "price_1SPqRpR4HtkFbUJKUciKecmZ",
    "cryptoAtomic": NumberLong(1000000),  // 1.00 USDC = 1000000 micro-USDC
    "cryptoAsset": "USDC",
    "cryptoAccount": "",
    "memoTemplate": "{{resource}}:{{nonce}}",
    "metadata": {
      "plan": "demo",
      "source": "mongodb_test"
    },
    "active": true,
    "createdAt": new Date(),
    "updatedAt": new Date()
  },
  {
    "_id": "demo-item-id-2",
    "description": "Premium API access (MongoDB test)",
    "fiatAtomic": NumberLong(500),        // \$5.00 = 500 cents
    "fiatAsset": "USD",
    "stripePriceId": "price_1SQCuhR4HtkFbUJKDUQpCA6D",
    "cryptoAtomic": NumberLong(5000000),  // 5.00 USDC = 5000000 micro-USDC
    "cryptoAsset": "USDC",
    "cryptoAccount": "",
    "memoTemplate": "{{resource}}:{{nonce}}",
    "metadata": {
      "product": "premium-api",
      "source": "mongodb_test"
    },
    "active": true,
    "createdAt": new Date(),
    "updatedAt": new Date()
  },
  {
    "_id": "demo-item-id-3",
    "description": "Enterprise subscription (MongoDB test)",
    "fiatAtomic": NumberLong(1000),       // \$10.00 = 1000 cents
    "fiatAsset": "USD",
    "stripePriceId": "",
    "cryptoAtomic": NumberLong(10000000), // 10.00 USDC = 10000000 micro-USDC
    "cryptoAsset": "USDC",
    "cryptoAccount": "",
    "memoTemplate": "{{resource}}:{{nonce}}",
    "metadata": {
      "product": "enterprise",
      "source": "mongodb_test"
    },
    "active": true,
    "createdAt": new Date(),
    "updatedAt": new Date()
  }
]);

// Create coupons collection with demo data
db.${MONGO_COUPONS_COLLECTION}.insertMany([
  {
    "_id": "SAVE20",
    "code": "SAVE20",
    "discountType": "percentage",
    "discountValue": 20.0,
    "currency": "",
    "scope": "all",
    "productIds": [],
    "paymentMethod": "",
    "autoApply": false,
    "appliesAt": "checkout",
    "usageLimit": 100000,
    "usageCount": 0,
    "startsAt": new Date("2025-01-01T00:00:00Z"),
    "expiresAt": new Date("2025-12-31T23:59:59Z"),
    "active": true,
    "metadata": {
      "campaign": "winter-sale",
      "cart_eligible": "true",
      "source": "mongodb_test"
    },
    "createdAt": new Date(),
    "updatedAt": new Date()
  },
  {
    "_id": "CRYPTO5AUTO",
    "code": "CRYPTO5AUTO",
    "discountType": "percentage",
    "discountValue": 5.0,
    "currency": "",
    "scope": "all",
    "productIds": [],
    "paymentMethod": "x402",
    "autoApply": true,
    "appliesAt": "checkout",
    "usageLimit": null,
    "usageCount": 0,
    "startsAt": null,
    "expiresAt": null,
    "active": true,
    "metadata": {
      "campaign": "crypto-auto",
      "description": "Extra 5% off for crypto (auto-applied)",
      "source": "mongodb_test"
    },
    "createdAt": new Date(),
    "updatedAt": new Date()
  },
  {
    "_id": "FIXED5",
    "code": "FIXED5",
    "discountType": "fixed",
    "discountValue": 0.50,
    "currency": "usd",
    "scope": "all",
    "productIds": [],
    "paymentMethod": "",
    "autoApply": true,
    "appliesAt": "checkout",
    "usageLimit": null,
    "usageCount": 0,
    "startsAt": null,
    "expiresAt": null,
    "active": true,
    "metadata": {
      "campaign": "fixed-discount-auto",
      "description": "50 cents off (auto-applied)",
      "source": "mongodb_test"
    },
    "createdAt": new Date(),
    "updatedAt": new Date()
  },
  {
    "_id": "NEWUSER10",
    "code": "NEWUSER10",
    "discountType": "fixed",
    "discountValue": 10.0,
    "currency": "usd",
    "scope": "all",
    "productIds": [],
    "paymentMethod": "",
    "autoApply": false,
    "appliesAt": "checkout",
    "usageLimit": null,
    "usageCount": 0,
    "startsAt": null,
    "expiresAt": null,
    "active": true,
    "metadata": {
      "campaign": "new-user",
      "cart_eligible": "true",
      "source": "mongodb_test"
    },
    "createdAt": new Date(),
    "updatedAt": new Date()
  }
]);

// Create indexes for products
db.${MONGO_PRODUCTS_COLLECTION}.createIndex({ "_id": 1 });
db.${MONGO_PRODUCTS_COLLECTION}.createIndex({ "active": 1 });
db.${MONGO_PRODUCTS_COLLECTION}.createIndex({ "createdAt": -1 });

// Create indexes for coupons
db.${MONGO_COUPONS_COLLECTION}.createIndex({ "_id": 1 });
db.${MONGO_COUPONS_COLLECTION}.createIndex({ "code": 1 }, { unique: true });
db.${MONGO_COUPONS_COLLECTION}.createIndex({ "active": 1 });
db.${MONGO_COUPONS_COLLECTION}.createIndex({ "expiresAt": 1 });
db.${MONGO_COUPONS_COLLECTION}.createIndex({ "autoApply": 1 });
db.${MONGO_COUPONS_COLLECTION}.createIndex({ "createdAt": -1 });

// Create sessions collection placeholder (for Stripe sessions)
db.${MONGO_SESSIONS_COLLECTION}.createIndex({ "sessionId": 1 }, { unique: true });
db.${MONGO_SESSIONS_COLLECTION}.createIndex({ "createdAt": -1 });

// Show results
print("\\n✅ MongoDB Test Data Population Complete!");
print("Database: ${MONGO_DB}");
print("Products inserted: " + db.${MONGO_PRODUCTS_COLLECTION}.countDocuments({}));
print("Coupons inserted: " + db.${MONGO_COUPONS_COLLECTION}.countDocuments({}));
print("\\nCollections created:");
print("  - ${MONGO_PRODUCTS_COLLECTION}");
print("  - ${MONGO_COUPONS_COLLECTION}");
print("  - ${MONGO_SESSIONS_COLLECTION}");
MONGO_EOF

    # Execute MongoDB script
    if mongosh "${MONGO_URI}" < "${MONGO_SCRIPT}"; then
        log_info "✅ MongoDB population successful!"
        return 0
    else
        log_error "Failed to populate MongoDB"
        return 1
    fi
}

# Populate PostgreSQL
populate_postgres() {
    log_info "Populating PostgreSQL at ${POSTGRES_URI}..."
    log_info "Tables: ${POSTGRES_PRODUCTS_TABLE}, ${POSTGRES_COUPONS_TABLE}, ${POSTGRES_SESSIONS_TABLE}"

    if ! check_postgres_cli; then
        return 1
    fi

    # Create temp file for PostgreSQL commands
    POSTGRES_SCRIPT=$(mktemp)
    trap "rm -f ${POSTGRES_SCRIPT}" EXIT

    cat > "${POSTGRES_SCRIPT}" <<POSTGRES_EOF
-- Drop existing tables (fresh start)
DROP TABLE IF EXISTS ${POSTGRES_PRODUCTS_TABLE} CASCADE;
DROP TABLE IF EXISTS ${POSTGRES_COUPONS_TABLE} CASCADE;
DROP TABLE IF EXISTS ${POSTGRES_SESSIONS_TABLE} CASCADE;

-- Create products table with test prefix
-- IMPORTANT: Uses ATOMIC units (BIGINT) for all money amounts (migration 005 schema)
--   - fiat_amount: cents for USD (BIGINT, 100 = \$1.00)
--   - fiat_currency: asset code (TEXT, 'USD')
--   - crypto_amount: micro-USDC for USDC (BIGINT, 1000000 = 1.00 USDC)
--   - crypto_token: asset code (TEXT, 'USDC')
CREATE TABLE ${POSTGRES_PRODUCTS_TABLE} (
    id TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    fiat_amount BIGINT NOT NULL DEFAULT 0,           -- Atomic units (cents for USD)
    fiat_currency TEXT NOT NULL DEFAULT 'USD',       -- Asset code (not lowercase!)
    stripe_price_id TEXT NOT NULL DEFAULT '',
    crypto_amount BIGINT DEFAULT NULL,               -- Atomic units (micro-USDC)
    crypto_token TEXT DEFAULT NULL,                  -- Asset code ('USDC')
    crypto_account TEXT NOT NULL DEFAULT '',
    memo_template TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}',
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create coupons table with test prefix
CREATE TABLE ${POSTGRES_COUPONS_TABLE} (
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

-- Create sessions table with test prefix (for Stripe sessions)
CREATE TABLE ${POSTGRES_SESSIONS_TABLE} (
    session_id TEXT PRIMARY KEY,
    resource_id TEXT NOT NULL,
    amount_cents BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'usd',
    status TEXT NOT NULL DEFAULT 'pending',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Insert demo products with ATOMIC units
-- fiat_amount: cents (100 = \$1.00)
-- crypto_amount: micro-USDC (1000000 = 1.00 USDC)
INSERT INTO ${POSTGRES_PRODUCTS_TABLE} (id, description, fiat_amount, fiat_currency, stripe_price_id, crypto_amount, crypto_token, memo_template, metadata, active, created_at, updated_at)
VALUES
  ('demo-item-id-1', 'Demo protected content (PostgreSQL test)', 100, 'USD', 'price_1SPqRpR4HtkFbUJKUciKecmZ', 1000000, 'USDC', '{{resource}}:{{nonce}}', '{"plan":"demo","source":"postgres_test"}', true, NOW(), NOW()),
  ('demo-item-id-2', 'Premium API access (PostgreSQL test)', 500, 'USD', 'price_1SQCuhR4HtkFbUJKDUQpCA6D', 5000000, 'USDC', '{{resource}}:{{nonce}}', '{"product":"premium-api","source":"postgres_test"}', true, NOW(), NOW()),
  ('demo-item-id-3', 'Enterprise subscription (PostgreSQL test)', 1000, 'USD', '', 10000000, 'USDC', '{{resource}}:{{nonce}}', '{"product":"enterprise","source":"postgres_test"}', true, NOW(), NOW());

-- Insert demo coupons
INSERT INTO ${POSTGRES_COUPONS_TABLE} (code, discount_type, discount_value, currency, scope, product_ids, payment_method, auto_apply, applies_at, usage_limit, usage_count, starts_at, expires_at, active, metadata, created_at, updated_at)
VALUES
  ('SAVE20', 'percentage', 20.0, '', 'all', '[]', '', false, 'checkout', 100000, 0, '2025-01-01 00:00:00', '2025-12-31 23:59:59', true, '{"campaign":"winter-sale","cart_eligible":"true","source":"postgres_test"}', NOW(), NOW()),
  ('CRYPTO5AUTO', 'percentage', 5.0, '', 'all', '[]', 'x402', true, 'checkout', NULL, 0, NULL, NULL, true, '{"campaign":"crypto-auto","description":"Extra 5% off for crypto (auto-applied)","source":"postgres_test"}', NOW(), NOW()),
  ('FIXED5', 'fixed', 0.50, 'usd', 'all', '[]', '', true, 'checkout', NULL, 0, NULL, NULL, true, '{"campaign":"fixed-discount-auto","description":"50 cents off (auto-applied)","source":"postgres_test"}', NOW(), NOW()),
  ('NEWUSER10', 'fixed', 10.0, 'usd', 'all', '[]', '', false, 'checkout', NULL, 0, NULL, NULL, true, '{"campaign":"new-user","cart_eligible":"true","source":"postgres_test"}', NOW(), NOW());

-- Create indexes for products
CREATE INDEX idx_${POSTGRES_PRODUCTS_TABLE}_active ON ${POSTGRES_PRODUCTS_TABLE}(active);
CREATE INDEX idx_${POSTGRES_PRODUCTS_TABLE}_created_at ON ${POSTGRES_PRODUCTS_TABLE}(created_at DESC);

-- Create indexes for coupons
CREATE INDEX idx_${POSTGRES_COUPONS_TABLE}_active ON ${POSTGRES_COUPONS_TABLE}(active);
CREATE INDEX idx_${POSTGRES_COUPONS_TABLE}_expires_at ON ${POSTGRES_COUPONS_TABLE}(expires_at);
CREATE INDEX idx_${POSTGRES_COUPONS_TABLE}_created_at ON ${POSTGRES_COUPONS_TABLE}(created_at DESC);
CREATE INDEX idx_${POSTGRES_COUPONS_TABLE}_auto_apply ON ${POSTGRES_COUPONS_TABLE}(auto_apply) WHERE auto_apply = true;

-- Create indexes for sessions
CREATE INDEX idx_${POSTGRES_SESSIONS_TABLE}_resource_id ON ${POSTGRES_SESSIONS_TABLE}(resource_id);
CREATE INDEX idx_${POSTGRES_SESSIONS_TABLE}_status ON ${POSTGRES_SESSIONS_TABLE}(status);
CREATE INDEX idx_${POSTGRES_SESSIONS_TABLE}_created_at ON ${POSTGRES_SESSIONS_TABLE}(created_at DESC);

-- Show results
SELECT '✅ PostgreSQL Test Data Population Complete!' as result;
SELECT 'Tables created with cedros_pay_test_ prefix' as info;
SELECT 'Products inserted: ' || COUNT(*)::text as products FROM ${POSTGRES_PRODUCTS_TABLE};
SELECT 'Coupons inserted: ' || COUNT(*)::text as coupons FROM ${POSTGRES_COUPONS_TABLE};
SELECT '' as separator;
SELECT 'Tables created:' as info;
SELECT '  - ${POSTGRES_PRODUCTS_TABLE}' as table_name
UNION ALL SELECT '  - ${POSTGRES_COUPONS_TABLE}'
UNION ALL SELECT '  - ${POSTGRES_SESSIONS_TABLE}';
POSTGRES_EOF

    # Execute PostgreSQL script
    if psql "${POSTGRES_URI}" < "${POSTGRES_SCRIPT}"; then
        log_info "✅ PostgreSQL population successful!"
        return 0
    else
        log_error "Failed to populate PostgreSQL"
        return 1
    fi
}

# Main script
main() {
    local target="${1:-both}"

    echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║  Cedros Pay - Demo Data Population (Test Mode)        ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
    echo ""
    log_warn "Using PREFIXED tables/collections: cedros_pay_test_*"
    log_warn "Safe for shared development databases"
    echo ""

    case "$target" in
        mongodb)
            populate_mongodb
            ;;
        postgres)
            populate_postgres
            ;;
        both)
            log_info "Populating both MongoDB and PostgreSQL..."
            populate_mongodb
            populate_postgres
            ;;
        *)
            log_error "Invalid target: $target"
            echo "Usage: $0 [mongodb|postgres|both]"
            exit 1
            ;;
    esac

    echo ""
    log_info "🎉 Demo data population complete!"
    echo ""
    echo -e "${YELLOW}Next steps:${NC}"
    echo "1. Update configs/local.yaml to use these test tables:"
    echo ""
    echo -e "${BLUE}For MongoDB:${NC}"
    echo "  paywall:"
    echo "    product_source: mongodb"
    echo "    mongodb_url: ${MONGO_URI}"
    echo "    mongodb_database: ${MONGO_DB}"
    echo "  coupons:"
    echo "    coupon_source: mongodb"
    echo "  storage:"
    echo "    backend: mongodb"
    echo ""
    echo "  schema_mapping:"
    echo "    products:"
    echo "      table_name: \"${MONGO_PRODUCTS_COLLECTION}\""
    echo "    coupons:"
    echo "      table_name: \"${MONGO_COUPONS_COLLECTION}\""
    echo ""
    echo -e "${BLUE}For PostgreSQL:${NC}"
    echo "  paywall:"
    echo "    product_source: postgres"
    echo "    postgres_url: ${POSTGRES_URI}"
    echo "  coupons:"
    echo "    coupon_source: postgres"
    echo "  storage:"
    echo "    backend: postgres"
    echo ""
    echo "  schema_mapping:"
    echo "    products:"
    echo "      table_name: \"${POSTGRES_PRODUCTS_TABLE}\""
    echo "    coupons:"
    echo "      table_name: \"${POSTGRES_COUPONS_TABLE}\""
    echo ""
    echo -e "${YELLOW}To clean up test data:${NC}"
    echo "  MongoDB: db.${MONGO_PRODUCTS_COLLECTION}.drop()"
    echo "  PostgreSQL: DROP TABLE ${POSTGRES_PRODUCTS_TABLE} CASCADE;"
}

# Run main function
main "$@"
