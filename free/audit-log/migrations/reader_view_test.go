// Purpose: the audit-log migrations define public.np_audit_log, the view audit-analytics and siem read (D-0959).
// Constraints: no database; the embedded SQL is checked as text.
package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

// TestReaderViewColumns checks that the view exists in the embedded migrations and exposes every column the
// readers select, read from the canonical table.
func TestReaderViewColumns(t *testing.T) {
	var sql string
	files, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".down.sql") {
			continue
		}
		b, err := fs.ReadFile(FS, f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "VIEW public.np_audit_log") {
			sql = string(b)
		}
	}
	if sql == "" {
		t.Fatal("no migration creates public.np_audit_log")
	}
	cases := []struct{ name, want string }{
		{"reads the canonical table", "FROM public.np_auditlog_events"},
		{"id", " AS id"},
		{"tenant_id", "e.tenant_id"},
		{"user_id", " AS user_id"},
		{"action", " AS action"},
		{"table_name", " AS table_name"},
		{"record_id", " AS record_id"},
		{"session_data", " AS session_data"},
		{"context", " AS context"},
		{"created_at", "e.created_at"},
		{"caller privileges", "security_invoker = true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(sql, tc.want) {
				t.Errorf("view definition lacks %q", tc.want)
			}
		})
	}
}
