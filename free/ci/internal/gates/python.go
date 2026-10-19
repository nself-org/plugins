package gates

// Purpose: the Python checks (P7-CI-64, Epic D33): python-test, python-lint,
// python-types, with the command chosen by the project's package manager.
// Inputs:  detector facts per directory: stack python, package_manager
// (uv > poetry > pip), test_framework pytest, lint_tool ruff, type_checker
// mypy, and the lockfile pip facts (requirements*.txt).
// Outputs: CheckDefs, and planned checks with the manager's argv and tool.
// Constraints: nothing installs outside the job root: uv and poetry use the
// project environment, pip a `.venv` in the working directory (HOME is the
// job root home, so no global or ~/.local install); python-test is required,
// python-lint and python-types advisory until configured or strict; python-types
// exists only when mypy is configured; requirement files reach the shell as
// positional arguments, never spliced into the script.

import (
	"path"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/detect"
)

const pythonFix = "Install the project's Python tools (uv, poetry or python3) and its test and type dependencies"

func pythonDefs() []CheckDef {
	return []CheckDef{
		def("python-test", "test", "uv", []string{"sh", "-c", "uv sync --frozen && uv run pytest"}, true, true, "fast", pythonFix),
		def("python-lint", "lint", "ruff", []string{"ruff", "check", "."}, false, true, "fast", "Install ruff"),
		def("python-types", "lint", "uv", []string{"sh", "-c", "uv sync --frozen && uv run mypy ."}, false, true, "fast", pythonFix),
	}
}

// pythonDir is what the detector found in one directory.
type pythonDir struct {
	manager      string
	pytest, mypy bool
	requirements []string
}

type pythonFacts map[string]*pythonDir

func pythonIndex(facts []detect.Fact) pythonFacts {
	idx := pythonFacts{}
	at := func(dir string) *pythonDir {
		if idx[dir] == nil {
			idx[dir] = &pythonDir{manager: "pip"}
		}
		return idx[dir]
	}
	for _, f := range facts {
		if f.Confidence != detect.Certain {
			continue
		}
		dir := path.Dir(f.Path)
		switch {
		case f.Kind == "package_manager" && (f.Name == "uv" || f.Name == "poetry" || f.Name == "pip"):
			at(dir).manager = f.Name
		case f.Kind == "test_framework" && f.Name == "pytest":
			// A tests/ directory file is evidence below the project directory.
			at(pythonProjectDir(f.Path)).pytest = true
		case f.Kind == "type_checker" && f.Name == "mypy":
			at(dir).mypy = true
		case f.Kind == "lockfile" && f.Name == "pip":
			d := at(dir)
			d.requirements = append(d.requirements, path.Base(f.Path))
		}
	}
	for _, d := range idx {
		sort.Strings(d.requirements)
	}
	return idx
}

// pythonProjectDir maps pytest evidence to its project directory: a file
// below a tests/ directory belongs to the directory holding tests/.
func pythonProjectDir(evidence string) string {
	dir := path.Dir(evidence)
	for d := dir; d != "." && d != "/"; d = path.Dir(d) {
		if path.Base(d) == "tests" {
			return path.Dir(d)
		}
	}
	return dir
}

// plan adds the Python checks of one project directory.
func (idx pythonFacts) plan(dir string, add addFn) {
	d := idx[dir]
	if d == nil {
		d = &pythonDir{manager: "pip"}
	}
	if d.pytest {
		add("python-test", dir, "", d.command("pytest", "-m pytest"))
	}
	add("python-lint", dir, "")
	if d.mypy {
		add("python-types", dir, "", d.command("mypy .", "-m mypy ."))
	}
}

// command picks the argv and tool of a check for the directory's manager.
// run is the uv/poetry tail, module the `python -m` tail of the pip case.
func (d *pythonDir) command(run, module string) func(*PlannedCheck) {
	return func(p *PlannedCheck) {
		switch d.manager {
		case "uv":
			p.Tool, p.Argv = "uv", []string{"sh", "-c", "uv sync --frozen && uv run " + run}
		case "poetry":
			p.Tool, p.Argv = "poetry", []string{"sh", "-c", "poetry install --no-root --sync && poetry run " + run}
		default:
			p.Tool, p.Argv = "python3", pipCommand(module, d.requirements)
		}
	}
}

// pipScript creates the venv, installs, then runs the module. With no
// requirements file the project itself is installed (editable).
const pipScript = `set -e
[ -d .venv ] || python3 -m venv .venv
if [ "$#" -eq 0 ]; then .venv/bin/python -m pip install --quiet -e .; fi
for f in "$@"; do .venv/bin/python -m pip install --quiet -r "$f"; done
exec .venv/bin/python `

func pipCommand(module string, requirements []string) []string {
	argv := []string{"sh", "-c", pipScript + module, "sh"}
	return append(argv, requirements...)
}

// pytestMissing reports output that shows pytest itself is not installed
// (not a failing test): the check is an unavailable tool, never a pass.
func pytestMissing(output string) bool {
	for _, marker := range []string{"No module named pytest", "Failed to spawn: `pytest`", "pytest: command not found", "poetry run pytest: command not found"} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}
