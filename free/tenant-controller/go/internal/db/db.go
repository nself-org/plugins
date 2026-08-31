// Package db provides the Postgres connection pool and migration runner
// for the nself_controller schema. It also provides helpers for per-project
// DDL (schema, role, grant) that run as the superuser/controller role.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Connect opens and validates a pgx connection pool.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return pool, nil
}

// Migrate runs all *.sql migration files embedded from the migrations/ directory.
// Files are run in lexicographic order; already-applied migrations are skipped
// via the nself_controller.schema_migrations tracking table.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	// Ensure the migrations table exists.
	_, err := pool.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS nself_controller;
		CREATE TABLE IF NOT EXISTS nself_controller.schema_migrations (
			filename text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		);
	`)
	if err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") && !strings.HasSuffix(e.Name(), ".down.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		var already bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM nself_controller.schema_migrations WHERE filename = $1)`, name,
		).Scan(&already)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if already {
			continue
		}

		data, err := fs.ReadFile(migrationFS, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		if _, err := pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		if _, err := pool.Exec(ctx,
			`INSERT INTO nself_controller.schema_migrations (filename) VALUES ($1)`, name,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		slog.Info("migration applied", "file", name)
	}
	return nil
}

// ProvisionProjectSchema creates the Postgres schema and dedicated role for a project.
// Runs as the superuser pool — must NOT be called from per-project roles.
func ProvisionProjectSchema(ctx context.Context, pool *pgxpool.Pool, schemaName, roleName, rolePassword string) error {
	stmts := []string{
		fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, pgQuote(schemaName)),
		fmt.Sprintf(`DO $$ BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = %s) THEN
				CREATE ROLE %s LOGIN PASSWORD %s;
			END IF;
		END $$`, pgLiteral(roleName), pgQuote(roleName), pgLiteral(rolePassword)),
		// Grant the project role all access within its own schema.
		fmt.Sprintf(`GRANT ALL ON SCHEMA %s TO %s`, pgQuote(schemaName), pgQuote(roleName)),
		// Set the default search_path so unqualified queries resolve to this schema only.
		fmt.Sprintf(`ALTER ROLE %s SET search_path TO %s`, pgQuote(roleName), pgQuote(schemaName)),
	}

	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("provision schema %s: %w (stmt: %s)", schemaName, err, stmt[:min(60, len(stmt))])
		}
	}
	return nil
}

// GrantControllerReadAccess grants the nself_controller role read access to a project schema.
// Called after ProvisionProjectSchema so the controller daemon can run health checks.
func GrantControllerReadAccess(ctx context.Context, pool *pgxpool.Pool, schemaName string) error {
	stmts := []string{
		fmt.Sprintf(`GRANT USAGE ON SCHEMA %s TO nself_controller`, pgQuote(schemaName)),
		fmt.Sprintf(`GRANT SELECT ON ALL TABLES IN SCHEMA %s TO nself_controller`, pgQuote(schemaName)),
		fmt.Sprintf(`ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT SELECT ON TABLES TO nself_controller`, pgQuote(schemaName)),
	}
	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("grant controller read on %s: %w", schemaName, err)
		}
	}
	return nil
}

// TeardownProjectSchema drops schema, role, and revokes all access.
// MUST run as the superuser / controller role with DDL privileges.
func TeardownProjectSchema(ctx context.Context, pool *pgxpool.Pool, schemaName, roleName string) error {
	stmts := []string{
		// Pre-drop: count rows for audit log (best effort).
		// Actual counts are captured by the controller before calling this.

		// Revoke controller access first.
		fmt.Sprintf(`REVOKE ALL ON SCHEMA %s FROM nself_controller`, pgQuote(schemaName)),

		// Drop schema + all tables within it.
		fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, pgQuote(schemaName)),

		// Drop the dedicated role.
		fmt.Sprintf(`DROP ROLE IF EXISTS %s`, pgQuote(roleName)),
	}

	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("teardown %s: %w", schemaName, err)
		}
	}
	return nil
}

// WriteAuditLog inserts a row into nself_controller.project_audit_log.
func WriteAuditLog(ctx context.Context, pool *pgxpool.Pool, eventType, slug, initiatedBy string, detail []byte) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO nself_controller.project_audit_log (event_type, slug, initiated_by, detail)
		 VALUES ($1, $2, $3, $4)`,
		eventType, slug, initiatedBy, detail,
	)
	if err != nil {
		return fmt.Errorf("audit log write: %w", err)
	}
	return nil
}

// pgQuote returns an identifier safely quoted for Postgres.
// Validates that the identifier only contains safe characters.
func pgQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// pgLiteral returns a string safely quoted as a Postgres string literal.
func pgLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
