# Database Migrations

This directory contains database migration scripts for setting up persistent storage backends.

## Storage Backends

Cedros Pay Server supports four storage backends:

1. **Memory** (default) - In-memory storage, data is lost on restart
2. **PostgreSQL** - Persistent relational database storage
3. **MongoDB** - Persistent document database storage
4. **File** - JSON file-based persistent storage

## Migrations

**001_initial_schema**: Core storage (cart quotes, refund quotes) - **DEPRECATED tables removed in latest version**
**002_add_products_coupons**: Products and coupons for database-backed catalogs (required for `product_source: postgres|mongodb` or `coupon_source: postgres|mongodb`)

## PostgreSQL Setup

### Prerequisites
- PostgreSQL 12 or higher
- Database user with CREATE TABLE permissions

### Running Migrations

```bash
# Run all migrations in order
psql "postgresql://user:password@localhost:5432/cedros_pay" < migrations/001_initial_schema.sql
psql "postgresql://user:password@localhost:5432/cedros_pay" < migrations/002_add_products_coupons.sql
```

Or interactively:

```bash
psql "postgresql://user:password@localhost:5432/cedros_pay"

# Run the migration scripts
\i migrations/001_initial_schema.sql
\i migrations/002_add_products_coupons.sql
```

### Configuration

Update your `configs/local.yaml`:

```yaml
storage:
  backend: "postgres"
  postgres_url: "postgresql://user:password@localhost:5432/cedros_pay?sslmode=disable"
```

## MongoDB Setup

### Prerequisites
- MongoDB 4.4 or higher
- Database user with read/write permissions

### Running Migrations

```bash
# Run all migrations in order
mongosh "mongodb://localhost:27017" < migrations/001_initial_schema.js
mongosh "mongodb://localhost:27017" < migrations/002_add_products_coupons.js

# Or connect first, then run
mongosh "mongodb://localhost:27017"
> load("migrations/001_initial_schema.js")
> load("migrations/002_add_products_coupons.js")
```

### Configuration

Update your `configs/local.yaml`:

```yaml
storage:
  backend: "mongodb"
  mongodb_url: "mongodb://localhost:27017"
  mongodb_database: "cedros_pay"
```

## File-Based Storage

File-based storage doesn't require any migration scripts. The JSON file is created automatically on first write.

### Configuration

Update your `configs/local.yaml`:

```yaml
storage:
  backend: "file"
  file_path: "./data/cedros-pay-storage.json"
```

Make sure the directory exists:

```bash
mkdir -p ./data
```

## Schema Overview

All storage backends maintain the same logical schema:

### stripe_sessions
Tracks Stripe checkout sessions and their completion status.

- `id` (TEXT/String, PRIMARY KEY) - Stripe session ID
- `resource_id` (TEXT/String) - Product/resource ID
- `status` (TEXT/String) - Session status
- `amount_cents` (BIGINT/Number) - Amount in cents
- `currency` (TEXT/String) - Currency code
- `customer_email` (TEXT/String, optional) - Customer email
- `metadata` (JSONB/Object, optional) - Custom metadata
- `created_at` (TIMESTAMP/Date) - Creation timestamp
- `completed_at` (TIMESTAMP/Date, optional) - Completion timestamp

### crypto_access
Records successful x402 crypto payments for access control.

- `resource_id` (TEXT/String) - Product/resource ID
- `wallet` (TEXT/String) - Customer wallet address
- `signature` (TEXT/String) - Transaction signature
- `granted_at` (TIMESTAMP/Date) - When access was granted
- `expires_at` (TIMESTAMP/Date) - When access expires

**Primary Key**: (resource_id, wallet)

### cart_quotes
Multi-item checkout quotes for batch purchases.

- `cart_id` (TEXT/String, PRIMARY KEY) - Unique cart identifier
- `items` (JSONB/Array) - Array of cart items
- `total_amount` (DOUBLE PRECISION/Number) - Total amount in tokens
- `token` (TEXT/String) - Token symbol (e.g., "USDC")
- `token_mint` (TEXT/String) - Token mint address
- `token_decimals` (SMALLINT/Number) - Token decimal places
- `metadata` (JSONB/Object, optional) - Custom metadata
- `created_at` (TIMESTAMP/Date) - Creation timestamp
- `expires_at` (TIMESTAMP/Date) - Expiration timestamp
- `paid_by` (TEXT/String, optional) - Wallet that paid
- `paid_at` (TIMESTAMP/Date, optional) - Payment timestamp

### refund_quotes
Refund request quotes (persist across restarts).

- `id` (TEXT/String, PRIMARY KEY) - Unique refund identifier
- `original_purchase_id` (TEXT/String) - Original purchase reference
- `recipient_wallet` (TEXT/String) - Wallet to receive refund
- `amount` (DOUBLE PRECISION/Number) - Refund amount in tokens
- `token` (TEXT/String) - Token symbol
- `token_mint` (TEXT/String) - Token mint address
- `token_decimals` (SMALLINT/Number) - Token decimal places
- `reason` (TEXT/String, optional) - Refund reason
- `metadata` (JSONB/Object, optional) - Custom metadata
- `created_at` (TIMESTAMP/Date) - Creation timestamp
- `expires_at` (TIMESTAMP/Date) - Expiration timestamp
- `processed_by` (TEXT/String, optional) - Server wallet that processed refund
- `processed_at` (TIMESTAMP/Date, optional) - Processing timestamp
- `signature` (TEXT/String, optional) - Transaction signature

## Automatic Cleanup

All storage backends include automatic cleanup of expired records:

- **PostgreSQL/MongoDB**: Background cleanup runs every 5 minutes (configurable in code)
- **File**: Background cleanup runs every 5 minutes (configurable in code)
- **Memory**: Background cleanup runs every 1 minute

Expired records are automatically removed:
- `crypto_access`: Records past `expires_at`
- `cart_quotes`: Quotes past `expires_at`
- `refund_quotes`: Unprocessed quotes past `expires_at` (processed refunds are kept)

## Migration Notes

- The PostgreSQL script uses `CREATE TABLE IF NOT EXISTS`, so it's safe to run multiple times
- MongoDB script creates collections and indexes idempotently
- All timestamps are stored in UTC
- JSONB fields in PostgreSQL allow efficient querying of nested data
- MongoDB uses native BSON types for flexible schema

## Troubleshooting

### PostgreSQL Connection Issues

```bash
# Test connection
psql "postgresql://user:password@localhost:5432/cedros_pay" -c "SELECT version();"

# Check if tables exist
psql "postgresql://user:password@localhost:5432/cedros_pay" -c "\dt"
```

### MongoDB Connection Issues

```bash
# Test connection
mongosh "mongodb://localhost:27017" --eval "db.version()"

# List collections
mongosh "mongodb://localhost:27017/cedros_pay" --eval "db.getCollectionNames()"
```

### File Permission Issues

```bash
# Ensure directory is writable
ls -la ./data/
chmod 755 ./data/
```
