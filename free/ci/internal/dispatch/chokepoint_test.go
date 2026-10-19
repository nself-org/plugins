package dispatch

// Purpose: R-SEC-22. dispatch.Admit is the only path that creates a lease or a
// local attempt, so no function outside the definers may call the store's
// admission functions. The scan parses the module's NON-TEST .go files; test
// files (CI-31 store tests, SCHED-18's race test, TRUST's CheckDispatcher) are
// out of scope, and the planted violation lives in a testdata module.
// Allowed users: internal/store and internal/lease (the definers),
// internal/dispatch, and run.CreateAttempt with run.LocalAdmit (the nil-hook
// path CreateAttempt calls; LocalAdmit itself may only be named in
// run/attempt.go and run/dispatch_hook.go). Any reference counts, not only a
// call, and files are parsed whatever their build tags say.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var (
	// chokeFuncs are the only functions that may name an admission function:
	// the function that wraps each definer's call, never a whole directory (a
	// wrapper inside internal/store would launder the call to any caller). A
	// method is written (Type).Name; run.CreateAttempt and run.LocalAdmit are
	// plain functions, so a method of that name elsewhere is not on the list.
	chokeFuncs = map[string]bool{
		"internal/run.CreateAttempt":    true,
		"internal/run.LocalAdmit":       true,
		"internal/dispatch.Admit":       true,
		"internal/lease.(Engine).Grant": true,
	}
	// definerDirs own the transition machinery (apply inside the admission
	// transactions); nowhere else may move an attempt to leased.
	definerDirs = map[string]bool{"internal/store": true}
	// transitionUsers are the functions outside the store that name
	// Store.Transition today, reviewed one by one: none moves an attempt to
	// leased. Store.Transition itself accepts queued -> leased (the P7-CI-31
	// store tests rely on it, and this Ticket cannot edit transitions.go; debt),
	// so a NEW user must be added here by a reviewer. A variable or constant
	// target, a method value or a wrapper all trip the scan the same way.
	transitionUsers = map[string]bool{
		"internal/dispatch.(Dispatcher).lose": true, // leased/running -> lost
		"internal/lease.(Engine).PollHosted":  true, // leased -> running (hosted liveness)
		"internal/run.Recover":                true, // lost -> retryable | needs-operator
		"internal/run.strandAttempts":         true, // queued -> cancelled, finalizing -> failed
		"internal/run.CreateAttempt":          true, // queued -> cancelled (refused admission)
		"internal/run.(runState).trans":       true, // the run loop's own steps; its callers are listed below
	}
	localFiles = map[string]bool{"internal/run/attempt.go": true, "internal/run/dispatch_hook.go": true}
)

// banned are the admission functions only the chokepoint may name.
var banned = map[string]bool{"AdmitAndLease": true, "AdmitPlacement": true, "LeaseGrant": true, "Grant": true, "LocalAdmit": true}

// reflective names reach a method by string or index, which no scan can follow;
// they are refused in every non-test file, in the definers too. Method is
// reflective only in a file that imports reflect (http.Request.Method is not).
var reflective = map[string]bool{"MethodByName": true}

// bannedUse names the admission function a node refers to, or "". A reference
// counts, not only a call: a method value (f := st.AdmitPlacement), a method
// expression ((*lease.Engine).Grant), a bare identifier (dot import) or the
// name as a string (reflection) all reach the function the same way.
func bannedUse(n ast.Node, declName *ast.Ident, usesReflect bool) string {
	switch x := n.(type) {
	case *ast.CallExpr:
		// Transition(ctx, id, from, to, detail) with to == "leased" creates a leased
		// attempt with no lease row, reservation or limit check; trans(id, from, to,
		// class) is the run loop's wrapper of it.
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			target := map[string]int{"Transition": 3, "trans": 2}[sel.Sel.Name]
			if target > 0 && len(x.Args) > target {
				if lit, ok := x.Args[target].(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Trim(lit.Value, "\"`") == "leased" {
					return "Transition(leased)"
				}
			}
		}
	case *ast.SelectorExpr:
		if banned[x.Sel.Name] || reflective[x.Sel.Name] || usesReflect && x.Sel.Name == "Method" || x.Sel.Name == "Transition" {
			return x.Sel.Name
		}
	case *ast.Ident:
		if banned[x.Name] && x != declName {
			return x.Name
		}
	case *ast.BasicLit:
		if x.Kind == token.STRING && banned[strings.Trim(x.Value, "\"`")] {
			return strings.Trim(x.Value, "\"`")
		}
	}
	return ""
}

