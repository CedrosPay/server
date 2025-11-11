package dbpool

import (
	"database/sql"
	"fmt"

	"github.com/CedrosPay/server/internal/config"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// SharedPool manages a single shared PostgreSQL connection pool.
// Multiple repositories and stores can use the same pool to reduce connection overhead.
type SharedPool struct {
	db *sql.DB
}

// NewSharedPool creates a new shared PostgreSQL connection pool.
func NewSharedPool(connectionString string, poolConfig config.PostgresPoolConfig) (*SharedPool, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	// Apply connection pool settings from config
	config.ApplyPostgresPoolSettings(db, poolConfig)

	return &SharedPool{db: db}, nil
}

// DB returns the underlying *sql.DB for use by repositories.
func (p *SharedPool) DB() *sql.DB {
	return p.db
}

// Close closes the shared connection pool.
// This should only be called once when the application shuts down.
// sql.DB.Close() is safe to call multiple times.
func (p *SharedPool) Close() error {
	return p.db.Close()
}
