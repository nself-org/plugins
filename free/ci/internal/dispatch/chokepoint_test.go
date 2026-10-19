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
	// Store.Transition today, with the most references each may hold. They are
	// reviewed one by one: none moves an attempt to leased. Store.Transition
	// itself accepts queued -> leased (the P7-CI-31 store tests rely on it and
	// this Ticket cannot edit transitions.go; debt), so a NEW reference changes a
	// count, or adds a function, and fails here until a reviewer extends the list.
	// A method value, a constant or concatenated target and a new wrapper all
	// change a count the same way.
	transitionUsers = map[string]int{
		"internal/dispatch.(Dispatcher).lose": 1, // leased/running -> lost
		"internal/lease.(Engine).PollHosted":  1, // leased -> running (hosted liveness)
		"internal/run.Recover":                2, // lost -> retryable | needs-operator
		"internal/run.strandAttempts":         2, // queued -> cancelled, finalizing -> failed (a computed target)
		"internal/run.CreateAttempt":          1, // queued -> cancelled (refused admission)
		"internal/run.(runState).trans":       1, // forwards its caller's target; every caller is checked below
	}
	// runnerWrappers (run.runState.trans and settle) forward their third argument to Store.Transition: every
	// call must name the target as a string literal other than "leased", and a
	// reference that is not a call (a method value) is refused. Only the wrappers
	// themselves forward a variable.
	runnerWrappers = map[string]bool{"trans": true, "settle": true}
	wrapperOwners  = map[string]bool{"internal/run.(runState).trans": true, "internal/run.(runState).settle": true}
	localFiles     = map[string]bool{"internal/run/attempt.go": true, "internal/run/dispatch_hook.go": true}
)

// banned are the admission functions only the chokepoint may name.
var banned = map[string]bool{"AdmitAndLease": true, "AdmitPlacement": true, "LeaseGrant": true, "Grant": true, "LocalAdmit": true}

// reflective names reach a method by string or index, which no scan can follow;
// they are refused in every non-test file, in the definers too. Method is
// reflective only in a file that imports reflect (http.Request.Method is not).
var reflective = map[string]bool{"MethodByName": true}

func isLiteral(e ast.Expr, want string) (isLit, matches bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false, false
	}
	return true, strings.Trim(lit.Value, "\"`") == want
}

// ownerOf names the function a declaration is: "pkg-level" for a var or const, the
// function name, or (Type).Name for a method (generic receivers unwrapped).
func ownerOf(decl ast.Decl) (string, *ast.Ident) {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok {
		return "package level", nil
	}
	owner := fn.Name.Name
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
	return owner, fn.Name
}

// scanFile reports the uses in one parsed file that the allowlists do not cover.
func scanFile(f *ast.File, rel string, add func(string)) {
	dir := filepath.ToSlash(filepath.Dir(rel))
	usesReflect := false
	for _, imp := range f.Imports {
		usesReflect = usesReflect || strings.Trim(imp.Path.Value, "\"`") == "reflect"
	}
	for _, cg := range f.Comments { // //go:linkname reaches a function without naming it in code
		for _, c := range cg.List {
			if strings.Contains(c.Text, "go:linkname") {
				add(rel + ": comment uses go:linkname")
			}
		}
	}
	for _, decl := range f.Decls {
		owner, declName := ownerOf(decl)
		key := dir + "." + owner
		report := func(name string) { add(rel + ": " + owner + " uses " + name) }
		callee := map[*ast.SelectorExpr]bool{} // selectors that are the function of a call
		transitions := 0
		ast.Inspect(decl, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					callee[sel] = true
				}
			}
			return true
		})
		ast.Inspect(decl, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				sel, ok := x.Fun.(*ast.SelectorExpr)
				if !ok || definerDirs[dir] {
					return true
				}
				switch {
				case sel.Sel.Name == "Transition" && len(x.Args) > 3:
					if _, leased := isLiteral(x.Args[3], "leased"); leased {
						report("Transition(leased)")
					}
				case runnerWrappers[sel.Sel.Name] && dir == "internal/run" && len(x.Args) > 2:
					if isLit, leased := isLiteral(x.Args[2], "leased"); leased || !isLit && !wrapperOwners[key] {
						report(sel.Sel.Name + "(" + map[bool]string{true: "leased", false: "non-literal"}[leased] + ")")
					}
				}
			case *ast.SelectorExpr:
				name := x.Sel.Name
				switch {
				case reflective[name], name == "Method" && usesReflect:
					report(name)
				case banned[name] && name != "LocalAdmit" && !chokeFuncs[key]:
					report(name)
				case name == "LocalAdmit" && !localFiles[rel]:
					report(name)
				case name == "Transition" && !definerDirs[dir]:
					transitions++
				case runnerWrappers[name] && dir == "internal/run" && !callee[x] && !wrapperOwners[key]:
					report(name + "(method value)")
				}
			case *ast.Ident:
				if banned[x.Name] && x != declName && !(x.Name == "LocalAdmit" && localFiles[rel]) && (x.Name == "LocalAdmit" || !chokeFuncs[key]) {
					report(x.Name)
				}
			case *ast.BasicLit:
				if x.Kind == token.STRING && banned[strings.Trim(x.Value, "\"`")] && !chokeFuncs[key] {
					report(strings.Trim(x.Value, "\"`"))
				}
			}
			return true
		})
		if transitions > transitionUsers[key] {
			report("Transition")
		}
	}
}

// scanChokepoint returns "file: func uses Name" for every use outside the
// allowlists, and how many files it read.
func scanChokepoint(t *testing.T, root string) (violations []string, files int) {
	t.Helper()
	seen := map[string]bool{}
	add := func(v string) { // a selector and its identifier are two nodes
		if !seen[v] {
			seen[v] = true
			violations = append(violations, v)
		}
	}
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
		// every build constraint is read: the parser sees the file whatever its tags say
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}
		files++
		scanFile(f, filepath.ToSlash(rel), add)
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
		"internal/run/other.go: Forward uses trans(non-literal)",
		"internal/run/other.go: Settle uses settle(leased)",
		"internal/run/other.go: ValueTrans uses trans(method value)",
		"internal/run/attempt.go: CreateAttempt uses Transition",
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
