package storage

import (
	"testing"
	"time"
)

func TestMemoryStore_Stop(t *testing.T) {
	store := NewMemoryStore()

	// Stop should not hang
	done := make(chan struct{})
	go func() {
		store.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() timed out")
	}
}

func TestPtrTime(t *testing.T) {
	now := time.Now()
	ptr := ptrTime(now)

	if ptr == nil {
		t.Fatal("ptrTime() returned nil")
	}
	if !ptr.Equal(now) {
		t.Errorf("ptrTime() = %v, want %v", *ptr, now)
	}
}
