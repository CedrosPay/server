package schema

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Mapping defines how to map between user's database schema and Cedros Pay's expected structure.
type Mapping struct {
	Products        ProductMapping         `yaml:"products"`
	Coupons         CouponMapping          `yaml:"coupons"`
	PaymentTracking PaymentTrackingMapping `yaml:"payment_tracking"`
}

// ProductMapping defines the mapping for products/resources table.
type ProductMapping struct {
	TableName  string               `yaml:"table_name"` // Table/collection name (default: "products")
	FieldMap   ProductFieldMap      `yaml:"field_map"`  // Field mappings
	CustomSQL  *CustomSQLQueries    `yaml:"custom_sql"` // Optional custom SQL for complex mappings
	Transforms map[string]Transform `yaml:"transforms"` // Field transformations
}

// ProductFieldMap maps Cedros Pay fields to user's database columns.
type ProductFieldMap struct {
	ID            string `yaml:"id"`              // Default: "id"
	Description   string `yaml:"description"`     // Default: "description"
	FiatAmount    string `yaml:"fiat_amount"`     // Default: "fiat_amount"
	FiatCurrency  string `yaml:"fiat_currency"`   // Default: "fiat_currency"
	StripePriceID string `yaml:"stripe_price_id"` // Default: "stripe_price_id"
	CryptoAmount  string `yaml:"crypto_amount"`   // Default: "crypto_amount"
	CryptoToken   string `yaml:"crypto_token"`    // Default: "crypto_token"
	CryptoAccount string `yaml:"crypto_account"`  // Default: "crypto_account"
	MemoTemplate  string `yaml:"memo_template"`   // Default: "memo_template"
	Metadata      string `yaml:"metadata"`        // Default: "metadata" (JSONB/JSON column)
	Active        string `yaml:"active"`          // Default: "active"
	CreatedAt     string `yaml:"created_at"`      // Default: "created_at"
	UpdatedAt     string `yaml:"updated_at"`      // Default: "updated_at"
}

// CouponMapping defines the mapping for coupons table.
type CouponMapping struct {
	TableName  string               `yaml:"table_name"`
	FieldMap   CouponFieldMap       `yaml:"field_map"`
	CustomSQL  *CustomSQLQueries    `yaml:"custom_sql"`
	Transforms map[string]Transform `yaml:"transforms"`
}

// CouponFieldMap maps Cedros Pay coupon fields to user's database columns.
type CouponFieldMap struct {
	Code          string `yaml:"code"`           // Default: "code"
	DiscountType  string `yaml:"discount_type"`  // Default: "discount_type"
	DiscountValue string `yaml:"discount_value"` // Default: "discount_value"
	Currency      string `yaml:"currency"`       // Default: "currency"
	Scope         string `yaml:"scope"`          // Default: "scope"
	ProductIDs    string `yaml:"product_ids"`    // Default: "product_ids" (array/JSONB)
	PaymentMethod string `yaml:"payment_method"` // Default: "payment_method"
	AutoApply     string `yaml:"auto_apply"`     // Default: "auto_apply"
	AppliesAt     string `yaml:"applies_at"`     // Default: "applies_at"
	UsageLimit    string `yaml:"usage_limit"`    // Default: "usage_limit"
	UsageCount    string `yaml:"usage_count"`    // Default: "usage_count"
	StartsAt      string `yaml:"starts_at"`      // Default: "starts_at"
	ExpiresAt     string `yaml:"expires_at"`     // Default: "expires_at"
	Active        string `yaml:"active"`         // Default: "active"
	Metadata      string `yaml:"metadata"`       // Default: "metadata"
	CreatedAt     string `yaml:"created_at"`     // Default: "created_at"
	UpdatedAt     string `yaml:"updated_at"`     // Default: "updated_at"
}

// PaymentTrackingMapping defines mappings for payment tracking tables.
type PaymentTrackingMapping struct {
	CartQuotes        TableMapping `yaml:"cart_quotes"`
	RefundQuotes      TableMapping `yaml:"refund_quotes"`
	PaymentSignatures TableMapping `yaml:"payment_signatures"`
	AdminNonces       TableMapping `yaml:"admin_nonces"`
}

// TableMapping is a generic table/collection mapping.
type TableMapping struct {
	TableName string            `yaml:"table_name"`
	FieldMap  map[string]string `yaml:"field_map"` // cedros_field -> user_column
}

// CustomSQLQueries allows users to override default SQL with custom queries.
// Useful for complex schemas with joins, views, or computed columns.
type CustomSQLQueries struct {
	Select     string `yaml:"select"`      // Custom SELECT query (use $1, $2 for params)
	Insert     string `yaml:"insert"`      // Custom INSERT query
	Update     string `yaml:"update"`      // Custom UPDATE query
	Delete     string `yaml:"delete"`      // Custom DELETE query
	ListActive string `yaml:"list_active"` // Custom query for listing active records
}

