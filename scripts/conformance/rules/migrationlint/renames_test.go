package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenames(t *testing.T) {
	cases := []struct {
		name    string
		sql     string
		want    []string
		errFrag string // non-empty: a problem containing this text is expected
	}{
		{"none", "CREATE TABLE IF NOT EXISTS a (x int);", []string{}, ""},
		{"literal", "ALTER TABLE IF EXISTS public.alpha_items RENAME TO np_alpha_items2;", []string{"alpha_items np_alpha_items2"}, ""},
		{"literal only and star", "ALTER TABLE ONLY a * RENAME TO b;", []string{"a b"}, ""},
		{"quoted and other schema", `ALTER TABLE "Old" RENAME TO "new";` + "\nALTER TABLE s.t RENAME TO u;", []string{"Old new", "s.t u"}, ""},
		{"public stripped", "ALTER TABLE public.a RENAME TO b;", []string{"a b"}, ""},
		{"literal in do block", "DO $$ BEGIN IF to_regclass('public.a') IS NOT NULL THEN ALTER TABLE public.a RENAME TO b; END IF; END $$;", []string{"a b"}, ""},
		{"column and constraint renames are not tables", "ALTER TABLE a RENAME COLUMN x TO y;\nALTER TABLE a RENAME CONSTRAINT c TO d;\nALTER TABLE a RENAME x TO y;", []string{}, ""},
		{"index sequence view", "ALTER INDEX i RENAME TO j;\nALTER SEQUENCE s RENAME TO t;\nALTER VIEW v RENAME TO w;\nALTER MATERIALIZED VIEW m RENAME TO n;", []string{}, ""},
		{"comment mentioning a rename", "-- ALTER TABLE a RENAME TO b;\nCOMMENT ON TABLE c IS 'ALTER TABLE a RENAME TO b';", []string{}, ""},
		{"dynamic", "DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES ('a', 'np_a'), ('b', 'np_b')) AS v(o, n) LOOP EXECUTE format('ALTER TABLE public.%I RENAME TO %I', t.o, t.n); END LOOP; END $$;", []string{"a np_a", "b np_b"}, ""},
		{"dynamic with leading commas, index rename ok", "DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES\n ('a', 'np_a')\n , ('b', 'np_b')\n) AS v(o, n) LOOP EXECUTE format('alter table public.%I\n rename to %I', t.o, t.n); EXECUTE format('ALTER INDEX public.%I RENAME TO %I', 'x', 'y'); END LOOP; END $$;", []string{"a np_a", "b np_b"}, ""},
		{"dynamic without values", "DO $$ BEGIN EXECUTE format('ALTER TABLE public.%I RENAME TO %I', 'a', 'b'); END $$;", []string{}, "dynamic table rename with no"},
		{"dynamic with non-pair values", "DO $$ BEGIN EXECUTE format('ALTER TABLE %I RENAME TO %I', 'a', 'b'); PERFORM 1 FROM (VALUES (1, 2), ('a')) AS v(x, y); END $$;", []string{}, "dynamic table rename with no"},
		{"string concat rename is unrecognised", "DO $$ BEGIN EXECUTE 'ALTER TABLE ' || 'a' || ' RENAME TO ' || 'b'; END $$;", []string{}, "unrecognised RENAME TO inside a string"},
		{"alter schema rename", "ALTER SCHEMA a RENAME TO b;", []string{}, "unrecognised RENAME TO"},
		{"alter function rename", "ALTER FUNCTION f() RENAME TO g;", []string{}, "unrecognised RENAME TO"},
		{"alter type rename in do string", "DO $$ BEGIN EXECUTE 'ALTER TYPE t RENAME TO u'; END $$;", []string{}, "unrecognised RENAME TO inside a string"},
		{"values in a non-rename do is ignored", "DO $$ BEGIN PERFORM 1 FROM (VALUES ('a', 'b')) AS v(x, y); END $$;", []string{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, errs := renames(c.sql)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("pairs = %q, want %q", got, c.want)
			}
			switch {
			case c.errFrag == "" && len(errs) > 0:
				t.Errorf("unexpected problems: %v", errs)
			case c.errFrag != "":
				found := false
				for _, e := range errs {
					found = found || strings.Contains(e.Msg, c.errFrag)
				}
				if !found {
					t.Errorf("problems %v lack %q", errs, c.errFrag)
				}
			}
		})
	}
}

func TestRunUsage(t *testing.T) {
	if run(nil) != 2 || run([]string{"bogus", "x"}) != 2 || run([]string{"idempotent"}) != 2 {
		t.Error("usage errors must exit 2")
	}
	if run([]string{"idempotent", "/nonexistent/file.sql"}) != 2 {
		t.Error("an unreadable file must exit 2")
	}
}
