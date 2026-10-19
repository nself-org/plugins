package dispatch

// Purpose: R-SEC-22. dispatch.Admit is the only path that creates a lease or a
// local attempt, so no function outside the definers may call the store's
// admission functions. The scan parses the module's NON-TEST .go files; test
// files (CI-31 store tests, SCHED-18's race test, TRUST's CheckDispatcher) are
// out of scope, and the planted violation lives in a testdata module.
// Allowed callers: internal/store and internal/lease (the definers),
// internal/dispatch, and run.CreateAttempt with run.LocalAdmit (the nil-hook
// path CreateAttempt calls; LocalAdmit itself may only be called from
// run/attempt.go and run/dispatch_hook.go).

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

// bannedCall names the admission function a call expression invokes, or "".
func bannedCall(c *ast.CallExpr) string {
	name := ""
	switch f := c.Fun.(type) {
	case *ast.SelectorExpr:
		name = f.Sel.Name
	case *ast.Ident:
		name = f.Name
	}
	switch name {
	case "AdmitAndLease", "AdmitPlacement", "LeaseGrant", "LocalAdmit":
		return name
	case "Grant":
		if len(c.Args) == 4 { // lease.Engine.Grant(ctx, host, demand, incarnation)
			return name
		}
	}
	return ""
}

// scanChokepoint returns "file: func calls Name" for every call outside the
// allowlist, and how many files it read.
func scanChokepoint(t *testing.T, root string) (violations []string, files int) {
	t.Helper()
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
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}
		files++
		for _, decl := range f.Decls {
			owner := "package level"
			if fn, ok := decl.(*ast.FuncDecl); ok {
				owner = fn.Name.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := bannedCall(call)
				switch {
				case name == "":
				case name == "LocalAdmit" && !localFiles[rel]:
					violations = append(violations, rel+": "+owner+" calls "+name)
				case name != "LocalAdmit" && !chokeDirs[dir] && !chokeFuncs[dir+"."+owner]:
					violations = append(violations, rel+": "+owner+" calls "+name)
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
		"internal/bad/bad.go: Closure calls AdmitPlacement",
		"internal/bad/bad.go: Grantor calls Grant",
		"internal/bad/bad.go: Sneak calls AdmitAndLease",
		"internal/run/other.go: Back calls LocalAdmit",
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
