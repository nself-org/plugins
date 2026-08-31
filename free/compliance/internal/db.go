package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxIface is the subset of *pgxpool.Pool used by handlers and schema init.
// Declaring it as an interface (rather than depending on the concrete
// *pgxpool.Pool type) lets tests substitute a pgxmock.PgxPoolIface without a
// real database. Mirrors the pattern established in paid/moderation.
type PgxIface interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Ping(ctx context.Context) error
}

// DB wraps a pgx connection pool and exposes schema initialisation.
type DB struct {
	Pool PgxIface

	closer interface{ Close() }
}

// NewDB opens a connection pool using the provided DATABASE_URL.
func NewDB(ctx context.Context, databaseURL string) (*DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{Pool: pool, closer: pool}, nil
}

// InitSchema creates all compliance tables and indexes if they do not exist.
func (d *DB) InitSchema(ctx context.Context) error {
	_, err := d.Pool.Exec(ctx, schema)
	return err
}

// Close releases all pool connections.
func (d *DB) Close() {
	if d.closer != nil {
		d.closer.Close()
	}
}

const schema = `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- DSARs
CREATE TABLE IF NOT EXISTS compliance_dsars (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  request_type VARCHAR(50) NOT NULL,
  request_number VARCHAR(50) NOT NULL,
  user_id VARCHAR(255),
  requester_email VARCHAR(255) NOT NULL,
  requester_name VARCHAR(255),
  verification_token VARCHAR(255),
  verification_sent_at TIMESTAMPTZ,
  verification_completed_at TIMESTAMPTZ,
  verified_by VARCHAR(255),
  description TEXT,
  data_categories TEXT[] DEFAULT '{}',
  specific_data_requested TEXT,
  status VARCHAR(30) NOT NULL DEFAULT 'pending',
  assigned_to VARCHAR(255),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  deadline TIMESTAMPTZ NOT NULL,
  data_package_url TEXT,
  data_package_size_bytes BIGINT,
  data_package_generated_at TIMESTAMPTZ,
  resolution_notes TEXT,
  rejection_reason TEXT,
  regulation VARCHAR(50) NOT NULL DEFAULT 'GDPR',
  jurisdiction VARCHAR(100),
  legal_basis TEXT,
  ip_address VARCHAR(45),
  user_agent TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, request_number)
);
CREATE INDEX IF NOT EXISTS idx_dsars_account ON compliance_dsars(source_account_id);
CREATE INDEX IF NOT EXISTS idx_dsars_user    ON compliance_dsars(source_account_id, user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dsars_status  ON compliance_dsars(source_account_id, status, deadline);
CREATE INDEX IF NOT EXISTS idx_dsars_number  ON compliance_dsars(source_account_id, request_number);

-- DSAR Activities
CREATE TABLE IF NOT EXISTS compliance_dsar_activities (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  dsar_id UUID NOT NULL REFERENCES compliance_dsars(id) ON DELETE CASCADE,
  activity_type VARCHAR(100) NOT NULL,
  description TEXT,
  performed_by VARCHAR(255),
  performed_by_name VARCHAR(255),
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dsar_activities_account ON compliance_dsar_activities(source_account_id);
CREATE INDEX IF NOT EXISTS idx_dsar_activities_dsar    ON compliance_dsar_activities(dsar_id, created_at);

-- Consents
CREATE TABLE IF NOT EXISTS compliance_consents (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  purpose VARCHAR(255) NOT NULL,
  purpose_description TEXT,
  status VARCHAR(20) NOT NULL DEFAULT 'granted',
  granted_at TIMESTAMPTZ,
  denied_at TIMESTAMPTZ,
  withdrawn_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  consent_method VARCHAR(100),
  consent_text TEXT,
  privacy_policy_version VARCHAR(50),
  ip_address VARCHAR(45),
  user_agent TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_consents_account ON compliance_consents(source_account_id);
CREATE INDEX IF NOT EXISTS idx_consents_user    ON compliance_consents(source_account_id, user_id, purpose);
CREATE INDEX IF NOT EXISTS idx_consents_status  ON compliance_consents(source_account_id, status, purpose);
CREATE INDEX IF NOT EXISTS idx_consents_expires ON compliance_consents(expires_at) WHERE expires_at IS NOT NULL;

-- Consent History
CREATE TABLE IF NOT EXISTS compliance_consent_history (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  consent_id UUID NOT NULL REFERENCES compliance_consents(id) ON DELETE CASCADE,
  previous_status VARCHAR(20),
  new_status VARCHAR(20) NOT NULL,
  change_reason VARCHAR(255),
  changed_by VARCHAR(255),
  ip_address VARCHAR(45),
  user_agent TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_consent_history_account  ON compliance_consent_history(source_account_id);
CREATE INDEX IF NOT EXISTS idx_consent_history_consent  ON compliance_consent_history(consent_id, created_at);

-- Privacy Policies
CREATE TABLE IF NOT EXISTS compliance_privacy_policies (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  version VARCHAR(50) NOT NULL,
  version_number INTEGER NOT NULL,
  title VARCHAR(255) NOT NULL,
  content TEXT NOT NULL,
  summary TEXT,
  changes_summary TEXT,
  is_active BOOLEAN NOT NULL DEFAULT false,
  requires_reacceptance BOOLEAN NOT NULL DEFAULT false,
  effective_from TIMESTAMPTZ NOT NULL,
  effective_until TIMESTAMPTZ,
  language VARCHAR(10) NOT NULL DEFAULT 'en',
  jurisdiction VARCHAR(100),
  created_by VARCHAR(255),
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, version)
);
CREATE INDEX IF NOT EXISTS idx_privacy_policies_account ON compliance_privacy_policies(source_account_id);
CREATE INDEX IF NOT EXISTS idx_privacy_policies_active  ON compliance_privacy_policies(source_account_id, is_active, effective_from);

-- Data Exports
CREATE TABLE IF NOT EXISTS compliance_data_exports (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  format VARCHAR(10) NOT NULL DEFAULT 'json',
  status VARCHAR(30) NOT NULL DEFAULT 'pending',
  export_url TEXT,
  expires_at TIMESTAMPTZ,
  error_message TEXT,
  requested_at TIMESTAMPTZ DEFAULT NOW(),
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_data_exports_account ON compliance_data_exports(source_account_id);
CREATE INDEX IF NOT EXISTS idx_data_exports_user    ON compliance_data_exports(source_account_id, user_id, created_at DESC);

-- Webhook Events
CREATE TABLE IF NOT EXISTS compliance_webhook_events (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(100) NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  delivered BOOLEAN NOT NULL DEFAULT false,
  delivered_at TIMESTAMPTZ,
  error_message TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_webhook_events_account   ON compliance_webhook_events(source_account_id);
CREATE INDEX IF NOT EXISTS idx_webhook_events_delivered ON compliance_webhook_events(delivered, created_at);
`
