package provision

// Purpose: TestSingleSSHExecSite proves free/ci starts ssh and scp from one
//   place only: the vendored cli sdk/go/remote funnel (R-AR-16). A second copy
//   of an ssh exec would bypass the sdk's path validation, option allowlist
//   and pinned host keys.
// Inputs:  every non-test .go file under free/ci, vendor included, parsed
//   with go/parser.
// Outputs: pass when the funnel package holds exactly one os/exec reference
//   and nothing else starts a process unsafely. Outside the funnel, every
//   call to exec.Command, exec.CommandContext (also through an alias, a dot
//   import or a function value), os.StartProcess, syscall.Exec or
//   syscall.ForkExec must name its program as a string literal or a
//   package-level const (resolved across every file of the package), that
//   program and every string literal in the call must not hold an ssh, scp,
//   rsync or ssh-keyscan invocation (so "ssh "+dest inside sh -c fails), and
//   a program the walk cannot resolve fails unless its file:function is in
//   dynamicProgram.
// Constraints: the planted-fixture subtests prove each bypass shape fails, so
//   a pass on the real tree is not vacuous.

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

// dynamicProgram lists the only call sites allowed a program the walk cannot
// resolve to a literal (file relative to free/ci : enclosing function). Each
// runs a gate command picked by name, never ssh.
var dynamicProgram = map[string]bool{
	"internal/legacy/gate_runners.go:runStep":   true,
	"internal/serve/serve_job_report.go:runCmd": true,
	"internal/serve/serve_job.go:runGateDirect": true,
	"internal/exec/process.go:runCommand":       true,
	// golang.org/x/sys/unix (vendored for the pure-Go SQLite driver, P7-CI-30):
	// the exported Exec wrapper around syscall.Exec. A library entry point, not
	// a call site; nothing under free/ci calls unix.Exec.
	"vendor/golang.org/x/sys/unix/syscall_unix.go:Exec":      true,
	"vendor/golang.org/x/sys/unix/syscall_zos_s390x.go:Exec": true,
}

type parsed struct {
	rel string
	f   *ast.File
}

// scanSites walks root and returns the number of os/exec references inside
// the funnel package and a description of every other offending call.
func scanSites(t *testing.T, root string) (funnel int, bad []string) {
	t.Helper()
	var files []parsed
	consts := map[string]map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			return perr
		}
		files = append(files, parsed{rel, f})
		dir := filepath.ToSlash(filepath.Dir(rel))
		if consts[dir] == nil {
			consts[dir] = map[string]string{}
		}
		collectConsts(f, consts[dir])
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(files) == 0 {
		t.Fatalf("no .go files under %s (vacuous walk)", root)
	}
	for _, p := range files {
		f2, b2 := inspectFile(p.f, p.rel, consts[filepath.ToSlash(filepath.Dir(p.rel))])
		funnel += f2
		bad = append(bad, b2...)
	}
	return funnel, bad
}

// collectConsts adds every string const of f (package level or local) to m.
func collectConsts(f *ast.File, m map[string]string) {
	for pass := 0; pass < 2; pass++ {
		ast.Inspect(f, func(n ast.Node) bool {
			gd, ok := n.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				return true
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				if len(vs.Names) != len(vs.Values) {
					continue
				}
				for i, v := range vs.Values {
					if s, ok := strValue(v, m); ok {
						m[vs.Names[i].Name] = s
					}
				}
			}
			return true
		})
	}
}

// strValue returns the string an expression holds when it is a literal or a known const.
func strValue(e ast.Expr, consts map[string]string) (string, bool) {
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

// importNames returns the local names of os/exec, os and syscall in f, and
// whether os/exec is dot-imported.
func importNames(f *ast.File) (execN, osN, sysN string, dot bool) {
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		name := ""
		if im.Name != nil {
			name = im.Name.Name
		}
		switch p {
		case "os/exec":
			if name == "." {
				dot = true
			} else if name != "_" {
				execN = "exec"
				if name != "" {
					execN = name
				}
			}
		case "os":
			osN = "os"
			if name != "" {
				osN = name
			}
		case "syscall":
			sysN = "syscall"
			if name != "" {
				sysN = name
			}
		}
	}
	return
}

