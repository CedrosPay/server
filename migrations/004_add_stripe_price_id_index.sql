-- Migration 004: Add index on products.stripe_price_id for performance
-- This migration adds an index to optimize GetProductByStripePriceID queries
-- Fixes N+1 query bug where we previously had to list all products and scan in Go

CREATE INDEX IF NOT EXISTS idx_products_stripe_price_id ON products(stripe_price_id) WHERE stripe_price_id IS NOT NULL;
