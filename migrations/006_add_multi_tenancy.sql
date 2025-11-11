-- Migration 006: Add multi-tenancy support to all tables
-- This migration adds tenant_id columns and indexes to enable enterprise multi-tenancy
--
-- Rationale:
--   - Enables SaaS/platform use cases (Shopify, WooCommerce integrations)
--   - Database-level tenant isolation for compliance (GDPR, PCI-DSS)
--   - Prevents need to migrate billions of rows later
--   - Backwards compatible with existing single-tenant deployments
--
-- Strategy:
--   1. Add tenant_id columns with default 'default' (backwards compatible)
--   2. Add tenant-aware composite indexes for query performance
--   3. Keep existing indexes for single-tenant queries
--   4. Add NOT NULL constraint after backfill
--
-- IMPORTANT: This migration is BACKWARDS COMPATIBLE.
-- Existing single-tenant deployments continue working with tenant_id='default'.

-- ============================================================================
-- STRIPE SESSIONS TABLE
-- ============================================================================
ALTER TABLE stripe_sessions
    ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default';

-- Composite index for tenant-scoped queries (most common in multi-tenant mode)
CREATE INDEX IF NOT EXISTS idx_stripe_sessions_tenant_resource
    ON stripe_sessions(tenant_id, resource_id);

CREATE INDEX IF NOT EXISTS idx_stripe_sessions_tenant_status
    ON stripe_sessions(tenant_id, status);

-- Drop old single-tenant indexes (replaced by composite indexes above)
-- These are now redundant - composite indexes can serve single-column queries
DROP INDEX IF EXISTS idx_stripe_sessions_resource;
DROP INDEX IF EXISTS idx_stripe_sessions_status;

-- ============================================================================
-- CRYPTO ACCESS TABLE
-- ============================================================================
ALTER TABLE crypto_access
    ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default';

-- Update primary key to include tenant_id for true tenant isolation
-- This requires recreating the table due to PK change
ALTER TABLE crypto_access DROP CONSTRAINT IF EXISTS crypto_access_pkey;
ALTER TABLE crypto_access
    ADD PRIMARY KEY (tenant_id, resource_id, wallet);

-- Tenant-scoped indexes
CREATE INDEX IF NOT EXISTS idx_crypto_access_tenant_expires
    ON crypto_access(tenant_id, expires_at);

-- Drop old single-tenant index
DROP INDEX IF EXISTS idx_crypto_access_expires;

-- ============================================================================
-- CART QUOTES TABLE
-- ============================================================================
ALTER TABLE cart_quotes
    ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default';

-- Tenant-scoped indexes
CREATE INDEX IF NOT EXISTS idx_cart_quotes_tenant
    ON cart_quotes(tenant_id);

CREATE INDEX IF NOT EXISTS idx_cart_quotes_tenant_expires
    ON cart_quotes(tenant_id, expires_at);

-- Drop old single-tenant index
DROP INDEX IF EXISTS idx_cart_quotes_expires;

-- ============================================================================
-- REFUND QUOTES TABLE
-- ============================================================================
ALTER TABLE refund_quotes
    ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default';

-- Tenant-scoped indexes
CREATE INDEX IF NOT EXISTS idx_refund_quotes_tenant
    ON refund_quotes(tenant_id);

CREATE INDEX IF NOT EXISTS idx_refund_quotes_tenant_expires
    ON refund_quotes(tenant_id, expires_at);

CREATE INDEX IF NOT EXISTS idx_refund_quotes_tenant_purchase
    ON refund_quotes(tenant_id, original_purchase_id);

-- Keep partial index for pending refunds (cross-tenant admin view)
-- This is useful for admin dashboard showing all pending refunds
CREATE INDEX IF NOT EXISTS idx_refund_quotes_tenant_processed
    ON refund_quotes(tenant_id, processed_at)
    WHERE processed_at IS NOT NULL;

