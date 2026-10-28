-- Boot-kit legacy probe (tests/boot/tables-upgrade.sh LEGACY pass): runs on the TS-era database after the converted boot and the
-- rename checks, with ON_ERROR_STOP. A statement that errors, or a DO block that RAISEs, fails the pass. Proves what the renames
-- cannot: migration 001 turned the dead-letter foreign keys ON DELETE SET NULL, and migration 002 normalised source_account_id to
-- TEXT NOT NULL DEFAULT 'primary' on a table the TS era created as a nullable VARCHAR(128). Test data only.

-- Both dead-letter foreign keys exist, one per parent, and both are ON DELETE SET NULL (confdeltype 'n'), not the TS-era NO ACTION.
DO $$
DECLARE
  parent TEXT;
  rule "char";
BEGIN
  FOREACH parent IN ARRAY ARRAY['np_webhooks_deliveries', 'np_webhooks_endpoints'] LOOP
    SELECT confdeltype INTO rule FROM pg_constraint
     WHERE conrelid = 'public.np_webhooks_dead_letters'::regclass AND contype = 'f'
       AND confrelid = ('public.' || parent)::regclass;
    IF NOT FOUND THEN
      RAISE EXCEPTION 'legacy-probe: np_webhooks_dead_letters has no foreign key to %', parent;
    END IF;
    IF rule <> 'n' THEN
      RAISE EXCEPTION 'legacy-probe: np_webhooks_dead_letters foreign key to % is ON DELETE % (want SET NULL)', parent, rule;
    END IF;
  END LOOP;
END $$;

-- source_account_id is TEXT NOT NULL DEFAULT 'primary' on both tables the plugin owns.
DO $$
DECLARE
  t TEXT;
  c RECORD;
BEGIN
  FOREACH t IN ARRAY ARRAY['np_webhooks_endpoints', 'np_webhooks_deliveries'] LOOP
    SELECT data_type, is_nullable, column_default INTO c FROM information_schema.columns
     WHERE table_schema = 'public' AND table_name = t AND column_name = 'source_account_id';
    IF NOT FOUND THEN
      RAISE EXCEPTION 'legacy-probe: % has no source_account_id', t;
    END IF;
    IF c.data_type <> 'text' OR c.is_nullable <> 'NO' OR c.column_default IS NULL OR c.column_default NOT LIKE '''primary''%' THEN
      RAISE EXCEPTION 'legacy-probe: %.source_account_id is % nullable=% default=% (want text NOT NULL DEFAULT ''primary'')',
        t, c.data_type, c.is_nullable, c.column_default;
    END IF;
  END LOOP;
  IF EXISTS (SELECT 1 FROM public.np_webhooks_endpoints WHERE source_account_id IS DISTINCT FROM 'primary')
     OR EXISTS (SELECT 1 FROM public.np_webhooks_deliveries WHERE source_account_id IS DISTINCT FROM 'primary') THEN
    RAISE EXCEPTION 'legacy-probe: a fixture row did not keep source_account_id primary';
  END IF;
END $$;

-- A renamed table still takes the plugin's inserts: the column default fills source_account_id.
INSERT INTO np_webhooks_endpoints (id, url, secret, events) VALUES ('00000000-0000-4000-8000-0000000000e1', 'https://example.invalid/probe', 'whsec_probe', '{probe.created}');
INSERT INTO np_webhooks_deliveries (id, endpoint_id, event_type, payload) VALUES ('00000000-0000-4000-8000-0000000000e2', '00000000-0000-4000-8000-0000000000e1', 'probe.created', '{}');
INSERT INTO np_webhooks_dead_letters (id, delivery_id, endpoint_id) VALUES ('00000000-0000-4000-8000-0000000000e3', '00000000-0000-4000-8000-0000000000e2', '00000000-0000-4000-8000-0000000000e1');
DO $$
BEGIN
  IF (SELECT source_account_id FROM np_webhooks_endpoints WHERE id = '00000000-0000-4000-8000-0000000000e1') IS DISTINCT FROM 'primary' THEN
    RAISE EXCEPTION 'legacy-probe: an endpoint inserted without source_account_id did not get ''primary''';
  END IF;
END $$;

-- Deleting an endpoint that has deliveries and dead letters succeeds (the TS-era NO ACTION key made it fail); the deliveries go
-- with it and the dead letter stays with both references cleared.
DELETE FROM np_webhooks_endpoints WHERE id = '00000000-0000-4000-8000-0000000000e1';
DO $$
DECLARE
  d RECORD;
BEGIN
  IF EXISTS (SELECT 1 FROM np_webhooks_deliveries WHERE id = '00000000-0000-4000-8000-0000000000e2') THEN
    RAISE EXCEPTION 'legacy-probe: deleting an endpoint left its delivery behind';
  END IF;
  SELECT delivery_id, endpoint_id INTO d FROM np_webhooks_dead_letters WHERE id = '00000000-0000-4000-8000-0000000000e3';
  IF NOT FOUND THEN
    RAISE EXCEPTION 'legacy-probe: the dead letter was deleted with its endpoint (want it kept)';
  END IF;
  IF d.delivery_id IS NOT NULL OR d.endpoint_id IS NOT NULL THEN
    RAISE EXCEPTION 'legacy-probe: the dead letter kept references (delivery_id=%, endpoint_id=%)', d.delivery_id, d.endpoint_id;
  END IF;
END $$;
