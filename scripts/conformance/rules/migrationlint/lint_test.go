package main

import (
	"fmt"
	"strings"
	"testing"
)

// lintLines renders violations as "line:message" for compact comparison.
func lintLines(src string) []string {
	var out []string
	for _, v := range lintFile(src) {
		out = append(out, fmt.Sprintf("%d:%s", v.Line, v.Msg))
	}
	return out
}

func TestLint(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string // "line:message-prefix" per violation, in order; nil = clean
	}{
		{"create table plain", "CREATE TABLE a (id int);", []string{"1:CREATE TABLE without IF NOT EXISTS"}},
		{"create table ok", "CREATE TABLE IF NOT EXISTS a (id int);", nil},
		{"unlogged and temp", "CREATE UNLOGGED TABLE a (x int);\nCREATE TEMP TABLE IF NOT EXISTS b (x int);", []string{"1:CREATE TABLE without"}},
		{"index plain", "\n\nCREATE INDEX i ON a (x);", []string{"3:CREATE INDEX without"}},
		{"unique index concurrently", "CREATE UNIQUE INDEX CONCURRENTLY i ON a (x);\nCREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS j ON a (x);", []string{"1:CREATE INDEX without"}},
		{"schema extension sequence", "CREATE SCHEMA s; CREATE EXTENSION e; CREATE SEQUENCE q;\nCREATE SCHEMA IF NOT EXISTS s;", []string{"1:CREATE SCHEMA", "1:CREATE EXTENSION", "1:CREATE SEQUENCE"}},
		{"matview", "CREATE MATERIALIZED VIEW m AS SELECT 1;\nCREATE MATERIALIZED VIEW IF NOT EXISTS m AS SELECT 1;", []string{"1:CREATE MATERIALIZED VIEW without"}},
		{"view function", "CREATE VIEW v AS SELECT 1;\nCREATE OR REPLACE VIEW v AS SELECT 1;\nCREATE FUNCTION f() RETURNS int AS $$ SELECT 1; $$ LANGUAGE sql;\nCREATE OR REPLACE FUNCTION f() RETURNS int AS $$ SELECT 1; $$ LANGUAGE sql;", []string{"1:CREATE VIEW needs", "3:CREATE FUNCTION needs"}},
		{"trigger plain", "CREATE TRIGGER t BEFORE UPDATE ON a FOR EACH ROW EXECUTE FUNCTION f();", []string{"1:CREATE TRIGGER t needs"}},
		{"trigger or replace", "CREATE OR REPLACE TRIGGER t BEFORE UPDATE ON a FOR EACH ROW EXECUTE FUNCTION f();", nil},
		{"trigger after drop", "DROP TRIGGER IF EXISTS t ON a;\nCREATE TRIGGER t BEFORE UPDATE ON a FOR EACH ROW EXECUTE FUNCTION f();", nil},
		{"trigger drop after create", "CREATE TRIGGER t BEFORE UPDATE ON a FOR EACH ROW EXECUTE FUNCTION f();\nDROP TRIGGER IF EXISTS t ON a;", []string{"1:CREATE TRIGGER t needs"}},
		{"policy", "CREATE POLICY p ON a USING (true);\nDROP POLICY IF EXISTS q ON a;\nCREATE POLICY q ON a USING (true);", []string{"1:CREATE POLICY p needs"}},
		{"type domain", "CREATE TYPE t AS ENUM ('a');\nCREATE DOMAIN d AS int;", []string{"1:CREATE TYPE", "2:CREATE DOMAIN"}},
		{"unknown create", "CREATE ROLE r;\nCREATE FOREIGN TABLE IF NOT EXISTS f (x int) SERVER s;", []string{"1:CREATE ROLE is not replay-safe"}},
		{"add column", "ALTER TABLE a ADD COLUMN x int;\nALTER TABLE a ADD y int;\nALTER TABLE a ADD COLUMN IF NOT EXISTS z int;", []string{"1:ADD COLUMN without", "2:ADD COLUMN without"}},
		{"add column commas in parens", "ALTER TABLE a ADD COLUMN IF NOT EXISTS x numeric(10,2) DEFAULT round(1.5, 1), ADD COLUMN IF NOT EXISTS y int;", nil},
		{"second action unguarded", "ALTER TABLE a ADD COLUMN IF NOT EXISTS x int, ADD COLUMN y int;", []string{"1:ADD COLUMN without"}},
		{"add constraint plain", "ALTER TABLE a ADD CONSTRAINT c CHECK (x > 0);", []string{"1:ADD CONSTRAINT c needs"}},
		{"add constraint after drop (same statement)", "ALTER TABLE a DROP CONSTRAINT IF EXISTS c, ADD CONSTRAINT c CHECK (x > 0);", nil},
		{"add constraint after drop (earlier statement)", "ALTER TABLE a DROP CONSTRAINT IF EXISTS c;\nALTER TABLE a ADD CONSTRAINT c CHECK (x > 0);", nil},
		{"add constraint drop for other name", "ALTER TABLE a DROP CONSTRAINT IF EXISTS d;\nALTER TABLE a ADD CONSTRAINT c CHECK (x > 0);", []string{"2:ADD CONSTRAINT c needs"}},
		{"unnamed constraints", "ALTER TABLE a ADD PRIMARY KEY (id);\nALTER TABLE a ADD UNIQUE (x);\nALTER TABLE a ADD FOREIGN KEY (x) REFERENCES b;\nALTER TABLE a ADD CHECK (x > 0);", []string{"1:unnamed ADD PRIMARY", "2:unnamed ADD UNIQUE", "3:unnamed ADD FOREIGN", "4:unnamed ADD CHECK"}},
		{"drop column constraint", "ALTER TABLE a DROP COLUMN x;\nALTER TABLE a DROP CONSTRAINT c;\nALTER TABLE a DROP COLUMN IF EXISTS x;\nALTER TABLE a DROP CONSTRAINT IF EXISTS c;", []string{"1:DROP COLUMN without", "2:DROP CONSTRAINT without"}},
		{"alter column forms pass", "ALTER TABLE a ALTER COLUMN x SET NOT NULL, ALTER COLUMN y DROP DEFAULT;\nALTER TABLE a ENABLE ROW LEVEL SECURITY;\nALTER TABLE a OWNER TO u;", nil},
		{"table rename", "ALTER TABLE a RENAME TO b;\nALTER TABLE IF EXISTS a RENAME TO b;", []string{"1:ALTER TABLE ... RENAME TO needs"}},
		{"column rename", "ALTER TABLE IF EXISTS a RENAME COLUMN x TO y;\nALTER TABLE a RENAME CONSTRAINT c TO d;\nALTER TABLE a RENAME x TO y;", []string{"1:RENAME COLUMN", "2:RENAME CONSTRAINT", "3:RENAME X"}},
		{"alter index rename", "ALTER INDEX i RENAME TO j;\nALTER INDEX IF EXISTS i RENAME TO j;\nALTER SEQUENCE s RENAME TO t;\nALTER VIEW v RENAME TO w;\nALTER MATERIALIZED VIEW IF EXISTS v RENAME TO w;", []string{"1:ALTER INDEX", "3:ALTER SEQUENCE", "4:ALTER VIEW"}},
		{"drop", "DROP TABLE a;\nDROP TABLE IF EXISTS a;\nDROP INDEX CONCURRENTLY i;\nDROP INDEX CONCURRENTLY IF EXISTS i;\nDROP MATERIALIZED VIEW m;", []string{"1:DROP without", "3:DROP without", "5:DROP without"}},
		{"insert", "INSERT INTO a (x) VALUES (1);\nINSERT INTO a (x) VALUES (1) ON CONFLICT DO NOTHING;\nINSERT INTO a (x) SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM a);", []string{"1:INSERT without"}},
		{"with insert", "WITH s AS (SELECT 1) INSERT INTO a (x) SELECT * FROM s;", []string{"1:INSERT without"}},
		{"with select passes", "WITH s AS (SELECT 1) SELECT * FROM s;", nil},
		{"others pass", "COMMENT ON TABLE a IS 'x';\nGRANT SELECT ON a TO u;\nUPDATE a SET x = 1;\nSET search_path = public;\nSELECT 1;", nil},
		// lexing
		{"semicolon in string", "COMMENT ON TABLE a IS 'a; CREATE TABLE b (x int); c';", nil},
		{"escaped quote", "COMMENT ON TABLE a IS 'it''s; CREATE TABLE b (x int)';\nCREATE TABLE c (x int);", []string{"2:CREATE TABLE without"}},
		{"E string backslash", "SELECT E'a\\'; CREATE TABLE b (x int);';\nCREATE TABLE c (x int);", []string{"2:CREATE TABLE without"}},
		{"line comment hides", "-- CREATE TABLE a (x int);\nSELECT 1;", nil},
		{"nested block comment", "/* outer /* CREATE TABLE a (x int); */ still comment CREATE TABLE b (x int); */\nCREATE TABLE c (x int);", []string{"2:CREATE TABLE without"}},
		{"quoted ident with semicolon", `CREATE TABLE IF NOT EXISTS "a;b" (x int);`, nil},
		{"dollar body hides semicolon", "CREATE OR REPLACE FUNCTION f() RETURNS void AS $f$ BEGIN CREATE TABLE x (a int); END $f$ LANGUAGE plpgsql;", nil},
		{"nested dollar tags", "DO $a$ BEGIN EXECUTE $b$ CREATE TABLE x (a int) $b$; END $a$;", []string{"1:unguarded dynamic SQL"}},
		{"line numbers after multiline string", "COMMENT ON TABLE a IS 'one\ntwo\nthree';\nCREATE TABLE b (x int);", []string{"4:CREATE TABLE without"}},
		{"line numbers after body", "CREATE OR REPLACE FUNCTION f() RETURNS int AS $$\nSELECT 1;\n$$ LANGUAGE sql;\nCREATE TABLE b (x int);", []string{"4:CREATE TABLE without"}},
		{"lowercase and spacing", "create   table\n  a\n(x int);", []string{"1:CREATE TABLE without"}},
		{"parameter not a body", "SELECT $1, $2;", nil},
		// DO blocks
		{"do exception guarded", "DO $$ BEGIN CREATE TYPE t AS ENUM ('a'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;", nil},
		{"do if exists probe guarded", "DO $$ BEGIN IF to_regclass('public.a') IS NOT NULL THEN EXECUTE 'ALTER TABLE a ADD COLUMN x int'; END IF; END $$;", nil},
		{"do if not exists subquery guarded", "DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'c') THEN ALTER TABLE a ADD CONSTRAINT c CHECK (x > 0); END IF; END $$;", nil},
		{"do catalog probe guarded", "DO $$ BEGIN IF (SELECT count(*) FROM information_schema.tables) > 0 THEN NULL; END IF; END $$;", nil},
		{"do unguarded execute", "DO $$\nBEGIN\n  EXECUTE 'CREATE TABLE x (a int)';\nEND $$;", []string{"3:unguarded dynamic SQL"}},
		{"do unguarded statement", "DO $$ BEGIN CREATE TABLE x (a int); END $$;", []string{"1:CREATE TABLE without"}},
		{"do unguarded statement after if", "DO $$ BEGIN IF true THEN CREATE TABLE x (a int); END IF; END $$;", []string{"1:CREATE TABLE without"}},
		{"do ddl if not exists is not a guard", "DO $$ BEGIN CREATE TABLE IF NOT EXISTS x (a int); INSERT INTO x VALUES (1); END $$;", []string{"1:INSERT without"}},
		{"do ddl drop if exists is not a guard", "DO $$ BEGIN IF true THEN DROP TABLE IF EXISTS x; END IF; EXECUTE 'SELECT 1'; END $$;", []string{"1:unguarded dynamic SQL"}},
		{"do body line numbers", "SELECT 1;\nDO $$\nDECLARE r RECORD;\nBEGIN\n  CREATE TABLE x (a int);\nEND $$;", []string{"5:CREATE TABLE without"}},
		{"do language clause", "DO LANGUAGE plpgsql $$ BEGIN EXECUTE 'x'; END $$;", []string{"1:unguarded dynamic SQL"}},
		{"do string body", "DO 'BEGIN EXECUTE ''x''; END';", []string{"1:unguarded dynamic SQL"}},
		{"do without body", "DO;", []string{"1:DO statement with no body"}},
		{"do loop guarded rename", "DO $$ DECLARE t RECORD; BEGIN FOR t IN SELECT a FROM (VALUES ('x')) AS v(a) LOOP IF to_regclass('public.' || t.a) IS NOT NULL THEN EXECUTE format('ALTER TABLE %I RENAME TO %I', t.a, 'y'); END IF; END LOOP; END $$;", nil},
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

func TestStripPL(t *testing.T) {
	cases := []struct{ in, first string }{
		{"BEGIN CREATE TABLE a (x int)", "CREATE"},
		{"IF a = 1 AND (b OR c) THEN DROP TABLE x", "DROP"},
		{"ELSIF x THEN INSERT INTO a VALUES (1)", "INSERT"},
		{"FOR t IN SELECT 1 LOOP ALTER TABLE a ADD COLUMN x int", "ALTER"},
		{"EXCEPTION WHEN others THEN UPDATE a SET x = 1", "UPDATE"},
		{"END IF", ""},
		{"IF x", ""},
	}
	for _, c := range cases {
		got := stripPL(lex(c.in, 1))
		first := ""
		if len(got) > 0 {
			first = got[0].U
		}
		if first != c.first {
			t.Errorf("stripPL(%q) starts with %q, want %q", c.in, first, c.first)
		}
	}
}
