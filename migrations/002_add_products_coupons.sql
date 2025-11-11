-- Migration 002: Add products and coupons tables
-- This migration adds support for database-backed products and coupons

-- Products table for database-backed product catalog
-- Column names match internal/products/postgres_repository.go
CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    fiat_amount DOUBLE PRECISION NOT NULL,  -- Stored as dollars (e.g., 1.99 = $1.99)
    fiat_currency TEXT NOT NULL DEFAULT 'usd',
    stripe_price_id TEXT,
    crypto_amount DOUBLE PRECISION,
    crypto_token TEXT,
    crypto_account TEXT,
    memo_template TEXT,
    metadata JSONB,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_products_active ON products(active);
CREATE INDEX IF NOT EXISTS idx_products_created ON products(created_at DESC);

-- Coupons table for database-backed coupon management
CREATE TABLE IF NOT EXISTS coupons (
    code TEXT PRIMARY KEY,
    discount_type TEXT NOT NULL CHECK (discount_type IN ('percentage', 'fixed')),
    discount_value DOUBLE PRECISION NOT NULL CHECK (discount_value >= 0),
    currency TEXT NOT NULL DEFAULT '',
    scope TEXT NOT NULL CHECK (scope IN ('all', 'specific')),
    product_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    payment_method TEXT NOT NULL DEFAULT '' CHECK (payment_method IN ('', 'stripe', 'x402')),
    auto_apply BOOLEAN NOT NULL DEFAULT false,
    usage_limit INTEGER,
    usage_count INTEGER NOT NULL DEFAULT 0,
    starts_at TIMESTAMP,
    expires_at TIMESTAMP,
    active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_limit_check CHECK (usage_limit IS NULL OR usage_limit >= 0),
    CONSTRAINT usage_count_check CHECK (usage_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_coupons_active ON coupons(active);
CREATE INDEX IF NOT EXISTS idx_coupons_auto_apply ON coupons(auto_apply) WHERE auto_apply = true;
CREATE INDEX IF NOT EXISTS idx_coupons_expires ON coupons(expires_at);
CREATE INDEX IF NOT EXISTS idx_coupons_created ON coupons(created_at DESC);
-- GIN index for efficient JSON containment queries on product_ids
CREATE INDEX IF NOT EXISTS idx_coupons_product_ids ON coupons USING gin(product_ids);
