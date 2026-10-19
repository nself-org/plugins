//go:build integration

package gates

// Purpose: TestPythonFixtures (P7-CI-64) runs the Python checks of the uv and
// pip fixtures for real on the verify host (python3 and uv on PATH, network
// for PyPI): a passing project passes, a failing test fails, ruff findings
// stay advisory, a missing tool or a missing pytest is E614 and never a pass,
// and the lockfiles enter the evidence input digests.
// Constraints: build tag integration, INTEGRATION=1; a missing prerequisite is
// t.Fatal, never t.Skip, so a green run proves the legs ran.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// copyFixture copies the Python fixture name into a new temp dir.
func copyFixture(t *testing.T, name string) string { return copyFixtureTree(t, "python", name) }

// planned detects root and returns its planned checks by id.
func planned(t *testing.T, root string) map[string]PlannedCheck {
	t.Helper()
	r := DefaultRegistry()
	facts, err := detect.DefaultRegistry().Detect(os.DirFS(root), r.Has)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]PlannedCheck{}
	for _, p := range r.Plan(facts, Options{}) {
		out[p.Def.ID] = p
	}
	return out
}

func runCheck(t *testing.T, root string, p PlannedCheck) model.Check {
	t.Helper()
	return (Runner{Executor: &ciexec.Executor{}, Home: t.TempDir()}).Run(context.Background(), root, p, model.PipelineConfig{})
}

func TestPythonFixtures(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Fatal("TestPythonFixtures needs INTEGRATION=1")
	}
	for _, tool := range []string{"python3", "uv", "sh"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s must be on PATH of the verify host: %v", tool, err)
		}
	}
	realPath := os.Getenv("PATH")

	t.Run("uv project passes and ruff findings are advisory", func(t *testing.T) {
		root := copyFixture(t, "uv")
		checks := planned(t, root)
		test, lint := runCheck(t, root, checks["python-test"]), runCheck(t, root, checks["python-lint"])
		if test.Result != "pass" || !test.Required {
			t.Fatalf("python-test: %+v", test)
		}
		if lint.Required {
			t.Fatalf("python-lint must be advisory: %+v", lint)
		}
		if lint.Result != "skip" && lint.Result != "fail" && lint.Result != "pass" {
			t.Fatalf("python-lint: %+v", lint)
		}
		if v := model.Verdict([]model.Check{test, lint}); v != "pass" {
			t.Fatalf("verdict %s with checks %+v %+v", v, test, lint)
		}
	})

	t.Run("a failing test fails the check", func(t *testing.T) {
		root := copyFixture(t, "uv")
		bad := "from uvfix import total\n\n\ndef test_total():\n    assert total(1, 2) == 4\n"
		if err := os.WriteFile(filepath.Join(root, "tests", "test_uvfix.py"), []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		c := runCheck(t, root, planned(t, root)["python-test"])
		if c.Result != "fail" || c.FailureClass != "code" {
			t.Fatalf("failing pytest: %+v", c)
		}
		if v := model.Verdict([]model.Check{c}); v != "fail" {
			t.Fatalf("verdict %s", v)
		}
	})

	t.Run("pip project passes in a venv inside the job workspace", func(t *testing.T) {
		root := copyFixture(t, "pip")
		c := runCheck(t, root, planned(t, root)["python-test"])
		if c.Result != "pass" {
			t.Fatalf("pip python-test: %+v", c)
		}
		// The executor runs in its own workspace copy: nothing lands in the source tree.
		if _, err := os.Stat(filepath.Join(root, ".venv")); err == nil {
			t.Fatal("the venv must not be created in the source tree")
		}
	})

	t.Run("pytest missing from the environment is E614", func(t *testing.T) {
		root := copyFixture(t, "pip")
		for _, name := range []string{"requirements.txt", "requirements-dev.txt"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte("# no test runner here\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		c := runCheck(t, root, planned(t, root)["python-test"])
		if c.Result != "error" || !strings.Contains(c.Excerpt, "E614") || !strings.Contains(c.Excerpt, "pytest") {
			t.Fatalf("pip project without pytest: %+v", c)
		}
	})

	t.Run("no uv on the host is E614", func(t *testing.T) {
		root := copyFixture(t, "uv")
		p := planned(t, root)["python-test"]
		t.Setenv("PATH", t.TempDir())
		c := runCheck(t, root, p)
		t.Setenv("PATH", realPath)
		if c.Result != "error" || !strings.Contains(c.Excerpt, "E614") || !strings.Contains(c.Excerpt, "Install the project's Python tools") {
			t.Fatalf("python-test without uv: %+v", c)
		}
	})

	t.Run("lockfiles enter the evidence input digests", func(t *testing.T) {
		for fixture, want := range map[string][]string{
			"uv":     {"uv.lock"},
			"poetry": {"poetry.lock"},
			"pip":    {"requirements.txt", "requirements-dev.txt"},
		} {
			d, err := evidence.InputsOf(os.DirFS(filepath.Join("..", "detect", "testdata", "python", fixture)))
			if err != nil {
				t.Fatal(err)
			}
			have := map[string]bool{}
			for _, l := range d.Lockfiles {
				have[l.Path] = true
			}
			for _, w := range want {
				if !have[w] {
					t.Errorf("%s: %s missing from input digests %+v", fixture, w, d.Lockfiles)
				}
			}
		}
		d, err := evidence.InputsOf(os.DirFS(filepath.Join("..", "detect", "testdata", "swift", "swiftpm")))
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Lockfiles) != 1 || d.Lockfiles[0].Path != "Package.resolved" {
			t.Errorf("swiftpm: Package.resolved missing from input digests %+v", d.Lockfiles)
		}
	})
}
