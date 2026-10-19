package protect

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const zeros = "0000000000000000000000000000000000000000"
const sha1 = "1111111111111111111111111111111111111111"

// stubBin writes an executable `name` into a new dir that exits with rc and
// records its argv; it returns the dir and the record file.
func stubBin(t *testing.T, name string, rc int) (dir, record string) {
	t.Helper()
	dir = t.TempDir()
	record = filepath.Join(dir, name+".args")
	script := fmt.Sprintf("#!/bin/sh\necho \"$NSELF_V15 $*\" >> %q\necho 'verdict text'\nexit %d\n", record, rc)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, record
}

// runHook feeds push lines to the hook with PATH limited to pathDir.
func runHook(t *testing.T, branch, pathDir string, lines ...string) (stderr string, code int) {
	t.Helper()
	hook := filepath.Join(t.TempDir(), "pre-push")
	if _, err := InstallHook(hook, branch); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", hook, "origin", "https://example.invalid/x.git")
	cmd.Env = []string{"PATH=" + pathDir}
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	var errb strings.Builder
	cmd.Stderr = &errb
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return errb.String(), ee.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return errb.String(), 0
}

func pushLine(local, remoteRef string) string {
	return fmt.Sprintf("refs/heads/work %s %s %s", local, remoteRef, zeros)
}

