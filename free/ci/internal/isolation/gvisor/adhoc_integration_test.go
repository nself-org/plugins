//go:build integration

package gvisor

// Purpose: TestUntrustedAdhocSandboxed, the end-to-end proof that `nself ci --trust untrusted -- <cmd>`
// (P7-CI-52) runs sandboxed once the profile reports available, and fails with E647 where it cannot.
// Inputs: the real binary built from ./cmd, docker with runsc, a fresh git repo.
// Constraints: nothing is faked; a missing prerequisite is a failure, never a skip.

import (
	"bytes"
	"encoding/json"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runCmd(t *testing.T, dir string, env []string, name string, args ...string) (string, string, int) {
	t.Helper()
	cmd := osexec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*osexec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return stdout.String(), stderr.String(), code
}

func adhocRig(t *testing.T) (bin, repo, home string) {
	t.Helper()
	tmp := t.TempDir()
	bin = filepath.Join(tmp, "nself-ci")
	if out, errs, code := runCmd(t, "../../..", []string{"CGO_ENABLED=0"}, "go", "build", "-mod=vendor", "-o", bin, "./cmd"); code != 0 {
		t.Fatalf("go build: %s %s", out, errs)
	}
	repo = filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, a := range [][]string{{"init", "-q"}, {"-c", "user.email=a@b", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, errs, code := runCmd(t, repo, nil, "git", a...); code != 0 {
			t.Fatalf("git %v: %s %s", a, out, errs)
		}
	}
	return bin, repo, filepath.Join(tmp, "home")
}

// On a runsc host the ad-hoc untrusted job runs sandboxed with network none and passes.
func TestUntrustedAdhocSandboxed(t *testing.T) {
	_, _ = prerequisites(t)
	bin, repo, home := adhocRig(t)
	out, errs, code := runCmd(t, repo, []string{"NSELF_V15=1", "NSELF_CI_HOME=" + home}, bin,
		"--no-status", "--json", "--trust", "untrusted", "--image", ProbeImage, "--", "sh", "-c", "echo ok")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, out, errs)
	}
	var doc struct {
		Data struct {
			Result string `json:"result"`
			Runner struct {
				Isolation string `json:"isolation"`
			} `json:"runner"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("--json did not print one JSON document: %v\n%s", err, out)
	}
	if doc.Data.Runner.Isolation != "sandboxed-container" || doc.Data.Result != "pass" {
		t.Fatalf("isolation %q result %q\n%s", doc.Data.Runner.Isolation, doc.Data.Result, out)
	}
}

// Negative control: with no container runtime on PATH the same command fails closed with E647 and nothing runs.
func TestUntrustedAdhocWithoutRuntimeFailsE647(t *testing.T) {
	bin, repo, home := adhocRig(t)
	git, err := osexec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	only := t.TempDir() // a PATH holding git and nothing else: no docker, no podman
	if err := os.Symlink(git, filepath.Join(only, "git")); err != nil {
		t.Fatal(err)
	}
	out, errs, code := runCmd(t, repo, []string{"NSELF_V15=1", "NSELF_CI_HOME=" + home, "PATH=" + only}, bin,
		"--no-status", "--trust", "untrusted", "--image", ProbeImage, "--", "sh", "-c", "echo ok")
	if code == 0 || !strings.Contains(out+errs, "E647") {
		t.Fatalf("exit %d, want a failure naming E647\nstdout: %s\nstderr: %s", code, out, errs)
	}
}
