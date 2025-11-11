## Multi-Tenancy Support

This package implements database-level multi-tenancy for Cedros Pay Server, enabling SaaS/platform use cases like Shopify integrations, WooCommerce plugins, and enterprise deployments.

## Why Multi-Tenancy?

**The Problem:** Without multi-tenancy support, adding `tenant_id` to billions of rows later = downtime nightmare + migration pain.

**The Solution:** Add `tenant_id` columns NOW (while tables are small) with backwards-compatible defaults.

## Architecture

### Database-Level Isolation

All tables include `tenant_id` column with tenant-scoped indexes:

```sql
CREATE TABLE products (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'default',
    ...
);

CREATE INDEX idx_products_tenant_active ON products(tenant_id, active);
```

### Backwards Compatibility

- **Default Tenant:** All existing data uses `tenant_id='default'`
- **Single-Tenant Mode:** Works without any changes (uses default tenant)
- **Migration Path:** Enable multi-tenancy when needed without data migration

## Usage

### Single-Tenant Deployment (Default)

No changes required. All data automatically uses `tenant_id='default'`:

```go
// Existing code continues to work unchanged
store.SaveCartQuote(ctx, quote)  // Uses default tenant
```

### Multi-Tenant Deployment

Enable tenant extraction middleware in server configuration:

```go
// In server.go ConfigureRouter
router.Use(tenant.Extraction)  // Extract tenant from request
```

### Tenant Extraction Methods

The middleware extracts tenant ID using multiple methods (in priority order):

#### Method 1: X-Tenant-ID Header (Recommended for API clients)

```bash
curl -H "X-Tenant-ID: acme-corp" https://api.cedrospay.com/products
```

#### Method 2: Subdomain (Recommended for web apps)

```bash
# Subdomain automatically extracted
https://acme-corp.api.cedrospay.com/products → tenant_id="acme-corp"
```

#### Method 3: JWT Claims (Recommended for auth-based routing)

```go
// Auth middleware sets tenant from JWT
claims := parseJWT(token)
ctx = tenant.WithTenant(ctx, claims.TenantID)
```

### Accessing Tenant in Code

```go
func (h *handlers) someHandler(w http.ResponseWriter, r *http.Request) {
    tenantID := tenant.FromContext(r.Context())
    // Use tenantID for tenant-scoped queries
}
```

## Enterprise Use Cases

### 1. Shopify App Integration

```
Shopify Store → shopify_store_12345.api.cedrospay.com
Each store gets isolated tenant with own:
- Products catalog
- Payment records
- Coupons
- Stripe connected account
```

### 2. WooCommerce Plugin

```
WooCommerce Site → X-Tenant-ID: woo_site_abc123
Plugin sends tenant ID with each API request
Merchant dashboard shows only their tenant's data
```

### 3. White-Label SaaS

```
Customer A → customer-a.payments.example.com
Customer B → customer-b.payments.example.com
Each customer sees isolated payment data
```

## Security & Compliance

### Database-Level Isolation

All queries automatically scoped by tenant:

```sql
-- Single-tenant query (old)
SELECT * FROM products WHERE active = true;

-- Multi-tenant query (new)
SELECT * FROM products WHERE tenant_id = 'acme-corp' AND active = true;
```

### GDPR Compliance

- **Data Deletion:** Delete all data for specific tenant
- **Data Export:** Export all tenant data for portability
- **Isolation:** Tenant data never mixes with other tenants

### PCI-DSS Compliance

- **Tenant Segregation:** Payment data isolated per tenant
- **Access Control:** Tenant can only access their own payment records

## Migration Guide

### Step 1: Run Database Migration

```bash
# Apply migration 006 to add tenant_id columns
psql -d cedros_pay < migrations/006_add_multi_tenancy.sql
```

**Important:** This migration is BACKWARDS COMPATIBLE. All existing data gets `tenant_id='default'`.

### Step 2: Enable Tenant Middleware (Optional)

For multi-tenant deployments, add middleware:

```go
router.Use(tenant.Extraction)
```

For single-tenant deployments, NO CODE CHANGES needed.

### Step 3: Add Tenant-Aware Queries (When Needed)

Update storage layer to filter by tenant:

```go
// Before (single-tenant)
func (r *ProductRepository) ListProducts(ctx context.Context) ([]Product, error) {
    query := "SELECT * FROM products WHERE active = true"
    // ...
}

// After (multi-tenant aware)
func (r *ProductRepository) ListProducts(ctx context.Context) ([]Product, error) {
    tenantID := tenant.FromContext(ctx)
    query := "SELECT * FROM products WHERE tenant_id = $1 AND active = true"
    // ...
}
```

## Performance Considerations

### Composite Indexes

All tenant queries use composite indexes for optimal performance:

```sql
-- Efficiently handles tenant-scoped queries
CREATE INDEX idx_products_tenant_active ON products(tenant_id, active);
```

### Query Planning

PostgreSQL query planner uses tenant indexes automatically:

```sql
EXPLAIN SELECT * FROM products WHERE tenant_id = 'acme' AND active = true;
-- Uses: idx_products_tenant_active
```

## Future Enhancements

### Tenant Management Table (Planned)

```sql
CREATE TABLE tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    stripe_account_id TEXT,    -- Connected Stripe account
    solana_wallet TEXT,          -- Tenant's payment wallet
    rate_limits JSONB,           -- Per-tenant rate limits
    features JSONB,              -- Feature flags per tenant
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL
);
```

### Tenant Provisioning API (Planned)

```bash
POST /admin/tenants
{
  "id": "acme-corp",
  "name": "Acme Corporation",
  "stripeAccountId": "acct_xxx",
  "solanaWallet": "xxx..."
}
```

### Tenant-Specific Settings (Planned)

```go
type TenantSettings struct {
    RateLimits      RateLimitSettings
    Features        FeatureFlags
    StripeAccountID string
    SolanaWallet    string
}
```

## Testing

Run tenant isolation tests:

```bash
go test ./internal/tenant -v
```

Verify tenant index performance:

```sql
EXPLAIN ANALYZE
SELECT * FROM products WHERE tenant_id = 'test' AND active = true;
```

## Troubleshooting

### Issue: Tenant not being extracted

**Solution:** Ensure tenant extraction middleware is enabled:

```go
router.Use(tenant.Extraction)
```

### Issue: Cross-tenant data leakage

**Solution:** Verify all queries filter by tenant_id:

```sql
-- ❌ Wrong: Missing tenant filter
SELECT * FROM products WHERE active = true;

-- ✅ Correct: Includes tenant filter
SELECT * FROM products WHERE tenant_id = $1 AND active = true;
```

### Issue: Performance degradation

**Solution:** Ensure tenant indexes exist:

```sql
\di+ idx_products_tenant_active  -- Check index exists
EXPLAIN SELECT ...               -- Verify index is used
```

## Standards & Best Practices

- **Tenant ID Format:** Alphanumeric + hyphens/underscores only (max 64 chars)
- **Default Tenant:** Always use `'default'` for single-tenant mode
- **Context Propagation:** Always pass tenant via context, never global variables
- **Index Strategy:** Composite indexes with tenant_id as first column
