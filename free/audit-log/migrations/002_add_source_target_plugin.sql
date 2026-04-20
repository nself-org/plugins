-- Migration: 002_add_source_target_plugin
-- Plugin: audit-log
-- Description: Adds source_plugin and target_plugin columns to
--              np_auditlog_events to capture the calling and receiving plugin
--              on inter-plugin events. S43-T18.
--
-- Both columns default to '' (empty string) so existing rows and ingest
-- calls that do not specify them remain valid. Indexes are added for
-- filtering audit trails by plugin name.

ALTER TABLE np_auditlog_events
    ADD COLUMN IF NOT EXISTS source_plugin TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS target_plugin TEXT NOT NULL DEFAULT '';

-- Index for filtering by calling plugin.
CREATE INDEX IF NOT EXISTS idx_np_auditlog_source_plugin
    ON np_auditlog_events (source_plugin)
    WHERE source_plugin != '';

-- Index for filtering by receiving plugin.
CREATE INDEX IF NOT EXISTS idx_np_auditlog_target_plugin
    ON np_auditlog_events (target_plugin)
    WHERE target_plugin != '';

COMMENT ON COLUMN np_auditlog_events.source_plugin IS
    'Plugin that originated the inter-plugin call (from X-Source-Plugin header). '
    'Empty for non-plugin events.';

COMMENT ON COLUMN np_auditlog_events.target_plugin IS
    'Plugin that received the inter-plugin call. '
    'Set by the receiving plugin at ingest time. '
    'Empty for non-plugin events.';
