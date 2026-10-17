// do.go: DO block handling of the idempotency lint (P7 D-0346). A DO block is replay-safe where an EXCEPTION handler or an
// existence-checked IF covers its statements; what sits outside is linted like top-level SQL and may not run EXECUTE.
package main

// isProbe reports whether t[i] is an existence probe: EXISTS ( ..), a to_reg* call or a catalog name.
func isProbe(t []tok, i int) bool {
	switch w(t, i) {
	case "EXISTS":
		return isPunct(t, i+1, "(")
	case "TO_REGCLASS", "TO_REGTYPE", "TO_REGPROC", "TO_REGNAMESPACE", "INFORMATION_SCHEMA", "PG_CATALOG",
		"PG_CLASS", "PG_INDEXES", "PG_CONSTRAINT", "PG_TYPE", "PG_TRIGGER", "PG_POLICIES":
		return true
	}
	return false
}

// protectedStmts marks the statements that sit in the protected part of a BEGIN..END block, from its BEGIN up to its
// EXCEPTION clause, when a handler of that clause catches the error a replay raises (handlerCatches) and does not re-raise
// it: the handler makes a failing replay harmless. Statements of the handler itself are not protected (an error there is
// not caught by the same block).
func protectedStmts(sts [][]tok) []bool {
	prot := make([]bool, len(sts))
	type blk struct{ begin, exc int }
	var stack []blk // each open BEGIN: its statement index and the index of its EXCEPTION statement (-1: none)
	for i, st := range sts {
		if w(st, 0) == "END" {
			if n := w(st, 1); n != "IF" && n != "LOOP" && n != "CASE" && len(stack) > 0 {
				b := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if b.exc >= 0 && handlerCatches(sts[b.begin:b.exc], sts[b.exc:i]) {
					for k := b.begin; k < b.exc; k++ {
						prot[k] = true
					}
				}
			}
			continue
		}
		rest, _ := stripPLn(st)
		for _, x := range st[:len(st)-len(rest)] {
			switch {
			case x.K == kWord && x.U == "BEGIN":
				stack = append(stack, blk{i, -1})
			case x.K == kWord && x.U == "EXCEPTION" && len(stack) > 0 && stack[len(stack)-1].exc < 0:
				stack[len(stack)-1].exc = i
			}
		}
	}
	return prot
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

// lintDO lints a DO block. Statements in the protected part of a BEGIN..END whose EXCEPTION handler catches the replay error
// are replay-safe. A statement inside an IF branch whose own condition probes the object it works on, or after an early
// `IF <probe> THEN RETURN; END IF;` clause, is covered by that guard when the guard's polarity fits the statement (see
// guard.go); an ELSE branch, a loop, a CASE or an IF without a probe is no guard. Everything else is linted with the
// ordinary rules and may not run dynamic SQL (EXECUTE).
func lintDO(t []tok, c *ctx) []viol {
	body, ok := doBody(t)
	if !ok {
		return []viol{{t[0].Line, "DO statement with no body to check"}}
	}
	sts := split(body)
	prot := protectedStmts(sts)
	c.inDO = true
	defer func() { c.inDO = false }()
	var out []viol
	var g guardState
	for i, st := range sts {
		if w(st, 0) == "END" {
			g.pop()
			continue
		}
		rest, _ := stripPLn(st)
		prefix := st[:len(st)-len(rest)]
		before := g.anyGuard()
		g.enter(prefix)
		if rest != nil && len(prefix) > 0 && w(rest, 0) == "RETURN" && len(rest) == 1 && g.earlyReturn(sts, i) {
			continue
		}
		if prot[i] {
			continue
		}
		dyn := func(ts []tok, guarded bool) {
			if guarded {
				return
			}
			for _, b := range ts {
				if b.K == kWord && b.U == "EXECUTE" {
					out = append(out, viol{b.Line, "unguarded dynamic SQL: EXECUTE in a DO block outside an EXCEPTION WHEN handler or an existence-checked IF"})
				}
			}
		}
		dyn(prefix, before)
		dyn(rest, g.anyGuard())
		if !g.covers(rest) {
			out = append(out, lintStmt(rest, c)...)
		}
	}
	return out
}

// selectInto reports a top-level SELECT ... INTO <table> (not inside parentheses).
func selectInto(t []tok) bool {
	depth := 0
	for i := range t {
		switch {
		case isPunct(t, i, "("):
			depth++
		case isPunct(t, i, ")"):
			depth--
		case depth == 0 && w(t, i) == "INTO":
			return true
		}
	}
	return false
}
