package gates

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func runWithoutTools(t *testing.T, p PlannedCheck) model.Check {
	t.Helper()
	return (Runner{Executor: &exec.Executor{}, Home: t.TempDir()}).Run(context.Background(), t.TempDir(), p, model.PipelineConfig{})
}

func TestPythonPlannedChecks(t *testing.T) {
	for _, name := range []string{"uv", "poetry", "pip", "no-tests"} {
		t.Run(name, func(t *testing.T) {
			planned := fixtureChecks(t, "python", name, Options{})
			checkPlanGolden(t, "python", name, planned)
			for _, p := range planned {
				switch p.Def.ID {
				case "python-test":
					if !p.Required {
						t.Error("python-test must be required by default")
					}
				case "python-lint", "python-types":
					if p.Required {
						t.Errorf("%s must be advisory by default", p.Def.ID)
					}
				}
			}
			if name == "no-tests" && len(find(planned, "python-test")) != 0 {
				t.Error("a Python repo without tests plans no python-test")
			}
			if (name == "poetry") != (len(find(planned, "python-types")) == 1) {
				t.Errorf("python-types only where mypy is configured (poetry fixture), got %d in %s", len(find(planned, "python-types")), name)
			}
		})
	}
}

func TestPythonCommandsPerManager(t *testing.T) {
	cases := []struct {
		fixture, tool string
		argv          []string
	}{
		{"uv", "uv", []string{"sh", "-c", "uv sync --frozen && uv run pytest"}},
		{"poetry", "poetry", []string{"sh", "-c", "export POETRY_VIRTUALENVS_IN_PROJECT=true; poetry install --no-root --sync && poetry run pytest"}},
		{"pip", "python3", []string{"sh", "-c", pipScript + pipRun + "-m pytest", "sh", "requirements-dev.txt", "requirements.txt"}},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			got := find(fixtureChecks(t, "python", c.fixture, Options{}), "python-test")
			if len(got) != 1 {
				t.Fatalf("want one python-test, got %d", len(got))
			}
			if got[0].ToolName() != c.tool || !reflect.DeepEqual(got[0].Command(), c.argv) {
				t.Fatalf("tool %s argv %q, want %s %q", got[0].ToolName(), got[0].Command(), c.tool, c.argv)
			}
		})
	}
}

func TestPythonAdvisoryUntilConfiguredOrStrict(t *testing.T) {
	req := true
	for name, o := range map[string]Options{
		"strict":    {Config: model.PipelineConfig{Presets: []model.Preset{"strict"}}},
		"container": {Container: true},
		"opt-in":    {Config: model.PipelineConfig{Checks: map[string]model.CheckDef{"python-lint": {Required: &req}}}},
	} {
		t.Run(name, func(t *testing.T) {
			lint := find(fixtureChecks(t, "python", "pip", o), "python-lint")
			if len(lint) != 1 || !lint[0].Required {
				t.Fatalf("python-lint must be required under %s: %+v", name, lint)
			}
		})
	}
}

// TestPipRequirementsAreArguments: requirement file names reach the shell as
// positional parameters, never inside the script text.
func TestPipRequirementsAreArguments(t *testing.T) {
	argv := pipCommand("-m pytest", []string{"requirements.txt; touch pwned", "-r evil.txt"}, true)
	if strings.Contains(argv[2], "pwned") || strings.Contains(argv[2], "evil") {
		t.Fatalf("a file name entered the script: %q", argv[2])
	}
	if argv[len(argv)-2] != "requirements.txt; touch pwned" || argv[len(argv)-1] != "-r evil.txt" {
		t.Fatalf("file names not passed as arguments: %q", argv)
	}
	if !strings.Contains(argv[2], ".venv") || strings.Contains(argv[2], "--user") {
		t.Fatalf("pip must install into the venv under the job root: %q", argv[2])
	}
}

func TestPytestMissingMarkers(t *testing.T) {
	for out, want := range map[string]bool{
		"/usr/bin/python: No module named pytest": true,
		"error: Failed to spawn: `pytest`":        true,
		"sh: pytest: command not found":           true,
		"Command not found: pytest":               true,
		"1 failed, 2 passed":                      false,
		"AssertionError":                          false,
	} {
		if pytestMissing(out) != want {
			t.Errorf("pytestMissing(%q) = %v", out, !want)
		}
	}
}

// TestPythonNeverPassesWithoutItsTool: a required python-test with no uv on
// the host is E614 with the remediation; the advisory lint is skipped.
func TestPythonNeverPassesWithoutItsTool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	planned := fixtureChecks(t, "python", "uv", Options{})
	c := runWithoutTools(t, find(planned, "python-test")[0])
	if c.Result != "error" || !strings.Contains(c.Excerpt, "E614") || !strings.Contains(c.Excerpt, "Install the project's Python tools") {
		t.Fatalf("python-test: %+v", c)
	}
	l := runWithoutTools(t, find(planned, "python-lint")[0])
	if l.Result != "skip" || l.Reason != "advisory.tool_missing" {
		t.Fatalf("python-lint: %+v", l)
	}
}

