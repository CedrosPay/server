-- Migration 005: Switch from float64 to int64 atomic units
-- This migration updates all money-related columns to use atomic units (BIGINT) instead of DOUBLE PRECISION
--
-- Rationale:
--   - Eliminate floating-point precision issues in financial calculations
--   - Align with Money type system using int64 atomic units
--   - Match backend representations: cents (USD), micro-USDC (USDC), lamports (SOL)
--
-- Migration Strategy:
--   1. Add new BIGINT columns for atomic amounts
--   2. Add asset/currency columns to track which asset is stored
--   3. Migrate existing data (multiply by appropriate decimal places)
--   4. Drop old DOUBLE PRECISION columns
--   5. Rename new columns to original names
--
-- IMPORTANT: This is a BREAKING migration. Run during maintenance window.

-- ============================================================================
-- CART QUOTES TABLE
-- ============================================================================
-- Old: total_amount DOUBLE PRECISION (e.g., 10.5 = 10.5 USDC)
-- New: total_amount_atomic BIGINT + total_asset TEXT
--
-- NOTE: Assuming all existing cart quotes are in USDC (6 decimals)
-- If you have mixed currencies, manual data migration required

ALTER TABLE cart_quotes
    ADD COLUMN total_amount_atomic BIGINT,
    ADD COLUMN total_asset TEXT DEFAULT 'USDC';

-- Migrate existing data: multiply by 10^6 for USDC
-- Example: 10.5 USDC → 10500000 atomic units
UPDATE cart_quotes
SET total_amount_atomic = FLOOR(total_amount * 1000000)::BIGINT
WHERE total_amount_atomic IS NULL;

-- Make new columns required
ALTER TABLE cart_quotes
    ALTER COLUMN total_amount_atomic SET NOT NULL,
    ALTER COLUMN total_asset SET NOT NULL,
    ALTER COLUMN total_asset DROP DEFAULT;

-- Drop old column and rename
ALTER TABLE cart_quotes DROP COLUMN total_amount;
ALTER TABLE cart_quotes RENAME COLUMN total_amount_atomic TO total_amount;

-- ============================================================================
-- REFUND QUOTES TABLE
-- ============================================================================
-- Old: amount DOUBLE PRECISION (e.g., 5.0 = 5.0 of token)
-- New: amount BIGINT + amount_asset TEXT
--
-- Also adds amount_asset column to match Money type pattern
-- The 'token' field will be used to populate amount_asset
-- Common decimals: 6 (USDC/USDT), 9 (SOL)

ALTER TABLE refund_quotes
    ADD COLUMN amount_atomic BIGINT,
    ADD COLUMN amount_asset TEXT;

-- Migrate existing data using token_decimals
-- Example: amount=5.0, token_decimals=6 → 5000000 atomic units
UPDATE refund_quotes
SET amount_atomic = FLOOR(amount * POW(10, token_decimals))::BIGINT,
    amount_asset = UPPER(token)
WHERE amount_atomic IS NULL;

-- Make new columns required
ALTER TABLE refund_quotes
    ALTER COLUMN amount_atomic SET NOT NULL,
    ALTER COLUMN amount_asset SET NOT NULL;

-- Drop old column and rename
ALTER TABLE refund_quotes DROP COLUMN amount;
ALTER TABLE refund_quotes RENAME COLUMN amount_atomic TO amount;

-- ============================================================================
-- PAYMENT TRANSACTIONS TABLE
-- ============================================================================
-- Old: amount DOUBLE PRECISION
-- New: amount_atomic BIGINT + asset TEXT
--
-- NOTE: If you can determine token from existing 'token' field, use that.
-- Otherwise, assume USDC (6 decimals) for existing rows.

ALTER TABLE payment_transactions
    ADD COLUMN amount_atomic BIGINT,
    ADD COLUMN asset TEXT;

-- Migrate existing data
-- If 'token' field contains asset code, use it; otherwise default to USDC
-- This example assumes USDC for all existing records
UPDATE payment_transactions
SET amount_atomic = FLOOR(amount * 1000000)::BIGINT,
    asset = COALESCE(NULLIF(token, ''), 'USDC')
WHERE amount_atomic IS NULL;

-- Make new columns required
ALTER TABLE payment_transactions
    ALTER COLUMN amount_atomic SET NOT NULL,
    ALTER COLUMN asset SET NOT NULL;

-- Drop old column and rename
ALTER TABLE payment_transactions DROP COLUMN amount;
ALTER TABLE payment_transactions RENAME COLUMN amount_atomic TO amount;

-- ============================================================================
-- PRODUCTS TABLE
-- ============================================================================
-- Old: fiat_amount DOUBLE PRECISION (dollars), crypto_amount DOUBLE PRECISION
-- New: fiat_amount_atomic BIGINT + fiat_asset TEXT,
--      crypto_amount_atomic BIGINT + crypto_asset TEXT

-- Add new columns
ALTER TABLE products
    ADD COLUMN fiat_amount_atomic BIGINT,
    ADD COLUMN fiat_asset TEXT,
    ADD COLUMN crypto_amount_atomic BIGINT,
    ADD COLUMN crypto_asset TEXT;

