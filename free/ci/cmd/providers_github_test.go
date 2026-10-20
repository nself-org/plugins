package main

// Purpose: the `providers github init` tests of P7-HOST-03: the command surface, byte-stable stdout,
// the write refusal on a hand-written file, the release pins path, and --check over real local git.
// Inputs:  temp git repositories under the test's TMPDIR, the templates/github renderer.
// Outputs: pass or fail.
// Constraints: no network; git only against local repositories; the release fetch is replaced.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

func newestPin(t *testing.T) string {
	t.Helper()
	commits, err := githubtmpl.SourceCommits()
	if err != nil || len(commits) == 0 {
		t.Fatalf("pins: %v", err)
	}
	return commits[len(commits)-1]
}

func TestGithubInitRegistered(t *testing.T) {
	e, ok := v15Subcommands["providers github init"]
	if !ok || !e.V15Only || e.Handler == nil || e.Flags == nil {
		t.Fatalf("entry: %+v ok=%v", e, ok)
	}
	if _, v14 := subcommands["providers github init"]; v14 {
		t.Fatal("the command is reachable from v1.4")
	}
}

// TestGithubInitStdout proves --stdout is byte-stable and equals the renderer's output for the newest pin.
func TestGithubInitStdout(t *testing.T) {
	want, err := githubtmpl.Render(githubtmpl.Options{AgentSource: githubtmpl.SourceMode, AgentCommit: newestPin(t)})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		code, out, errs := capture(t, func() int { return runGithubInit([]string{"--stdout", "--agent-source", "source"}) })
		if code != 0 || out != string(want) || errs != "" {
			t.Fatalf("run %d: code=%d err=%q identical=%v", i, code, errs, out == string(want))
		}
	}
	for _, bad := range [][]string{{"--stdout", "--check"}, {"extra"}, {"--agent-source", "carrier-pigeon", "--stdout"}} {
		if code, _, _ := capture(t, func() int { return runGithubInit(bad) }); code == 0 {
			t.Errorf("%v succeeded", bad)
		}
	}
}

func TestGithubInitWrite(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := capture(t, func() int { return runGithubInit([]string{"--dir", dir, "--agent-source", "source", "--json"}) })
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json: %v: %s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, githubtmpl.Path))
	if err != nil || !strings.HasPrefix(string(raw), githubtmpl.HeaderMarker) {
		t.Fatalf("file: %v", err)
	}
	if info, _ := os.Stat(filepath.Join(dir, githubtmpl.Path)); info.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", info.Mode())
	}
	if code, _, _ = capture(t, func() int { return runGithubInit([]string{"--dir", dir, "--agent-source", "source"}) }); code != 0 {
		t.Fatalf("second write: %d", code)
	}
	hand := filepath.Join(dir, githubtmpl.Path)
	if err = os.WriteFile(hand, []byte("name: mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errs := capture(t, func() int { return runGithubInit([]string{"--dir", dir, "--agent-source", "source"}) })
	if after, _ := os.ReadFile(hand); code == 0 || string(after) != "name: mine\n" || !strings.Contains(errs, "not generated") {
		t.Fatalf("a hand-written file was overwritten: code=%d err=%q", code, errs)
	}
}

// TestGithubInitRelease proves release mode embeds only what the verified fetch returned, and that
// without a verified release (no pinned version, no trusted key) nothing is written.
func TestGithubInitRelease(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := capture(t, func() int { return runGithubInit([]string{"--dir", dir, "--agent-source", "release"}) }); code == 0 || !strings.Contains(errs, "--agent-source source") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, githubtmpl.Path)); err == nil {
		t.Fatal("a file was written without a verified release")
	}
	saved := githubReleasePins
	defer func() { githubReleasePins = saved }()
	githubReleasePins = func(context.Context) (string, []githubtmpl.ReleasePin, error) {
		return "1.5.0", []githubtmpl.ReleasePin{{RunnerOS: "Linux", RunnerArch: "X64", URL: "https://example.test/v1.5.0/nself-ci-agent-linux-amd64", SHA256: strings.Repeat("cd", 32)}}, nil
	}
	code, out, _ := capture(t, func() int { return runGithubInit([]string{"--stdout", "--agent-source", "release"}) })
	if code != 0 || !strings.Contains(out, "# agent-release: 1.5.0") || !strings.Contains(out, strings.Repeat("cd", 32)) {
		t.Fatalf("code=%d", code)
	}
}

func ghGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.test", "-c", "protocol.file.allow=always", "-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// TestGithubInitCheck proves --check compares the default-branch and untrusted-ref copies through
// local git, exits 1 naming the ref when either is missing or drifts, and prints the exact commands
// that create or update the untrusted ref.
func TestGithubInitCheck(t *testing.T) {
	base := t.TempDir()
	remote, work := filepath.Join(base, "remote.git"), filepath.Join(base, "work")
	git(t, base, "init", "--bare", "--initial-branch=main", remote)
	git(t, base, "init", "--initial-branch=main", work)
	git(t, work, "remote", "add", "origin", remote)
	args := []string{"--dir", work, "--agent-source", "source"}
	if code, _, _ := capture(t, func() int { return runGithubInit(args) }); code != 0 {
		t.Fatal("init failed")
	}
	git(t, work, "add", githubtmpl.Path)
	git(t, work, "commit", "-m", "workflow")
	git(t, work, "push", "origin", "main")
	git(t, work, "fetch", "origin")
	git(t, work, "remote", "set-head", "origin", "main")
	check := func() (int, string, string) {
		return capture(t, func() int { return runGithubInit(append([]string{"--check"}, args...)) })
	}
	code, out, errs := check()
	if code != 1 || !strings.Contains(out, "origin/main") || !strings.Contains(out, "ok") || !strings.Contains(out, "origin/"+hostedDefaultRef+" ") ||
		!strings.Contains(out, "missing") || !strings.Contains(out, "git push origin origin/main:refs/heads/"+hostedDefaultRef) || !strings.Contains(errs, "origin/"+hostedDefaultRef) {
		t.Fatalf("missing untrusted ref: code=%d\nout=%s\nerr=%s", code, out, errs)
	}
	git(t, work, "push", "origin", "origin/main:refs/heads/"+hostedDefaultRef)
	git(t, work, "fetch", "origin")
	if code, out, errs = check(); code != 0 || strings.Contains(out, "missing") || strings.Contains(out, "drifted") {
		t.Fatalf("clean: code=%d\n%s\n%s", code, out, errs)
	}
	edited := []byte(githubtmpl.HeaderMarker + "\n# template: v1\nname: edited\n")
	ghGit(t, work, "checkout", "-q", "-b", "side")
	if err := os.WriteFile(filepath.Join(work, githubtmpl.Path), edited, 0o644); err != nil {
		t.Fatal(err)
	}
	ghGit(t, work, "commit", "-qam", "edited")
	ghGit(t, work, "push", "--force", "origin", "side:refs/heads/"+hostedDefaultRef)
	ghGit(t, work, "fetch", "origin")
	code, out, errs = check()
	if code != 1 || statusLines(out, "drifted") != 1 || !strings.Contains(out, "git push --force-with-lease origin origin/main:refs/heads/"+hostedDefaultRef) ||
		!strings.Contains(errs, "origin/"+hostedDefaultRef+" differs") || strings.Contains(errs, "origin/main") {
		t.Fatalf("untrusted ref drift: code=%d\n%s\n%s", code, out, errs)
	}
	ghGit(t, work, "push", "--force", "origin", "side:refs/heads/main")
	ghGit(t, work, "fetch", "origin")
	code, out, errs = check()
	if code != 1 || statusLines(out, "drifted") != 2 || !strings.Contains(errs, "origin/main, origin/"+hostedDefaultRef+" differ") {
		t.Fatalf("both copies drifted: code=%d\n%s\n%s", code, out, errs)
	}
	code, out, _ = capture(t, func() int { return runGithubInit(append([]string{"--check", "--json"}, args...)) })
	var env struct {
		Data struct {
			OK     bool `json:"ok"`
			Copies []struct{ Ref, Status string }
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || code != 1 || env.Data.OK || len(env.Data.Copies) != 2 {
		t.Fatalf("json check: code=%d err=%v out=%s", code, err, out)
	}
}

const hostedDefaultRef = "nself-ci/untrusted"

// statusLines counts the report lines (not the advice) that end in status.
func statusLines(out, status string) int {
	n := 0
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, "origin/") && strings.HasSuffix(strings.TrimSpace(ln), status) {
			n++
		}
	}
	return n
}
