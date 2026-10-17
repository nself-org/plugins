// handler.go: which EXCEPTION handlers make a DO block replay-safe (P7 D-0619 F2).
package main

import "strings"

// replaySQLStates are the SQLSTATE codes of the errors a replayed CREATE/ALTER/DROP raises (duplicate_*, undefined_*,
// invalid_table_definition); 23505 (unique_violation) only fits replayed data statements. OTHERS catches everything.
var replaySQLStates = map[string]bool{"42P07": true, "42710": true, "42701": true, "42P06": true, "42723": true, "42P16": true,
	"42704": true, "42P01": true, "42703": true, "42883": true}

const uniqueViolation = "UNIQUE_VIOLATION"

// handlerCatches reports whether the EXCEPTION part (statements from the EXCEPTION statement to before its END) has a
// WHEN clause that catches a replay error of the protected statements and does not re-raise: a bare or EXCEPTION-level
// RAISE in its body means the replay still fails.
func handlerCatches(body, handler [][]tok) bool {
	dml := true // the protected statements are data statements only (unique_violation is then the replay error)
	for _, st := range body {
		rest, _ := stripPLn(st)
		switch w(rest, 0) {
		case "", "INSERT", "UPDATE", "DELETE", "SELECT", "PERFORM", "WITH", "NULL", "RETURN", "RAISE":
		default:
			dml = false
		}
	}
	catching, any := false, false
	for _, st := range handler {
		prefix := st
		if rest, _ := stripPLn(st); rest != nil {
			prefix = st[:len(st)-len(rest)]
		}
		k := 0
		if w(prefix, 0) == "EXCEPTION" {
			k = 1
		}
		if w(prefix, k) == "WHEN" { // a new clause
			if catching && any {
				return true
			}
			catching, any = clauseCatches(prefix[k+1:], dml), true
		}
		if rest, _ := stripPLn(st); catching && w(rest, 0) == "RAISE" {
			switch w(rest, 1) {
			case "NOTICE", "WARNING", "INFO", "LOG", "DEBUG":
			default:
				catching = false
			}
		}
	}
	return catching && any
}

// clauseCatches reports whether a WHEN condition list (up to THEN) names a replay error.
func clauseCatches(cond []tok, dml bool) bool {
	for i, x := range cond {
		switch {
		case x.K == kWord && x.U == "OTHERS":
			return true
		case x.K == kWord && x.U == "SQLSTATE" && i+1 < len(cond) && cond[i+1].K == kStr:
			code := strings.ToUpper(cond[i+1].U)
			if replaySQLStates[code] || (dml && code == "23505") {
				return true
			}
		case x.K == kWord && (strings.HasPrefix(x.U, "DUPLICATE_") || strings.HasPrefix(x.U, "UNDEFINED_") || x.U == "INVALID_TABLE_DEFINITION"):
			return true
		case x.K == kWord && x.U == uniqueViolation && dml:
			return true
		}
	}
	return false
}