-- Migrate fiat amounts
-- Assumes fiat_currency='usd' → multiply by 100 for cents
-- Example: 1.99 USD → 199 cents
UPDATE products
SET fiat_amount_atomic = FLOOR(fiat_amount * 100)::BIGINT,
    fiat_asset = UPPER(fiat_currency)
WHERE fiat_amount_atomic IS NULL;

-- Migrate crypto amounts
-- Assumes crypto_token is USDC (6 decimals) for existing records
-- If you have SOL or other tokens, adjust accordingly
UPDATE products
SET crypto_amount_atomic = CASE
        WHEN crypto_amount IS NOT NULL THEN FLOOR(crypto_amount * 1000000)::BIGINT
        ELSE NULL
    END,
    crypto_asset = CASE
        WHEN crypto_token IS NOT NULL THEN UPPER(crypto_token)
        ELSE NULL
    END
WHERE crypto_amount_atomic IS NULL;

-- Make fiat columns required (crypto can be NULL)
ALTER TABLE products
    ALTER COLUMN fiat_amount_atomic SET NOT NULL,
    ALTER COLUMN fiat_asset SET NOT NULL;

-- Drop old columns and rename
ALTER TABLE products
    DROP COLUMN fiat_amount,
    DROP COLUMN fiat_currency,
    DROP COLUMN crypto_amount,
    DROP COLUMN crypto_token;

ALTER TABLE products
    RENAME COLUMN fiat_amount_atomic TO fiat_amount;
ALTER TABLE products
    RENAME COLUMN fiat_asset TO fiat_currency;
ALTER TABLE products
    RENAME COLUMN crypto_amount_atomic TO crypto_amount;
ALTER TABLE products
    RENAME COLUMN crypto_asset TO crypto_token;

-- ============================================================================
-- COUPONS TABLE
-- ============================================================================
-- Old: discount_value DOUBLE PRECISION (can be percentage or fixed amount)
-- New: discount_value_atomic BIGINT
--
-- Migration strategy:
--   - For 'percentage' type: multiply by 100 to get basis points (e.g., 10% → 1000 bp)
--   - For 'fixed' type: multiply by appropriate decimals based on currency
--     - USD → multiply by 100 (cents)
--     - USDC/crypto → multiply by 10^6

ALTER TABLE coupons
    ADD COLUMN discount_value_atomic BIGINT;

-- Migrate percentage discounts: convert to basis points
-- Example: 10.5% → 1050 basis points
-- Example: 25% → 2500 basis points
UPDATE coupons
SET discount_value_atomic = FLOOR(discount_value * 100)::BIGINT
WHERE discount_type = 'percentage' AND discount_value_atomic IS NULL;

-- Migrate fixed discounts: assume USD cents for currency='' or 'usd'
-- For other currencies, adjust multiplier accordingly
UPDATE coupons
SET discount_value_atomic = FLOOR(discount_value * 100)::BIGINT
WHERE discount_type = 'fixed'
    AND (currency = '' OR LOWER(currency) = 'usd')
    AND discount_value_atomic IS NULL;

-- If you have fixed USDC discounts, uncomment this:
-- UPDATE coupons
-- SET discount_value_atomic = FLOOR(discount_value * 1000000)::BIGINT
-- WHERE discount_type = 'fixed'
--     AND LOWER(currency) = 'usdc'
--     AND discount_value_atomic IS NULL;

-- Make new column required
ALTER TABLE coupons
    ALTER COLUMN discount_value_atomic SET NOT NULL;

-- Drop old column and rename
ALTER TABLE coupons DROP COLUMN discount_value;
ALTER TABLE coupons RENAME COLUMN discount_value_atomic TO discount_value;

-- Update constraint to use BIGINT
ALTER TABLE coupons DROP CONSTRAINT IF EXISTS coupons_discount_value_check;
ALTER TABLE coupons ADD CONSTRAINT coupons_discount_value_check CHECK (discount_value >= 0);

-- ============================================================================
-- VERIFICATION
-- ============================================================================
-- After migration, verify data integrity:
--
-- 1. Check cart quotes:
--    SELECT id, total_amount, total_asset FROM cart_quotes LIMIT 10;
--
-- 2. Check refund quotes:
--    SELECT id, amount, token, token_decimals FROM refund_quotes LIMIT 10;
--
-- 3. Check products:
--    SELECT id, fiat_amount, fiat_currency, crypto_amount, crypto_token FROM products LIMIT 10;
--
-- 4. Check coupons:
--    SELECT code, discount_type, discount_value, currency FROM coupons LIMIT 10;

-- ============================================================================
-- ROLLBACK (if needed - RUN BEFORE migration if you want safety net)
-- ============================================================================
-- IMPORTANT: This rollback script is provided for reference only.
-- In a production environment, take a full database backup before migration.
-- Do NOT run this script - it's just documentation of reverse operations.
--
-- To rollback:
-- 1. Restore from backup (RECOMMENDED)
-- 2. OR manually reverse using these statements (DATA LOSS - precision lost):
--
-- ALTER TABLE cart_quotes ADD COLUMN total_amount DOUBLE PRECISION;
-- UPDATE cart_quotes SET total_amount = total_amount_atomic / 1000000.0;
-- ... (similar for other tables)
