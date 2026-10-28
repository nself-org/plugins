package internal

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/nself-web3/migrations"
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

// Every table the handlers query is created by the migrations.
func TestMigrationsCreateTables(t *testing.T) {
	var all strings.Builder
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".down.sql") {
			continue
		}
		b, err := fs.ReadFile(migrations.FS, f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	for _, n := range []string{
		"wallets", "collections", "nfts", "tokens", "token_balances", "token_gates",
		"gate_checks", "daos", "proposals", "votes", "transactions", "events",
	} {
		if !strings.Contains(all.String(), "CREATE TABLE IF NOT EXISTS public.np_web3_"+n+" ") {
			t.Errorf("no migration creates table np_web3_%s", n)
		}
	}
}
