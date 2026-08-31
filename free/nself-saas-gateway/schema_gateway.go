package main

// schema_gateway.go — gateway-owned control tables (boot-ensured).
//
// Purpose: DDL for tables ONLY the gateway reads/writes (the shared np_saas_*
//   control tables live in shared/saas/schema.go and are ensured by every
//   plugin). Everything here is idempotent CREATE/ALTER IF NOT EXISTS and
//   runs after saas.EnsureSchema under the same advisory-lock pattern.
// Inputs:  ctx, *sql.DB (shared ops-Postgres).
// Outputs: error on DDL failure (fatal in cloud mode).
// Constraints: additive-only; never drops or rewrites data.

import (
	"context"
	"database/sql"
	"fmt"
)

// gatewayAdvisoryLockKey — 73xx range is nSentry; 7391 = gateway tables
// (7390 is the shared saas schema lock).
const gatewayAdvisoryLockKey = 7391

// gatewaySchemaDDL are the idempotent statements for gateway-owned tables.
var gatewaySchemaDDL = []string{
	// Down-detector state: one row per cloud monitor the detector has acted
	// on. status drives idempotency (open exactly one incident per outage;
	// resolve it exactly once on recovery).
	`CREATE TABLE IF NOT EXISTS np_saas_monitor_state (
		monitor_id        TEXT        PRIMARY KEY,
		tenant_id         UUID        NOT NULL,
		status            TEXT        NOT NULL DEFAULT 'up'
		                              CHECK (status IN ('up','down')),
		consecutive_fails INTEGER     NOT NULL DEFAULT 0,
		incident_id       TEXT        NOT NULL DEFAULT '',
		updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_np_saas_monitor_state_tenant
		ON np_saas_monitor_state (tenant_id)`,

	// SSL-expiry alert ledger: one row per (monitor, not_after) the detector
	// has alerted on, so a cert nearing expiry alerts once per check window
	// and re-alerts only after renewal produces a new not_after.
	`CREATE TABLE IF NOT EXISTS np_saas_ssl_alerts (
		monitor_id TEXT        NOT NULL,
		tenant_id  UUID        NOT NULL,
		not_after  TIMESTAMPTZ NOT NULL,
		alerted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (monitor_id, not_after)
	)`,

	// Team members (seats). The tenant owner is NOT a row here — it lives on
	// np_saas_tenants (owner_email) and always counts as seat #1; the Seats
	// tier quota gates 1 + COUNT(members). Invites store only the SHA-256 of
	// the join token (raw token goes in the welcome email exactly once).
	`CREATE TABLE IF NOT EXISTS np_saas_team_members (
		id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id         UUID        NOT NULL REFERENCES np_saas_tenants(tenant_id)
		                              ON DELETE CASCADE,
		email             TEXT        NOT NULL,
		name              TEXT        NOT NULL DEFAULT '',
		role              TEXT        NOT NULL DEFAULT 'member'
		                              CHECK (role IN ('member')),
		status            TEXT        NOT NULL DEFAULT 'invited'
		                              CHECK (status IN ('invited','active')),
		invite_token_hash TEXT,
		invited_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		joined_at         TIMESTAMPTZ,
		UNIQUE (tenant_id, email)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_np_saas_team_members_tenant
		ON np_saas_team_members (tenant_id)`,

	// Member password login (set at invite ACCEPT — POST /v1/join). bcrypt
	// hash, same policy as the tenant owner's owner_password_hash.
	`ALTER TABLE np_saas_team_members ADD COLUMN IF NOT EXISTS password_hash TEXT`,
	// Fast join-token lookup (GET/POST /v1/join resolve by token hash).
	`CREATE INDEX IF NOT EXISTS idx_np_saas_team_members_invite_token
		ON np_saas_team_members (invite_token_hash) WHERE invite_token_hash IS NOT NULL`,

	// Status-page email subscribers (down/up notifications for a tenant's
	// public page). status_page_id NULL = all the tenant's pages.
	`CREATE TABLE IF NOT EXISTS np_saas_subscribers (
		id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id      UUID        NOT NULL REFERENCES np_saas_tenants(tenant_id)
		                           ON DELETE CASCADE,
		status_page_id UUID        REFERENCES np_saas_status_pages(id)
		                           ON DELETE CASCADE,
		email          TEXT        NOT NULL,
		created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE (tenant_id, status_page_id, email)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_np_saas_subscribers_tenant
		ON np_saas_subscribers (tenant_id)`,

	// Subscriber confirmation (double-opt-in ready). DEFAULT true so every
	// pre-existing opt-in keeps receiving notices; a future confirm flow can
	// create new rows false-until-confirmed without another migration.
	`ALTER TABLE np_saas_subscribers ADD COLUMN IF NOT EXISTS confirmed BOOLEAN NOT NULL DEFAULT true`,

	// Subscriber notice ledger: one row per (subscriber, incident, state)
	// email the notifier has CLAIMED. INSERT ... ON CONFLICT DO NOTHING is the
	// claim (subscriber_notify.go); an SMTP failure deletes the row so the
	// next detector pass retries. UNIQUE key = exactly-once per transition.
	`CREATE TABLE IF NOT EXISTS np_saas_subscriber_notices (
		id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id     UUID        NOT NULL,
		subscriber_id UUID        NOT NULL REFERENCES np_saas_subscribers(id)
		                          ON DELETE CASCADE,
		incident_id   TEXT        NOT NULL,
		state         TEXT        NOT NULL,
		sent_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE (subscriber_id, incident_id, state)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_np_saas_subscriber_notices_tenant
		ON np_saas_subscriber_notices (tenant_id)`,

	// Status-page components: explicit monitor→page curation. When a page
	// has component rows, the public renderer shows EXACTLY the rows with
	// public=true (fail-closed default false); with no rows it falls back to
	// the host-shape heuristic (handlers_statuspublic.go).
	`CREATE TABLE IF NOT EXISTS np_saas_status_page_components (
		id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id      UUID        NOT NULL,
		status_page_id UUID        NOT NULL REFERENCES np_saas_status_pages(id)
		                           ON DELETE CASCADE,
		monitor_id     TEXT        NOT NULL,
		name           TEXT        NOT NULL DEFAULT '',
		public         BOOLEAN     NOT NULL DEFAULT false,
		position       INTEGER     NOT NULL DEFAULT 0,
		created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE (status_page_id, monitor_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_np_saas_spc_tenant
		ON np_saas_status_page_components (tenant_id)`,

	// Email verification state for the tenant owner (set by /v1/verify-email).
	`ALTER TABLE np_saas_tenants ADD COLUMN IF NOT EXISTS verified BOOLEAN NOT NULL DEFAULT false`,

	// CI-failure events (the SaaS equivalent of the self-hosted Sentry
	// Bundle's GitHub-Actions bridge): one row per pipeline run a tenant's
	// CI reports in via POST /v1/ci/events (handlers_ci.go). Gateway-owned —
	// no separate CI plugin; ingest is thin enough to live here directly.
	`CREATE TABLE IF NOT EXISTS np_saas_ci_events (
		id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id  UUID        NOT NULL,
		repo       TEXT        NOT NULL,
		workflow   TEXT        NOT NULL DEFAULT '',
		status     TEXT        NOT NULL CHECK (status IN ('success','failure','cancelled')),
		run_url    TEXT        NOT NULL DEFAULT '',
		sha        TEXT        NOT NULL DEFAULT '',
		title      TEXT        NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_np_saas_ci_events_tenant_created
		ON np_saas_ci_events (tenant_id, created_at DESC)`,

	// One-time email tokens (verify-email + password-reset). Only the
	// SHA-256 of the raw token is stored; the raw token travels exactly once
	// inside the email. Rows are single-use (used_at) and expire.
	`CREATE TABLE IF NOT EXISTS np_saas_email_tokens (
		token_hash TEXT        PRIMARY KEY,
		tenant_id  UUID        NOT NULL REFERENCES np_saas_tenants(tenant_id)
		                       ON DELETE CASCADE,
		kind       TEXT        NOT NULL CHECK (kind IN ('verify','reset')),
		expires_at TIMESTAMPTZ NOT NULL,
		used_at    TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_np_saas_email_tokens_tenant
		ON np_saas_email_tokens (tenant_id, kind)`,
}

// ensureGatewaySchema creates the gateway-owned tables if they do not exist.
// Idempotent + advisory-locked (parallel gateway boots on one DB never race).
func ensureGatewaySchema(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("gateway: acquire conn: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, gatewayAdvisoryLockKey); err != nil {
		return fmt.Errorf("gateway: advisory lock: %w", err)
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, gatewayAdvisoryLockKey) //nolint:errcheck

	for i, stmt := range gatewaySchemaDDL {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("gateway: ensure schema stmt %d: %w", i, err)
		}
	}
	return nil
}
