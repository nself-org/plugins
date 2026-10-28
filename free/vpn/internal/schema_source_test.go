package internal

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/nself-vpn/migrations"
)

var runtimeDDL = regexp.MustCompile(`(?i)\b(CREATE|ALTER|DROP)\s+(OR\s+REPLACE\s+)?(TABLE|VIEW|INDEX|UNIQUE\s+INDEX|EXTENSION)\b`)

// The boot migrations are the only source of the schema: no non-test Go source in this package may run DDL.
func TestNoRuntimeDDL(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if loc := runtimeDDL.FindIndex(b); loc != nil {
			t.Errorf("%s runs DDL (%q): move it into migrations/", f, b[loc[0]:loc[1]])
		}
	}
}

// The four reporting views live in the migrations (they used to exist only through InitSchema).
func TestMigrationsCreateViews(t *testing.T) {
	var all strings.Builder
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		b, err := fs.ReadFile(migrations.FS, f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	for _, v := range []string{"vpn_active_connections", "vpn_server_stats", "vpn_download_history", "vpn_provider_uptime"} {
		if !strings.Contains(all.String(), "CREATE OR REPLACE VIEW public."+v+" ") {
			t.Errorf("no migration creates view %s", v)
		}
	}
}
