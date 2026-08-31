-- Migration 001: audit-analytics tables.
-- Idempotent: uses CREATE TABLE IF NOT EXISTS.
-- Depends on: np_audit_log, np_tenants, np_users (core nSelf schema).

CREATE TABLE IF NOT EXISTS np_audit_anomalies (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID,
    user_id       UUID,
    anomaly_type  TEXT NOT NULL CHECK (anomaly_type IN (
                      'bulk_delete',
                      'new_ip',
                      'impossible_travel',
                      'after_hours_admin',
                      'freq_spike'
                  )),
    severity      TEXT NOT NULL DEFAULT 'medium' CHECK (severity IN ('low','medium','high','critical')),
    z_score       NUMERIC(6,2),
    detected_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    context       JSONB NOT NULL DEFAULT '{}',
    reviewed_by   UUID,
    reviewed_at   TIMESTAMPTZ,
    disposition   TEXT CHECK (disposition IN ('false_positive','true_positive','escalated'))
);

COMMENT ON TABLE np_audit_anomalies IS 'Scored anomaly records from the audit-analytics plugin. One row per detected anomaly event.';
COMMENT ON COLUMN np_audit_anomalies.z_score    IS 'Z-score of action frequency vs 7-day rolling baseline for (user, action, hour).';
COMMENT ON COLUMN np_audit_anomalies.context    IS 'Detection context: {action, count, baseline, ip, location}.';
COMMENT ON COLUMN np_audit_anomalies.disposition IS 'Reviewer disposition: false_positive | true_positive | escalated.';

CREATE INDEX IF NOT EXISTS idx_audit_anomalies_tenant     ON np_audit_anomalies (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_anomalies_user        ON np_audit_anomalies (user_id);
CREATE INDEX IF NOT EXISTS idx_audit_anomalies_severity    ON np_audit_anomalies (severity, detected_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_anomalies_unreviewed  ON np_audit_anomalies (detected_at DESC) WHERE reviewed_at IS NULL;

-- Hasura row filter enforces tenant isolation: {"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}

CREATE TABLE IF NOT EXISTS np_audit_privileged_reviews (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID,
    audit_log_id UUID,
    action       TEXT NOT NULL,
    actor_id     UUID,
    due_at       TIMESTAMPTZ NOT NULL,
    reviewer_id  UUID,
    reviewed_at  TIMESTAMPTZ,
    notes        TEXT
);

COMMENT ON TABLE np_audit_privileged_reviews IS 'Review queue for privileged audit actions (role_grant, plugin_install, schema_alter, gdpr_delete). Items become overdue after due_at.';
COMMENT ON COLUMN np_audit_privileged_reviews.due_at IS 'audit_log.created_at + NSELF_AUDIT_PRIVILEGED_REVIEW_TTL seconds.';

CREATE INDEX IF NOT EXISTS idx_privileged_reviews_tenant   ON np_audit_privileged_reviews (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_privileged_reviews_overdue  ON np_audit_privileged_reviews (due_at) WHERE reviewed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_privileged_reviews_actor    ON np_audit_privileged_reviews (actor_id);
