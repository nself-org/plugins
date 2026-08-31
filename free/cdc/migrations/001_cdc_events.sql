-- CDC plugin: logical replication slot + publication + internal event buffer.
-- Created by: nself plugin install cdc
-- Dropped by:  nself plugin uninstall cdc

-- Replication slot (pgoutput decoder, embedded mode reads directly).
SELECT pg_create_logical_replication_slot('nself_cdc', 'pgoutput')
WHERE NOT EXISTS (
    SELECT 1 FROM pg_replication_slots WHERE slot_name = 'nself_cdc'
);

-- Publication covering all current and future tables.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_publication WHERE pubname = 'nself_pub') THEN
        CREATE PUBLICATION nself_pub FOR ALL TABLES;
    END IF;
END $$;

-- Internal buffer: events written here when broker is unavailable.
-- Drained and marked brokered=true on reconnect.
CREATE TABLE IF NOT EXISTS np_cdc_events (
    id          BIGSERIAL    PRIMARY KEY,
    table_name  TEXT         NOT NULL,
    operation   TEXT         NOT NULL CHECK (operation IN ('INSERT', 'UPDATE', 'DELETE')),
    lsn         TEXT         NOT NULL,
    payload     JSONB        NOT NULL,
    brokered    BOOLEAN      NOT NULL DEFAULT false,
    occurred_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS np_cdc_events_brokered_occurred
    ON np_cdc_events (brokered, occurred_at);

-- Vacuum-friendly: autovacuum more aggressively because brokered rows are deleted after replay.
ALTER TABLE np_cdc_events
    SET (autovacuum_vacuum_scale_factor = 0.01,
         autovacuum_analyze_scale_factor = 0.05);
