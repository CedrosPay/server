-- Migration 007: Add persistent webhook queue
-- This migration adds the webhook_queue table for persistent webhook delivery across server restarts.
--
-- Purpose: Ensure webhook delivery reliability even during server crashes/restarts.
-- Before: Webhooks delivered via in-memory goroutines (lost on restart)
-- After: Webhooks persisted to database with retry logic

-- Webhook queue for persistent delivery
CREATE TABLE IF NOT EXISTS webhook_queue (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    payload JSONB NOT NULL,
    headers JSONB,
    event_type TEXT NOT NULL,  -- 'payment' or 'refund'
    status TEXT NOT NULL,      -- 'pending', 'processing', 'failed', 'success'
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    last_error TEXT,
    last_attempt_at TIMESTAMP,
    next_attempt_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP
);

-- Index for efficient webhook polling (workers query: status=pending AND next_attempt_at <= NOW())
CREATE INDEX IF NOT EXISTS idx_webhook_queue_pending ON webhook_queue(status, next_attempt_at)
    WHERE status = 'pending';

-- Index for admin UI filtering by status
CREATE INDEX IF NOT EXISTS idx_webhook_queue_status ON webhook_queue(status);

-- Index for admin UI listing (newest first)
CREATE INDEX IF NOT EXISTS idx_webhook_queue_created ON webhook_queue(created_at DESC);

-- Index for cleanup jobs (remove old completed webhooks)
CREATE INDEX IF NOT EXISTS idx_webhook_queue_completed ON webhook_queue(completed_at)
    WHERE completed_at IS NOT NULL;