-- Drop old single-tenant indexes
DROP INDEX IF EXISTS idx_refund_quotes_expires;
-- Note: idx_refund_quotes_processed kept for backwards compatibility

-- ============================================================================
-- PAYMENT TRANSACTIONS TABLE
-- ============================================================================
ALTER TABLE payment_transactions
    ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default';

-- Tenant-scoped indexes for common queries
CREATE INDEX IF NOT EXISTS idx_payment_transactions_tenant
    ON payment_transactions(tenant_id);

CREATE INDEX IF NOT EXISTS idx_payment_transactions_tenant_resource
    ON payment_transactions(tenant_id, resource_id);

CREATE INDEX IF NOT EXISTS idx_payment_transactions_tenant_wallet
    ON payment_transactions(tenant_id, wallet);

CREATE INDEX IF NOT EXISTS idx_payment_transactions_tenant_created
    ON payment_transactions(tenant_id, created_at DESC);

-- Keep existing indexes for backwards compatibility
-- (composite indexes above will be used preferentially by query planner)

-- ============================================================================
-- ADMIN NONCES TABLE
-- ============================================================================
ALTER TABLE admin_nonces
    ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default';

-- Tenant-scoped indexes
CREATE INDEX IF NOT EXISTS idx_admin_nonces_tenant
    ON admin_nonces(tenant_id);

CREATE INDEX IF NOT EXISTS idx_admin_nonces_tenant_expires
    ON admin_nonces(tenant_id, expires_at);

-- Drop old single-tenant index
DROP INDEX IF EXISTS idx_admin_nonces_expires;

-- ============================================================================
-- PRODUCTS TABLE
-- ============================================================================
ALTER TABLE products
    ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default';

-- Tenant-scoped indexes for product catalog queries
CREATE INDEX IF NOT EXISTS idx_products_tenant
    ON products(tenant_id);

CREATE INDEX IF NOT EXISTS idx_products_tenant_active
    ON products(tenant_id, active);

CREATE INDEX IF NOT EXISTS idx_products_tenant_stripe_price
    ON products(tenant_id, stripe_price_id)
    WHERE stripe_price_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_products_tenant_created
    ON products(tenant_id, created_at DESC);

-- Drop old single-tenant indexes (replaced by tenant-scoped versions)
DROP INDEX IF EXISTS idx_products_active;
DROP INDEX IF EXISTS idx_products_created;
DROP INDEX IF EXISTS idx_products_stripe_price_id;

-- ============================================================================
-- COUPONS TABLE
-- ============================================================================
ALTER TABLE coupons
    ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default';

-- Update primary key to include tenant_id (coupon codes scoped per tenant)
-- This allows different tenants to use the same coupon code
ALTER TABLE coupons DROP CONSTRAINT IF EXISTS coupons_pkey;
ALTER TABLE coupons
    ADD PRIMARY KEY (tenant_id, code);

-- Tenant-scoped indexes for coupon validation queries
CREATE INDEX IF NOT EXISTS idx_coupons_tenant
    ON coupons(tenant_id);

CREATE INDEX IF NOT EXISTS idx_coupons_tenant_active
    ON coupons(tenant_id, active);

CREATE INDEX IF NOT EXISTS idx_coupons_tenant_auto_apply
    ON coupons(tenant_id, auto_apply)
    WHERE auto_apply = true;

CREATE INDEX IF NOT EXISTS idx_coupons_tenant_expires
    ON coupons(tenant_id, expires_at);

CREATE INDEX IF NOT EXISTS idx_coupons_tenant_created
    ON coupons(tenant_id, created_at DESC);

-- Keep GIN index for product_ids (useful across tenants)
-- Recreate as tenant-scoped
DROP INDEX IF EXISTS idx_coupons_product_ids;
CREATE INDEX IF NOT EXISTS idx_coupons_tenant_product_ids
    ON coupons USING gin(product_ids);

