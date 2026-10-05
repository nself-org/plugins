package provision

// Purpose: TestSingleSSHExecSite proves free/ci starts ssh and scp from one
//   place only: the vendored cli sdk/go/remote funnel (R-AR-16). A second copy
//   of an ssh exec would bypass the sdk's path validation, option allowlist
//   and pinned host keys.
// Inputs:  every non-test .go file under free/ci, vendor included, parsed
//   with go/parser.
// Outputs: pass when the funnel package holds exactly one os/exec reference
//   and no other file calls exec.Command or exec.CommandContext on ssh, scp,
//   rsync or ssh-keyscan: a string literal, or a package-level const or var
//   of the same file that holds one. A program the walk cannot resolve
//   statically (the gate runners pick commands by name) is not counted.
// Constraints: the planted-fixture subtests prove the walker fails on a
//   second site, on aliased imports, on a const program and on a missing
//   funnel, so a pass on the real tree is not vacuous.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const funnelDir = "vendor/github.com/nself-org/cli/sdk/go/v2/remote"

var sshWord = regexp.MustCompile(`(^|[\s/])(ssh|scp|rsync|ssh-keyscan)(\s|$)`)

// scanSites walks root and returns the number of os/exec references inside
// the funnel package and a description of every other offending exec call.
func scanSites(t *testing.T, root string) (funnel int, bad []string) {
	t.Helper()
	files := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files++
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			return perr
		}
		f2, b2 := inspectFile(f, rel)
		funnel += f2
		bad = append(bad, b2...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if files == 0 {
		t.Fatalf("no .go files under %s (vacuous walk)", root)
	}
	return funnel, bad
}

// inspectFile counts exec references in the funnel and flags exec calls elsewhere.
func inspectFile(f *ast.File, rel string) (funnel int, bad []string) {
	alias := ""
	for _, im := range f.Imports {
		if p, _ := strconv.Unquote(im.Path.Value); p == "os/exec" {
			alias = "exec"
			if im.Name != nil {
				alias = im.Name.Name
			}
		}
	}
	if alias == "" || alias == "_" {
		return 0, nil
	}
	consts := fileStrings(f)
	inFunnel := filepath.ToSlash(filepath.Dir(rel)) == funnelDir
	isExec := func(n ast.Expr) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext") {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		return ok && id.Name == alias
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if isExec(x.Fun) && !inFunnel {
				if why := badCall(x, consts); why != "" {
					bad = append(bad, rel+": "+why)
				}
			}
		case *ast.SelectorExpr:
			if inFunnel && isExec(x) {
				funnel++
			}
		}
		return true
	})
	return funnel, bad
}

// fileStrings maps the package-level string const and var names of f to their literal.
func fileStrings(f *ast.File) map[string]string {
	m := map[string]string{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok || len(vs.Names) != len(vs.Values) {
				continue
			}
			for i, v := range vs.Values {
				if l, ok := v.(*ast.BasicLit); ok && l.Kind == token.STRING {
					m[vs.Names[i].Name], _ = strconv.Unquote(l.Value)
				}
			}
		}
	}
	return m
}

// strArg returns the string an exec argument holds when it is a literal or a
// file-level string const or var.
func strArg(e ast.Expr, consts map[string]string) (string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind == token.STRING {
			v, _ := strconv.Unquote(x.Value)
			return v, true
		}
	case *ast.Ident:
		v, ok := consts[x.Name]
		return v, ok
	}
	return "", false
}

// badCall explains why an exec call outside the funnel is refused, or "".
func badCall(c *ast.CallExpr, consts map[string]string) string {
	prog := 0
	if sel := c.Fun.(*ast.SelectorExpr); sel.Sel.Name == "CommandContext" {
		prog = 1
	}
	if len(c.Args) <= prog {
		return "exec call without a program"
	}
	for _, a := range c.Args[prog:] {
		if v, ok := strArg(a, consts); ok && sshWord.MatchString(v) {
			return "exec call that starts " + strconv.Quote(v)
		}
	}
	return ""
}

func TestSingleSSHExecSite(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, funnelDir)); err != nil {
		t.Fatalf("vendored sdk/go/remote not found: %v", err)
	}

	t.Run("tree", func(t *testing.T) {
		funnel, bad := scanSites(t, root)
		if funnel != 1 {
			t.Errorf("os/exec references in %s = %d, want exactly 1", funnelDir, funnel)
		}
		if len(bad) != 0 {
			t.Errorf("ssh/scp exec outside the funnel: %v", bad)
		}
	})

	write := func(t *testing.T, dir, rel, src string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	funnelSrc := "package remote\nimport \"os/exec\"\nvar commandContext = exec.CommandContext\n"
	cases := []struct {
		name       string
		src        string
		wantFunnel int
		wantBad    bool
	}{
		{"clean", "package x\nimport \"os/exec\"\nfunc f() { exec.Command(\"bash\", \"-c\", \"true\") }\n", 1, false},
		{"planted ssh", "package x\nimport \"os/exec\"\nfunc f() { exec.Command(\"ssh\", \"h\") }\n", 1, true},
		{"planted scp ctx", "package x\nimport (\"context\"; \"os/exec\")\nfunc f(c context.Context) { exec.CommandContext(c, \"scp\", \"a\", \"b\") }\n", 1, true},
		{"aliased import", "package x\nimport e \"os/exec\"\nfunc f() { e.Command(\"ssh\") }\n", 1, true},
		{"shell string", "package x\nimport \"os/exec\"\nfunc f() { exec.Command(\"sh\", \"-c\", \"ssh h id\") }\n", 1, true},
		{"const program", "package x\nimport \"os/exec\"\nconst p = \"ssh\"\nfunc f() { exec.Command(p, \"h\") }\n", 1, true},
		{"unresolvable program", "package x\nimport \"os/exec\"\nfunc f(p string) { exec.Command(p) }\n", 1, false},
	}
	for _, tc := range cases {
		t.Run("planted/"+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, funnelDir+"/exec.go", funnelSrc)
			write(t, dir, "cmd/x.go", tc.src)
			funnel, bad := scanSites(t, dir)
			if funnel != tc.wantFunnel || (len(bad) > 0) != tc.wantBad {
				t.Fatalf("funnel=%d bad=%v, want funnel=%d bad=%v", funnel, bad, tc.wantFunnel, tc.wantBad)
			}
		})
	}
	t.Run("planted/no funnel", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "cmd/x.go", "package x\n")
		if funnel, _ := scanSites(t, dir); funnel == 1 {
			t.Fatal("a tree without the funnel counted as one site")
		}
	})
	t.Run("planted/second funnel site", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, funnelDir+"/exec.go", funnelSrc+"var other = exec.Command\n")
		if funnel, _ := scanSites(t, dir); funnel != 2 {
			t.Fatalf("funnel = %d, want 2", funnel)
		}
	})
}