// scanChokepoint returns "file: func uses Name" for every use outside the
// allowlist, and how many files it read.
func scanChokepoint(t *testing.T, root string) (violations []string, files int) {
	t.Helper()
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (d.Name() == "vendor" || d.Name() == "testdata" || d.Name() == ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		dir := filepath.ToSlash(filepath.Dir(rel))
		// every build constraint is read: the parser sees the file whatever its tags say
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}
		files++
		for _, cg := range f.Comments { // //go:linkname reaches a function without naming it in code
			for _, c := range cg.List {
				if strings.Contains(c.Text, "go:linkname") {
					if v := rel + ": comment uses go:linkname"; !seen[v] {
						seen[v] = true
						violations = append(violations, v)
					}
				}
			}
		}
		usesReflect := false
		for _, imp := range f.Imports {
			usesReflect = usesReflect || strings.Trim(imp.Path.Value, "\"`") == "reflect"
		}
		for _, decl := range f.Decls {
			owner, declName := "package level", (*ast.Ident)(nil)
			if fn, ok := decl.(*ast.FuncDecl); ok {
				owner, declName = fn.Name.Name, fn.Name
				if fn.Recv != nil && len(fn.Recv.List) == 1 {
					recv := ast.Expr(fn.Recv.List[0].Type)
					if star, ok := recv.(*ast.StarExpr); ok {
						recv = star.X
					}
					switch g := recv.(type) { // a generic receiver T[K] names the same type
					case *ast.IndexExpr:
						recv = g.X
					case *ast.IndexListExpr:
						recv = g.X
					}
					if id, ok := recv.(*ast.Ident); ok {
						owner = "(" + id.Name + ")." + fn.Name.Name
					}
				}
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				name := bannedUse(n, declName, usesReflect)
				switch {
				case name == "":
				case (name == "Transition(leased)" || name == "Transition") && definerDirs[dir]:
				case name == "Transition" && transitionUsers[dir+"."+owner]:
				case reflective[name], name == "Method", name == "Transition(leased)", name == "Transition", name == "LocalAdmit" && !localFiles[rel], name != "LocalAdmit" && name != "Transition" && !chokeFuncs[dir+"."+owner]:
					if v := rel + ": " + owner + " uses " + name; !seen[v] { // a selector and its identifier are two nodes
						seen[v] = true
						violations = append(violations, v)
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(violations)
	return violations, files
}

func TestAdmitChokepoint(t *testing.T) {
	planted, read := scanChokepoint(t, filepath.Join("testdata", "chokepoint"))
	want := []string{
		"internal/bad/bad.go: Closure uses AdmitPlacement",
		"internal/bad/bad.go: Grantor uses Grant",
		"internal/bad/bad.go: Alias uses Transition",
		"internal/bad/bad.go: Leased uses Transition",
		"internal/bad/bad.go: Leased uses Transition(leased)",
		"internal/bad/bad.go: Variable uses Transition",
		"internal/bad/bad.go: ReflectIndex uses Method",
		"internal/bad/bad.go: MethodExpr uses Grant",
		"internal/bad/bad.go: Reflect2 uses MethodByName",
		"internal/bad/bad.go: Reflect uses MethodByName",
		"internal/bad/linkname.go: comment uses go:linkname",
		"internal/bad/bad.go: Reflect uses AdmitAndLease",
		"internal/bad/bad.go: Sneak uses AdmitAndLease",
		"internal/bad/bad.go: ValueAlias uses AdmitPlacement",
		"internal/run/other.go: (T).CreateAttempt uses AdmitAndLease",
		"internal/run/other.go: Back uses LocalAdmit",
		"internal/store/wrapper.go: (Store).Take uses AdmitAndLease",
		"internal/run/other.go: Value uses LocalAdmit",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(planted, want) {
		t.Fatalf("planted module (%d files read):\n got %q\nwant %q", read, planted, want)
	}
	shipped, read := scanChokepoint(t, filepath.Join("..", ".."))
	if read < 100 {
		t.Fatalf("the shipped scan read %d files; the module root is wrong", read)
	}
	if len(shipped) != 0 {
		t.Fatalf("calls outside the chokepoint (%d files read):\n%s", read, strings.Join(shipped, "\n"))
	}
}
