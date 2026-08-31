-- HIPAA plugin: core tables
-- Migration 001: PHI column registry, PHI audit log, BAA records

-- PHI column registry: tracks which table.column combinations contain PHI
CREATE TABLE IF NOT EXISTS np_phi_columns (
  id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id TEXT       NOT NULL DEFAULT 'primary',
  tenant_id        UUID,
  table_name       TEXT        NOT NULL,
  column_name      TEXT        NOT NULL,
  phi_category     TEXT        NOT NULL
    CHECK (phi_category IN ('name','dob','ssn','mrn','address','phone','email','other')),
  de_id_method     TEXT        NOT NULL DEFAULT 'mask'
    CHECK (de_id_method IN ('mask','tokenize','redact')),
  created_by       TEXT,
  audit_note       TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (source_account_id, table_name, column_name)
);

CREATE INDEX IF NOT EXISTS idx_phi_columns_account
  ON np_phi_columns (source_account_id);

-- PHI-specific audit log: every access to a registered PHI column
-- Retention enforced via RLS in migration 002.
CREATE TABLE IF NOT EXISTS np_phi_audit_log (
  id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id TEXT       NOT NULL DEFAULT 'primary',
  tenant_id        UUID,
  accessor_id      UUID,
  accessor_email   TEXT,
  table_name       TEXT        NOT NULL,
  column_names     TEXT[]      NOT NULL,
  row_count        INT         NOT NULL DEFAULT 0,
  purpose          TEXT        CHECK (purpose IN ('treatment','payment','operations','other')),
  query_hash       TEXT,
  accessed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  retain_until     DATE        NOT NULL
    GENERATED ALWAYS AS (
      (accessed_at + INTERVAL '6 years')::DATE
    ) STORED
);

CREATE INDEX IF NOT EXISTS idx_phi_audit_log_account
  ON np_phi_audit_log (source_account_id, accessed_at DESC);

CREATE INDEX IF NOT EXISTS idx_phi_audit_log_accessor
  ON np_phi_audit_log (accessor_id, accessed_at DESC);

-- BAA records: Business Associate Agreement tracking
CREATE TABLE IF NOT EXISTS np_baa_records (
  id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id TEXT     NOT NULL DEFAULT 'primary',
  tenant_id      UUID,
  signed_by      TEXT        NOT NULL,
  signer_email   TEXT,
  signed_at      TIMESTAMPTZ,
  effective_date DATE,
  expiry_date    DATE,
  document_url   TEXT,
  status         TEXT        NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','active','terminated')),
  terminated_at  TIMESTAMPTZ,
  termination_reason TEXT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_baa_records_account
  ON np_baa_records (source_account_id, status);
