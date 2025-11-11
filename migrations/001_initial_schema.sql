-- Initial schema for cedros-pay-server PostgreSQL storage
-- This migration creates all required tables for persistent storage
--
-- IMPORTANT: This schema uses DOUBLE PRECISION for amounts, which is DEPRECATED.
-- After running this migration, immediately run migration 005 to convert to atomic units (BIGINT).
-- The DOUBLE PRECISION columns will cause floating-point precision issues in production.

-- Stripe checkout sessions
CREATE TABLE IF NOT EXISTS stripe_sessions (
    id TEXT PRIMARY KEY,
    resource_id TEXT NOT NULL,
    status TEXT NOT NULL,
    amount_cents BIGINT NOT NULL,
    currency TEXT NOT NULL,
    customer_email TEXT,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_stripe_sessions_resource ON stripe_sessions(resource_id);
CREATE INDEX IF NOT EXISTS idx_stripe_sessions_status ON stripe_sessions(status);

-- Crypto payment access records
CREATE TABLE IF NOT EXISTS crypto_access (
    resource_id TEXT NOT NULL,
    wallet TEXT NOT NULL,
    signature TEXT NOT NULL,
    granted_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    PRIMARY KEY (resource_id, wallet)
);

CREATE INDEX IF NOT EXISTS idx_crypto_access_expires ON crypto_access(expires_at);

-- Cart quotes for multi-item checkouts
-- WARNING: total_amount uses DOUBLE PRECISION (DEPRECATED - precision loss)
-- Migration 005 converts to: total_amount BIGINT + total_asset TEXT
CREATE TABLE IF NOT EXISTS cart_quotes (
    id TEXT PRIMARY KEY,
    items JSONB NOT NULL,
    total_amount DOUBLE PRECISION NOT NULL,  -- DEPRECATED: Use migration 005 to convert to BIGINT
    token TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    wallet_paid_by TEXT
);

CREATE INDEX IF NOT EXISTS idx_cart_quotes_expires ON cart_quotes(expires_at);

-- Refund quotes
-- WARNING: amount uses DOUBLE PRECISION (DEPRECATED - precision loss)
-- Migration 005 converts to: amount BIGINT + amount_asset TEXT
CREATE TABLE IF NOT EXISTS refund_quotes (
    id TEXT PRIMARY KEY,
    original_purchase_id TEXT NOT NULL,
    recipient_wallet TEXT NOT NULL,
    amount DOUBLE PRECISION NOT NULL,  -- DEPRECATED: Use migration 005 to convert to BIGINT
    token TEXT NOT NULL,
    token_mint TEXT NOT NULL,
    token_decimals SMALLINT NOT NULL,
    reason TEXT,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    processed_by TEXT,
    processed_at TIMESTAMP,
    signature TEXT
);

CREATE INDEX IF NOT EXISTS idx_refund_quotes_expires ON refund_quotes(expires_at);
CREATE INDEX IF NOT EXISTS idx_refund_quotes_processed ON refund_quotes(processed_at) WHERE processed_at IS NOT NULL;
