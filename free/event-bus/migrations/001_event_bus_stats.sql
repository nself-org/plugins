-- 001_event_bus_stats.sql
-- Internal stats table for the event-bus plugin (CS_9).
-- Populated by the Go service on every publish/subscribe tick.

CREATE TABLE IF NOT EXISTS np_event_bus_stats (
  subject        TEXT        PRIMARY KEY,
  msg_count      BIGINT      NOT NULL DEFAULT 0,
  consumer_count INT         NOT NULL DEFAULT 0,
  last_msg_at    TIMESTAMPTZ,
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE np_event_bus_stats IS
  'Per-subject statistics collected by the nSelf event-bus plugin (CS_9).';