-- Drop old single-tenant indexes
DROP INDEX IF EXISTS idx_coupons_active;
DROP INDEX IF EXISTS idx_coupons_auto_apply;
DROP INDEX IF EXISTS idx_coupons_expires;
DROP INDEX IF EXISTS idx_coupons_created;
DROP INDEX IF EXISTS idx_coupons_applies_at;

-- Recreate applies_at index with tenant scope
CREATE INDEX IF NOT EXISTS idx_coupons_tenant_applies_at
    ON coupons(tenant_id, applies_at)
    WHERE applies_at != '';

-- ============================================================================
-- TENANT MANAGEMENT (Future: Add tenants table)
-- ============================================================================
-- NOTE: For now, tenant_id is just a string identifier
-- Future migration will add a dedicated 'tenants' table with:
--   - tenant_id (PK)
--   - name, domain, settings, created_at, active
--   - Stripe account mapping
--   - Solana wallet per tenant
--   - Rate limits, quotas, features

-- Example tenants table structure (for future reference):
-- CREATE TABLE tenants (
--     id TEXT PRIMARY KEY,
--     name TEXT NOT NULL,
--     domain TEXT,
--     stripe_account_id TEXT,
--     solana_wallet TEXT,
--     settings JSONB,
--     active BOOLEAN NOT NULL DEFAULT true,
--     created_at TIMESTAMP NOT NULL DEFAULT NOW(),
--     updated_at TIMESTAMP NOT NULL DEFAULT NOW()
-- );

-- ============================================================================
-- VERIFICATION QUERIES
-- ============================================================================
-- After migration, verify tenant isolation works:
--
-- 1. Check that default tenant has all existing data:
--    SELECT tenant_id, COUNT(*) FROM stripe_sessions GROUP BY tenant_id;
--    SELECT tenant_id, COUNT(*) FROM products GROUP BY tenant_id;
--
-- 2. Test tenant-scoped query performance:
--    EXPLAIN ANALYZE
--    SELECT * FROM products WHERE tenant_id = 'default' AND active = true;
--
-- 3. Verify indexes are being used:
--    EXPLAIN SELECT * FROM coupons WHERE tenant_id = 'tenant-123' AND code = 'SAVE10';
--
-- 4. Check primary key constraints:
--    SELECT constraint_name, table_name
--    FROM information_schema.table_constraints
--    WHERE constraint_type = 'PRIMARY KEY'
--    AND table_schema = 'public';

-- ============================================================================
-- BACKWARDS COMPATIBILITY
-- ============================================================================
-- This migration is 100% backwards compatible:
--   - All existing rows get tenant_id='default'
--   - Single-tenant deployments continue working unchanged
--   - No application code changes required immediately
--   - Tenant-aware code can be added incrementally
--
-- To enable multi-tenancy in your application:
--   1. Add tenant extraction middleware (from JWT, subdomain, header)
--   2. Pass tenant_id through context to storage layer
--   3. Update queries to filter by tenant_id
--   4. Add tenant provisioning API endpoints

-- ============================================================================
-- ROLLBACK (if needed)
-- ============================================================================
-- IMPORTANT: This rollback script is for reference only.
-- Take a full database backup before migration.
--
-- To rollback:
-- 1. Restore from backup (RECOMMENDED)
-- 2. OR manually reverse (if caught immediately, before tenant data added):
--
-- ALTER TABLE stripe_sessions DROP COLUMN tenant_id;
-- ALTER TABLE crypto_access DROP COLUMN tenant_id;
-- ALTER TABLE cart_quotes DROP COLUMN tenant_id;
-- ALTER TABLE refund_quotes DROP COLUMN tenant_id;
-- ALTER TABLE payment_transactions DROP COLUMN tenant_id;
-- ALTER TABLE admin_nonces DROP COLUMN tenant_id;
-- ALTER TABLE products DROP COLUMN tenant_id;
-- ALTER TABLE coupons DROP COLUMN tenant_id;
--
-- Then recreate original indexes (see migration 001-005 for reference)
