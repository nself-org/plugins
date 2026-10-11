// lint.go: the migration idempotency lint (P7 D-0346). A boot migration file is applied again over live tables when a
// plugin is reinstalled (remove drops the ledger schema, not the tables), so every statement must be safe to replay.
// lintFile walks the statements of one file in order; a DROP TRIGGER/POLICY/CONSTRAINT IF EXISTS earlier in the file
// licenses the matching CREATE/ADD later. DO blocks are replay-safe when guarded (see guarded); an unguarded block is
// linted statement by statement and may not run dynamic SQL.
package main

import "fmt"

type viol struct {
	Line int
	Msg  string
}

// ctx remembers the DROP ... IF EXISTS statements seen so far in the file.
type ctx struct {
	trig, pol, cons map[string]bool
	inDO            bool // linting the body of a DO block (SELECT .. INTO is a plpgsql assignment there)
}

func newCtx() *ctx {
	return &ctx{trig: map[string]bool{}, pol: map[string]bool{}, cons: map[string]bool{}}
}

func lintFile(src string) []viol {
	c := newCtx()
	var out []viol
	for _, st := range split(lexFile(src)) {
		out = append(out, lintStmt(st, c)...)
	}
	return out
}

func lintStmt(t []tok, c *ctx) []viol {
	if len(t) == 0 {
		return nil
	}
	var out []viol
	bad := func(format string, a ...any) { out = append(out, viol{t[0].Line, fmt.Sprintf(format, a...)}) }
	switch w(t, 0) {
	case "CREATE":
		lintCreate(t, c, bad)
	case "ALTER":
		lintAlter(t, c, bad)
	case "DROP":
		k := ifExists(t, 1, 5)
		if k < 0 {
			bad("DROP without IF EXISTS fails when the object is already gone")
			break
		}
		if n := ident(t, k+2); n != "" {
			switch w(t, 1) {
			case "TRIGGER":
				c.trig[key(n, onTable(t, k+3))] = true
			case "POLICY":
				c.pol[key(n, onTable(t, k+3))] = true
			}
		}
	case "INSERT":
		lintInsert(t, bad)
	case "WITH":
		if contains(t, "INSERT", "INTO") {
			lintInsert(t, bad)
		}
	case "SELECT":
		if !c.inDO && selectInto(t) {
			bad("SELECT ... INTO creates a table and fails when it exists: use CREATE TABLE IF NOT EXISTS ... AS")
		}
	case "DO":
		return lintDO(t, c)
	}
	return out
}

func lintInsert(t []tok, bad func(string, ...any)) {
	if !contains(t, "ON", "CONFLICT") && !contains(t, "NOT", "EXISTS") {
		bad("INSERT without ON CONFLICT or a NOT EXISTS guard inserts the rows again on replay")
	}
}

func lintCreate(t []tok, c *ctx, bad func(string, ...any)) {
	i, orRep := 1, false
	if has(t, i, "OR", "REPLACE") {
		orRep, i = true, i+2
	}
	for {
		switch w(t, i) {
		case "GLOBAL", "LOCAL", "TEMP", "TEMPORARY", "UNLOGGED", "UNIQUE", "RECURSIVE", "TRUSTED", "CONSTRAINT":
			i++
			continue
		}
		break
	}
	obj := w(t, i)
	need := func(j int, what string) {
		if !ine(t, j) {
			bad("CREATE %s without IF NOT EXISTS fails when it already exists", what)
		}
	}
	switch obj {
	case "TABLE", "SCHEMA", "EXTENSION", "SEQUENCE":
		need(i+1, obj)
	case "INDEX":
		if w(t, i+1) == "CONCURRENTLY" {
			need(i+2, obj)
		} else {
			need(i+1, obj)
		}
	case "MATERIALIZED":
		need(i+2, "MATERIALIZED VIEW")
	case "VIEW", "FUNCTION", "PROCEDURE", "AGGREGATE", "RULE":
		if !orRep {
			bad("CREATE %s needs CREATE OR REPLACE", obj)
		}
	case "TRIGGER":
		if !orRep && !c.trig[key(ident(t, i+1), onTable(t, i+2))] {
			bad("CREATE TRIGGER %s needs OR REPLACE or an earlier DROP TRIGGER IF EXISTS %s ON the same table", ident(t, i+1), ident(t, i+1))
		}
	case "POLICY":
		if !c.pol[key(ident(t, i+1), onTable(t, i+2))] {
			bad("CREATE POLICY %s needs an earlier DROP POLICY IF EXISTS %s ON the same table", ident(t, i+1), ident(t, i+1))
		}
	case "TYPE", "DOMAIN":
		bad("CREATE %s fails when it already exists: wrap it in a guarded DO block", obj)
	default:
		if !orRep && !ine(t, i+1) && !ine(t, i+2) {
			bad("CREATE %s is not replay-safe: wrap it in a guarded DO block", obj)
		}
	}
}

