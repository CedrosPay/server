# Schema Mapping Guide

Cedros Pay allows you to integrate with existing database schemas by providing a flexible mapping configuration. This is useful when you already have product catalogs, coupon systems, or payment tracking tables that don't exactly match Cedros Pay's expected structure.

## Quick Start

1. **Create a schema mapping file**:
   ```bash
   cp configs/schema-mapping.example.yaml configs/schema-mapping.yaml
   ```

2. **Configure your environment**:
   ```bash
   SCHEMA_MAPPING_FILE=./configs/schema-mapping.yaml
   ```

3. **Customize the mapping** to match your database schema.

## When to Use Schema Mapping

Use schema mapping when:

- ✅ You have an existing products table with different column names
- ✅ Your prices are stored in cents instead of dollars
- ✅ You need to avoid table name conflicts
- ✅ Your data structure includes nested JSON fields
- ✅ You want to use database views or complex queries

## Basic Example: Rename Columns

If your products table uses different column names:

```yaml
products:
  table_name: "products"
  field_map:
    id: "product_id"        # Your column → Cedros field
    description: "name"
    fiat_amount: "price"
    active: "is_available"
```

## Example: Price in Cents

If prices are stored in cents:

```yaml
products:
  field_map:
    fiat_amount: "price_cents"
  transforms:
    fiat_amount:
      type: "divide"
      factor: 100  # Convert cents to dollars
```

## Example: Nested JSON

If price is nested in a JSON column:

```yaml
products:
  field_map:
    fiat_amount: "pricing_json"
  transforms:
    fiat_amount:
      type: "json_field"
      field: "usd_price"  # Extract pricing_json->>'usd_price'
```

## Example: Custom SQL

For complex schemas requiring joins or views:

```yaml
products:
  custom_sql:
    select: |
      SELECT
        p.id,
        p.name as description,
        (p.price_cents::float / 100) as fiat_amount,
        p.currency as fiat_currency,
        s.stripe_price_id,
        p.is_active as active
      FROM products p
      LEFT JOIN stripe_integrations s ON s.product_id = p.id
      WHERE p.id = $1
    list_active: |
      SELECT * FROM products WHERE is_active = true AND deleted_at IS NULL
```

## Example: Avoid Table Conflicts

If "payment_signatures" already exists:

```yaml
payment_tracking:
  payment_signatures:
    table_name: "cedros_payment_sigs"
  admin_nonces:
    table_name: "cedros_admin_nonces"
```

## Field Transformations

Supported transformation types:

| Type | Description | Example |
|------|-------------|---------|
| `divide` | Divide by factor | Convert cents to dollars |
| `multiply` | Multiply by factor | Convert dollars to cents |
| `json_field` | Extract JSON field | Get nested value |
| `expression` | Custom SQL | Complex calculations |
| `constant` | Return constant | Default all to same value |

## Default Values

If not specified, Cedros Pay uses these defaults:

### Products
- Table: `products`
- Columns: `id`, `description`, `fiat_amount`, `crypto_amount`, etc.

### Coupons
- Table: `coupons`
- Columns: `code`, `discount_type`, `discount_value`, etc.

### Payment Tracking
- Tables: `cart_quotes`, `refund_quotes`, `payment_signatures`, `admin_nonces`

## Complete Reference

See `configs/schema-mapping.example.yaml` for all available options and detailed examples.

## Database Support

| Database | Status |
|----------|--------|
| PostgreSQL | ✅ Fully supported |
| MongoDB | 🚧 Coming soon |
| MySQL | 🔜 Planned |

## Migration from Hardcoded Schema

If you're already using Cedros Pay with the default schema, you don't need to do anything. Schema mapping is optional and only needed when integrating with existing databases.

## Troubleshooting

**Q: My SELECT query returns NULL for a field**

A: Check that your column name in `field_map` exactly matches your database column, including case sensitivity.

**Q: Prices are still wrong after applying transform**

A: Ensure the transform type is correct (`divide` for cents→dollars, `multiply` for dollars→cents).

**Q: Can I use database views?**

A: Yes! Set `table_name` to your view name. Make sure the view is updatable if you need INSERT/UPDATE operations.

**Q: How do I test my mapping?**

A: Start the server and check logs for SQL queries. Use `LOG_LEVEL=debug` to see generated SQL.

## Best Practices

1. **Start simple**: Only map fields that differ from defaults
2. **Test queries**: Verify SQL with `\set ON_ERROR_STOP on` in psql
3. **Use transforms carefully**: Prefer database-side transforms (views) when possible
4. **Document custom SQL**: Add comments explaining complex queries
5. **Version control**: Commit your schema mapping file

## Need Help?

- 📖 Full example: `configs/schema-mapping.example.yaml`
- 🐛 Report issues: https://github.com/anthropics/cedros-pay/issues
- 💬 Ask questions: Discord community
