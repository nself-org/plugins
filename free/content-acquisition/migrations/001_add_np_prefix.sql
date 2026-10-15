-- Migration 001: add the np_contentacquisition_ prefix to the content-acquisition tables (guarded renames only).
-- Old names: acquisition_*. New names: np_contentacquisition_acquisition_*.
-- Guarded, idempotent, no transaction control (sdk/go/migrate refuses it and applies each file itself):
--   * a table is renamed only when the old name exists in public AND the new name is free;
--   * when both names exist (the plugin's own schema bootstrap already created the new one), both are left untouched;
--   * the queue sequence is renamed only when the old queue table is gone (it was renamed), the old sequence exists in public and the new name is free.
-- A fresh database has none of the old tables, so this file is a no-op there; the tables are created by the plugin at start.

DO $$
DECLARE
  t RECORD;
BEGIN
  FOR t IN
    SELECT old_name, new_name FROM (VALUES
      ('acquisition_queue', 'np_contentacquisition_acquisition_queue')
      , ('acquisition_history', 'np_contentacquisition_acquisition_history')
      , ('acquisition_rules', 'np_contentacquisition_acquisition_rules')
    ) AS v(old_name, new_name)
  LOOP
    IF to_regclass('public.' || quote_ident(t.old_name)) IS NOT NULL
       AND to_regclass('public.' || quote_ident(t.new_name)) IS NULL THEN
      EXECUTE format('ALTER TABLE public.%I RENAME TO %I', t.old_name, t.new_name);
    END IF;
  END LOOP;

  IF to_regclass('public.acquisition_queue') IS NULL
     AND to_regclass('public.acquisition_queue_id_seq') IS NOT NULL
     AND to_regclass('public.np_contentacquisition_acquisition_queue_id_seq') IS NULL THEN
    ALTER SEQUENCE public.acquisition_queue_id_seq RENAME TO np_contentacquisition_acquisition_queue_id_seq;
  END IF;
END $$;
