-- +goose Up
CREATE TABLE IF NOT EXISTS np_sync_events (
    event_id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    tenant_id UUID NULL,
    device_id UUID NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id UUID NOT NULL,
    op TEXT NOT NULL CHECK (op IN ('insert','update','delete')),
    hlc_wall_ms BIGINT NOT NULL,
    hlc_lamport BIGINT NOT NULL,
    payload JSONB,
    schema_version INTEGER NOT NULL DEFAULT 1,
    signature BYTEA NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX np_sync_events_user_hlc ON np_sync_events (user_id, hlc_wall_ms, hlc_lamport);
CREATE INDEX np_sync_events_entity ON np_sync_events (entity_type, entity_id, hlc_wall_ms DESC);

CREATE TABLE IF NOT EXISTS np_devices (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    tenant_id UUID NULL,
    device_pubkey BYTEA NOT NULL,
    label TEXT,
    platform TEXT,
    last_seen_at TIMESTAMPTZ DEFAULT now(),
    revoked_at TIMESTAMPTZ NULL
);
CREATE INDEX np_devices_user ON np_devices (user_id, revoked_at);

CREATE TABLE IF NOT EXISTS np_sync_cursors (
    device_id UUID PRIMARY KEY REFERENCES np_devices(id) ON DELETE CASCADE,
    last_hlc_wall_ms BIGINT NOT NULL DEFAULT 0,
    last_hlc_lamport BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- np_sync_state: materialized current state per (user_id, entity_type, entity_id).
-- One row per entity, holding the latest non-delete event. Used by
-- handleSnapshot to stream a device's current state without replaying every
-- historical event.
--
-- Refresh policy: REFRESH MATERIALIZED VIEW CONCURRENTLY np_sync_state, driven
-- by a periodic job (S03 ticket) or on-demand after large batch ingests. The
-- UNIQUE index supports CONCURRENTLY.
CREATE MATERIALIZED VIEW IF NOT EXISTS np_sync_state AS
SELECT DISTINCT ON (user_id, entity_type, entity_id)
       event_id,
       user_id,
       tenant_id,
       device_id,
       entity_type,
       entity_id,
       op,
       hlc_wall_ms,
       hlc_lamport,
       payload,
       schema_version,
       received_at
  FROM np_sync_events
 ORDER BY user_id, entity_type, entity_id, hlc_wall_ms DESC, hlc_lamport DESC;

CREATE UNIQUE INDEX IF NOT EXISTS np_sync_state_pk
    ON np_sync_state (user_id, entity_type, entity_id);
CREATE INDEX IF NOT EXISTS np_sync_state_user_hlc
    ON np_sync_state (user_id, hlc_wall_ms, hlc_lamport);

-- +goose Down
DROP MATERIALIZED VIEW IF EXISTS np_sync_state;
DROP TABLE IF EXISTS np_sync_cursors;
DROP TABLE IF EXISTS np_devices;
DROP TABLE IF EXISTS np_sync_events;
