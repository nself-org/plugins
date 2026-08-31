package internal

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the minimal subset of *pgxpool.Pool used by this plugin's
// handlers. Extracting it as an interface lets tests substitute a mock
// (e.g. pgxmock) without a live database, while production code continues
// to pass a real *pgxpool.Pool unchanged.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// DB wraps a pgxpool connection pool.
type DB struct {
	Pool Querier

	// pool holds the concrete pgxpool.Pool when constructed via NewDB, so
	// Close() can release real connections. Nil when Pool is a test double.
	pool *pgxpool.Pool
}

// NewDB creates and returns a connected database pool.
func NewDB(databaseURL string) (*DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("database connected")
	return &DB{Pool: pool, pool: pool}, nil
}

// Close closes the connection pool.
func (d *DB) Close() {
	if d.pool != nil {
		d.pool.Close()
	}
}

// InitSchema creates all required tables if they do not exist.
func (d *DB) InitSchema(ctx context.Context) error {
	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS acl_roles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			name VARCHAR(128) NOT NULL,
			display_name VARCHAR(255),
			description TEXT,
			parent_role_id UUID REFERENCES acl_roles(id) ON DELETE SET NULL,
			level INTEGER NOT NULL DEFAULT 0,
			is_system BOOLEAN NOT NULL DEFAULT false,
			metadata JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(source_account_id, name)
		);

		CREATE INDEX IF NOT EXISTS idx_acl_roles_source_account ON acl_roles(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_acl_roles_name ON acl_roles(name);
		CREATE INDEX IF NOT EXISTS idx_acl_roles_parent ON acl_roles(parent_role_id);

		CREATE TABLE IF NOT EXISTS acl_permissions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			resource VARCHAR(128) NOT NULL,
			action VARCHAR(64) NOT NULL,
			description TEXT,
			conditions JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(source_account_id, resource, action)
		);

		CREATE INDEX IF NOT EXISTS idx_acl_permissions_source_account ON acl_permissions(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_acl_permissions_resource_action ON acl_permissions(resource, action);

		CREATE TABLE IF NOT EXISTS acl_role_permissions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			role_id UUID NOT NULL REFERENCES acl_roles(id) ON DELETE CASCADE,
			permission_id UUID NOT NULL REFERENCES acl_permissions(id) ON DELETE CASCADE,
			granted BOOLEAN NOT NULL DEFAULT true,
			conditions JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(role_id, permission_id)
		);

		CREATE INDEX IF NOT EXISTS idx_acl_role_permissions_role ON acl_role_permissions(role_id);
		CREATE INDEX IF NOT EXISTS idx_acl_role_permissions_permission ON acl_role_permissions(permission_id);

		CREATE TABLE IF NOT EXISTS acl_user_roles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			user_id VARCHAR(255) NOT NULL,
			role_id UUID NOT NULL REFERENCES acl_roles(id) ON DELETE CASCADE,
			granted_by VARCHAR(255),
			expires_at TIMESTAMPTZ,
			scope VARCHAR(255),
			scope_id VARCHAR(255),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(source_account_id, user_id, role_id, scope, scope_id)
		);

		CREATE INDEX IF NOT EXISTS idx_acl_user_roles_user_id ON acl_user_roles(user_id);
		CREATE INDEX IF NOT EXISTS idx_acl_user_roles_role_id ON acl_user_roles(role_id);

		CREATE TABLE IF NOT EXISTS acl_policies (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			name VARCHAR(255) NOT NULL,
			description TEXT,
			effect VARCHAR(8) NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow','deny')),
			principal_type VARCHAR(32) NOT NULL CHECK (principal_type IN ('role','user','group')),
			principal_value VARCHAR(255) NOT NULL,
			resource_pattern VARCHAR(255) NOT NULL,
			action_pattern VARCHAR(255) NOT NULL,
			conditions JSONB NOT NULL DEFAULT '{}',
			priority INTEGER NOT NULL DEFAULT 0,
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_acl_policies_source_account ON acl_policies(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_acl_policies_priority ON acl_policies(priority DESC);
		CREATE INDEX IF NOT EXISTS idx_acl_policies_enabled ON acl_policies(enabled);

		CREATE TABLE IF NOT EXISTS acl_webhook_events (
			id VARCHAR(255) PRIMARY KEY,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			event_type VARCHAR(128),
			payload JSONB,
			processed BOOLEAN NOT NULL DEFAULT false,
			processed_at TIMESTAMPTZ,
			error TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_acl_webhook_events_processed ON acl_webhook_events(processed);
	`

	_, err := d.Pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	log.Println("schema initialized")
	return nil
}
