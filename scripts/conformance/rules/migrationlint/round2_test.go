package main

import (
	"strings"
	"testing"
)

// Fix round 2 (plugins#168 delta review): F1 narrow guard RETURN, F2 CASE bodies, F3 handler statements.
func TestLintRound2(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string // "line:message-prefix" per violation, in order; nil = clean
	}{
		// F1: only `IF <probe> THEN RETURN; END IF;` directly in the outermost block guards what follows
		{"F1 RETURN after DDL in the IF", "DO $$ BEGIN IF to_regclass($q$t$q$) IS NULL THEN CREATE TABLE t(a int); RETURN; END IF; CREATE INDEX k ON t(c); END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 RETURN inside a loop IF", "DO $$ BEGIN IF to_regclass($q$t$q$) IS NULL THEN NULL; END IF; FOR r IN SELECT 1 LOOP IF true THEN RETURN; END IF; END LOOP; CREATE INDEX k ON t(c); END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 guard points the wrong way (ELSE RETURN)", "DO $$ BEGIN IF EXISTS (SELECT 1 FROM pg_class) THEN NULL; ELSE RETURN; END IF; CREATE INDEX k ON t(c); END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 RETURN guard without a probe in its condition", "DO $$ BEGIN IF to_regclass('public.t') IS NULL THEN NULL; END IF; IF x > 1 THEN RETURN; END IF; CREATE INDEX k ON t(c); END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 guard nested in another IF", "DO $$ BEGIN IF true THEN IF to_regclass('public.t') IS NULL THEN RETURN; END IF; END IF; CREATE INDEX k ON t(c); END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 guard inside a nested block", "DO $$ BEGIN BEGIN IF to_regclass('public.t') IS NULL THEN RETURN; END IF; END; CREATE INDEX k ON t(c); END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 guard inside a loop", "DO $$ BEGIN FOR r IN SELECT 1 LOOP IF to_regclass('public.t') IS NULL THEN RETURN; END IF; END LOOP; CREATE INDEX k ON t(c); END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 real guard clause still guards", "DO $$ DECLARE v int; BEGIN IF to_regclass('public.t') IS NULL OR NOT EXISTS (SELECT 1 FROM pg_class) THEN RETURN; END IF; CREATE INDEX k ON t(c); EXECUTE 'ANALYZE t'; END $$;", nil},
		// F2: CASE branches and loop bodies are linted unless a guard covers them
		{"F2 CASE branch ddl", "DO $$ BEGIN CASE WHEN true THEN CREATE INDEX k ON t(c); END CASE; END $$;", []string{"1:CREATE INDEX without"}},
		{"F2 CASE branch insert, second WHEN and ELSE", "DO $$ BEGIN CASE WHEN false THEN NULL; WHEN true THEN INSERT INTO t VALUES (1); ELSE INSERT INTO t VALUES (2); END CASE; END $$;", []string{"1:INSERT without", "1:INSERT without"}},
		{"F2 simple CASE", "DO $$ BEGIN CASE v WHEN 1 THEN CREATE INDEX k ON t(c); END CASE; END $$;", []string{"1:CREATE INDEX without"}},
		{"F2 CASE with EXECUTE", "DO $$ BEGIN CASE WHEN true THEN EXECUTE 'CREATE TABLE x (a int)'; END CASE; END $$;", []string{"1:unguarded dynamic SQL"}},
		{"F2 CASE inside a guarded IF stays guarded, END CASE does not close the IF", "DO $$ BEGIN IF to_regclass('public.t') IS NOT NULL THEN CASE WHEN true THEN NULL; END CASE; CREATE INDEX k ON t(c); END IF; END $$;", nil},
		{"F2 END LOOP does not close the IF", "DO $$ BEGIN IF to_regclass('public.t') IS NOT NULL THEN FOR r IN SELECT 1 LOOP NULL; END LOOP; CREATE INDEX k ON t(c); END IF; END $$;", nil},
		{"F2 CASE after the IF closes is unguarded", "DO $$ BEGIN IF to_regclass('public.t') IS NOT NULL THEN NULL; END IF; CASE WHEN true THEN CREATE INDEX k ON t(c); END CASE; END $$;", []string{"1:CREATE INDEX without"}},
		{"F2 WHILE loop body", "DO $$ BEGIN WHILE true LOOP CREATE INDEX k ON t(c); EXIT; END LOOP; END $$;", []string{"1:CREATE INDEX without"}},
		{"F2 bare LOOP body", "DO $$ BEGIN LOOP CREATE INDEX k ON t(c); EXIT; END LOOP; END $$;", []string{"1:CREATE INDEX without"}},
		{"F2 FOR header with a CASE expression", "DO $$ BEGIN FOR r IN SELECT CASE WHEN a THEN 1 ELSE 2 END FROM t LOOP CREATE INDEX k ON t(c); END LOOP; END $$;", []string{"1:CREATE INDEX without"}},
		// F3: an error in the handler is not caught by its own block
		{"F3 handler statement", "DO $$ BEGIN BEGIN NULL; EXCEPTION WHEN others THEN CREATE INDEX x ON t(a); END; END $$;", []string{"1:CREATE INDEX without"}},
		{"F3 second handler statement", "DO $$ BEGIN CREATE INDEX i ON t(a); EXCEPTION WHEN duplicate_table THEN NULL; WHEN others THEN INSERT INTO log VALUES (1); END $$;", []string{"1:INSERT without"}},
		{"F3 handler inside an outer protected block is covered by the outer handler", "DO $$ BEGIN BEGIN NULL; EXCEPTION WHEN others THEN CREATE INDEX x ON t(a); END; EXCEPTION WHEN others THEN NULL; END $$;", nil},
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
