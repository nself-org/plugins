package truth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// fixtureOrg makes local repositories (clone sources for the drill) and a
// rules checkout; it returns the clone base, the repo list file and the root.
func fixtureOrg(t *testing.T, repos map[string]string) (base, list string) {
	t.Helper()
	base = t.TempDir()
	var names []string
	for name, body := range repos {
		dir := filepath.Join(base, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		gitIn(t, dir, "init", "-q", "-b", "main")
		gitIn(t, dir, "add", "-A")
		gitIn(t, dir, "commit", "-q", "-m", "fixture")
		names = append(names, name)
	}
	list = filepath.Join(t.TempDir(), "repos.txt")
	if err := os.WriteFile(list, []byte("# public repositories\n"+strings.Join(names, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return base, list
}

// runDrill runs scripts/drill/truth-sweep/run offline: the clone source and the
// repository list are local, the engine is the freshly built runner, and the
// rules are the shipped org rules read from the real repository root.
func runDrill(t *testing.T, base, list string, extra ...string) (code int, stdout, stderr string) {
	t.Helper()
	for _, tool := range []string{"bash", "git", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not on PATH", tool)
		}
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "scripts", "drill", "truth-sweep", "run"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), "TRUTH_RUNNER="+buildRunner(t), "TRUTH_REPOS_FILE="+list, "TRUTH_CLONE_BASE="+base, "TMPDIR="+t.TempDir())
	cmd.Env = append(cmd.Env, extra...)
	var o, e strings.Builder
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return code, o.String(), e.String()
}

func TestDrillSweepsEveryRepoAndFailsOnAHit(t *testing.T) {
	base, list := fixtureOrg(t, map[string]string{"alpha": "fine\n", "beta": "the Plugins Pro tier\n"})
	code, out, errw := runDrill(t, base, list)
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, errw)
	}
	if !strings.Contains(errw, "PASS truth-sweep.alpha") || !strings.Contains(errw, "FAIL truth-sweep.beta") {
		t.Errorf("stderr: %s", errw)
	}
	if strings.Count(strings.TrimSpace(out), "\n") != 0 || !strings.Contains(out, `"ci.evidence/v1"`) || strings.Count(out, `"id":"truth-sweep.`) != 2 {
		t.Errorf("stdout must be one document with one check per repository: %q", out)
	}
}

func TestDrillFailsWhenACloneFails(t *testing.T) {
	base, list := fixtureOrg(t, map[string]string{"alpha": "fine\n"})
	if err := os.WriteFile(list, []byte("alpha\nmissing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errw := runDrill(t, base, list)
	if code != 2 || !strings.Contains(errw, "ERROR truth-sweep.missing") || !strings.Contains(out, `"result":"error"`) {
		t.Errorf("a repository that was not read is not clean: exit %d\n%s\n%s", code, errw, out)
	}
}

func TestDrillRefusesAnEmptyRepositoryList(t *testing.T) {
	base, _ := fixtureOrg(t, map[string]string{"alpha": "fine\n"})
	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, []byte("# none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errw := runDrill(t, base, empty); code != 2 || !strings.Contains(errw, "list is empty") {
		t.Errorf("exit %d: %s", code, errw)
	}
}
