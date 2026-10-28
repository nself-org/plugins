package internal

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/nself-torrent-manager/migrations"
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

// Every table and view the queries use is created by the migrations.
func TestMigrationsCreateTablesAndViews(t *testing.T) {
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
	for _, n := range []string{
		"np_torrentmanager_torrent_clients", "np_torrentmanager_sources", "np_torrentmanager_torrent_downloads",
		"np_torrentmanager_torrent_files", "np_torrentmanager_torrent_trackers", "np_torrentmanager_search_cache",
		"np_torrentmanager_seeding_policy", "np_torrentmanager_stats", "np_torrentmanager_seeding_policies",
	} {
		if !strings.Contains(all.String(), "CREATE TABLE IF NOT EXISTS public."+n+" ") {
			t.Errorf("no migration creates table %s", n)
		}
	}
	for _, v := range []string{"torrent_active_downloads", "torrent_completed_downloads", "torrent_seeding_torrents"} {
		if !strings.Contains(all.String(), "CREATE OR REPLACE VIEW public."+v+" ") {
			t.Errorf("no migration creates view %s", v)
		}
	}
}
