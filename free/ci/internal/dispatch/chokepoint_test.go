package dispatch

// Purpose: R-SEC-22. dispatch.Admit is the only path that creates a lease or a
// local attempt, so no function outside the definers may name the store's
// admission functions. The scan parses the module's NON-TEST .go files; test
// files (CI-31 store tests, SCHED-18's race test, TRUST's CheckDispatcher) are
// out of scope, and the planted violations live in a testdata module.
// Allowed users: only the functions that wrap each definer: dispatch.Admit,
// lease.(Engine).Grant, run.CreateAttempt and run.LocalAdmit (the nil-hook path
// CreateAttempt calls; LocalAdmit itself may only be named in run/attempt.go and
// run/dispatch_hook.go). Any reference counts, not only a call (method values,
// method expressions, bare identifiers, the name as a string), reflection by
// name or index and //go:linkname are refused, and files are parsed whatever
// their build tags say. A syntactic scan cannot follow dataflow (a stored func
// value, an interface wrapper), which is why the one remaining way to lease an
// attempt without admission, Store.Transition to "leased", is refused at
// runtime (store.Transition, in test binaries too, TestSchedAdmissionTransitionGuard) and not here.

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
		"internal/storetest.Admit":      true, // test fixtures only; TestStoretestIsTestOnly keeps it out of shipped code
	}
	localFiles = map[string]bool{"internal/run/attempt.go": true, "internal/run/dispatch_hook.go": true}
)

// banned are the admission functions only the chokepoint may name.
var banned = map[string]bool{"AdmitAndLease": true, "AdmitPlacement": true, "LeaseGrant": true, "Grant": true, "LocalAdmit": true}

// reflective names reach a method by string or index, which no scan can follow;
// they are refused in every non-test file, in the definers too. Method is
// reflective only in a file that imports reflect (http.Request.Method is not).
var reflective = map[string]bool{"MethodByName": true}

// ownerOf names the function a declaration is: "package level" for a var or
// const, the function name, or (Type).Name for a method (generic receivers
// unwrapped).
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

// scanFile reports the uses in one parsed file that the allowlist does not cover.
func scanFile(f *ast.File, rel string, grants map[token.Pos]bool, add func(string)) {
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
		ast.Inspect(decl, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				name := x.Sel.Name
				if grants[x.Sel.Pos()] {
					return true
				}
				switch {
				case reflective[name], name == "Method" && usesReflect:
					report(name)
				case name == "LocalAdmit":
					if !localFiles[rel] {
						report(name)
					}
				case banned[name] && !chokeFuncs[key]:
					report(name)
				}
			case *ast.Ident:
				if banned[x.Name] && !grants[x.Pos()] && x != declName && !(x.Name == "LocalAdmit" && localFiles[rel]) && (x.Name == "LocalAdmit" || !chokeFuncs[key]) {
					report(x.Name)
				}
			case *ast.BasicLit:
				if x.Kind == token.STRING && banned[strings.Trim(x.Value, "\"`")] && !chokeFuncs[key] {
					report(strings.Trim(x.Value, "\"`"))
				}
			}
			return true
		})
	}
}

// scanChokepoint returns "file: func uses Name" for every use outside the
// allowlist, and how many files it read.
func scanChokepoint(t *testing.T, root string) (violations []string, files int) {
	t.Helper()
	root, absErr := filepath.Abs(root)
	if absErr != nil {
		t.Fatal(absErr)
	}
	roleGrants := roleGrantPositions(t, root)
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
		scanFile(f, filepath.ToSlash(rel), roleGrants[filepath.Clean(path)], add)
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
		"internal/bad/bad.go: MethodExpr uses Grant",
		"internal/bad/bad.go: Reflect uses AdmitAndLease",
		"internal/bad/bad.go: Reflect uses MethodByName",
		"internal/bad/bad.go: Reflect2 uses MethodByName",
		"internal/bad/bad.go: ReflectIndex uses Method",
		"internal/bad/bad.go: Sneak uses AdmitAndLease",
		"internal/bad/bad.go: ValueAlias uses AdmitPlacement",
		"internal/bad/linkname.go: comment uses go:linkname",
		"internal/run/other.go: (T).CreateAttempt uses AdmitAndLease",
		"internal/run/other.go: Back uses LocalAdmit",
		"internal/run/other.go: Value uses LocalAdmit",
		"internal/store/wrapper.go: (Store).Take uses AdmitAndLease",
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

// TestStoretestIsTestOnly: storetest.Admit may name AdmitPlacement because it is
// a test fixture, so no non-test file of the module may import the package.
func TestStoretestIsTestOnly(t *testing.T) {
	root := filepath.Join("..", "..")
	read := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (d.Name() == "vendor" || d.Name() == "testdata" || d.Name() == ".git" || d.Name() == "storetest") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}
		read++
		for _, imp := range f.Imports {
			if strings.HasSuffix(strings.Trim(imp.Path.Value, "\"`"), "/internal/storetest") {
				t.Errorf("%s imports internal/storetest, a test-only package", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read < 100 {
		t.Fatalf("read %d files; the module root is wrong", read)
	}
}
