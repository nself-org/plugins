// inserts.go: the per-INSERT replay guard rules and the WITH ... SELECT INTO check of the idempotency lint (P7 D-0619 F3, F5).
package main

// withTail returns the main statement of a WITH statement: the tokens from the first top-level SELECT, INSERT, UPDATE,
// DELETE, VALUES or TABLE after its common table expressions.
func withTail(t []tok) []tok {
	depth := 0
	for i := 1; i < len(t); i++ {
		switch {
		case isPunct(t, i, "("):
			depth++
		case isPunct(t, i, ")"):
			depth--
		case depth == 0:
			switch w(t, i) {
			case "SELECT", "INSERT", "UPDATE", "DELETE", "VALUES", "TABLE", "MERGE":
				return t[i:]
			}
		}
	}
	return nil
}

// insertViols turns the unguarded INSERTs of a statement into violations.
func insertViols(t []tok) []viol {
	var out []viol
	for _, l := range lintInserts(t) {
		out = append(out, viol{l, "INSERT without ON CONFLICT or a NOT EXISTS guard inserts the rows again on replay"})
	}
	return out
}

// lintInserts checks every INSERT INTO of a statement on its own (D-0619 F3): it needs an ON CONFLICT clause or a NOT EXISTS
// predicate of its own, at its own nesting level (a clause of an outer or sibling statement does not count), and the
// NOT EXISTS subquery has to read the table the INSERT writes.
func lintInserts(t []tok) (lines []int) {
	depth := make([]int, len(t)+1) // paren depth before token i
	for i := range t {
		depth[i+1] = depth[i]
		if isPunct(t, i, "(") {
			depth[i+1]++
		} else if isPunct(t, i, ")") {
			depth[i+1]--
		}
	}
	for i := range t {
		if !has(t, i, "INSERT", "INTO") {
			continue
		}
		d := depth[i]
		end := len(t)
		for k := i + 1; k < len(t); k++ {
			if depth[k+1] < d {
				end = k
				break
			}
		}
		_, target, _ := qname(t, i+2)
		guarded := false
		for k := i; k < end && !guarded; k++ {
			if depth[k] != d {
				continue
			}
			switch {
			case has(t, k, "ON", "CONFLICT"):
				guarded = true
			case has(t, k, "NOT", "EXISTS") && isPunct(t, k+2, "("):
				guarded = target == "" || subqueryMentions(t, k+2, target)
			}
		}
		if !guarded {
			lines = append(lines, t[i].Line)
		}
	}
	return lines
}

// subqueryMentions reports whether the parenthesised group opening at t[open] names the table.
func subqueryMentions(t []tok, open int, table string) bool {
	depth := 0
	for k := open; k < len(t); k++ {
		switch {
		case isPunct(t, k, "("):
			depth++
		case isPunct(t, k, ")"):
			if depth--; depth == 0 {
				return false
			}
		case ident(t, k) == table && (t[k].K == kWord || t[k].K == kQID):
			return true
		}
	}
	return false
}
