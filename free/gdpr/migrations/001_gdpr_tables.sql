-- GDPR request audit trail (Art. 30 — records of processing).
-- This table is APPEND-ONLY. The RLS policy below blocks DELETE for all roles.
-- Never drop or truncate this table.

CREATE TABLE IF NOT EXISTS np_gdpr_requests (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID        REFERENCES np_tenants(id) ON DELETE SET NULL,
    request_type     TEXT        NOT NULL CHECK (request_type IN ('export','delete','restrict')),
    subject_type     TEXT        NOT NULL CHECK (subject_type IN ('user','tenant')),
    subject_id       TEXT        NOT NULL,
    requested_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deadline         DATE        NOT NULL DEFAULT (NOW() + INTERVAL '30 days'),
    status           TEXT        NOT NULL DEFAULT 'pending'
                                 CHECK (status IN ('pending','processing','complete','failed')),
    completed_at     TIMESTAMPTZ,
    artifact_url     TEXT,
    artifact_expires TIMESTAMPTZ,
    notes            TEXT
);

CREATE INDEX IF NOT EXISTS idx_np_gdpr_requests_subject
    ON np_gdpr_requests (subject_id, subject_type);

CREATE INDEX IF NOT EXISTS idx_np_gdpr_requests_status
    ON np_gdpr_requests (status, deadline);

-- Append-only RLS: select + insert are allowed; delete + update of completed
-- rows are denied. Only the gdpr_admin role may update status.
ALTER TABLE np_gdpr_requests ENABLE ROW LEVEL SECURITY;

CREATE POLICY gdpr_requests_select ON np_gdpr_requests
    FOR SELECT USING (true);

CREATE POLICY gdpr_requests_insert ON np_gdpr_requests
    FOR INSERT WITH CHECK (true);

-- No DELETE policy → DELETE is denied for all roles.
-- UPDATE is allowed only on status transitions (not on completed rows).
CREATE POLICY gdpr_requests_update ON np_gdpr_requests
    FOR UPDATE USING (status NOT IN ('complete', 'failed'));


-- Plugin GDPR registry: third-party plugins register which tables they own
-- so the cascade export/delete can include their data automatically.

CREATE TABLE IF NOT EXISTS np_gdpr_plugin_registry (
    id            UUID  PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_name   TEXT  NOT NULL UNIQUE,
    -- JSONB array of {table, user_col, strategy: "anonymize"|"delete"}
    user_tables   JSONB NOT NULL DEFAULT '[]',
    -- JSONB array for tenant-level cascade (Enterprise gate)
    tenant_tables JSONB NOT NULL DEFAULT '[]',
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_gdpr_registry_plugin
    ON np_gdpr_plugin_registry (plugin_name);
