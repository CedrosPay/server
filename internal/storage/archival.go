package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/CedrosPay/server/internal/metrics"
	"github.com/rs/zerolog"
)

// ArchivalConfig holds configuration for automatic payment signature archival.
type ArchivalConfig struct {
	Enabled         bool          // Enable automatic archival (default: false)
	RetentionPeriod time.Duration // How long to keep payment signatures (default: 90 days)
	RunInterval     time.Duration // How often to run archival (default: 24 hours)
}

// DefaultArchivalConfig returns sensible defaults for signature archival.
func DefaultArchivalConfig() ArchivalConfig {
	return ArchivalConfig{
		Enabled:         false,
		RetentionPeriod: 90 * 24 * time.Hour, // 90 days
		RunInterval:     24 * time.Hour,      // Daily
	}
}

// ArchivalService automatically archives old payment signatures on a schedule.
type ArchivalService struct {
	store    Store
	config   ArchivalConfig
	logger   zerolog.Logger
	metrics  *metrics.Metrics
	stopChan chan struct{}
	doneChan chan struct{}
}

// NewArchivalService creates a new archival service.
func NewArchivalService(store Store, config ArchivalConfig, metricsCollector *metrics.Metrics, logger zerolog.Logger) *ArchivalService {
	return &ArchivalService{
		store:    store,
		config:   config,
		logger:   logger,
		metrics:  metricsCollector,
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
}

// Start begins the archival service background loop.
func (s *ArchivalService) Start() {
	if !s.config.Enabled {
		s.logger.Info().Msg("archival: service disabled")
		close(s.doneChan)
		return
	}

	s.logger.Info().
		Dur("retentionPeriod", s.config.RetentionPeriod).
		Dur("runInterval", s.config.RunInterval).
		Msg("archival: service started")

	go s.run()
}

// Stop gracefully stops the archival service.
func (s *ArchivalService) Stop() {
	close(s.stopChan)
	<-s.doneChan
	s.logger.Info().Msg("archival: service stopped")
}

// run is the main archival loop.
func (s *ArchivalService) run() {
	defer close(s.doneChan)

	// Run immediately on startup
	s.runArchival()

	ticker := time.NewTicker(s.config.RunInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runArchival()
		case <-s.stopChan:
			return
		}
	}
}

// runArchival performs a single archival pass.
func (s *ArchivalService) runArchival() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cutoffTime := time.Now().Add(-s.config.RetentionPeriod)

	s.logger.Info().
		Time("cutoffTime", cutoffTime).
		Msg("archival: starting archival pass")

	// Archive old payment signatures
	paymentCount, err := s.store.ArchiveOldPayments(ctx, cutoffTime)
	if err != nil {
		s.logger.Error().Err(err).Msg("archival: failed to archive old payments")
	} else if paymentCount > 0 {
		s.logger.Info().
			Int64("count", paymentCount).
			Time("olderThan", cutoffTime).
			Msg("archival: archived old payment signatures")
	}

	// Cleanup expired nonces
	nonceCount, err := s.store.CleanupExpiredNonces(ctx)
	if err != nil {
		s.logger.Error().Err(err).Msg("archival: failed to cleanup expired nonces")
	} else if nonceCount > 0 {
		s.logger.Info().
			Int64("count", nonceCount).
			Msg("archival: cleaned up expired nonces")
	}

	// Record archival metrics
	totalRecords := paymentCount + nonceCount
	if s.metrics != nil && totalRecords > 0 {
		s.metrics.ObserveArchival(totalRecords)
	}

	s.logger.Info().
		Int64("paymentsArchived", paymentCount).
		Int64("noncesDeleted", nonceCount).
		Msg("archival: archival pass completed")
}

// RunNow immediately runs an archival pass (useful for testing or manual triggers).
func (s *ArchivalService) RunNow() error {
	if !s.config.Enabled {
		return fmt.Errorf("archival service is disabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cutoffTime := time.Now().Add(-s.config.RetentionPeriod)

	paymentCount, err := s.store.ArchiveOldPayments(ctx, cutoffTime)
	if err != nil {
		return fmt.Errorf("archive old payments: %w", err)
	}

	nonceCount, err := s.store.CleanupExpiredNonces(ctx)
	if err != nil {
		return fmt.Errorf("cleanup expired nonces: %w", err)
	}

	// Record archival metrics
	totalRecords := paymentCount + nonceCount
	if s.metrics != nil && totalRecords > 0 {
		s.metrics.ObserveArchival(totalRecords)
	}

	s.logger.Info().
		Int64("paymentsArchived", paymentCount).
		Int64("noncesDeleted", nonceCount).
		Msg("archival: manual archival completed")

	return nil
}
