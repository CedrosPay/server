package schema

import (
	"fmt"
	"strings"
)

// PostgresMapper generates SQL queries using schema mappings.
type PostgresMapper struct {
	mapping *Mapping
}

// NewPostgresMapper creates a new Postgres schema mapper.
func NewPostgresMapper(mapping *Mapping) *PostgresMapper {
	return &PostgresMapper{mapping: mapping}
}

// ProductQueries returns SQL queries for product operations.
func (m *PostgresMapper) ProductQueries() *ProductSQLQueries {
	pm := m.mapping.Products
	fm := pm.FieldMap

	return &ProductSQLQueries{
		TableName:             pm.TableName,
		SelectOne:             m.buildProductSelectQuery(fm, fmt.Sprintf("WHERE %s = $1", fm.ID)),
		SelectByStripePriceID: m.buildProductSelectQuery(fm, fmt.Sprintf("WHERE %s = $1", fm.StripePriceID)),
		ListActive:            m.buildProductSelectQuery(fm, fmt.Sprintf("WHERE %s = true", fm.Active)),
		Insert:                m.buildProductInsertQuery(fm),
		Update:                m.buildProductUpdateQuery(fm),
		Delete: fmt.Sprintf("UPDATE %s SET %s = false, %s = NOW() WHERE %s = $1",
			pm.TableName, fm.Active, fm.UpdatedAt, fm.ID),
		ColumnMap: fm,
	}
}

// CouponQueries returns SQL queries for coupon operations.
func (m *PostgresMapper) CouponQueries() *CouponSQLQueries {
	cm := m.mapping.Coupons
	fm := cm.FieldMap

	return &CouponSQLQueries{
		TableName:  cm.TableName,
		SelectOne:  m.buildCouponSelectQuery(fm, fmt.Sprintf("WHERE %s = $1", fm.Code)),
		ListActive: m.buildCouponSelectQuery(fm, fmt.Sprintf("WHERE %s = true", fm.Active)),
		SelectAutoApply: m.buildCouponSelectQuery(fm, fmt.Sprintf(
			"WHERE %s = true AND %s = true AND (%s = $1 OR %s = 'all') AND (%s = $2 OR %s = '')",
			fm.Active, fm.AutoApply, fm.Scope, fm.Scope, fm.PaymentMethod, fm.PaymentMethod)),
		Insert: m.buildCouponInsertQuery(fm),
		Update: m.buildCouponUpdateQuery(fm),
		IncrementUsage: fmt.Sprintf("UPDATE %s SET %s = %s + 1, %s = NOW() WHERE %s = $1",
			cm.TableName, fm.UsageCount, fm.UsageCount, fm.UpdatedAt, fm.Code),
		Delete: fmt.Sprintf("UPDATE %s SET %s = false, %s = NOW() WHERE %s = $1",
			cm.TableName, fm.Active, fm.UpdatedAt, fm.Code),
		ColumnMap: fm,
	}
}

// PaymentTrackingQueries returns table names for payment tracking.
func (m *PostgresMapper) PaymentTrackingQueries() *PaymentTrackingSQLQueries {
	pt := m.mapping.PaymentTracking

	return &PaymentTrackingSQLQueries{
		CartQuotesTable:        pt.CartQuotes.TableName,
		RefundQuotesTable:      pt.RefundQuotes.TableName,
		PaymentSignaturesTable: pt.PaymentSignatures.TableName,
		AdminNoncesTable:       pt.AdminNonces.TableName,
	}
}

// buildProductSelectQuery builds a SELECT query for products.
func (m *PostgresMapper) buildProductSelectQuery(fm ProductFieldMap, whereClause string) string {
	// Check for custom SQL
	if m.mapping.Products.CustomSQL != nil && m.mapping.Products.CustomSQL.Select != "" {
		return m.mapping.Products.CustomSQL.Select
	}

	columns := []string{
		m.applyTransform("id", fm.ID),
		m.applyTransform("description", fm.Description),
		m.applyTransform("fiat_amount", fm.FiatAmount),
		m.applyTransform("fiat_currency", fm.FiatCurrency),
		m.applyTransform("stripe_price_id", fm.StripePriceID),
		m.applyTransform("crypto_amount", fm.CryptoAmount),
		m.applyTransform("crypto_token", fm.CryptoToken),
		m.applyTransform("crypto_account", fm.CryptoAccount),
		m.applyTransform("memo_template", fm.MemoTemplate),
		fmt.Sprintf("%s as metadata", fm.Metadata),
		fmt.Sprintf("%s as active", fm.Active),
		fmt.Sprintf("%s as created_at", fm.CreatedAt),
		fmt.Sprintf("%s as updated_at", fm.UpdatedAt),
	}

	return fmt.Sprintf("SELECT %s FROM %s %s",
		strings.Join(columns, ", "),
		m.mapping.Products.TableName,
		whereClause)
}

