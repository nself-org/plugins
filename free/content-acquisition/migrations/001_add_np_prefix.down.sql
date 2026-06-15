-- Rollback: Rename np_ prefix back to old names for content-acquisition tables
-- Reverse of 001_add_np_prefix.sql

BEGIN;

-- Rename sequences back (if they were renamed)
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_sequences WHERE sequencename = 'np_contentacquisition_acquisition_queue_id_seq') THEN
    ALTER SEQUENCE np_contentacquisition_acquisition_queue_id_seq RENAME TO acquisition_queue_id_seq;
  END IF;
END $$;

-- Rename tables back
ALTER TABLE IF EXISTS np_contentacquisition_acquisition_rules RENAME TO acquisition_rules;
ALTER TABLE IF EXISTS np_contentacquisition_acquisition_history RENAME TO acquisition_history;
ALTER TABLE IF EXISTS np_contentacquisition_acquisition_queue RENAME TO acquisition_queue;

COMMIT;
