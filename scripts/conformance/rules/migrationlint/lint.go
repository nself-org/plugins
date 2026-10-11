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
type ctx struct{ trig, pol, cons map[string]bool }

func newCtx() *ctx { return &ctx{map[string]bool{}, map[string]bool{}, map[string]bool{}} }

func lintFile(src string) []viol {
	c := newCtx()
	var out []viol
	for _, st := range split(lex(src, 1)) {
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
				c.trig[n] = true
			case "POLICY":
				c.pol[n] = true
			}
		}
	case "INSERT":
		lintInsert(t, bad)
	case "WITH":
		if contains(t, "INSERT", "INTO") {
			lintInsert(t, bad)
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
		if !orRep && !c.trig[ident(t, i+1)] {
			bad("CREATE TRIGGER %s needs OR REPLACE or an earlier DROP TRIGGER IF EXISTS %s", ident(t, i+1), ident(t, i+1))
		}
	case "POLICY":
		if !c.pol[ident(t, i+1)] {
			bad("CREATE POLICY %s needs an earlier DROP POLICY IF EXISTS %s", ident(t, i+1), ident(t, i+1))
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
		_, _, i = qname(t, i)
		for _, a := range splitTop(t[min(i, len(t)):]) {
			lintAction(a, ex, c, bad)
		}
	}
}

func lintAction(a []tok, ex bool, c *ctx, bad func(string, ...any)) {
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
			if n := ident(a, k+1); !c.cons[n] {
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
			c.cons[ident(a, k+2)] = true
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

// guarded reports whether a DO body is replay-safe by itself: it handles errors (EXCEPTION WHEN) or branches (IF)
// on an existence probe. DDL "IF [NOT] EXISTS name" is not a branch and not a probe: EXISTS counts only before "(".
func guarded(body []tok) bool {
	if contains(body, "EXCEPTION", "WHEN") {
		return true
	}
	branch, probe := false, false
	for i := range body {
		switch w(body, i) {
		case "IF", "ELSIF", "ELSEIF":
			j := i + 1
			if has(body, j, "NOT", "EXISTS") {
				j++
			}
			if w(body, j) != "EXISTS" || isPunct(body, j+1, "(") {
				branch = true
			}
		case "EXISTS":
			probe = probe || isPunct(body, i+1, "(")
		case "TO_REGCLASS", "TO_REGTYPE", "TO_REGPROC", "TO_REGNAMESPACE", "INFORMATION_SCHEMA", "PG_CATALOG",
			"PG_CLASS", "PG_INDEXES", "PG_CONSTRAINT", "PG_TYPE", "PG_TRIGGER", "PG_POLICIES":
			probe = true
		}
	}
	return branch && probe
}

// doBody returns the tokens of a DO statement's body: its $$ body, else its last string literal.
func doBody(t []tok) ([]tok, bool) {
	pick := -1
	for i, x := range t {
		if x.K == kBody {
			return lex(x.U, x.BodyLine), true
		}
		if x.K == kStr {
			pick = i
		}
	}
	if pick < 0 {
		return nil, false
	}
	return lex(t[pick].U, t[pick].Line), true
}

func lintDO(t []tok, c *ctx) []viol {
	body, ok := doBody(t)
	if !ok {
		return []viol{{t[0].Line, "DO statement with no body to check"}}
	}
	if guarded(body) {
		return nil
	}
	var out []viol
	for _, b := range body {
		if b.K == kWord && b.U == "EXECUTE" {
			out = append(out, viol{b.Line, "unguarded dynamic SQL: EXECUTE in a DO block with no EXCEPTION WHEN or existence-checked IF"})
		}
	}
	for _, st := range split(body) {
		out = append(out, lintStmt(stripPL(st), c)...)
	}
	return out
}