// buildProductInsertQuery builds an INSERT query for products.
func (m *PostgresMapper) buildProductInsertQuery(fm ProductFieldMap) string {
	if m.mapping.Products.CustomSQL != nil && m.mapping.Products.CustomSQL.Insert != "" {
		return m.mapping.Products.CustomSQL.Insert
	}

	columns := []string{
		fm.ID, fm.Description, fm.FiatAmount, fm.FiatCurrency, fm.StripePriceID,
		fm.CryptoAmount, fm.CryptoToken, fm.CryptoAccount, fm.MemoTemplate,
		fm.Metadata, fm.Active, fm.CreatedAt, fm.UpdatedAt,
	}

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		m.mapping.Products.TableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))
}

// buildProductUpdateQuery builds an UPDATE query for products.
func (m *PostgresMapper) buildProductUpdateQuery(fm ProductFieldMap) string {
	if m.mapping.Products.CustomSQL != nil && m.mapping.Products.CustomSQL.Update != "" {
		return m.mapping.Products.CustomSQL.Update
	}

	updates := []string{
		fmt.Sprintf("%s = $2", fm.Description),
		fmt.Sprintf("%s = $3", fm.FiatAmount),
		fmt.Sprintf("%s = $4", fm.FiatCurrency),
		fmt.Sprintf("%s = $5", fm.StripePriceID),
		fmt.Sprintf("%s = $6", fm.CryptoAmount),
		fmt.Sprintf("%s = $7", fm.CryptoToken),
		fmt.Sprintf("%s = $8", fm.CryptoAccount),
		fmt.Sprintf("%s = $9", fm.MemoTemplate),
		fmt.Sprintf("%s = $10", fm.Metadata),
		fmt.Sprintf("%s = $11", fm.Active),
		fmt.Sprintf("%s = NOW()", fm.UpdatedAt),
	}

	return fmt.Sprintf("UPDATE %s SET %s WHERE %s = $1",
		m.mapping.Products.TableName,
		strings.Join(updates, ", "),
		fm.ID)
}

// buildCouponSelectQuery builds a SELECT query for coupons.
func (m *PostgresMapper) buildCouponSelectQuery(fm CouponFieldMap, whereClause string) string {
	if m.mapping.Coupons.CustomSQL != nil && m.mapping.Coupons.CustomSQL.Select != "" {
		return m.mapping.Coupons.CustomSQL.Select
	}

	columns := []string{
		fmt.Sprintf("%s as code", fm.Code),
		m.applyTransform("discount_type", fm.DiscountType),
		m.applyTransform("discount_value", fm.DiscountValue),
		fmt.Sprintf("%s as currency", fm.Currency),
		fmt.Sprintf("%s as scope", fm.Scope),
		fmt.Sprintf("%s as product_ids", fm.ProductIDs),
		fmt.Sprintf("%s as payment_method", fm.PaymentMethod),
		fmt.Sprintf("%s as auto_apply", fm.AutoApply),
		fmt.Sprintf("%s as applies_at", fm.AppliesAt),
		fmt.Sprintf("%s as usage_limit", fm.UsageLimit),
		fmt.Sprintf("%s as usage_count", fm.UsageCount),
		fmt.Sprintf("%s as starts_at", fm.StartsAt),
		fmt.Sprintf("%s as expires_at", fm.ExpiresAt),
		fmt.Sprintf("%s as active", fm.Active),
		fmt.Sprintf("%s as metadata", fm.Metadata),
		fmt.Sprintf("%s as created_at", fm.CreatedAt),
		fmt.Sprintf("%s as updated_at", fm.UpdatedAt),
	}

	return fmt.Sprintf("SELECT %s FROM %s %s",
		strings.Join(columns, ", "),
		m.mapping.Coupons.TableName,
		whereClause)
}

