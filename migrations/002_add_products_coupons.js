// Migration 002: Add products and coupons collections
// Run this script with: mongosh <your-mongodb-url> < migrations/002_add_products_coupons.js

// Switch to the cedros_pay database (adjust the name as needed)
use cedros_pay;

// Create products collection and indexes
// Field names match internal/products/mongodb_repository.go (camelCase)
db.createCollection("products", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["_id", "description", "fiatAmount", "fiatCurrency", "active"],
      properties: {
        _id: { bsonType: "string" },
        description: { bsonType: "string" },
        fiatAmount: { bsonType: "double", minimum: 0 },  // Stored as dollars (e.g., 1.99 = $1.99)
        fiatCurrency: { bsonType: "string" },
        stripePriceId: { bsonType: ["string", "null"] },
        cryptoAmount: { bsonType: ["double", "null"], minimum: 0 },
        cryptoToken: { bsonType: ["string", "null"] },
        cryptoAccount: { bsonType: ["string", "null"] },
        memoTemplate: { bsonType: ["string", "null"] },
        metadata: { bsonType: ["object", "null"] },
        active: { bsonType: "bool" },
        createdAt: { bsonType: "date" },
        updatedAt: { bsonType: "date" }
      }
    }
  }
});

db.products.createIndex({ "_id": 1 }, { unique: true });
db.products.createIndex({ "active": 1 });
db.products.createIndex({ "createdAt": -1 });

print("Created products collection with indexes");

// Create coupons collection and indexes
// Field names match internal/coupons/mongodb_repository.go (camelCase with _id)
db.createCollection("coupons", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["_id", "discountType", "discountValue", "scope", "active"],
      properties: {
        _id: { bsonType: "string" },
        discountType: { enum: ["percentage", "fixed"] },
        discountValue: { bsonType: "double", minimum: 0 },
        currency: { bsonType: "string" },
        scope: { enum: ["all", "specific"] },
        productIds: { bsonType: "array", items: { bsonType: "string" } },
        paymentMethod: { enum: ["", "stripe", "x402"] },
        autoApply: { bsonType: "bool" },
        usageLimit: { bsonType: ["int", "null"], minimum: 0 },
        usageCount: { bsonType: "int", minimum: 0 },
        startsAt: { bsonType: ["date", "null"] },
        expiresAt: { bsonType: ["date", "null"] },
        active: { bsonType: "bool" },
        metadata: { bsonType: ["object", "null"] },
        createdAt: { bsonType: "date" },
        updatedAt: { bsonType: "date" }
      }
    }
  }
});

db.coupons.createIndex({ "_id": 1 }, { unique: true });
db.coupons.createIndex({ "active": 1 });
db.coupons.createIndex({ "autoApply": 1 });
db.coupons.createIndex({ "expiresAt": 1 });
db.coupons.createIndex({ "createdAt": -1 });

print("Created coupons collection with indexes");

print("\nMongoDB products and coupons schema setup complete!");
print("Collections created: products, coupons");
