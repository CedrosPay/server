package storage

import "time"

const (
	// CleanupInterval is how often the cleanup goroutine runs to remove expired records.
	CleanupInterval = 1 * time.Hour
)
