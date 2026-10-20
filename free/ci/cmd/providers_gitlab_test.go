package main

// Purpose: the `providers gitlab init` tests of P7-HOST-06: the command surface, byte-stable stdout
// held to the golden of templates/gitlab, the write refusal on a hand-written file, and --check over
// real local git.
// Inputs:  temp git repositories under the test's TMPDIR, the templates/gitlab renderer and golden.
// Outputs: pass or fail.
// Constraints: no network; git only against local repositories.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

func TestGitlabInitRegistered(t *testing.T) {
	e, ok := v15Subcommands["providers gitlab init"]
	if !ok || !e.V15Only || e.Handler == nil || e.Flags == nil {
		t.Fatalf("entry: %+v ok=%v", e, ok)
	}
	if _, v14 := subcommands["providers gitlab init"]; v14 {
		t.Fatal("the command is reachable from v1.4")
	}
}

// TestGitlabInitStdout proves acceptance line 5: `providers gitlab init --stdout` is byte-stable and
// equals the golden file, which carries the GENERATED marker and the template version.
func TestGitlabInitStdout(t *testing.T) {
	want, err := os.ReadFile("../templates/gitlab/testdata/include.golden.yml")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		code, out, errs := capture(t, func() int { return runGitlabInit([]string{"--stdout"}) })
		if code != 0 || out != string(want) || errs != "" {
			t.Fatalf("run %d: code=%d err=%q identical=%v", i, code, errs, out == string(want))
		}
	}
	if !strings.HasPrefix(string(want), gitlabtmpl.HeaderMarker) || !strings.Contains(string(want), "# template: v1") {
		t.Error("the golden lost its marker or template version")
	}
	for _, bad := range [][]string{{"--stdout", "--check"}, {"extra"}, {"--nope"}} {
		if code, _, _ := capture(t, func() int { return runGitlabInit(bad) }); code == 0 {
			t.Errorf("%v succeeded", bad)
		}
	}
}

func TestGitlabInitWrite(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := capture(t, func() int { return runGitlabInit([]string{"--dir", dir, "--json"}) })
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json: %v: %s", err, out)
	}
	path := filepath.Join(dir, gitlabtmpl.Path)
	raw, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(raw), gitlabtmpl.HeaderMarker) {
		t.Fatalf("file: %v", err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", info.Mode())
	}
	if code, out, _ = capture(t, func() int { return runGitlabInit([]string{"--dir", dir}) }); code != 0 || !strings.Contains(out, "include:") {
		t.Fatalf("second write: %d %q", code, out)
	}
	if err = os.WriteFile(path, []byte("job: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errs := capture(t, func() int { return runGitlabInit([]string{"--dir", dir}) })
	if after, _ := os.ReadFile(path); code == 0 || string(after) != "job: {}\n" || !strings.Contains(errs, "not generated") {
		t.Fatalf("a hand-written file was overwritten: code=%d err=%q", code, errs)
	}
}

// TestGitlabInitCheck proves --check compares the default-branch and untrusted-ref copies through local
// git, exits 1 naming the ref when either is missing or drifts, and prints the exact commands.
func TestGitlabInitCheck(t *testing.T) {
	base := t.TempDir()
	remote, work := filepath.Join(base, "remote.git"), filepath.Join(base, "work")
	git(t, base, "init", "--bare", "--initial-branch=main", remote)
	git(t, base, "init", "--initial-branch=main", work)
	git(t, work, "remote", "add", "origin", remote)
	if code, _, _ := capture(t, func() int { return runGitlabInit([]string{"--dir", work}) }); code != 0 {
		t.Fatal("init failed")
	}
	git(t, work, "add", gitlabtmpl.Path)
	git(t, work, "commit", "-m", "include")
	git(t, work, "push", "origin", "main")
	git(t, work, "fetch", "origin")
	git(t, work, "remote", "set-head", "origin", "main")
	check := func(extra ...string) (int, string, string) {
		return capture(t, func() int { return runGitlabInit(append([]string{"--check", "--dir", work}, extra...)) })
	}
	code, out, errs := check()
	if code != 1 || !strings.Contains(out, "missing") || !strings.Contains(out, "git push origin origin/main:refs/heads/"+hostedDefaultRef) || !strings.Contains(errs, "origin/"+hostedDefaultRef) {
		t.Fatalf("missing untrusted ref: code=%d\nout=%s\nerr=%s", code, out, errs)
	}
	git(t, work, "push", "origin", "origin/main:refs/heads/"+hostedDefaultRef)
	git(t, work, "fetch", "origin")
	if code, out, errs = check(); code != 0 || strings.Contains(out, "missing") || strings.Contains(out, "drifted") {
		t.Fatalf("clean: code=%d\n%s\n%s", code, out, errs)
	}
	ghGit(t, work, "checkout", "-q", "-b", "side")
	if err := os.WriteFile(filepath.Join(work, gitlabtmpl.Path), []byte(gitlabtmpl.HeaderMarker+"\n# template: v1\njob: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ghGit(t, work, "commit", "-qam", "edited")
	ghGit(t, work, "push", "--force", "origin", "side:refs/heads/"+hostedDefaultRef)
	ghGit(t, work, "fetch", "origin")
	code, out, errs = check()
	if code != 1 || statusLines(out, "drifted") != 1 || !strings.Contains(out, "git push --force-with-lease origin origin/main:refs/heads/"+hostedDefaultRef) || !strings.Contains(errs, "differs from") {
		t.Fatalf("untrusted ref drift: code=%d\n%s\n%s", code, out, errs)
	}
	code, out, _ = check("--json")
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
