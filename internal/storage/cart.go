package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/CedrosPay/server/internal/money"
)

// ErrCartExpired is returned when attempting to use an expired cart quote.
var ErrCartExpired = errors.New("storage: cart quote expired")

// CartItem represents a single item in a cart quote.
type CartItem struct {
	ResourceID string            // Resource ID from paywall config
	Quantity   int64             // Number of this item
	Price      money.Money       // Price per unit (locked at quote time)
	Metadata   map[string]string // Per-item custom metadata
}

// CartQuote represents a temporary cart with locked prices and expiration.
type CartQuote struct {
	ID           string            // Unique cart ID (cart_abc123...)
	Items        []CartItem        // All items in the cart
	Total        money.Money       // Total price (sum of all items)
	Metadata     map[string]string // Cart-level metadata (user_id, campaign, etc.)
	CreatedAt    time.Time         // When quote was generated
	ExpiresAt    time.Time         // When quote becomes invalid
	WalletPaidBy string            // Set after payment verification (for idempotency)
}

// IsExpiredAt returns true if the cart quote has passed its expiration time at the given moment.
func (c *CartQuote) IsExpiredAt(now time.Time) bool {
	return now.After(c.ExpiresAt)
}

// GenerateCartID creates a cryptographically random cart identifier.
func GenerateCartID() (string, error) {
	b := make([]byte, 16) // 128 bits of randomness
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate cart id: %w", err)
	}
	return "cart_" + hex.EncodeToString(b), nil
}

// SaveCartQuote stores a cart quote with automatic expiration.
func (m *MemoryStore) SaveCartQuote(_ context.Context, quote CartQuote) error {
	if err := validateAndPrepareCartQuote(&quote, 0); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cartQuotes == nil {
		m.cartQuotes = make(map[string]CartQuote)
	}
	m.cartQuotes[quote.ID] = quote
	return nil
}

// GetCartQuote retrieves a cart quote by ID.
// Returns ErrNotFound if cart doesn't exist, ErrCartExpired if expired.
func (m *MemoryStore) GetCartQuote(_ context.Context, cartID string) (CartQuote, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	quote, ok := m.cartQuotes[cartID]
	if !ok {
		return CartQuote{}, ErrNotFound
	}
	now := time.Now()
	if quote.IsExpiredAt(now) {
		return CartQuote{}, ErrCartExpired
	}
	return quote, nil
}

// MarkCartPaid records the wallet that paid for a cart (for idempotency).
func (m *MemoryStore) MarkCartPaid(_ context.Context, cartID, wallet string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	quote, ok := m.cartQuotes[cartID]
	if !ok {
		return ErrNotFound
	}
	quote.WalletPaidBy = wallet
	m.cartQuotes[cartID] = quote
	return nil
}

// HasCartAccess checks if a wallet has already paid for a cart.
func (m *MemoryStore) HasCartAccess(_ context.Context, cartID, wallet string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	quote, ok := m.cartQuotes[cartID]
	if !ok {
		return false
	}
	return quote.WalletPaidBy == wallet
}

// SaveCartQuotes stores multiple cart quotes in a single operation.
// All quotes are validated before any are stored (atomic batch).
func (m *MemoryStore) SaveCartQuotes(_ context.Context, quotes []CartQuote) error {
	if len(quotes) == 0 {
		return nil // No-op for empty batch
	}

	// Validate all quotes first (fail fast before modifying state)
	for i := range quotes {
		if err := validateAndPrepareCartQuote(&quotes[i], 0); err != nil {
			return fmt.Errorf("quote %d: %w", i, err)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cartQuotes == nil {
		m.cartQuotes = make(map[string]CartQuote)
	}

	// Store all quotes (already validated)
	for _, quote := range quotes {
		m.cartQuotes[quote.ID] = quote
	}

	return nil
}

// GetCartQuotes retrieves multiple cart quotes by ID.
// Returns a slice with found quotes - missing or expired carts are skipped (partial results).
func (m *MemoryStore) GetCartQuotes(_ context.Context, cartIDs []string) ([]CartQuote, error) {
	if len(cartIDs) == 0 {
		return []CartQuote{}, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	quotes := make([]CartQuote, 0, len(cartIDs))

	for _, cartID := range cartIDs {
		if quote, ok := m.cartQuotes[cartID]; ok && !quote.IsExpiredAt(now) {
			quotes = append(quotes, quote)
		}
	}

	return quotes, nil
}

// removeExpiredCarts deletes all expired cart quotes.
func (m *MemoryStore) removeExpiredCarts() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for cartID, quote := range m.cartQuotes {
		if quote.IsExpiredAt(now) {
			delete(m.cartQuotes, cartID)
		}
	}
}
