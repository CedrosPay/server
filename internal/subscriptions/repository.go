package subscriptions

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Common errors returned by repository operations.
var (
	ErrNotFound           = errors.New("subscription not found")
	ErrAlreadyExists      = errors.New("subscription already exists")
	ErrInvalidSubscription = errors.New("invalid subscription data")
)

// Repository defines the interface for subscription storage.
type Repository interface {
	// Create stores a new subscription.
	Create(ctx context.Context, sub Subscription) error

	// Get retrieves a subscription by ID.
	Get(ctx context.Context, id string) (Subscription, error)

	// Update modifies an existing subscription.
	Update(ctx context.Context, sub Subscription) error

	// Delete removes a subscription (soft delete by setting status to cancelled).
	Delete(ctx context.Context, id string) error

	// GetByWallet finds an active subscription for a wallet and product.
	GetByWallet(ctx context.Context, wallet, productID string) (Subscription, error)

	// GetByStripeSubscriptionID finds a subscription by Stripe subscription ID.
	GetByStripeSubscriptionID(ctx context.Context, stripeSubID string) (Subscription, error)

	// GetByStripeCustomerID finds all subscriptions for a Stripe customer.
	GetByStripeCustomerID(ctx context.Context, customerID string) ([]Subscription, error)

	// ListByProduct returns all subscriptions for a product.
	ListByProduct(ctx context.Context, productID string) ([]Subscription, error)

	// ListActive returns all active subscriptions, optionally filtered by product.
	ListActive(ctx context.Context, productID string) ([]Subscription, error)

	// ListExpiring returns subscriptions expiring before the given time.
	ListExpiring(ctx context.Context, before time.Time) ([]Subscription, error)

	// UpdateStatus changes a subscription's status.
	UpdateStatus(ctx context.Context, id string, status Status) error

	// ExtendPeriod updates the current period end time (for renewals).
	ExtendPeriod(ctx context.Context, id string, newStart, newEnd time.Time) error

	// Close releases any resources held by the repository.
	Close() error
}

// RepositoryConfig holds configuration for creating a repository.
type RepositoryConfig struct {
	Backend         string         // "memory" or "postgres"
	PostgresURL     string         // Connection string for postgres
	PostgresDB      *sql.DB        // Optional shared database connection
	TableName       string         // Custom table name (default: "subscriptions")
	GracePeriodHours int           // Hours after expiry before blocking access
}

// NewRepository creates a repository based on configuration.
func NewRepository(cfg RepositoryConfig) (Repository, error) {
	return NewRepositoryWithDB(cfg, nil)
}

// NewRepositoryWithDB creates a repository with an optional shared database connection.
func NewRepositoryWithDB(cfg RepositoryConfig, sharedDB *sql.DB) (Repository, error) {
	switch cfg.Backend {
	case "memory", "":
		return NewMemoryRepository(), nil
	case "postgres":
		if sharedDB != nil {
			repo := NewPostgresRepositoryWithDB(sharedDB)
			if cfg.TableName != "" {
				repo = repo.WithTableName(cfg.TableName)
			}
			return repo, nil
		}
		if cfg.PostgresURL == "" {
			return nil, errors.New("postgres_url required for postgres backend")
		}
		repo, err := NewPostgresRepository(cfg.PostgresURL)
		if err != nil {
			return nil, err
		}
		if cfg.TableName != "" {
			repo = repo.WithTableName(cfg.TableName)
		}
		return repo, nil
	default:
		return nil, errors.New("unknown subscription repository backend: " + cfg.Backend)
	}
}
