package lifecycle

import (
	"io"
	"sync"

	"github.com/rs/zerolog/log"
)

// Manager handles graceful cleanup of resources with error aggregation.
// This consolidates the defer Close() pattern used across cmd/server/main.go and pkg/cedros/app.go.
type Manager struct {
	mu        sync.Mutex
	resources []resource
}

type resource struct {
	name   string
	closer io.Closer
}

// NewManager creates a new resource lifecycle manager.
func NewManager() *Manager {
	return &Manager{
		resources: make([]resource, 0),
	}
}

// Register adds a resource to be closed when the manager is closed.
// Resources are closed in reverse order of registration (LIFO).
func (m *Manager) Register(name string, closer io.Closer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources = append(m.resources, resource{name: name, closer: closer})
}

// RegisterFunc wraps a cleanup function as a Closer for convenience.
func (m *Manager) RegisterFunc(name string, fn func() error) {
	m.Register(name, closerFunc(fn))
}

// Close closes all registered resources in reverse order.
// It aggregates all errors and logs them, returning the first error encountered.
// This ensures all cleanup attempts are made even if some fail.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	// Close in reverse order (LIFO - last registered, first closed)
	for i := len(m.resources) - 1; i >= 0; i-- {
		res := m.resources[i]
		if err := res.closer.Close(); err != nil {
			log.Error().
				Err(err).
				Str("resource", res.name).
				Msg("lifecycle.close_resource_failed")
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// closerFunc adapts a function to the io.Closer interface.
type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
}