// TestHookRefuses: the hook refuses a push to the default branch when the
// evidence check does not pass (missing, stale, invalid or unbound all exit
// non-zero) and allows it when it passes.
func TestHookRefuses(t *testing.T) {
	for _, rc := range []int{1, 10, 11, 12, 2} {
		t.Run(fmt.Sprintf("verify exit %d refuses", rc), func(t *testing.T) {
			dir, record := stubBin(t, "nself", rc)
			stderr, code := runHook(t, "main", dir, pushLine(sha1, "refs/heads/main"))
			if code != 1 {
				t.Fatalf("exit = %d, want 1; stderr: %s", code, stderr)
			}
			if !strings.Contains(stderr, "refusing the push") || !strings.Contains(stderr, sha1) || !strings.Contains(stderr, fmt.Sprintf("exit %d", rc)) {
				t.Fatalf("stderr = %q", stderr)
			}
			if rec, _ := os.ReadFile(record); !strings.Contains(string(rec), "ci evidence verify "+sha1+" --json") {
				t.Fatalf("the hook must call `nself ci evidence verify <sha> --json`, called %q", rec)
			}
		})
	}
	t.Run("passing evidence allows", func(t *testing.T) {
		dir, record := stubBin(t, "nself", 0)
		stderr, code := runHook(t, "main", dir, pushLine(sha1, "refs/heads/main"))
		if code != 0 || stderr != "" {
			t.Fatalf("exit = %d, stderr = %q", code, stderr)
		}
		if rec, _ := os.ReadFile(record); !strings.Contains(string(rec), sha1) {
			t.Fatalf("not asked about the pushed sha: %q", rec)
		}
	})
	t.Run("one bad ref among several refuses", func(t *testing.T) {
		dir, _ := stubBin(t, "nself", 12)
		_, code := runHook(t, "main", dir, pushLine(sha1, "refs/heads/topic"), pushLine(sha1, "refs/heads/main"))
		if code != 1 {
			t.Fatalf("exit = %d", code)
		}
	})
	t.Run("other branches and tags are not checked", func(t *testing.T) {
		dir, record := stubBin(t, "nself", 1)
		_, code := runHook(t, "main", dir, pushLine(sha1, "refs/heads/topic"), pushLine(sha1, "refs/tags/v1"))
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if _, err := os.Stat(record); err == nil {
			t.Fatal("the evidence check ran for a ref that is not the default branch")
		}
	})
	t.Run("deleting the branch is not checked", func(t *testing.T) {
		dir, record := stubBin(t, "nself", 1)
		_, code := runHook(t, "main", dir, pushLine(zeros, "refs/heads/main"))
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if _, err := os.Stat(record); err == nil {
			t.Fatal("a delete push must not run the check")
		}
	})
	t.Run("no binary on PATH fails closed", func(t *testing.T) {
		stderr, code := runHook(t, "main", t.TempDir(), pushLine(sha1, "refs/heads/main"))
		if code != 1 || !strings.Contains(stderr, "neither nself nor nself-ci") {
			t.Fatalf("exit = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("falls back to nself-ci with NSELF_V15", func(t *testing.T) {
		dir, record := stubBin(t, "nself-ci", 0)
		_, code := runHook(t, "main", dir, pushLine(sha1, "refs/heads/main"))
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if rec, _ := os.ReadFile(record); !strings.HasPrefix(string(rec), "1 evidence verify "+sha1) {
			t.Fatalf("record = %q", rec)
		}
	})
	t.Run("the check cannot eat the remaining push lines", func(t *testing.T) {
		// A check that reads stdin would swallow the second line; the hook gives it /dev/null.
		dir := t.TempDir()
		record := filepath.Join(dir, "calls")
		script := fmt.Sprintf("#!/bin/sh\ncat >/dev/null\necho call >> %q\nexit 0\n", record)
		_ = os.WriteFile(filepath.Join(dir, "nself"), []byte(script), 0o755)
		_, code := runHook(t, "main", dir, pushLine(sha1, "refs/heads/main"), pushLine("2222222222222222222222222222222222222222", "refs/heads/main"))
		rec, _ := os.ReadFile(record)
		if code != 0 || strings.Count(string(rec), "call") != 2 {
			t.Fatalf("exit = %d, calls = %q", code, rec)
		}
	})
}

// TestHookWithGit installs the hook in a real repository and pushes.
func TestHookWithGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	root := t.TempDir()
	remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	git(root, "init", "--bare", "-b", "main", remote)
	git(root, "init", "-b", "main", work)
	git(work, "commit", "--allow-empty", "-m", "one")
	git(work, "remote", "add", "origin", remote)
	if _, err := InstallHook(filepath.Join(work, ".git", "hooks", "pre-push"), "main"); err != nil {
		t.Fatal(err)
	}
	push := func(rc int) (string, error) {
		dir, _ := stubBin(t, "nself", rc)
		c := exec.Command("git", "push", "origin", "main")
		c.Dir = work
		c.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := c.CombinedOutput()
		return string(out), err
	}
	if out, err := push(12); err == nil || !strings.Contains(out, "refusing the push") {
		t.Fatalf("an unbound-evidence push went through: %v\n%s", err, out)
	}
	if out := git(remote, "branch", "--list"); strings.TrimSpace(out) != "" {
		t.Fatalf("the refused push still created a branch: %q", out)
	}
	if out, err := push(0); err != nil {
		t.Fatalf("a push with passing evidence was refused: %v\n%s", err, out)
	}
}

func TestInstallHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks", "pre-push")
	changed, err := InstallHook(path, "main")
	if err != nil || !changed {
		t.Fatalf("first install: %v %v", changed, err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o755 {
		t.Fatalf("hook mode = %v %v", st, err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), HookMarker) || !strings.Contains(string(body), `branch="main"`) {
		t.Fatalf("body = %s", body)
	}
	if changed, err := InstallHook(path, "main"); err != nil || changed {
		t.Fatalf("second install must be a no-op: %v %v", changed, err)
	}
}

func TestInstallHookNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-push")
	mine := "#!/bin/sh\necho mine\n"
	if err := os.WriteFile(path, []byte(mine), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := InstallHook(path, "main")
	var pe *Error
	if !errors.As(err, &pe) || pe.Code != CodeHookExists {
		t.Fatalf("err = %v, want E642", err)
	}
	if !strings.Contains(pe.Message, "nself ci evidence verify") {
		t.Fatalf("the error must say how to chain: %s", pe.Message)
	}
	if got, _ := os.ReadFile(path); string(got) != mine {
		t.Fatalf("existing hook was changed: %q", got)
	}
	// A managed hook for another branch is not ours to rewrite either.
	other := filepath.Join(t.TempDir(), "pre-push")
	_, _ = InstallHook(other, "trunk")
	if _, err := InstallHook(other, "main"); !errors.As(err, &pe) || pe.Code != CodeHookExists {
		t.Fatalf("err = %v", err)
	}
	if _, err := InstallHook(filepath.Join(t.TempDir(), "x"), "main; rm -rf /"); err == nil {
		t.Fatal("a branch name that is not a branch name must be refused")
	}
}
