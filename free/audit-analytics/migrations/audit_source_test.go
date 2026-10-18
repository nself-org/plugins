// Purpose: pin the contract between audit-analytics/siem (readers of public.np_audit_log) and audit-log (the writer).
// Inputs: the SQL and Go sources of the three plugins, read from the repo tree.
// Outputs: test failures when a reader queries np_audit_log but no shipped plugin publishes it (hq debt D-0959).
package migrations

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readDir(t *testing.T, dir, suffix string) map[string]string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	out := map[string]string{}
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, suffix) || strings.HasSuffix(n, ".down.sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		out[n] = string(b)
	}
	return out
}

// The readers query np_audit_log; audit-log must publish it, with every column the readers select.
func TestAuditLogPublishesNpAuditLog(t *testing.T) {
	var view string
	for _, sql := range readDir(t, filepath.Join("..", "..", "audit-log", "migrations"), ".sql") {
		if regexp.MustCompile(`(?is)CREATE\s+(OR\s+REPLACE\s+)?(VIEW|TABLE)\s+(IF\s+NOT\s+EXISTS\s+)?(public\.)?np_audit_log\b`).MatchString(sql) {
			view = sql
		}
	}
	if view == "" {
		t.Fatal("no audit-log migration creates public.np_audit_log, but audit-analytics and siem read it")
	}
	// Columns the readers use: scorer.go, top_actors.go, 002 heatmap, siem reader.go. A select item is "expr AS name" or a bare "e.name".
	have := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?im)\bAS\s+(\w+)\s*,?\s*$|^\s*e\.(\w+)\s*,?\s*$`).FindAllStringSubmatch(view, -1) {
		have[m[1]+m[2]] = true
	}
	for _, col := range []string{"id", "tenant_id", "user_id", "action", "table_name", "record_id", "session_data", "context", "created_at"} {
		if !have[col] {
			t.Errorf("np_audit_log view does not expose column %q", col)
		}
	}
}

// 002 skips the heatmap view when np_audit_log is absent and its ledger row is never retried, so a later migration must retry it.
func TestHeatmapViewCreationIsRetried(t *testing.T) {
	creators := 0
	for name, sql := range readDir(t, ".", ".sql") {
		if strings.Contains(sql, "CREATE MATERIALIZED VIEW public.np_audit_heatmap") {
			creators++
			if !strings.Contains(sql, "to_regclass('public.np_audit_log') IS NOT NULL") {
				t.Errorf("%s creates the heatmap without the np_audit_log guard", name)
			}
		}
	}
	if creators < 2 {
		t.Fatalf("heatmap view is created by %d migration(s); need a later one to retry after 002 skips it", creators)
	}
}