func lintAlter(t []tok, c *ctx, bad func(string, ...any)) {
	switch w(t, 1) {
	case "INDEX", "SEQUENCE", "VIEW", "MATERIALIZED":
		if contains(t, "RENAME", "TO") && ifExists(t, 2, 3) < 0 {
			bad("ALTER %s ... RENAME TO needs IF EXISTS", w(t, 1))
		}
	case "TABLE":
		i, ex := 2, false
		if has(t, i, "IF", "EXISTS") {
			ex, i = true, i+2
		}
		if w(t, i) == "ONLY" {
			i++
		}
		_, tbl, i := qname(t, i)
		for _, a := range splitTop(t[min(i, len(t)):]) {
			lintAction(a, ex, tbl, c, bad)
		}
	case "TYPE":
		for i := range t {
			if has(t, i, "ADD", "VALUE") && !ine(t, i+2) {
				bad("ALTER TYPE ... ADD VALUE needs IF NOT EXISTS")
			}
			if has(t, i, "RENAME", "VALUE") {
				bad("ALTER TYPE ... RENAME VALUE fails on replay: do it in a guarded DO block")
			}
		}
	}
}

func lintAction(a []tok, ex bool, tbl string, c *ctx, bad func(string, ...any)) {
	if len(a) == 0 {
		return
	}
	switch w(a, 0) {
	case "ADD":
		k := 1
		if w(a, k) == "COLUMN" {
			k++
			if !ine(a, k) {
				bad("ADD COLUMN without IF NOT EXISTS fails when the column exists")
			}
			return
		}
		switch w(a, k) {
		case "CONSTRAINT":
			if n := ident(a, k+1); !c.cons[key(n, tbl)] {
				bad("ADD CONSTRAINT %s needs an earlier DROP CONSTRAINT IF EXISTS %s", n, n)
			}
		case "PRIMARY", "UNIQUE", "FOREIGN", "CHECK", "EXCLUDE":
			bad("unnamed ADD %s constraint cannot be dropped first: name it and drop it IF EXISTS before", w(a, k))
		default:
			if !ine(a, k) {
				bad("ADD COLUMN without IF NOT EXISTS fails when the column exists")
			}
		}
	case "DROP":
		k := 1
		if w(a, k) == "COLUMN" || w(a, k) == "CONSTRAINT" {
			k++
		}
		if !has(a, k, "IF", "EXISTS") {
			bad("DROP %s without IF EXISTS fails when it is already gone", w(a, 1))
			return
		}
		if w(a, 1) == "CONSTRAINT" {
			c.cons[key(ident(a, k+2), tbl)] = true
		}
	case "SET":
		if w(a, 1) == "SCHEMA" && !ex {
			bad("ALTER TABLE ... SET SCHEMA fails on replay: use ALTER TABLE IF EXISTS or a guarded DO block")
		}
	case "ATTACH":
		bad("ATTACH PARTITION fails on replay: do it in a guarded DO block")
	case "ALTER":
		if contains(a, "ADD", "GENERATED") {
			bad("ADD GENERATED ... AS IDENTITY fails when the column is already an identity: do it in a guarded DO block")
		}
	case "RENAME":
		if w(a, 1) == "TO" {
			if !ex {
				bad("ALTER TABLE ... RENAME TO needs ALTER TABLE IF EXISTS")
			}
			return
		}
		bad("RENAME %s fails on replay: do it in a guarded DO block", w(a, 1))
	}
}
