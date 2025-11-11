-- Migration 003: Add applies_at column to coupons table
-- This migration adds support for two-phase coupon system (catalog vs checkout)

-- Add applies_at column with default empty string (backward compatible)
ALTER TABLE coupons
ADD COLUMN IF NOT EXISTS applies_at TEXT NOT NULL DEFAULT ''
CHECK (applies_at IN ('', 'catalog', 'checkout'));

-- Add index for efficient filtering by applies_at
CREATE INDEX IF NOT EXISTS idx_coupons_applies_at ON coupons(applies_at) WHERE applies_at != '';

-- Add comment explaining the column
COMMENT ON COLUMN coupons.applies_at IS 'Controls when coupon is displayed: "catalog" for product pages (must be specific), "checkout" for cart/checkout (must be all), empty for backward compatibility';