// TestPytestRunnerRemovedIsE614: when the environment has no pytest, the
// check is the unavailable tool (E614), not a failing test and never a pass.
func TestPytestRunnerRemovedIsE614(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"sh", "ps"} {
		tool, err := pyFindTool(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(tool, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	// A uv that syncs fine and finds no pytest to run.
	fake := "#!/bin/sh\nif [ \"$1\" = run ]; then echo 'error: Failed to spawn: `pytest`' >&2; exit 2; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "uv"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	p := find(fixtureChecks(t, "python", "uv", Options{}), "python-test")[0]
	root := t.TempDir()
	c := (Runner{Executor: &exec.Executor{}, Home: t.TempDir()}).Run(context.Background(), root, p, model.PipelineConfig{})
	if c.Result != "error" || c.FailureClass != "infra" || !strings.Contains(c.Excerpt, "E614") || !strings.Contains(c.Excerpt, "pytest") {
		t.Fatalf("python-test with no pytest: %+v", c)
	}
}

func pyFindTool(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, name)
		if s, e := os.Stat(p); e == nil && !s.IsDir() {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

// TestPythonIndexIgnoresUncertainFacts: an unknown manifest plans nothing.
func TestPythonIndexIgnoresUncertainFacts(t *testing.T) {
	facts := []detect.Fact{{Kind: "manifest", Name: "unknown", Path: "pyproject.toml", Confidence: detect.Unknown}}
	for _, p := range DefaultRegistry().Plan(facts, Options{}) {
		if strings.HasPrefix(p.Def.ID, "python-") {
			t.Fatalf("planned from an unknown manifest: %+v", p)
		}
	}
}

// pyPlan plans checks from hand-written facts (certain, as the detector emits them).
func pyPlan(facts ...[3]string) []PlannedCheck {
	var fs []detect.Fact
	for _, f := range facts {
		fs = append(fs, detect.Fact{Kind: f[0], Name: f[1], Path: f[2], Confidence: detect.Certain})
	}
	return DefaultRegistry().Plan(fs, Options{})
}

// TestPytestEvidenceBelongsToTheNearestProject: config or tests found below a
// directory named tests (or any ancestor) plan python-test for the project
// that holds them, not for a guessed parent.
func TestPytestEvidenceBelongsToTheNearestProject(t *testing.T) {
	got := find(pyPlan(
		[3]string{"stack", "python", "libs/tests/pkg/pyproject.toml"},
		[3]string{"package_manager", "pip", "libs/tests/pkg/pyproject.toml"},
		[3]string{"test_framework", "pytest", "libs/tests/pkg/pytest.ini"},
	), "python-test")
	if len(got) != 1 || got[0].Workdir != "libs/tests/pkg" {
		t.Fatalf("python-test must be planned in libs/tests/pkg: %+v", got)
	}
	got = find(pyPlan(
		[3]string{"stack", "python", "pyproject.toml"},
		[3]string{"package_manager", "pip", "pyproject.toml"},
		[3]string{"stack", "python", "svc/pyproject.toml"},
		[3]string{"package_manager", "pip", "svc/pyproject.toml"},
		[3]string{"test_framework", "pytest", "svc/tests/test_a.py"},
	), "python-test")
	if len(got) != 1 || got[0].Workdir != "svc" {
		t.Fatalf("tests under svc/ belong to svc, not the root project: %+v", got)
	}
}

// TestPipInstallsTheProjectItself: a pyproject.toml, setup.cfg or setup.py
// project is installed editable after its requirements; a requirements-only
// directory is not.
func TestPipInstallsTheProjectItself(t *testing.T) {
	cmd := func(manifest string) string {
		p := find(pyPlan(
			[3]string{"stack", "python", manifest},
			[3]string{"package_manager", "pip", manifest},
			[3]string{"lockfile", "pip", "requirements-dev.txt"},
			[3]string{"test_framework", "pytest", "tests/test_a.py"},
		), "python-test")
		if len(p) != 1 {
			t.Fatalf("want one python-test for %s, got %d", manifest, len(p))
		}
		return p[0].Command()[2]
	}
	for _, m := range []string{"pyproject.toml", "setup.cfg", "setup.py"} {
		if !strings.Contains(cmd(m), "pip install --quiet -e .") {
			t.Errorf("%s project must be installed editable", m)
		}
	}
	if strings.Contains(cmd("requirements.txt"), "-e .") {
		t.Error("a requirements-only directory must not install itself")
	}
}

func TestSwiftTestOnlyWithTests(t *testing.T) {
	noTests := pyPlan([3]string{"stack", "swiftpm", "Package.swift"})
	if len(find(noTests, "swift-build")) != 1 || len(find(noTests, "swift-test")) != 0 {
		t.Fatalf("a package without tests plans swift-build only: %+v", noTests)
	}
	withTests := pyPlan([3]string{"stack", "swiftpm", "Package.swift"}, [3]string{"test_framework", "swift test", "Package.swift"})
	if len(find(withTests, "swift-test")) != 1 {
		t.Fatalf("a package with tests plans swift-test: %+v", withTests)
	}
}

// TestXcodeSchemeLessContainerStillDisambiguates: a project with a shared
// scheme next to a workspace without one needs -project, or xcodebuild sees two.
func TestXcodeSchemeLessContainerStillDisambiguates(t *testing.T) {
	got := find(pyPlan(
		[3]string{"stack", "xcode", "App.xcodeproj/project.pbxproj"},
		[3]string{"xcode_scheme", "App", "App.xcodeproj/xcshareddata/xcschemes/App.xcscheme"},
		[3]string{"stack", "xcode", "App.xcworkspace/contents.xcworkspacedata"},
	), "xcode-test")
	if len(got) != 1 || got[0].Command()[2] != "-project" {
		t.Fatalf("want xcodebuild test -project App.xcodeproj ...: %+v", got)
	}
}
