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

// TestGitlabInitUntrustedDir proves --untrusted-dir writes exactly the two generated files of the untrusted
// ref (variables root by default, inputs root with --inputs), never overwrites a hand-written file, and
// the pair equals what the provider compares at the pipeline sha.
func TestGitlabInitUntrustedDir(t *testing.T) {
	want, err := os.ReadFile("../templates/gitlab/testdata/include.golden.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		flags []string
		kind  string
	}{{nil, gitlabtmpl.RootVariables}, {[]string{"--inputs"}, gitlabtmpl.RootInputs}} {
		dir := t.TempDir()
		if code, _, errs := capture(t, func() int { return runGitlabInit(append([]string{"--untrusted-dir", dir}, c.flags...)) }); code != 0 {
			t.Fatalf("%s: code %d %s", c.kind, code, errs)
		}
		root, rerr := os.ReadFile(filepath.Join(dir, gitlabtmpl.RootPath))
		inc, ierr := os.ReadFile(filepath.Join(dir, gitlabtmpl.Path))
		if rerr != nil || ierr != nil || string(root) != string(gitlabtmpl.RenderRoot(c.kind)) || string(inc) != string(want) {
			t.Fatalf("%s: files differ: %v %v", c.kind, rerr, ierr)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 2 { // .gitlab-ci.yml and .gitlab/
			t.Errorf("%s: %d entries in the directory", c.kind, len(entries))
		}
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, gitlabtmpl.RootPath), []byte("job: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := capture(t, func() int { return runGitlabInit([]string{"--untrusted-dir", dir}) }); code == 0 || !strings.Contains(errs, "not generated") {
		t.Errorf("a hand-written root was overwritten: %d %q", code, errs)
	}
	for _, bad := range [][]string{{"--inputs"}, {"--untrusted-dir", dir, "--check"}, {"--untrusted-dir", dir, "--stdout"}} {
		if code, _, _ := capture(t, func() int { return runGitlabInit(bad) }); code == 0 {
			t.Errorf("%v succeeded", bad)
		}
	}
}

// TestGitlabInitCheck proves --check compares the default-branch include and BOTH files of the untrusted
// ref through local git, exits 1 naming each file that is missing or drifts, accepts either generated
// root, and prints the recipe that creates the untrusted ref as an orphan commit of the two files.
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
	if code != 1 || strings.Count(out, "missing") != 2 || !strings.Contains(out, "--untrusted-dir") || !strings.Contains(out, "HEAD:refs/heads/"+hostedDefaultRef) ||
		!strings.Contains(errs, "origin/"+hostedDefaultRef+":"+gitlabtmpl.RootPath) {
		t.Fatalf("missing untrusted ref: code=%d\nout=%s\nerr=%s", code, out, errs)
	}
	// create the untrusted ref the way the recipe says: an orphan commit of the two generated files
	publish := func(flags ...string) {
		d := t.TempDir()
		if code, _, errs := capture(t, func() int { return runGitlabInit(append([]string{"--untrusted-dir", d}, flags...)) }); code != 0 {
			t.Fatalf("untrusted-dir: %d %s", code, errs)
		}
		git(t, d, "init", "-q")
		git(t, d, "add", "-A")
		ghGit(t, d, "commit", "-qm", "nself ci untrusted ref")
		ghGit(t, d, "push", "--force", remote, "HEAD:refs/heads/"+hostedDefaultRef)
		git(t, work, "fetch", "origin")
	}
	for _, flags := range [][]string{nil, {"--inputs"}} {
		publish(flags...)
		if code, out, errs = check(); code != 0 || strings.Contains(out, "missing") || strings.Contains(out, "drifted") {
			t.Fatalf("clean %v: code=%d\n%s\n%s", flags, code, out, errs)
		}
	}
	// the project's own root on the untrusted ref (what copying the default branch would give)
	ghGit(t, work, "checkout", "-q", "-b", "side")
	if err := os.WriteFile(filepath.Join(work, gitlabtmpl.RootPath), []byte("stages: [test]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ghGit(t, work, "add", gitlabtmpl.RootPath)
	ghGit(t, work, "commit", "-qm", "own root")
	ghGit(t, work, "push", "--force", "origin", "side:refs/heads/"+hostedDefaultRef)
	ghGit(t, work, "fetch", "origin")
	code, out, errs = check()
	if code != 1 || strings.Count(out, "drifted") != 1 || !strings.Contains(out, gitlabtmpl.RootPath) || !strings.Contains(errs, "origin/"+hostedDefaultRef+":"+gitlabtmpl.RootPath+" differs from") {
		t.Fatalf("own root on the untrusted ref: code=%d\n%s\n%s", code, out, errs)
	}
	code, out, _ = check("--json")
	var env struct {
		Data struct {
			OK     bool `json:"ok"`
			Copies []struct{ Ref, Status string }
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || code != 1 || env.Data.OK || len(env.Data.Copies) != 3 {
		t.Fatalf("json check: code=%d err=%v out=%s", code, err, out)
	}
}
