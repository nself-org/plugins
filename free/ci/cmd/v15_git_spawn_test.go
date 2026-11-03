//go:build !windows

package main

// Purpose: count real git processes on the complete tiny bare-run path.
// Inputs: one committed Go module and a PATH shim that execs the real git.
// Outputs: an invocation budget regression, including argument logs on failure.
// Constraints: gate tools are deterministic stubs, so toolchain VCS probes and
// installed advisory scanners cannot change the coordinator's process budget.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/trace"
)

func TestRunV15EmitsTraceSpans(t *testing.T) {
	if os.Getenv("NSELF_CI_TRACE_TEST_CHILD") == "1" {
		_, out, code := tinyGitRun(t, false)
		if code != 0 {
			t.Fatalf("tiny run exit %d: %s", code, out)
		}
		return
	}
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	t.Setenv("NSELF_CI_TRACE", path)
	t.Setenv("NSELF_CI_TRACE_TEST_CHILD", "1")
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunV15EmitsTraceSpans$")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("trace run: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	spans := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var span trace.Span
		if err := json.Unmarshal([]byte(line), &span); err != nil {
			t.Fatalf("invalid trace line %q: %v", line, err)
		}
		if strings.HasPrefix(span.Name, "v15.") {
			spans[span.Name] = true
		}
	}
	if !spans["v15.total"] || len(spans) < 4 {
		t.Fatalf("want v15.total and at least three other V15 spans, got %v", spans)
	}
}

func TestTinyRunGitSpawnBudget(t *testing.T) {
	calls, out, code := tinyGitRun(t, false)
	if code != 0 {
		t.Fatalf("tiny run exit %d: %s", code, out)
	}
	// Plan 4; entry/end revision 4; source trust 6; project identity 2;
	// options/policy 2; evidence 7. No config/lockfiles, snapshot or status.
	// The previous path required 28: six plan calls and one notify remote read.
	if len(calls) == 0 || len(calls) > 25 {
		t.Fatalf("tiny run spawned %d git processes (budget 25):\n%s", len(calls), strings.Join(calls, "\n"))
	}
}

func tinyGitRun(t *testing.T, origin bool) ([]string, string, int) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	realGit, err = filepath.Abs(realGit)
	if err != nil {
		t.Fatal(err)
	}
	r := &evRepo{t: t, dir: t.TempDir(), home: t.TempDir(), tmp: t.TempDir()}
	t.Chdir(r.dir)
	home, bin := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_NAMESPACE", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NSELF_INVOKED_BY", "")
	t.Setenv("NSELF_CI_COORDINATOR", "")
	t.Setenv("NSELF_CI_BASE_SHA", "")
	t.Setenv("NSELF_CI_CACHE_DISABLE", "")
	t.Setenv("NSELF_CI_REPO", r.dir)
	t.Setenv("NSELF_CI_HOME", r.home)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("TMPDIR", r.tmp)
	git(t, r.dir, "init", "-q", "-b", "main")
	put(t, r.dir, "go.mod", "module tiny.example/fixture\n\ngo 1.22\n")
	put(t, r.dir, "x.go", "package fixture\n")
	git(t, r.dir, "add", "go.mod", "x.go")
	git(t, r.dir, "commit", "-q", "-m", "fixture")
	if origin {
		remote := filepath.Join(t.TempDir(), "origin.git")
		git(t, r.dir, "init", "-q", "--bare", "-b", "main", remote)
		git(t, r.dir, "remote", "add", "origin", remote)
		git(t, r.dir, "push", "-q", "origin", "main")
		git(t, r.dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	}
	log := filepath.Join(t.TempDir(), "git-calls")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	shim := "#!/bin/sh\nline=$(printf '%s\\t' \"$@\")\nprintf '%s\\n' \"$line\" >> " + quote(log) + "\nexec " + quote(realGit) + " \"$@\"\n"
	writeExecutable(t, filepath.Join(bin, "git"), shim)
	writeExecutable(t, filepath.Join(bin, "go"), "#!/bin/sh\nif [ \"$1\" = version ]; then printf 'go version go1.22.0 linux/amd64\\n'; fi\n")
	for _, name := range []string{"gofmt", "gitleaks"} {
		writeExecutable(t, filepath.Join(bin, name), "#!/bin/sh\nexit 0\n")
	}
	for _, name := range []string{"env", "sh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	// Only required fixture tools are visible; optional scanners stay absent.
	t.Setenv("PATH", bin)
	oldWhere, oldChosen := whereValue, whereChosen
	whereValue = ""
	t.Cleanup(func() { whereValue, whereChosen = oldWhere, oldChosen })
	out, stderr, code := r.ci("--no-status")
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n"), out + stderr, code
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
