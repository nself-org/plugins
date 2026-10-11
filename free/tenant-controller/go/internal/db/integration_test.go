//go:build integration

package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run with:
//
//	PGPASSWORD=<throwaway> TC_PG_DSN=postgres://postgres@localhost:55432/postgres \
//	  go test -tags integration ./internal/db/...
//
// Each case runs in its own throwaway database created on the DSN's server.

const legacyLayoutSQL = `
CREATE SCHEMA nself_controller;
CREATE TABLE nself_controller.schema_migrations (
  filename text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE nself_controller.projects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug text NOT NULL UNIQUE,
  domain text NOT NULL UNIQUE,
  schema_name text NOT NULL UNIQUE,
  role_name text NOT NULL UNIQUE,
  hasura_source text NOT NULL UNIQUE,
  hasura_port integer NOT NULL UNIQUE,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleting')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE TABLE nself_controller.project_audit_log (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type text NOT NULL,
  slug text NOT NULL,
  initiated_by text NOT NULL,
  detail jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE nself_controller.port_map (
  port integer PRIMARY KEY,
  slug text NOT NULL REFERENCES nself_controller.projects (slug) ON DELETE CASCADE,
  service text NOT NULL DEFAULT 'hasura',
  allocated_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION nself_controller.set_updated_at() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at := now(); RETURN NEW; END; $$;
CREATE TRIGGER trg_projects_updated_at BEFORE UPDATE ON nself_controller.projects
  FOR EACH ROW EXECUTE FUNCTION nself_controller.set_updated_at();
INSERT INTO nself_controller.projects (slug, domain, schema_name, role_name, hasura_source, hasura_port)
  VALUES ('alpha','alpha.example.test','project_alpha','role_alpha','src_alpha',8101),
         ('beta','beta.example.test','project_beta','role_beta','src_beta',8102);
INSERT INTO nself_controller.schema_migrations (filename) VALUES ('001_controller_schema.sql');
`

func requireDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TC_PG_DSN")
	if dsn == "" {
		t.Skip("TC_PG_DSN not set")
	}
	return dsn
}

// newDB creates a throwaway database and returns a pool connected to it.
func newDB(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	name := fmt.Sprintf("tc_it_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatalf("create database: %v", err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect %s: %v", name, err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		admin.Close()
	})
	return pool
}

func schemaExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname = $1)`, name).Scan(&ok); err != nil {
		t.Fatalf("schemaExists: %v", err)
	}
	return ok
}

func count(t *testing.T, pool *pgxpool.Pool, q string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestMigrateRenamesLegacySchemaLosslessly(t *testing.T) {
	dsn := requireDSN(t)
	ctx := context.Background()
	pool := newDB(t, dsn)

	if _, err := pool.Exec(ctx, legacyLayoutSQL); err != nil {
		t.Fatalf("seed legacy layout: %v", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if schemaExists(t, pool, "nself_controller") {
		t.Error("nself_controller schema should be gone")
	}
	if !schemaExists(t, pool, "np_tenant_controller") {
		t.Fatal("np_tenant_controller schema missing")
	}
	if n := count(t, pool, `SELECT COUNT(*) FROM np_tenant_controller.projects`); n != 2 {
		t.Errorf("projects rows = %d, want 2", n)
	}
	if n := count(t, pool, `SELECT COUNT(*) FROM np_tenant_controller.schema_migrations WHERE filename = '001_controller_schema.sql'`); n != 1 {
		t.Errorf("schema_migrations kept row count = %d, want 1", n)
	}

	// The updated_at trigger must still fire after the rename.
	var before, after time.Time
	if err := pool.QueryRow(ctx, `SELECT updated_at FROM np_tenant_controller.projects WHERE slug='alpha'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := pool.Exec(ctx, `UPDATE np_tenant_controller.projects SET status='suspended' WHERE slug='alpha'`); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT updated_at FROM np_tenant_controller.projects WHERE slug='alpha'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.After(before) {
		t.Errorf("updated_at trigger did not fire: before=%v after=%v", before, after)
	}

	// Second Migrate is a no-op.
	migrationsBefore := count(t, pool, `SELECT COUNT(*) FROM np_tenant_controller.schema_migrations`)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if n := count(t, pool, `SELECT COUNT(*) FROM np_tenant_controller.schema_migrations`); n != migrationsBefore {
		t.Errorf("second Migrate changed schema_migrations: %d -> %d", migrationsBefore, n)
	}
	if n := count(t, pool, `SELECT COUNT(*) FROM np_tenant_controller.projects`); n != 2 {
		t.Errorf("projects rows after second Migrate = %d, want 2", n)
	}
}

func TestMigrateFreshDatabase(t *testing.T) {
	dsn := requireDSN(t)
	ctx := context.Background()
	pool := newDB(t, dsn)

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if schemaExists(t, pool, "nself_controller") {
		t.Error("fresh install must not create nself_controller")
	}
	for _, tbl := range []string{"projects", "project_audit_log", "port_map", "schema_migrations"} {
		if n := count(t, pool, `SELECT COUNT(*) FROM np_tenant_controller.`+tbl); n < 0 {
			t.Errorf("table %s unreadable", tbl)
		}
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestMigrateErrorsWhenBothSchemasExist(t *testing.T) {
	dsn := requireDSN(t)
	ctx := context.Background()
	pool := newDB(t, dsn)

	if _, err := pool.Exec(ctx, `CREATE SCHEMA nself_controller; CREATE SCHEMA np_tenant_controller;`); err != nil {
		t.Fatal(err)
	}
	err := Migrate(ctx, pool)
	if err == nil {
		t.Fatal("Migrate must fail when both schemas exist")
	}
	if !strings.Contains(err.Error(), "nself_controller") || !strings.Contains(err.Error(), "np_tenant_controller") {
		t.Errorf("error should name both schemas: %v", err)
	}
	// Nothing was merged or removed.
	if !schemaExists(t, pool, "nself_controller") || !schemaExists(t, pool, "np_tenant_controller") {
		t.Error("both schemas must still exist after the failed Migrate")
	}
}
