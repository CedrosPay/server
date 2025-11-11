// Initial schema for cedros-pay-server MongoDB storage
// Run this script with: mongosh <your-mongodb-url> < migrations/001_initial_schema.js

// Switch to the cedros_pay database (adjust the name as needed)
use cedros_pay;

// Create stripe_sessions collection and indexes
db.createCollection("stripe_sessions");
db.stripe_sessions.createIndex({ "id": 1 }, { unique: true });
db.stripe_sessions.createIndex({ "resource_id": 1 });
db.stripe_sessions.createIndex({ "status": 1 });

print("Created stripe_sessions collection with indexes");

// Create crypto_access collection and indexes
db.createCollection("crypto_access");
db.crypto_access.createIndex({ "resource_id": 1, "wallet": 1 }, { unique: true });
db.crypto_access.createIndex({ "expires_at": 1 });

print("Created crypto_access collection with indexes");

// Create cart_quotes collection and indexes
db.createCollection("cart_quotes");
db.cart_quotes.createIndex({ "_id": 1 }, { unique: true });
db.cart_quotes.createIndex({ "expiresat": 1 });

print("Created cart_quotes collection with indexes");

// Create refund_quotes collection and indexes
db.createCollection("refund_quotes");
db.refund_quotes.createIndex({ "id": 1 }, { unique: true });
db.refund_quotes.createIndex({ "expires_at": 1 });
db.refund_quotes.createIndex({ "processed_at": 1 });

print("Created refund_quotes collection with indexes");

print("\nMongoDB schema setup complete!");
print("Collections created: stripe_sessions, crypto_access, cart_quotes, refund_quotes");
