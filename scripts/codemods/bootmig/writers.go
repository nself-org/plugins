package main

// Purpose: decide whether a handler may have committed its response before the /health literal.
// Inputs: the enclosing function body, the response-writer name and the literal position.
// Outputs: true when the site must be wired by hand.

import (
	"go/ast"
	"go/token"
)

// writesHeaderBefore reports whether the response may already be committed before pos: an earlier statement in body
// (one that ends before the statement holding pos) calls <w>.Write or <w>.WriteHeader, or passes <w> as an argument to
// any call (http.Error(w, ...), json.NewEncoder(w).Encode(...), helper(w, ...), fmt.Fprintf(w, ...)). <w>.Header()
// calls do not count, and neither do calls in the statement that holds pos (the usual
// `json.NewEncoder(w).Encode(map{...})`).
//
// An if/else branch that ends in a return is skipped: control cannot fall from it to pos, which is the common
// `if err := ping(); err != nil { http.Error(w, ...); return }` guard. Any other earlier write counts, even in a branch
// that may not run, so such a site goes to manual wiring rather than risk a second WriteHeader. The exemption does not
// reach into a func literal that ends before pos (its return leaves only the closure): any writer use there counts.
//
// The check is syntactic and direct-use only (see callTouchesWriter): aliases such as `rw := w`, type assertions like
// `w.(http.Flusher)`, and a writer wrapped in a composite or struct are not tracked.
func writesHeaderBefore(body *ast.BlockStmt, w string, pos token.Pos) bool {
	if body == nil {
		return false
	}
	// Start of the innermost non-block statement that holds pos.
	stmtStart := pos
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil || n.Pos() > pos || pos >= n.End() {
			return false
		}
		if st, ok := n.(ast.Stmt); ok {
			if _, block := st.(*ast.BlockStmt); !block {
				stmtStart = st.Pos()
			}
		}
		return true
	})
	exits := map[*ast.BlockStmt]bool{} // branches that return before reaching pos
	hit := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.IfStmt:
			for _, b := range []ast.Stmt{v.Body, v.Else} {
				if blk, ok := b.(*ast.BlockStmt); ok && endsInReturn(blk) && !(blk.Pos() <= pos && pos < blk.End()) {
					exits[blk] = true
				}
			}
		case *ast.BlockStmt:
			if exits[v] {
				return false
			}
		case *ast.CallExpr:
			if v.End() <= stmtStart && callTouchesWriter(v, w) {
				hit = true
			}
		}
		return !hit
	})
	return hit
}

// endsInReturn reports whether the last statement of b is a return.
func endsInReturn(b *ast.BlockStmt) bool {
	if len(b.List) == 0 {
		return false
	}
	_, ok := b.List[len(b.List)-1].(*ast.ReturnStmt)
	return ok
}

// callTouchesWriter reports whether call is <w>.Write/<w>.WriteHeader or takes <w> as a direct argument. Syntactic and
// direct-use only: aliases (rw := w), w.(http.Flusher) and a writer wrapped in a composite are not tracked.
func callTouchesWriter(call *ast.CallExpr, w string) bool {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "WriteHeader" || sel.Sel.Name == "Write") {
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == w {
			return true
		}
	}
	for _, a := range call.Args {
		if id, ok := a.(*ast.Ident); ok && id.Name == w {
			return true
		}
	}
	return false
}
