-- Migration: Add np_ prefix to content-acquisition tables
-- Run this on existing installations before upgrading to the latest plugin version.
-- These tables did not have the required np_ prefix in earlier releases.

BEGIN;

ALTER TABLE IF EXISTS acquisition_queue RENAME TO np_contentacquisition_acquisition_queue;
ALTER TABLE IF EXISTS acquisition_history RENAME TO np_contentacquisition_acquisition_history;
ALTER TABLE IF EXISTS acquisition_rules RENAME TO np_contentacquisition_acquisition_rules;

-- Update sequences (if any exist with old names)
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_sequences WHERE sequencename = 'acquisition_queue_id_seq') THEN
    ALTER SEQUENCE acquisition_queue_id_seq RENAME TO np_contentacquisition_acquisition_queue_id_seq;
  END IF;
END $$;

COMMIT;
