-- D-0501: exercised by the boot kit after the TS-era fixture is migrated.
-- Roll back destructive checks so the retained legacy rows remain available.
BEGIN;

DO $$
DECLARE
  t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY['np_webhooks_endpoints', 'np_webhooks_deliveries'] LOOP
    IF NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema = 'public' AND table_name = t AND column_name = 'source_account_id'
        AND data_type = 'text' AND is_nullable = 'NO' AND column_default IS NOT NULL
    ) THEN
      RAISE EXCEPTION '%.source_account_id must be TEXT NOT NULL DEFAULT primary', t;
    END IF;
  END LOOP;
  FOREACH t IN ARRAY ARRAY['np_webhooks_endpoints', 'np_webhooks_deliveries'] LOOP
    IF NOT EXISTS (
      SELECT 1 FROM pg_constraint
      WHERE conrelid = 'public.np_webhooks_dead_letters'::regclass AND contype = 'f'
        AND confrelid = ('public.' || t)::regclass AND confdeltype = 'n'
    ) THEN
      RAISE EXCEPTION 'dead-letter FK to % must use ON DELETE SET NULL', t;
    END IF;
  END LOOP;
END $$;

-- The legacy endpoint has both a delivery and a dead letter. Its deletion must
-- cascade to the delivery while retaining the dead letter with both FKs cleared.
DELETE FROM public.np_webhooks_endpoints WHERE id = '00000000-0000-4000-8000-0000000000c1';
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.np_webhooks_deliveries WHERE id = '00000000-0000-4000-8000-0000000000c2') THEN
    RAISE EXCEPTION 'endpoint deletion did not cascade to its delivery';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM public.np_webhooks_dead_letters
    WHERE id = '00000000-0000-4000-8000-0000000000c4' AND endpoint_id IS NULL AND delivery_id IS NULL
  ) THEN
    RAISE EXCEPTION 'endpoint deletion must retain its dead letter with null references';
  END IF;
END $$;

-- Account IDs longer than the TS-era VARCHAR(128) must work after normalization.
INSERT INTO public.np_webhooks_endpoints (id, source_account_id, url, secret, events)
VALUES ('00000000-0000-4000-8000-0000000000d1', repeat('a', 129), 'https://example.invalid/probe', 'fixture', '{user.created}');
INSERT INTO public.np_webhooks_deliveries (endpoint_id, source_account_id, event_type, payload)
VALUES ('00000000-0000-4000-8000-0000000000d1', repeat('a', 129), 'user.created', '{}');
-- Check the default by behavior: PostgreSQL can retain a varchar cast in the
-- default expression even after the column is retyped to TEXT.
INSERT INTO public.np_webhooks_endpoints (id, url, secret, events)
VALUES ('00000000-0000-4000-8000-0000000000d2', 'https://example.invalid/default', 'fixture', '{user.created}');
INSERT INTO public.np_webhooks_deliveries (endpoint_id, event_type, payload)
VALUES ('00000000-0000-4000-8000-0000000000d2', 'user.created', '{}');
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.np_webhooks_endpoints
    WHERE id = '00000000-0000-4000-8000-0000000000d2' AND source_account_id = 'primary')
    OR NOT EXISTS (SELECT 1 FROM public.np_webhooks_deliveries
    WHERE endpoint_id = '00000000-0000-4000-8000-0000000000d2' AND source_account_id = 'primary') THEN
    RAISE EXCEPTION 'omitted source_account_id must default to primary';
  END IF;
END $$;
ROLLBACK;
