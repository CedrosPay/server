package callbacks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// NoopDLQStore is a DLQ store that discards all failed webhooks.
type NoopDLQStore struct{}

func (NoopDLQStore) SaveFailedWebhook(context.Context, FailedWebhook) error { return nil }
func (NoopDLQStore) ListFailedWebhooks(context.Context, int) ([]FailedWebhook, error) {
	return []FailedWebhook{}, nil
}
func (NoopDLQStore) DeleteFailedWebhook(context.Context, string) error { return nil }

// MemoryDLQStore stores failed webhooks in memory (for testing/development).
type MemoryDLQStore struct {
	mu       sync.RWMutex
	webhooks map[string]FailedWebhook
}

// NewMemoryDLQStore creates an in-memory DLQ store.
func NewMemoryDLQStore() *MemoryDLQStore {
	return &MemoryDLQStore{
		webhooks: make(map[string]FailedWebhook),
	}
}

func (m *MemoryDLQStore) SaveFailedWebhook(ctx context.Context, webhook FailedWebhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhooks[webhook.ID] = webhook
	return nil
}

func (m *MemoryDLQStore) ListFailedWebhooks(ctx context.Context, limit int) ([]FailedWebhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]FailedWebhook, 0, len(m.webhooks))
	for _, webhook := range m.webhooks {
		result = append(result, webhook)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *MemoryDLQStore) DeleteFailedWebhook(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.webhooks, id)
	return nil
}

// FileDLQStore stores failed webhooks in a JSON file.
type FileDLQStore struct {
	mu       sync.RWMutex
	filePath string
	webhooks map[string]FailedWebhook
}

// NewFileDLQStore creates a file-based DLQ store.
func NewFileDLQStore(filePath string) (*FileDLQStore, error) {
	store := &FileDLQStore{
		filePath: filePath,
		webhooks: make(map[string]FailedWebhook),
	}

	// Load existing data if file exists
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load DLQ file: %w", err)
	}

	return store, nil
}

func (f *FileDLQStore) SaveFailedWebhook(ctx context.Context, webhook FailedWebhook) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.webhooks[webhook.ID] = webhook
	return f.persist()
}

func (f *FileDLQStore) ListFailedWebhooks(ctx context.Context, limit int) ([]FailedWebhook, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make([]FailedWebhook, 0, len(f.webhooks))
	for _, webhook := range f.webhooks {
		result = append(result, webhook)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (f *FileDLQStore) DeleteFailedWebhook(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.webhooks, id)
	return f.persist()
}

func (f *FileDLQStore) load() error {
	data, err := os.ReadFile(f.filePath)
	if err != nil {
		return err
	}

	var webhooks map[string]FailedWebhook
	if err := json.Unmarshal(data, &webhooks); err != nil {
		return fmt.Errorf("unmarshal DLQ data: %w", err)
	}

	f.webhooks = webhooks
	return nil
}

func (f *FileDLQStore) persist() error {
	data, err := json.MarshalIndent(f.webhooks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal DLQ data: %w", err)
	}

	// Write to temp file first, then rename (atomic operation)
	tmpPath := f.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write DLQ file: %w", err)
	}

	if err := os.Rename(tmpPath, f.filePath); err != nil {
		os.Remove(tmpPath) // Clean up temp file on error
		return fmt.Errorf("rename DLQ file: %w", err)
	}

	return nil
}

// Close ensures all data is persisted (no-op for file store).
func (f *FileDLQStore) Close() error {
	return nil
}
