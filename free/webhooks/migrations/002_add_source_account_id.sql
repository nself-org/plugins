-- Migration 002: create the Go webhook tables, then add source_account_id (Multi-App Isolation).
-- Required by the Multi-Tenant Convention Wall hard rule: all np_* tables carry source_account_id TEXT NOT NULL DEFAULT 'primary'.
--
-- Why creation lives here: migration 001 only renames legacy tables (it creates nothing), and the plugin no longer
-- creates schema at start-up, so a fresh database gets np_webhooks_endpoints and np_webhooks_deliveries from this file.
-- Order matters: on an install holding the old webhook_* tables, 001 has already renamed them, so CREATE ... IF NOT
-- EXISTS keeps them and the ADD COLUMN / CREATE INDEX statements below add what is missing. Idempotent, no transaction control.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" SCHEMA public;

CREATE TABLE IF NOT EXISTS public.np_webhooks_endpoints (
    id                UUID PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    url               TEXT NOT NULL,
    description       TEXT,
    secret            VARCHAR(255) NOT NULL,
    events            TEXT[] NOT NULL,
    headers           JSONB DEFAULT '{}',
    enabled           BOOLEAN DEFAULT TRUE,
    failure_count     INTEGER DEFAULT 0,
    last_success_at   TIMESTAMPTZ,
    last_failure_at   TIMESTAMPTZ,
    disabled_at       TIMESTAMPTZ,
    disabled_reason   TEXT,
    metadata          JSONB DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_webhooks_endpoints_enabled
    ON public.np_webhooks_endpoints (enabled);
CREATE INDEX IF NOT EXISTS idx_np_webhooks_endpoints_events
    ON public.np_webhooks_endpoints USING GIN(events);
CREATE INDEX IF NOT EXISTS idx_np_webhooks_endpoints_created
    ON public.np_webhooks_endpoints (created_at DESC);

CREATE TABLE IF NOT EXISTS public.np_webhooks_deliveries (
    id                UUID PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    endpoint_id       UUID NOT NULL REFERENCES public.np_webhooks_endpoints(id) ON DELETE CASCADE,
    event_type        VARCHAR(128) NOT NULL,
    payload           JSONB NOT NULL,
    status            VARCHAR(32) DEFAULT 'pending',
    response_status   INTEGER,
    response_body     TEXT,
    response_time_ms  INTEGER,
    attempt_count     INTEGER DEFAULT 0,
    max_attempts      INTEGER DEFAULT 5,
    next_retry_at     TIMESTAMPTZ,
    error_message     TEXT,
    signature         VARCHAR(255),
    delivered_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_endpoint
    ON public.np_webhooks_deliveries (endpoint_id);
CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_status
    ON public.np_webhooks_deliveries (status);
CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_event_type
    ON public.np_webhooks_deliveries (event_type);
CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_next_retry
    ON public.np_webhooks_deliveries (next_retry_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_created
    ON public.np_webhooks_deliveries (created_at DESC);

-- source_account_id must end up TEXT NOT NULL DEFAULT 'primary' on both tables. A TS-era table (renamed by 001) has it as a
-- nullable VARCHAR(128): add it where missing, fill NULLs with 'primary', then fix type, default and nullability. Each step is
-- skipped when the column is already (text, NOT NULL). The single-column account index is created only when no index on
-- exactly (source_account_id) exists (001 renames the TS-era one, which would otherwise be duplicated).
DO $$
DECLARE
  t TEXT;
  ix TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY['np_webhooks_endpoints', 'np_webhooks_deliveries'] LOOP
    EXECUTE format('ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT %L', t, 'primary');
    IF NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema = 'public' AND table_name = t AND column_name = 'source_account_id'
        AND data_type = 'text' AND is_nullable = 'NO'
    ) THEN
      EXECUTE format('UPDATE public.%I SET source_account_id = %L WHERE source_account_id IS NULL', t, 'primary');
      EXECUTE format('ALTER TABLE public.%I ALTER COLUMN source_account_id TYPE TEXT', t);
      EXECUTE format('ALTER TABLE public.%I ALTER COLUMN source_account_id SET DEFAULT %L', t, 'primary');
      EXECUTE format('ALTER TABLE public.%I ALTER COLUMN source_account_id SET NOT NULL', t);
    END IF;
    IF NOT EXISTS (
      SELECT 1 FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = i.indkey[0]
      WHERE i.indrelid = ('public.' || quote_ident(t))::regclass
        AND i.indnatts = 1 AND i.indpred IS NULL AND i.indexprs IS NULL AND a.attname = 'source_account_id'
    ) THEN
      ix := 'idx_' || t || '_account';
      EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON public.%I (source_account_id)', ix, t);
    END IF;
  END LOOP;
END $$;
