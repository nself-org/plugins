// Regression tests for D-0922: a repo whose root carries only a Cargo.toml
// (the bundles repo) kept its Go modules under paid/*/go. Stack detection saw
// "rust" alone, cargo was absent on some hosts, every rust gate skipped, and a
// Go-only change failed with gate:no-substantive-checks.
package legacy

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// nestedGoRepo builds a root with a Cargo.toml and one Go module in paid/x/go,
// plus module files under directories the scan must ignore.
func nestedGoRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "Cargo.toml", "[workspace]\nmembers = []\n")
	writeFile(t, root, "paid/x/go/go.mod", "module example.test/x\n\ngo 1.21\n")
	writeFile(t, root, "paid/x/go/x.go", "package x\n\nfunc One() int { return 1 }\n")
	writeFile(t, root, "paid/x/go/x_test.go", "package x\n\nimport \"testing\"\n\nfunc TestOne(t *testing.T) {\n\tif One() != 1 {\n\t\tt.Fatal(\"one\")\n\t}\n}\n")
	writeFile(t, root, "node_modules/dep/go.mod", "module example.test/dep\n")
	writeFile(t, root, "target/go.mod", "module example.test/target\n")
	writeFile(t, root, "vendor/m/go.mod", "module example.test/m\n")
	return root
}

func TestDetectStacks_NestedGoModuleUnderCargoRoot(t *testing.T) {
	stacks := detectStacks(nestedGoRepo(t))
	want := []string{"go", "rust"}
	if !reflect.DeepEqual(stacks, want) {
		t.Fatalf("detectStacks = %v, want %v", stacks, want)
	}
}

func TestGoModuleDirs(t *testing.T) {
	root := nestedGoRepo(t)
	if got, want := goModuleDirs(root), []string{"paid/x/go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("goModuleDirs = %v, want %v (node_modules, target and vendor are skipped)", got, want)
	}
	writeFile(t, root, "go.mod", "module example.test/root\n")
	if got := goModuleDirs(root); !reflect.DeepEqual(got, []string{"."}) {
		t.Fatalf("a root go.mod must stay the only module, got %v", got)
	}
}

// With cargo missing, a Go-only repo that also has a root Cargo.toml must be
// gated by its Go modules, not rejected for having no substantive check.
func TestRun_NestedGoModuleIsGatedWhenCargoIsMissing(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("skip: go not on PATH")
	}
	gofmtBin, err := exec.LookPath("gofmt")
	if err != nil {
		t.Skip("skip: gofmt not on PATH")
	}
	bin := t.TempDir() // a PATH with go and gofmt only: no cargo, no gitleaks
	for _, b := range []string{goBin, gofmtBin} {
		if err := os.Symlink(b, filepath.Join(bin, filepath.Base(b))); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("GOFLAGS", "-mod=mod")

	res, err := Run(Config{RepoRoot: nestedGoRepo(t), SkipGitleaks: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("Go-only change must pass:\n%s", dumpGates(res))
	}
	var sawTest bool
	for _, g := range res.Gates {
		if g.Name == "gate:no-substantive-checks" {
			t.Fatalf("unexpected gate:no-substantive-checks:\n%s", dumpGates(res))
		}
		if strings.HasPrefix(g.Name, "go:test") && strings.Contains(g.Name, "paid/x/go") && g.Substantive && !g.Skipped {
			sawTest = true
		}
	}
	if !sawTest {
		t.Fatalf("no go:test gate ran for paid/x/go:\n%s", dumpGates(res))
	}
}
