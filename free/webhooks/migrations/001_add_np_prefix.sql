-- Migration 001: add the np_webhooks_ prefix to the webhooks tables (guarded renames only).
-- Renames the tables (and their indexes) an older release created without the np_ prefix.
-- Old names: webhook_*. New names: np_webhooks_*.
-- Guarded, idempotent, no transaction control (sdk/go/migrate refuses it, and applies each file itself):
--   * a table is renamed only when the old name exists in public AND the new name is free;
--   * when both names exist, both are left untouched (nothing merged, nothing dropped); the plugin log reports it;
--   * an index is renamed only when it sits on a renamed np_ table and the new index name is free;
--   * the dead-letter foreign keys to deliveries and endpoints become ON DELETE SET NULL (see the second block).
-- A fresh database has none of the old tables, so this file is a no-op there; the tables themselves are created by the next migration file.

DO $$
DECLARE
  t RECORD;
  i RECORD;
  new_idx TEXT;
BEGIN
  FOR t IN
    SELECT old_name, new_name FROM (VALUES
      ('webhook_endpoints', 'np_webhooks_endpoints')
      , ('webhook_deliveries', 'np_webhooks_deliveries')
      , ('webhook_event_types', 'np_webhooks_event_types')
      , ('webhook_dead_letters', 'np_webhooks_dead_letters')
    ) AS v(old_name, new_name)
  LOOP
    IF to_regclass('public.' || quote_ident(t.old_name)) IS NOT NULL
       AND to_regclass('public.' || quote_ident(t.new_name)) IS NULL THEN
      EXECUTE format('ALTER TABLE public.%I RENAME TO %I', t.old_name, t.new_name);
    END IF;
  END LOOP;

  FOR i IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND tablename IN ('np_webhooks_endpoints', 'np_webhooks_deliveries', 'np_webhooks_event_types', 'np_webhooks_dead_letters')
      AND starts_with(indexname, 'idx_webhook_')
  LOOP
    new_idx := 'idx_np_webhooks_' || substr(i.indexname, length('idx_webhook_') + 1);
    IF to_regclass('public.' || quote_ident(new_idx)) IS NULL THEN
      EXECUTE format('ALTER INDEX public.%I RENAME TO %I', i.indexname, new_idx);
    END IF;
  END LOOP;
END $$;

-- The TS-era dead-letter table references deliveries and endpoints with the default NO ACTION, which would make deleting an
-- endpoint (cascading to its deliveries) fail once dead letters exist. Replace each such foreign key, found by the table it
-- references whatever its name, with the same key ON DELETE SET NULL. Data is kept. Idempotent: a key already SET NULL (or any
-- other explicit ON DELETE rule) is skipped, and nothing happens when the dead-letter table does not exist.
DO $$
DECLARE
  c RECORD;
BEGIN
  IF to_regclass('public.np_webhooks_dead_letters') IS NULL
     OR to_regclass('public.np_webhooks_deliveries') IS NULL
     OR to_regclass('public.np_webhooks_endpoints') IS NULL THEN
    RETURN;
  END IF;
  FOR c IN
    SELECT conname, pg_get_constraintdef(oid) AS def
    FROM pg_constraint
    WHERE conrelid = 'public.np_webhooks_dead_letters'::regclass
      AND contype = 'f'
      AND confrelid IN ('public.np_webhooks_deliveries'::regclass, 'public.np_webhooks_endpoints'::regclass)
      AND confdeltype = 'a'
  LOOP
    EXECUTE format('ALTER TABLE public.np_webhooks_dead_letters DROP CONSTRAINT %I', c.conname);
    EXECUTE format('ALTER TABLE public.np_webhooks_dead_letters ADD CONSTRAINT %I %s ON DELETE SET NULL', c.conname, c.def);
  END LOOP;
END $$;
