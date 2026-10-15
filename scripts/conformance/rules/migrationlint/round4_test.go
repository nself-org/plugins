package main

import (
	"reflect"
	"strings"
	"testing"
)

// D-0619 F1, F2, F3, F5 (idempotency lint) and F6, F7 (rename extractor). Every "want" case below passed (exit 0) before the
// change; the clean cases pin the guards that stay accepted.
func TestLintRound4(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string // "line:message-prefix" per violation, in order; nil = clean
	}{
		// F1: guards are per branch and fit the statement
		{"F1 probe in one IF, unrelated IF true", "DO $$ BEGIN IF to_regclass('public.a') IS NULL THEN NULL; END IF; IF true THEN CREATE INDEX k ON a(c); END IF; END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 ELSE branch is not covered", "DO $$ BEGIN IF to_regclass('public.k') IS NULL THEN NULL; ELSE CREATE INDEX k ON a(c); END IF; END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 ELSIF without its own probe", "DO $$ BEGIN IF to_regclass('public.k') IS NULL THEN NULL; ELSIF x > 1 THEN CREATE INDEX k ON a(c); END IF; END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 ELSIF with its own probe", "DO $$ BEGIN IF x > 1 THEN NULL; ELSIF to_regclass('public.k') IS NULL THEN CREATE INDEX k ON a(c); END IF; END $$;", nil},
		{"F1 guard on the table, not the index (RETURN when the table is missing)", "DO $$ BEGIN IF to_regclass('public.t') IS NULL THEN RETURN; END IF; CREATE INDEX k ON t(c); END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 guard on the table, not the index (IF the table exists)", "DO $$ BEGIN IF to_regclass('public.t') IS NOT NULL THEN CREATE INDEX k ON t(c); END IF; END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 guard on another object", "DO $$ BEGIN IF to_regclass('public.other') IS NULL THEN CREATE INDEX k ON t(c); END IF; END $$;", []string{"1:CREATE INDEX without"}},
		{"F1 the index itself is probed", "DO $$ BEGIN IF to_regclass('public.k') IS NULL THEN CREATE INDEX k ON t(c); END IF; END $$;", nil},
		{"F1 early RETURN when the index exists", "DO $$ BEGIN IF to_regclass('public.k') IS NOT NULL THEN RETURN; END IF; CREATE INDEX k ON t(c); END $$;", nil},
		{"F1 a drop needs the exists polarity", "DO $$ BEGIN IF to_regclass('public.x') IS NULL THEN DROP TABLE x; END IF; END $$;", []string{"1:DROP without"}},
		{"F1 a drop under an exists guard", "DO $$ BEGIN IF to_regclass('public.x') IS NOT NULL THEN DROP TABLE x; END IF; END $$;", nil},
		{"F1 label before a nested BEGIN keeps the scopes", "DO $$ BEGIN IF to_regclass('public.k') IS NULL THEN <<b>> BEGIN NULL; END b; END IF; CREATE INDEX k ON t(c); END $$;", []string{"1:CREATE INDEX without"}},
		// F2: the handler has to catch the replay error and not re-raise
		{"F2 unique_violation around CREATE TABLE", "DO $$ BEGIN CREATE TABLE t(c int); EXCEPTION WHEN unique_violation THEN NULL; END $$;", []string{"1:CREATE TABLE without"}},
		{"F2 handler that re-raises", "DO $$ BEGIN CREATE TABLE t(c int); EXCEPTION WHEN duplicate_table THEN RAISE; END $$;", []string{"1:CREATE TABLE without"}},
		{"F2 handler that raises an exception", "DO $$ BEGIN CREATE TABLE t(c int); EXCEPTION WHEN others THEN RAISE EXCEPTION 'no'; END $$;", []string{"1:CREATE TABLE without"}},
		{"F2 unrelated condition", "DO $$ BEGIN CREATE TABLE t(c int); EXCEPTION WHEN division_by_zero THEN NULL; END $$;", []string{"1:CREATE TABLE without"}},
		{"F2 a later clause catches it", "DO $$ BEGIN CREATE TABLE t(c int); EXCEPTION WHEN division_by_zero THEN NULL; WHEN duplicate_table THEN NULL; END $$;", nil},
		{"F2 a clause that only warns", "DO $$ BEGIN CREATE TABLE t(c int); EXCEPTION WHEN duplicate_table THEN RAISE NOTICE 'exists'; END $$;", nil},
		{"F2 others", "DO $$ BEGIN CREATE TABLE t(c int); EXCEPTION WHEN OTHERS THEN NULL; END $$;", nil},
		{"F2 sqlstate duplicate_table", "DO $$ BEGIN CREATE TABLE t(c int); EXCEPTION WHEN SQLSTATE '42P07' THEN NULL; END $$;", nil},
		{"F2 sqlstate unique violation does not fit DDL", "DO $$ BEGIN CREATE TABLE t(c int); EXCEPTION WHEN SQLSTATE '23505' THEN NULL; END $$;", []string{"1:CREATE TABLE without"}},
		{"F2 unique_violation fits a data statement", "DO $$ BEGIN INSERT INTO t VALUES (1); EXCEPTION WHEN unique_violation THEN NULL; END $$;", nil},
		// F3: each INSERT has its own guard
		{"F3 outer ON CONFLICT does not guard the CTE insert", "WITH a AS (INSERT INTO t VALUES (1) RETURNING c)\nINSERT INTO u SELECT c FROM a ON CONFLICT DO NOTHING;", []string{"1:INSERT without"}},
		{"F3 NOT EXISTS on an unrelated table", "INSERT INTO t SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM unrelated);", []string{"1:INSERT without"}},
		{"F3 NOT EXISTS inside a CTE", "WITH s AS (SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM t)) INSERT INTO t SELECT * FROM s;", []string{"1:INSERT without"}},
		{"F3 NOT EXISTS reading the target", "INSERT INTO public.t (a) SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM public.t WHERE a = 1);", nil},
		{"F3 both inserts guarded", "WITH a AS (INSERT INTO t VALUES (1) ON CONFLICT DO NOTHING RETURNING c) INSERT INTO u SELECT c FROM a ON CONFLICT DO NOTHING;", nil},
		// F5
		{"F5 WITH ... SELECT INTO", "WITH s AS (SELECT 1 AS a) SELECT a INTO newt FROM s;", []string{"1:SELECT ... INTO"}},
		{"F5 WITH ... INSERT INTO is not a select into", "WITH s AS (SELECT 1 AS a) INSERT INTO t SELECT a FROM s ON CONFLICT DO NOTHING;", nil},
		{"F5 WITH ... SELECT INTO a variable in a DO", "DO $$ DECLARE v int; BEGIN WITH s AS (SELECT 1 AS a) SELECT a INTO v FROM s; END $$;", nil},
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

func TestRenamesRound4(t *testing.T) {
	cases := []struct {
		name    string
		sql     string
		want    []string
		errFrag string
	}{
		// F6: comments and spacing inside the string do not hide the rename
		{"F6 comment between RENAME and TO", "DO $$ BEGIN EXECUTE 'ALTER TABLE a RENAME /* c */ TO b'; END $$;", []string{}, "dynamic table rename with no"},
		{"F6 comment inside a format string", "DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES ('a', 'b')) AS v(o, n) LOOP EXECUTE format('ALTER TABLE %I RENAME /* c */ TO %I', t.o, t.n); END LOOP; END $$;", []string{"a b"}, ""},
		{"F6 line comment and newlines", "DO $$ BEGIN EXECUTE 'alter table a rename -- c\n to b'; END $$;", []string{}, "dynamic table rename with no"},
		{"F6 RENAME split from TO", "DO $$ BEGIN EXECUTE 'ALTER TABLE a RENAME ' || 'TO b'; END $$;", []string{}, "unrecognised RENAME"},
		{"F6 column rename in a string is fine", "DO $$ BEGIN EXECUTE 'ALTER TABLE a RENAME COLUMN x TO y'; END $$;", []string{}, ""},
		// F7: the format arguments are tied to the VALUES list
		{"F7 unrelated VALUES list", "DO $$ BEGIN PERFORM 1 FROM (VALUES ('a', 'b')) AS v(x, y); EXECUTE format('ALTER TABLE %I RENAME TO %I', 'c', 'd'); END $$;", []string{"c d"}, ""},
		{"F7 swapped fields", "DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES ('a', 'b')) AS v(o, n) LOOP EXECUTE format('ALTER TABLE %I RENAME TO %I', t.n, t.o); END LOOP; END $$;", []string{}, "dynamic table rename with no"},
		{"F7 arguments from two variables", "DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES ('a', 'b')) AS v(o, n) LOOP EXECUTE format('ALTER TABLE %I RENAME TO %I', t.o, u.n); END LOOP; END $$;", []string{}, "dynamic table rename with no"},
		{"F7 a different loop variable", "DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES ('a', 'b')) AS v(o, n) LOOP EXECUTE format('ALTER TABLE %I RENAME TO %I', r.o, r.n); END LOOP; END $$;", []string{}, "dynamic table rename with no"},
		{"F7 a tuple that is not a pair of strings", "DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES ('a', 'b'), ('c', 'd', 'e')) AS v(o, n) LOOP EXECUTE format('ALTER TABLE %I RENAME TO %I', t.o, t.n); END LOOP; END $$;", []string{}, "dynamic table rename with no"},
		{"F7 format with a third argument", "DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES ('a', 'b')) AS v(o, n) LOOP EXECUTE format('ALTER TABLE %I RENAME TO %I', t.o, t.n, t.x); END LOOP; END $$;", []string{}, "dynamic table rename with no"},
		{"F7 bound pair is read", "DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES ('x', 'y')) AS v(o, n) LOOP EXECUTE format('ALTER TABLE %I RENAME TO %I', t.o, t.n); END LOOP; END $$;", []string{"x y"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, errs := renames(c.sql)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("pairs = %q, want %q", got, c.want)
			}
			found := c.errFrag == ""
			for _, e := range errs {
				found = found || strings.Contains(e.Msg, c.errFrag)
			}
			if !found || (c.errFrag == "" && len(errs) > 0) {
				t.Errorf("problems %v, want fragment %q", errs, c.errFrag)
			}
		})
	}
}