// Transform defines how to transform a field value during read/write.
type Transform struct {
	Type   string      `yaml:"type"`   // "multiply", "divide", "json_field", "constant", "expression"
	Factor float64     `yaml:"factor"` // For multiply/divide
	Field  string      `yaml:"field"`  // For json_field extraction
	Value  interface{} `yaml:"value"`  // For constant transforms
	Expr   string      `yaml:"expr"`   // For SQL expressions (e.g., "CAST(price_cents AS FLOAT) / 100")
}

// LoadMapping loads schema mapping from a YAML file.
func LoadMapping(path string) (*Mapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read schema mapping file: %w", err)
	}

	var mapping Mapping
	if err := yaml.Unmarshal(data, &mapping); err != nil {
		return nil, fmt.Errorf("parse schema mapping: %w", err)
	}

	// Apply defaults
	applyDefaults(&mapping)

	return &mapping, nil
}

// applyDefaults fills in default values for any unspecified fields.
func applyDefaults(m *Mapping) {
	// Products defaults
	if m.Products.TableName == "" {
		m.Products.TableName = "products"
	}
	applyProductFieldDefaults(&m.Products.FieldMap)

	// Coupons defaults
	if m.Coupons.TableName == "" {
		m.Coupons.TableName = "coupons"
	}
	applyCouponFieldDefaults(&m.Coupons.FieldMap)

	// Payment tracking defaults
	applyPaymentTrackingDefaults(&m.PaymentTracking)
}

func applyProductFieldDefaults(fm *ProductFieldMap) {
	if fm.ID == "" {
		fm.ID = "id"
	}
	if fm.Description == "" {
		fm.Description = "description"
	}
	if fm.FiatAmount == "" {
		fm.FiatAmount = "fiat_amount"
	}
	if fm.FiatCurrency == "" {
		fm.FiatCurrency = "fiat_currency"
	}
	if fm.StripePriceID == "" {
		fm.StripePriceID = "stripe_price_id"
	}
	if fm.CryptoAmount == "" {
		fm.CryptoAmount = "crypto_amount"
	}
	if fm.CryptoToken == "" {
		fm.CryptoToken = "crypto_token"
	}
	if fm.CryptoAccount == "" {
		fm.CryptoAccount = "crypto_account"
	}
	if fm.MemoTemplate == "" {
		fm.MemoTemplate = "memo_template"
	}
	if fm.Metadata == "" {
		fm.Metadata = "metadata"
	}
	if fm.Active == "" {
		fm.Active = "active"
	}
	if fm.CreatedAt == "" {
		fm.CreatedAt = "created_at"
	}
	if fm.UpdatedAt == "" {
		fm.UpdatedAt = "updated_at"
	}
}

func applyCouponFieldDefaults(fm *CouponFieldMap) {
	if fm.Code == "" {
		fm.Code = "code"
	}
	if fm.DiscountType == "" {
		fm.DiscountType = "discount_type"
	}
	if fm.DiscountValue == "" {
		fm.DiscountValue = "discount_value"
	}
	if fm.Currency == "" {
		fm.Currency = "currency"
	}
	if fm.Scope == "" {
		fm.Scope = "scope"
	}
	if fm.ProductIDs == "" {
		fm.ProductIDs = "product_ids"
	}
	if fm.PaymentMethod == "" {
		fm.PaymentMethod = "payment_method"
	}
	if fm.AutoApply == "" {
		fm.AutoApply = "auto_apply"
	}
	if fm.AppliesAt == "" {
		fm.AppliesAt = "applies_at"
	}
	if fm.UsageLimit == "" {
		fm.UsageLimit = "usage_limit"
	}
	if fm.UsageCount == "" {
		fm.UsageCount = "usage_count"
	}
	if fm.StartsAt == "" {
		fm.StartsAt = "starts_at"
	}
	if fm.ExpiresAt == "" {
		fm.ExpiresAt = "expires_at"
	}
	if fm.Active == "" {
		fm.Active = "active"
	}
	if fm.Metadata == "" {
		fm.Metadata = "metadata"
	}
	if fm.CreatedAt == "" {
		fm.CreatedAt = "created_at"
	}
	if fm.UpdatedAt == "" {
		fm.UpdatedAt = "updated_at"
	}
}

func applyPaymentTrackingDefaults(pt *PaymentTrackingMapping) {
	if pt.CartQuotes.TableName == "" {
		pt.CartQuotes.TableName = "cart_quotes"
	}
	if pt.RefundQuotes.TableName == "" {
		pt.RefundQuotes.TableName = "refund_quotes"
	}
	if pt.PaymentSignatures.TableName == "" {
		pt.PaymentSignatures.TableName = "payment_signatures"
	}
	if pt.AdminNonces.TableName == "" {
		pt.AdminNonces.TableName = "admin_nonces"
	}
}
