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
	chokeDirs  = map[string]bool{"internal/store": true, "internal/lease": true, "internal/dispatch": true}
	chokeFuncs = map[string]bool{"internal/run.CreateAttempt": true, "internal/run.LocalAdmit": true}
	localFiles = map[string]bool{"internal/run/attempt.go": true, "internal/run/dispatch_hook.go": true}
)

// banned are the admission functions only the chokepoint may name.
var banned = map[string]bool{"AdmitAndLease": true, "AdmitPlacement": true, "LeaseGrant": true, "Grant": true, "LocalAdmit": true}

// bannedUse names the admission function a node refers to, or "". A reference
// counts, not only a call: a method value (f := st.AdmitPlacement), a method
// expression ((*lease.Engine).Grant), a bare identifier (dot import) or the
// name as a string (reflection) all reach the function the same way.
func bannedUse(n ast.Node, declName *ast.Ident) string {
	switch x := n.(type) {
	case *ast.SelectorExpr:
		if banned[x.Sel.Name] {
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
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}
		files++
		for _, decl := range f.Decls {
			owner, declName := "package level", (*ast.Ident)(nil)
			if fn, ok := decl.(*ast.FuncDecl); ok {
				owner, declName = fn.Name.Name, fn.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				name := bannedUse(n, declName)
				switch {
				case name == "":
				case name == "LocalAdmit" && !localFiles[rel], name != "LocalAdmit" && !chokeDirs[dir] && !chokeFuncs[dir+"."+owner]:
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
		"internal/bad/bad.go: MethodExpr uses Grant",
		"internal/bad/bad.go: Reflect uses AdmitAndLease",
		"internal/bad/bad.go: Sneak uses AdmitAndLease",
		"internal/bad/bad.go: ValueAlias uses AdmitPlacement",
		"internal/run/other.go: Back uses LocalAdmit",
		"internal/run/other.go: Value uses LocalAdmit",
	}
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