// buildCouponInsertQuery builds an INSERT query for coupons.
func (m *PostgresMapper) buildCouponInsertQuery(fm CouponFieldMap) string {
	if m.mapping.Coupons.CustomSQL != nil && m.mapping.Coupons.CustomSQL.Insert != "" {
		return m.mapping.Coupons.CustomSQL.Insert
	}

	columns := []string{
		fm.Code, fm.DiscountType, fm.DiscountValue, fm.Currency, fm.Scope,
		fm.ProductIDs, fm.PaymentMethod, fm.AutoApply, fm.AppliesAt,
		fm.UsageLimit, fm.UsageCount, fm.StartsAt, fm.ExpiresAt,
		fm.Active, fm.Metadata, fm.CreatedAt, fm.UpdatedAt,
	}

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		m.mapping.Coupons.TableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))
}

// buildCouponUpdateQuery builds an UPDATE query for coupons.
func (m *PostgresMapper) buildCouponUpdateQuery(fm CouponFieldMap) string {
	if m.mapping.Coupons.CustomSQL != nil && m.mapping.Coupons.CustomSQL.Update != "" {
		return m.mapping.Coupons.CustomSQL.Update
	}

	updates := []string{
		fmt.Sprintf("%s = $2", fm.DiscountType),
		fmt.Sprintf("%s = $3", fm.DiscountValue),
		fmt.Sprintf("%s = $4", fm.Currency),
		fmt.Sprintf("%s = $5", fm.Scope),
		fmt.Sprintf("%s = $6", fm.ProductIDs),
		fmt.Sprintf("%s = $7", fm.PaymentMethod),
		fmt.Sprintf("%s = $8", fm.AutoApply),
		fmt.Sprintf("%s = $9", fm.AppliesAt),
		fmt.Sprintf("%s = $10", fm.UsageLimit),
		fmt.Sprintf("%s = $11", fm.UsageCount),
		fmt.Sprintf("%s = $12", fm.StartsAt),
		fmt.Sprintf("%s = $13", fm.ExpiresAt),
		fmt.Sprintf("%s = $14", fm.Active),
		fmt.Sprintf("%s = $15", fm.Metadata),
		fmt.Sprintf("%s = NOW()", fm.UpdatedAt),
	}

	return fmt.Sprintf("UPDATE %s SET %s WHERE %s = $1",
		m.mapping.Coupons.TableName,
		strings.Join(updates, ", "),
		fm.Code)
}

// applyTransform applies a field transformation if configured.
func (m *PostgresMapper) applyTransform(fieldName, columnName string) string {
	// Check products transforms
	if transform, ok := m.mapping.Products.Transforms[fieldName]; ok {
		return m.transformSQL(columnName, transform, fieldName)
	}

	// Check coupons transforms
	if transform, ok := m.mapping.Coupons.Transforms[fieldName]; ok {
		return m.transformSQL(columnName, transform, fieldName)
	}

	// No transform, use column as-is
	return fmt.Sprintf("%s as %s", columnName, fieldName)
}

// transformSQL generates SQL for a transform.
func (m *PostgresMapper) transformSQL(columnName string, t Transform, alias string) string {
	switch t.Type {
	case "multiply":
		return fmt.Sprintf("(%s * %f) as %s", columnName, t.Factor, alias)
	case "divide":
		return fmt.Sprintf("(%s::float / %f) as %s", columnName, t.Factor, alias)
	case "expression":
		// Use custom SQL expression, but alias it
		return fmt.Sprintf("(%s) as %s", t.Expr, alias)
	case "constant":
		// Return constant value
		return fmt.Sprintf("'%v' as %s", t.Value, alias)
	case "json_field":
		// Extract JSON field
		return fmt.Sprintf("%s->>'%s' as %s", columnName, t.Field, alias)
	default:
		// Unknown transform type, use column as-is
		return fmt.Sprintf("%s as %s", columnName, alias)
	}
}

// ProductSQLQueries holds generated SQL queries for products.
type ProductSQLQueries struct {
	TableName             string
	SelectOne             string
	SelectByStripePriceID string
	ListActive            string
	Insert                string
	Update                string
	Delete                string
	ColumnMap             ProductFieldMap
}

// CouponSQLQueries holds generated SQL queries for coupons.
type CouponSQLQueries struct {
	TableName       string
	SelectOne       string
	ListActive      string
	SelectAutoApply string
	Insert          string
	Update          string
	IncrementUsage  string
	Delete          string
	ColumnMap       CouponFieldMap
}

// PaymentTrackingSQLQueries holds table names for payment tracking.
type PaymentTrackingSQLQueries struct {
	CartQuotesTable        string
	RefundQuotesTable      string
	PaymentSignaturesTable string
	AdminNoncesTable       string
}
