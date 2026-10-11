package db

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// legacySchemaRef matches a schema-qualified reference to the old schema,
// e.g. "nself_controller.projects". The Postgres role of the same name is
// referenced without a trailing dot (GRANT ... TO nself_controller) and does
// not match.
var legacySchemaRef = regexp.MustCompile(`\bnself_controller\.`)

func TestEmbeddedMigrationsUseNewSchema(t *testing.T) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded migrations found")
	}
	for _, e := range entries {
		data, err := fs.ReadFile(migrationFS, "migrations/"+e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := string(data)
		if m := legacySchemaRef.FindString(src); m != "" {
			t.Errorf("%s still has schema-qualified legacy reference %q", e.Name(), m)
		}
		if strings.Contains(src, "SCHEMA IF NOT EXISTS nself_controller") {
			t.Errorf("%s still creates schema nself_controller", e.Name())
		}
		if strings.Contains(src, "SCHEMA nself_controller") {
			t.Errorf("%s still references SCHEMA nself_controller", e.Name())
		}
	}
	data, _ := fs.ReadFile(migrationFS, "migrations/001_controller_schema.sql")
	if !strings.Contains(string(data), "CREATE SCHEMA IF NOT EXISTS np_tenant_controller") {
		t.Error("embedded 001 must create np_tenant_controller")
	}
}

// TestGoSQLUsesNewSchema scans the Go sources under go/internal and go/cmd for
// schema-qualified legacy references. Role names and Prometheus metric names
// (nself_controller_*) are allowed.
func TestGoSQLUsesNewSchema(t *testing.T) {
	roots := []string{"..", filepath.Join("..", "..", "cmd")}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(data), "\n") {
				if legacySchemaRef.MatchString(line) {
					t.Errorf("%s:%d has schema-qualified legacy reference: %s", path, i+1, strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

func TestRoleGrantsStillTargetLegacyRole(t *testing.T) {
	data, err := os.ReadFile("db.go")
	if err != nil {
		t.Fatalf("read db.go: %v", err)
	}
	src := string(data)
	for _, want := range []string{
		"GRANT USAGE ON SCHEMA %s TO nself_controller",
		"GRANT SELECT ON ALL TABLES IN SCHEMA %s TO nself_controller",
		"REVOKE ALL ON SCHEMA %s FROM nself_controller",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("db.go lost role grant %q (the role name must stay nself_controller)", want)
		}
	}
	if controllerSchema != "np_tenant_controller" || legacyControllerSchema != "nself_controller" {
		t.Errorf("unexpected schema constants: %q %q", controllerSchema, legacyControllerSchema)
	}
}

func TestRenameSQLIsGuarded(t *testing.T) {
	for _, want := range []string{
		"pg_namespace",
		"RAISE EXCEPTION",
		"ALTER SCHEMA nself_controller RENAME TO np_tenant_controller",
	} {
		if !strings.Contains(renameLegacySchemaSQL, want) {
			t.Errorf("renameLegacySchemaSQL missing %q", want)
		}
	}
	if strings.Contains(strings.ToUpper(renameLegacySchemaSQL), "DROP ") {
		t.Error("rename SQL must never remove anything")
	}
}

// TestTopLevelRenameMigrationMatchesBootstrap keeps migrations/003 in sync with
// the Go bootstrap statement (whitespace-insensitive body comparison).
func TestTopLevelRenameMigrationMatchesBootstrap(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "003_rename_controller_schema.sql"))
	if err != nil {
		t.Fatalf("read 003: %v", err)
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	var body []string
	for _, l := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "--") {
			body = append(body, l)
		}
	}
	if norm(strings.Join(body, "\n")) != norm(renameLegacySchemaSQL) {
		t.Error("migrations/003_rename_controller_schema.sql drifted from renameLegacySchemaSQL")
	}
}
