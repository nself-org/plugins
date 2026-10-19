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

// pythonDir is what the detector found in one project directory.
type pythonDir struct {
	manager      string
	pytest, mypy bool
	project      bool // the primary manifest describes an installable project
	requirements []string
}

type pythonFacts map[string]*pythonDir

func pythonIndex(facts []detect.Fact) pythonFacts {
	idx := pythonFacts{}
	for _, f := range facts {
		if f.Kind == "stack" && f.Name == "python" && f.Confidence == detect.Certain {
			base := path.Base(f.Path)
			idx[path.Dir(f.Path)] = &pythonDir{manager: "pip", project: base == "pyproject.toml" || base == "setup.py"}
		}
	}
	for _, f := range facts {
		if f.Confidence != detect.Certain {
			continue
		}
		// Evidence may sit below the project (a file under tests/): it belongs
		// to the nearest project directory that holds it.
		d := idx[nearestProject(idx, path.Dir(f.Path))]
		if d == nil {
			continue
		}
		switch {
		case f.Kind == "package_manager" && (f.Name == "uv" || f.Name == "poetry" || f.Name == "pip"):
			d.manager = f.Name
		case f.Kind == "test_framework" && f.Name == "pytest":
			d.pytest = true
		case f.Kind == "type_checker" && f.Name == "mypy":
			d.mypy = true
		case f.Kind == "lockfile" && f.Name == "pip" && path.Dir(f.Path) == nearestProject(idx, path.Dir(f.Path)):
			d.requirements = append(d.requirements, path.Base(f.Path))
		}
	}
	for _, d := range idx {
		sort.Strings(d.requirements)
	}
	return idx
}

// nearestProject returns the deepest project directory that is dir or one of its ancestors.
func nearestProject(idx pythonFacts, dir string) string {
	for d := dir; ; d = path.Dir(d) {
		if _, ok := idx[d]; ok {
			return d
		}
		if d == "." || d == "/" {
			return ""
		}
	}
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
			p.Tool, p.Argv = "poetry", []string{"sh", "-c", "export POETRY_VIRTUALENVS_IN_PROJECT=true; poetry install --no-root --sync && poetry run " + run}
		default:
			p.Tool, p.Argv = "python3", pipCommand(module, d.requirements, d.project)
		}
	}
}

// pipScript creates the venv, then installs each requirements file (the
// positional arguments), then runs the module.
const pipScript = `set -e
[ -d .venv ] || python3 -m venv .venv
for f in "$@"; do .venv/bin/python -m pip install --quiet -r "$f"; done
`

// pipProject installs the project itself (editable) when its manifest is
// pyproject.toml or setup.py (a setup.cfg alone cannot be installed), so a
// src-layout package imports.
const pipProject = `.venv/bin/python -m pip install --quiet -e .
`

const pipRun = `exec .venv/bin/python `

func pipCommand(module string, requirements []string, project bool) []string {
	script := pipScript
	if project {
		script += pipProject
	}
	argv := []string{"sh", "-c", script + pipRun + module, "sh"}
	return append(argv, requirements...)
}

// pytestMissing reports output that shows pytest itself is not installed
// (not a failing test): the check is an unavailable tool, never a pass.
func pytestMissing(output string) bool { return pythonToolMissing("pytest", output) }

// pythonToolMissing reports output that shows a Python tool run through the
// project's manager (python -m, uv run, poetry run, a bare command) is not
// installed.
func pythonToolMissing(tool, output string) bool {
	for _, marker := range []string{"No module named " + tool, "Failed to spawn: `" + tool + "`", tool + ": command not found", "Command not found: " + tool} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}
