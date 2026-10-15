package main

import (
	"strings"
	"testing"
)

// D-0619 F4: UPDATE, DELETE and TRUNCATE are not provably replay-safe; the lint fails them (a real one needs an entry in
// exceptions.yaml). A guarded DO block still covers them, and SELECT/INSERT ... ON CONFLICT stay clean.
func TestLintDataStatements(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string // "line:message-prefix" per violation, in order; nil = clean
	}{
		{"F4 update", "UPDATE t SET a = a + 1;", []string{"1:UPDATE"}},
		{"F4 delete", "\nDELETE FROM t WHERE a = 1;", []string{"2:DELETE"}},
		{"F4 truncate", "TRUNCATE TABLE t;\nTRUNCATE t;", []string{"1:TRUNCATE", "2:TRUNCATE"}},
		{"F4 with-update", "WITH x AS (SELECT 1) UPDATE t SET a = 1;", []string{"1:UPDATE"}},
		{"F4 with-delete", "WITH x AS (SELECT 1) DELETE FROM t;", []string{"1:DELETE"}},
		{"F4 unguarded DO body", "DO $$ BEGIN UPDATE t SET a = 1; END $$;", []string{"1:UPDATE"}},
		{"F4 guarded DO body is covered", "DO $$ BEGIN IF to_regclass('public.t') IS NOT NULL THEN UPDATE t SET a = 1; END IF; END $$;", nil},
		{"F4 EXCEPTION-protected DO body is covered", "DO $$ BEGIN DELETE FROM t; EXCEPTION WHEN others THEN NULL; END $$;", nil},
		{"F4 plain select and insert on conflict stay clean", "SELECT 1;\nINSERT INTO t VALUES (1) ON CONFLICT DO NOTHING;", nil},
		{"F4 update word inside a trigger statement", "DROP TRIGGER IF EXISTS g ON t;\nCREATE TRIGGER g BEFORE UPDATE ON t FOR EACH ROW EXECUTE FUNCTION f();", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := lintLines(c.sql)
			if len(got) != len(c.want) {
				t.Fatalf("got %d violations %q, want %d %q", len(got), got, len(c.want), c.want)
			}
			for i := range got {
				if !strings.HasPrefix(got[i], c.want[i]) {
					t.Errorf("violation %d = %q, want prefix %q", i, got[i], c.want[i])
				}
			}
		})
	}
}
