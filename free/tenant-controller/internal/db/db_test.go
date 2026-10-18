package db

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/nself-tenant-controller/migrations"
)

// legacySchemaRef matches a schema-qualified reference to the old schema,
// e.g. "nself_controller.projects". The Postgres role of the same name is
// referenced without a trailing dot (GRANT ... TO nself_controller) and does
// not match.
var legacySchemaRef = regexp.MustCompile(`\bnself_controller\.`)

func TestEmbeddedMigrationsUseNewSchema(t *testing.T) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded migrations found")
	}
	for _, e := range entries {
		// 003 is the rename migration itself and names the legacy schema on purpose; the go:embed pattern also
		// matches embed.go's neighbours only, so skip nothing else.
		if strings.HasPrefix(e.Name(), "003_") {
			continue
		}
		data, err := fs.ReadFile(migrations.FS, e.Name())
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
	data, _ := fs.ReadFile(migrations.FS, "001_controller_schema.sql")
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

// TestRenameMigrationOnlyGuards pins migrations/003 to a guard: the rename runs in Go before the boot ledger
// creates np_tenant_controller, so the SQL file must not rename anything (the legacy-renames gate also
// rejects an ALTER SCHEMA ... RENAME it cannot read).
func TestRenameMigrationOnlyGuards(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "003_rename_controller_schema.sql"))
	if err != nil {
		t.Fatalf("read 003: %v", err)
	}
	up := strings.ToUpper(string(data))
	if strings.Contains(up, "RENAME TO") {
		t.Error("migrations/003 must not rename; db.Migrate does it before the boot migrations")
	}
	if !strings.Contains(up, "RAISE EXCEPTION") {
		t.Error("migrations/003 must fail when the legacy schema is still present")
	}
}
