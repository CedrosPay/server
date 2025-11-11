-- Migration 008: Fix webhook_queue table schema
-- This migration adds missing columns to existing webhook_queue tables
--
-- Purpose: Ensure webhook_queue table has all required columns for persistent webhook delivery
-- Context: Earlier versions may have created webhook_queue with incomplete schema

-- Add missing payload column (required for webhook body)
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Ensure all required columns exist with proper defaults
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS headers JSONB;
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS event_type TEXT NOT NULL DEFAULT 'payment';
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS max_attempts INTEGER NOT NULL DEFAULT 5;
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS last_error TEXT;
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMP;
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMP NOT NULL DEFAULT NOW();
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS created_at TIMESTAMP NOT NULL DEFAULT NOW();
ALTER TABLE webhook_queue ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP;

-- Remove default constraints after adding columns (we don't want defaults on new inserts)
ALTER TABLE webhook_queue ALTER COLUMN payload DROP DEFAULT;
ALTER TABLE webhook_queue ALTER COLUMN event_type DROP DEFAULT;
ALTER TABLE webhook_queue ALTER COLUMN status DROP DEFAULT;
ALTER TABLE webhook_queue ALTER COLUMN next_attempt_at DROP DEFAULT;
ALTER TABLE webhook_queue ALTER COLUMN created_at DROP DEFAULT;

-- Create indexes if they don't exist
CREATE INDEX IF NOT EXISTS idx_webhook_queue_pending ON webhook_queue(status, next_attempt_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_webhook_queue_status ON webhook_queue(status);
CREATE INDEX IF NOT EXISTS idx_webhook_queue_created ON webhook_queue(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_webhook_queue_completed ON webhook_queue(completed_at) WHERE completed_at IS NOT NULL;