// inspectFile counts exec references in the funnel and flags process starts elsewhere.
func inspectFile(f *ast.File, rel string, consts map[string]string) (funnel int, bad []string) {
	execN, osN, sysN, dot := importNames(f)
	inFunnel := filepath.ToSlash(filepath.Dir(rel)) == funnelDir
	// starter reports the program-argument index when fun names a process start.
	starter := func(fun ast.Expr) (int, bool) {
		switch x := fun.(type) {
		case *ast.SelectorExpr:
			id, ok := x.X.(*ast.Ident)
			if !ok {
				return 0, false
			}
			switch {
			case execN != "" && id.Name == execN && x.Sel.Name == "Command",
				osN != "" && id.Name == osN && x.Sel.Name == "StartProcess",
				sysN != "" && id.Name == sysN && (x.Sel.Name == "Exec" || x.Sel.Name == "ForkExec"):
				return 0, true
			case execN != "" && id.Name == execN && x.Sel.Name == "CommandContext":
				return 1, true
			}
		case *ast.Ident:
			if dot && x.Name == "Command" {
				return 0, true
			}
			if dot && x.Name == "CommandContext" {
				return 1, true
			}
		}
		return 0, false
	}
	for _, decl := range f.Decls {
		fn := ""
		if fd, ok := decl.(*ast.FuncDecl); ok {
			fn = fd.Name.Name
		}
		called := map[ast.Node]bool{}
		ast.Inspect(decl, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && inFunnel {
				if id, ok := sel.X.(*ast.Ident); ok && execN != "" && id.Name == execN &&
					(sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
					funnel++
				}
			}
			if inFunnel {
				return true
			}
			if call, ok := n.(*ast.CallExpr); ok {
				if idx, ok := starter(call.Fun); ok {
					called[call.Fun] = true
					if why := badCall(call, idx, consts, dynamicProgram[rel+":"+fn]); why != "" {
						bad = append(bad, rel+": "+why)
					}
				}
				return true
			}
			if e, ok := n.(ast.Expr); ok && !called[n] {
				if _, ok := starter(e); ok {
					bad = append(bad, rel+": process start used as a function value")
				}
			}
			return true
		})
	}
	return funnel, bad
}

// badCall explains why a process start outside the funnel is refused, or "".
func badCall(c *ast.CallExpr, prog int, consts map[string]string, exempt bool) string {
	if len(c.Args) <= prog {
		return "process start without a program"
	}
	why := ""
	if v, ok := strValue(c.Args[prog], consts); !ok && !exempt {
		why = "process start whose program is not a literal or package const"
	} else if ok && sshWord.MatchString(v) {
		why = "process start of " + strconv.Quote(v)
	}
	for _, a := range c.Args {
		ast.Inspect(a, func(n ast.Node) bool {
			if e, ok := n.(ast.Expr); ok && why == "" {
				if v, ok := strValue(e, consts); ok && sshWord.MatchString(v) {
					why = "process start with an ssh invocation in " + strconv.Quote(v)
				}
			}
			return true
		})
	}
	return why
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
		name    string
		src     string
		extra   string // second file of the same package
		wantBad bool
	}{
		{"clean", "package x\nimport \"os/exec\"\nfunc f() { exec.Command(\"bash\", \"-c\", \"true\") }\n", "", false},
		{"planted ssh", "package x\nimport \"os/exec\"\nfunc f() { exec.Command(\"ssh\", \"h\") }\n", "", true},
		{"planted scp ctx", "package x\nimport (\"context\"; \"os/exec\")\nfunc f(c context.Context) { exec.CommandContext(c, \"scp\", \"a\", \"b\") }\n", "", true},
		{"aliased import", "package x\nimport e \"os/exec\"\nfunc f() { e.Command(\"ssh\") }\n", "", true},
		{"shape1 function value alias", "package x\nimport \"os/exec\"\nvar f = exec.Command\nfunc g() { f(\"ssh\", \"h\") }\n", "", true},
		{"shape2 local string program", "package x\nimport \"os/exec\"\nfunc g() { p := \"ssh\"; exec.Command(p, \"h\") }\n", "", true},
		{"shape3 shell concatenation", "package x\nimport (\"context\"; \"os/exec\")\nfunc g(ctx context.Context, dest string) { exec.CommandContext(ctx, \"sh\", \"-c\", \"ssh \"+dest) }\n", "", true},
		{"shape4 dot import", "package x\nimport . \"os/exec\"\nfunc g() { Command(\"ssh\", \"h\") }\n", "", true},
		{"shape5 const in another file", "package x\nimport \"os/exec\"\nfunc g() { exec.Command(prog, \"h\") }\n", "package x\nconst prog = \"ssh\"\n", true},
		{"os.StartProcess ssh", "package x\nimport \"os\"\nfunc g() { os.StartProcess(\"/usr/bin/ssh\", nil, nil) }\n", "", true},
		{"syscall.Exec variable", "package x\nimport \"syscall\"\nfunc g(p string) { syscall.Exec(p, nil, nil) }\n", "", true},
		{"unresolvable program", "package x\nimport \"os/exec\"\nfunc f(p string) { exec.Command(p) }\n", "", true},
		{"const non-ssh program", "package x\nimport \"os/exec\"\nconst p = \"git\"\nfunc f() { exec.Command(p, \"status\") }\n", "", false},
	}
	for _, tc := range cases {
		t.Run("planted/"+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, funnelDir+"/exec.go", funnelSrc)
			write(t, dir, "cmd/x.go", tc.src)
			if tc.extra != "" {
				write(t, dir, "cmd/y.go", tc.extra)
			}
			funnel, bad := scanSites(t, dir)
			if funnel != 1 || (len(bad) > 0) != tc.wantBad {
				t.Fatalf("funnel=%d bad=%v, want funnel=1 bad=%v", funnel, bad, tc.wantBad)
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
