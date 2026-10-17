package main

// Purpose: run the functions command tree in-process the way main does, with
// stdout and stderr captured, plus the golden-file helper of the JSON tests.
//
// Inputs: args for run, and a testEnv (private project dir, a closed or live
// functions port, optional stub docker).
//
// Outputs: cliResult{Stdout, Stderr, Exit}; golden comparisons against
// testdata/json (set GOLDEN_UPDATE=1 to rewrite them).
//
// Constraints: no network beyond 127.0.0.1 test servers, no real docker (PATH is
// a private directory), fake values only. Not parallel: it swaps os.Stdout,
// os.Stderr, the working directory and the environment.

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/nself-org/cli/sdk/go/v2/compat"
)

// goldenDir is testdata/json, resolved before any test changes directory.
var goldenDir = func() string {
	d, err := filepath.Abs(filepath.Join("..", "testdata", "json"))
	if err != nil {
		panic(err)
	}
	return d
}()

type cliResult struct {
	Stdout, Stderr string
	Exit           int
}

// testEnv is one private project directory with the functions port pointed at
// a closed port (nothing answers) unless a test sets FUNCTIONS_PORT itself.
type testEnv struct {
	Dir  string
	Port string
}

// newTestEnv chdirs into a fresh project dir and gives docker an empty PATH.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()
	t.Setenv("FUNCTIONS_PORT", port)
	t.Setenv("PROJECT_NAME", "fakeproj")
	t.Setenv("PATH", t.TempDir())
	return &testEnv{Dir: dir, Port: port}
}

// stubDocker puts a docker stand-in on PATH. It prints stdout lines only (the
// two streams are read concurrently, so their order is not fixed) and exits
// with code.
func stubDocker(t *testing.T, exit int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"2026-01-01 GET /v1/fn-a 200\"\necho \"unrelated line\"\n" +
		"echo \"fn-a started\"\nexit " + strconv.Itoa(exit) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// resetFlags returns every flag of the tree to its default; cobra keeps flag
// values between Execute calls on the same command objects.
func resetFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
	for _, sub := range c.Commands() {
		resetFlags(sub)
	}
}

// runCLI executes args as `nself functions <args>`.
func runCLI(t *testing.T, args ...string) cliResult {
	t.Helper()
	resetFlags(rootCmd)
	dir := t.TempDir()
	so, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	se, err := os.Create(filepath.Join(dir, "err"))
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = so, se
	code := run(args)
	os.Stdout, os.Stderr = oldOut, oldErr
	_ = so.Close()
	_ = se.Close()
	out, _ := os.ReadFile(filepath.Join(dir, "out"))
	errb, _ := os.ReadFile(filepath.Join(dir, "err"))
	return cliResult{Stdout: string(out), Stderr: string(errb), Exit: code}
}

// scrub replaces the variable parts of an output with the golden placeholders.
func (e *testEnv) scrub(s string) string {
	dirs := []string{e.Dir}
	if r, err := filepath.EvalSymlinks(e.Dir); err == nil && r != e.Dir {
		dirs = append(dirs, r)
	}
	for _, d := range dirs {
		s = strings.ReplaceAll(s, d, "@DIR@")
	}
	return strings.ReplaceAll(s, e.Port, "@PORT@")
}

// modeTag is the golden-name suffix of the active compat mode.
func modeTag() string {
	if compat.V15() {
		return "v15"
	}
	return "v14"
}

// golden compares got with testdata/json/<name>.golden.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(goldenDir, name+".golden")
	// v14 goldens are the pre-contract bytes captured from the original code: never rewritten.
	if os.Getenv("GOLDEN_UPDATE") == "1" && !strings.Contains(name, "_v14") {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v", name, err)
	}
	if string(want) != got {
		t.Errorf("golden %s differs\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}
