-- Boot-kit legacy probe (tests/boot/tables-upgrade.sh LEGACY pass, D-0501): runs on the TS-era database AFTER the converted boot and
-- the rename checks. ON_ERROR_STOP: any RAISE or failing statement fails the pass. Pins the two legacy-only fixes that the pair
-- checks cannot see: the dead-letter foreign keys (migration 001) and the source_account_id shape (migration 002).

-- 1. Dead-letter FKs to deliveries and endpoints: present, ON DELETE SET NULL, valid, not deferrable.
DO $$
DECLARE
  parent TEXT;
  c RECORD;
BEGIN
  FOREACH parent IN ARRAY ARRAY['np_webhooks_deliveries', 'np_webhooks_endpoints'] LOOP
    SELECT confdeltype, convalidated, condeferrable INTO c
    FROM pg_constraint
    WHERE conrelid = 'public.np_webhooks_dead_letters'::regclass AND contype = 'f'
      AND confrelid = ('public.' || parent)::regclass;
    IF NOT FOUND THEN
      RAISE EXCEPTION 'legacy-probe: np_webhooks_dead_letters has no foreign key to %', parent;
    END IF;
    IF c.confdeltype <> 'n' THEN
      RAISE EXCEPTION 'legacy-probe: dead-letter foreign key to % is ON DELETE % (want SET NULL, n)', parent, c.confdeltype;
    END IF;
    IF NOT c.convalidated OR c.condeferrable THEN
      RAISE EXCEPTION 'legacy-probe: dead-letter foreign key to % is NOT VALID or DEFERRABLE', parent;
    END IF;
  END LOOP;
END $$;

-- 2. source_account_id is TEXT NOT NULL DEFAULT 'primary' on both tables, and the fixture rows that held NULL were filled.
DO $$
DECLARE
  t TEXT;
  col RECORD;
BEGIN
  FOREACH t IN ARRAY ARRAY['np_webhooks_endpoints', 'np_webhooks_deliveries'] LOOP
    SELECT data_type, is_nullable, column_default INTO col
    FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = t AND column_name = 'source_account_id';
    IF NOT FOUND THEN
      RAISE EXCEPTION 'legacy-probe: public.%.source_account_id is missing', t;
    END IF;
    IF col.data_type <> 'text' OR col.is_nullable <> 'NO' OR col.column_default NOT LIKE '''primary''%' THEN
      RAISE EXCEPTION 'legacy-probe: public.%.source_account_id is % nullable=% default=% (want text NOT NULL DEFAULT ''primary'')',
        t, col.data_type, col.is_nullable, col.column_default;
    END IF;
  END LOOP;
  IF (SELECT count(*) FROM public.np_webhooks_endpoints
      WHERE id = '00000000-0000-4000-8000-0000000000c5' AND source_account_id = 'primary') <> 1
     OR (SELECT count(*) FROM public.np_webhooks_deliveries
         WHERE id = '00000000-0000-4000-8000-0000000000c6' AND source_account_id = 'primary') <> 1 THEN
    RAISE EXCEPTION 'legacy-probe: the fixture rows with a NULL source_account_id were not normalised to ''primary''';
  END IF;
END $$;

-- 3. Behaviour: deleting an endpoint with dead letters succeeds (cascading its deliveries) and keeps the dead letter with NULL
--    parent ids; a new row with no source_account_id takes 'primary'. Rolled back, so the database is left as it was.
BEGIN;
DELETE FROM public.np_webhooks_endpoints WHERE id = '00000000-0000-4000-8000-0000000000c1';
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.np_webhooks_dead_letters
                 WHERE id = '00000000-0000-4000-8000-0000000000c4' AND delivery_id IS NULL AND endpoint_id IS NULL) THEN
    RAISE EXCEPTION 'legacy-probe: deleting the endpoint did not leave the dead letter with NULL delivery_id and endpoint_id';
  END IF;
END $$;
INSERT INTO public.np_webhooks_endpoints (id, url, secret, events)
  VALUES ('00000000-0000-4000-8000-0000000000c7', 'https://example.invalid/probe', 'whsec_probe', '{user.created}');
DO $$
BEGIN
  IF (SELECT source_account_id FROM public.np_webhooks_endpoints WHERE id = '00000000-0000-4000-8000-0000000000c7') <> 'primary' THEN
    RAISE EXCEPTION 'legacy-probe: an insert without source_account_id did not default to ''primary''';
  END IF;
END $$;
ROLLBACK;
