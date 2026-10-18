-- CDC plugin: internal event buffer.
-- The replication slot and publication are created by the plugin at start (replication.Migrate): they need
-- wal_level=logical and a superuser, so they cannot be part of a migration that must apply on a fresh database.
-- Dropped by:  nself plugin uninstall cdc

-- Internal buffer: events written here when broker is unavailable.
-- Drained and marked brokered=true on reconnect.
CREATE TABLE IF NOT EXISTS public.np_cdc_events (
    id          BIGSERIAL    PRIMARY KEY,
    table_name  TEXT         NOT NULL,
    operation   TEXT         NOT NULL CHECK (operation IN ('INSERT', 'UPDATE', 'DELETE')),
    lsn         TEXT         NOT NULL,
    payload     JSONB        NOT NULL,
    brokered    BOOLEAN      NOT NULL DEFAULT false,
    occurred_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS np_cdc_events_brokered_occurred
    ON public.np_cdc_events (brokered, occurred_at);

-- Vacuum-friendly: autovacuum more aggressively because brokered rows are deleted after replay.
ALTER TABLE public.np_cdc_events
    SET (autovacuum_vacuum_scale_factor = 0.01,
         autovacuum_analyze_scale_factor = 0.05);
