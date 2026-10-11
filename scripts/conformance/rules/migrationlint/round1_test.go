package main

import (
	"reflect"
	"strings"
	"testing"
)

// Fix round 1 (plugins#168 review): S3 S4 S5 N1 N5. Every case here failed before the change.

func TestLintRound1(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string // "line:message-prefix" per violation, in order; nil = clean
	}{
		// S4: an IF probe guards only the statements inside IF..END IF
		{"S4 unrelated probe, ddl outside the IF", "DO $$ BEGIN IF EXISTS (SELECT 1) THEN NULL; END IF; CREATE INDEX idx ON t(a); END $$;", []string{"1:CREATE INDEX without"}},
		{"S4 insert outside the IF", "DO $$ BEGIN IF (SELECT count(*) FROM pg_class) > 0 THEN INSERT INTO t VALUES (1); END IF; INSERT INTO t VALUES (2); END $$;", []string{"1:INSERT without"}},
		{"S4 execute outside the IF", "DO $$ BEGIN IF to_regclass('public.a') IS NOT NULL THEN NULL; END IF; EXECUTE 'CREATE TABLE x (a int)'; END $$;", []string{"1:unguarded dynamic SQL"}},
		{"S4 statements inside the IF stay guarded", "DO $$ BEGIN IF to_regclass('public.a') IS NOT NULL THEN CREATE INDEX idx ON a(x); EXECUTE 'ANALYZE a'; ELSE CREATE TABLE b (x int); END IF; END $$;", nil},
		{"S4 nested IF", "DO $$ BEGIN IF to_regclass('public.a') IS NOT NULL THEN IF true THEN CREATE INDEX idx ON a(x); END IF; END IF; END $$;", nil},
		{"S4 guard clause with RETURN guards the rest", "DO $$ BEGIN IF to_regclass('public.a') IS NULL THEN RETURN; END IF; EXECUTE 'ALTER TABLE a ADD COLUMN y int'; END $$;", nil},
		{"S4 statement before the guard clause is linted", "DO $$ BEGIN CREATE INDEX idx ON a(x); IF to_regclass('public.a') IS NULL THEN RETURN; END IF; END $$;", []string{"1:CREATE INDEX without"}},
		{"S4 nested BEGIN with EXCEPTION protects only itself", "DO $$ BEGIN CREATE INDEX i1 ON a(x); BEGIN CREATE INDEX i2 ON a(y); EXCEPTION WHEN duplicate_table THEN NULL; END; END $$;", []string{"1:CREATE INDEX without"}},
		{"S4 EXCEPTION block alone is protected", "DO $$ BEGIN CREATE INDEX i1 ON a(x); EXCEPTION WHEN duplicate_table THEN NULL; END $$;", nil},
		{"S4 loop is not a guard", "DO $$ BEGIN FOR r IN SELECT 1 LOOP EXECUTE 'x'; END LOOP; END $$;", []string{"1:unguarded dynamic SQL"}},
		// S5
		{"S5 add value", "ALTER TYPE e ADD VALUE 'b';\nALTER TYPE e ADD VALUE IF NOT EXISTS 'c';", []string{"1:ALTER TYPE ... ADD VALUE needs"}},
		{"S5 rename value", "ALTER TYPE e RENAME VALUE 'a' TO 'b';", []string{"1:ALTER TYPE ... RENAME VALUE"}},
		{"S5 set schema", "ALTER TABLE t SET SCHEMA s;\nALTER TABLE IF EXISTS t SET SCHEMA s;", []string{"1:ALTER TABLE ... SET SCHEMA"}},
		{"S5 attach partition", "ALTER TABLE p ATTACH PARTITION c FOR VALUES IN (1);", []string{"1:ATTACH PARTITION"}},
		{"S5 add generated identity", "ALTER TABLE t ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY;\nALTER TABLE t ALTER COLUMN id SET DEFAULT 1;", []string{"1:ADD GENERATED"}},
		{"S5 select into", "SELECT * INTO newt FROM t;\nSELECT a, (SELECT 1) FROM t;", []string{"1:SELECT ... INTO"}},
		{"S5 select into inside a guarded do is a plpgsql assignment", "DO $$ DECLARE v int; BEGIN IF to_regclass('public.t') IS NOT NULL THEN SELECT count(*) INTO v FROM t; END IF; END $$;", nil},
		{"S5 select into variable in an unguarded do is not flagged", "DO $$ DECLARE v int; BEGIN SELECT count(*) INTO v FROM t; END $$;", nil},
		{"S5 guarded do may do these", "DO $$ BEGIN IF to_regclass('public.t') IS NOT NULL THEN ALTER TABLE t SET SCHEMA s; END IF; END $$;", nil},
		// N1: the DROP licenses a CREATE/ADD on the same table only
		{"N1 trigger other table", "DROP TRIGGER IF EXISTS tg ON u;\nCREATE TRIGGER tg BEFORE UPDATE ON t FOR EACH ROW EXECUTE FUNCTION f();", []string{"2:CREATE TRIGGER tg needs"}},
		{"N1 trigger same table", "DROP TRIGGER IF EXISTS tg ON public.t;\nCREATE TRIGGER tg BEFORE INSERT OR UPDATE OF a ON t FOR EACH ROW EXECUTE FUNCTION f();", nil},
		{"N1 policy other table", "DROP POLICY IF EXISTS p ON u;\nCREATE POLICY p ON t USING (true);", []string{"2:CREATE POLICY p needs"}},
		{"N1 policy same table", "DROP POLICY IF EXISTS p ON t;\nCREATE POLICY p ON t USING (true);", nil},
		{"N1 constraint other table", "ALTER TABLE t DROP CONSTRAINT IF EXISTS c;\nALTER TABLE u ADD CONSTRAINT c CHECK (x > 0);", []string{"2:ADD CONSTRAINT c needs"}},
		{"N1 constraint same table", "ALTER TABLE public.t DROP CONSTRAINT IF EXISTS c;\nALTER TABLE t ADD CONSTRAINT c CHECK (x > 0);", nil},
		// N5
		{"N5 byte order mark", "\xef\xbb\xbfCREATE INDEX i ON t(a);", []string{"1:CREATE INDEX without"}},
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

// S3: a rename hidden in any dollar-quoted or function body is seen; one it cannot classify fails closed.
func TestRenamesBodies(t *testing.T) {
	cases := []struct {
		name    string
		sql     string
		want    []string
		errFrag string
	}{
		{"function body literal", "CREATE OR REPLACE FUNCTION mv() RETURNS void AS $f$ BEGIN ALTER TABLE IF EXISTS np_x RENAME TO y; END $f$ LANGUAGE plpgsql;\nSELECT mv();", []string{"np_x y"}, ""},
		{"function body dynamic with values", "CREATE OR REPLACE FUNCTION mv() RETURNS void AS $f$ BEGIN PERFORM 1 FROM (VALUES ('a', 'b')) AS v(o, n); EXECUTE format('ALTER TABLE %I RENAME TO %I', 'a', 'b'); END $f$ LANGUAGE plpgsql;", []string{"a b"}, ""},
		{"function body nested tag dynamic without values", "CREATE FUNCTION mv() RETURNS void AS $f$ BEGIN EXECUTE format($q$ALTER TABLE %I RENAME TO %I$q$, 'a', 'b'); END $f$ LANGUAGE plpgsql;", []string{}, "dynamic table rename with no"},
		{"function body schema rename", "CREATE FUNCTION mv() RETURNS void AS $f$ BEGIN ALTER SCHEMA a RENAME TO b; END $f$ LANGUAGE plpgsql;", []string{}, "unrecognised RENAME TO"},
		{"procedure string body", "CREATE PROCEDURE mv() LANGUAGE plpgsql AS 'BEGIN ALTER TABLE a RENAME TO b; END';", []string{"a b"}, ""},
		{"do body nested tag dynamic with values", "DO $$ BEGIN FOR t IN SELECT o, n FROM (VALUES ('a', 'b')) AS v(o, n) LOOP EXECUTE format($q$ALTER TABLE %I RENAME TO %I$q$, t.o, t.n); END LOOP; END $$;", []string{"a b"}, ""},
		{"function body without rename", "CREATE FUNCTION f() RETURNS int AS $f$ SELECT 1; $f$ LANGUAGE sql;", []string{}, ""},
		{"any body is read as code (conservative)", "COMMENT ON TABLE a IS $c$ ALTER TABLE a RENAME TO b $c$;", []string{"a b"}, ""},
		{"unreadable body statement fails closed", "COMMENT ON TABLE a IS $c$ RENAME TO b $c$;", []string{}, "unrecognised RENAME TO"},
		{"byte order mark", "\xef\xbb\xbfALTER TABLE a RENAME TO b;", []string{"a b"}, ""},
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
