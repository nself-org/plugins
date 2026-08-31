package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the minimal subset of *pgxpool.Pool used by handlers. It exists
// as a testability seam: production code passes a *pgxpool.Pool, tests pass a
// pgxmock.PgxPoolIface — both satisfy this interface structurally.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Ping(ctx context.Context) error
}

// DB wraps a pgxpool connection pool.
type DB struct {
	Pool   Querier
	closer func()
}

// NewDB opens a connection pool using the provided DATABASE_URL.
func NewDB(ctx context.Context, dsn string) (*DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return &DB{Pool: pool, closer: pool.Close}, nil
}

// Close releases all pool connections.
func (d *DB) Close() {
	if d.closer != nil {
		d.closer()
	}
}

// InitSchema creates all required tables, indexes, and views idempotently.
func (d *DB) InitSchema(ctx context.Context) error {
	ddl := `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Devices
CREATE TABLE IF NOT EXISTS np_dev_devices (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id   VARCHAR(128) NOT NULL DEFAULT 'primary',
  app_id              VARCHAR(64)  NOT NULL DEFAULT 'default',
  device_id           VARCHAR(255) NOT NULL,
  name                VARCHAR(255),
  device_type         VARCHAR(64)  NOT NULL,
  model               VARCHAR(128),
  firmware_version    VARCHAR(64),
  status              VARCHAR(32)  NOT NULL DEFAULT 'unregistered',
  trust_level         VARCHAR(16)  NOT NULL DEFAULT 'untrusted',
  enrollment_token    VARCHAR(255),
  enrollment_challenge VARCHAR(255),
  enrolled_at         TIMESTAMPTZ,
  enrolled_by         VARCHAR(255),
  public_key          TEXT,
  last_seen_at        TIMESTAMPTZ,
  last_ip             VARCHAR(45),
  capabilities        JSONB        NOT NULL DEFAULT '[]',
  config              JSONB        NOT NULL DEFAULT '{}',
  labels              JSONB        NOT NULL DEFAULT '{}',
  metadata            JSONB        NOT NULL DEFAULT '{}',
  revoked_at          TIMESTAMPTZ,
  revoked_by          VARCHAR(255),
  revoke_reason       TEXT,
  created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, app_id, device_id)
);
CREATE INDEX IF NOT EXISTS idx_np_dev_devices_source ON np_dev_devices(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_dev_devices_status ON np_dev_devices(status);
CREATE INDEX IF NOT EXISTS idx_np_dev_devices_type   ON np_dev_devices(device_type);
CREATE INDEX IF NOT EXISTS idx_np_dev_devices_seen   ON np_dev_devices(last_seen_at);

-- Commands
CREATE TABLE IF NOT EXISTS np_dev_commands (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  app_id            VARCHAR(64)  NOT NULL DEFAULT 'default',
  device_id         VARCHAR(255) NOT NULL,
  command_type      VARCHAR(64)  NOT NULL,
  command_id        VARCHAR(255) NOT NULL DEFAULT gen_random_uuid()::text,
  payload           JSONB        NOT NULL DEFAULT '{}',
  status            VARCHAR(32)  NOT NULL DEFAULT 'dispatched',
  priority          VARCHAR(16)  NOT NULL DEFAULT 'normal',
  dispatched_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  acked_at          TIMESTAMPTZ,
  started_at        TIMESTAMPTZ,
  completed_at      TIMESTAMPTZ,
  result            JSONB,
  error             TEXT,
  timeout_seconds   INTEGER      NOT NULL DEFAULT 60,
  deadline          TIMESTAMPTZ,
  retry_count       INTEGER      NOT NULL DEFAULT 0,
  max_retries       INTEGER      NOT NULL DEFAULT 3,
  idempotency_key   VARCHAR(255) NOT NULL DEFAULT gen_random_uuid()::text,
  metadata          JSONB        NOT NULL DEFAULT '{}',
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_dev_commands_device ON np_dev_commands(device_id);
CREATE INDEX IF NOT EXISTS idx_np_dev_commands_status ON np_dev_commands(status);

-- Telemetry
CREATE TABLE IF NOT EXISTS np_dev_telemetry (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  app_id            VARCHAR(64)  NOT NULL DEFAULT 'default',
  device_id         VARCHAR(255) NOT NULL,
  telemetry_type    VARCHAR(64)  NOT NULL,
  data              JSONB        NOT NULL DEFAULT '{}',
  recorded_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  received_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_dev_telemetry_device ON np_dev_telemetry(device_id);
CREATE INDEX IF NOT EXISTS idx_np_dev_telemetry_type   ON np_dev_telemetry(telemetry_type);
CREATE INDEX IF NOT EXISTS idx_np_dev_telemetry_time   ON np_dev_telemetry(recorded_at);

-- Ingest Sessions
CREATE TABLE IF NOT EXISTS np_dev_ingest_sessions (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id   VARCHAR(128) NOT NULL DEFAULT 'primary',
  app_id              VARCHAR(64)  NOT NULL DEFAULT 'default',
  device_id           VARCHAR(255) NOT NULL,
  stream_id           VARCHAR(255) NOT NULL,
  status              VARCHAR(32)  NOT NULL DEFAULT 'idle',
  ingest_url          TEXT,
  protocol            VARCHAR(16)  NOT NULL DEFAULT 'rtmp',
  channel             VARCHAR(255),
  quality             VARCHAR(64),
  bitrate_kbps        INTEGER,
  fps                 FLOAT,
  resolution          VARCHAR(32),
  started_at          TIMESTAMPTZ,
  last_heartbeat_at   TIMESTAMPTZ,
  ended_at            TIMESTAMPTZ,
  bytes_ingested      BIGINT       NOT NULL DEFAULT 0,
  frames_dropped      BIGINT       NOT NULL DEFAULT 0,
  error_count         INTEGER      NOT NULL DEFAULT 0,
  last_error          TEXT,
  metadata            JSONB        NOT NULL DEFAULT '{}',
  created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_dev_ingest_device ON np_dev_ingest_sessions(device_id);
CREATE INDEX IF NOT EXISTS idx_np_dev_ingest_status ON np_dev_ingest_sessions(status);

-- Audit Log
CREATE TABLE IF NOT EXISTS np_dev_audit_log (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  app_id            VARCHAR(64)  NOT NULL DEFAULT 'default',
  device_id         VARCHAR(255),
  action            VARCHAR(128) NOT NULL,
  actor_id          VARCHAR(255),
  details           JSONB        NOT NULL DEFAULT '{}',
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_dev_audit_device ON np_dev_audit_log(device_id);
CREATE INDEX IF NOT EXISTS idx_np_dev_audit_action ON np_dev_audit_log(action);

-- Bootstrap Tokens
CREATE TABLE IF NOT EXISTS np_dev_bootstrap_tokens (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name              VARCHAR(255) NOT NULL,
  token             VARCHAR(255) NOT NULL UNIQUE,
  capabilities      JSONB        NOT NULL DEFAULT '[]',
  expires_at        TIMESTAMPTZ  NOT NULL,
  used              BOOLEAN      NOT NULL DEFAULT FALSE,
  used_by_device_id UUID,
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
`
	_, err := d.Pool.Exec(ctx, ddl)
	return err
}
