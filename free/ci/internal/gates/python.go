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
// exists only when mypy is configured; requirement files reach the helper
// script as arguments, never spliced into it; no check runs through sh, so a
// Windows host can run them (D-1029).

import (
	"path"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/detect"
)

const pythonFix = "Install the project's Python tools (uv, poetry or python3) and its test and type dependencies"

func pythonDefs() []CheckDef {
	return []CheckDef{
		def("python-test", "test", "uv", []string{"uv", "run", "--frozen", "pytest"}, true, true, "fast", pythonFix),
		def("python-lint", "lint", "ruff", []string{"ruff", "check", "."}, false, true, "fast", "Install ruff"),
		def("python-types", "lint", "uv", []string{"uv", "run", "--frozen", "mypy", "."}, false, true, "fast", pythonFix),
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
		case f.Kind == "lockfile" && f.Name == "pip" && requirementsInstalled(path.Base(f.Path)) && path.Dir(f.Path) == nearestProject(idx, path.Dir(f.Path)):
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
			p.Tool, p.Argv = "uv", append([]string{"uv", "run", "--frozen"}, strings.Fields(run)...)
		case "poetry":
			p.Tool, p.Argv = "poetry", append([]string{pythonTool(), "-c", poetryScript}, strings.Fields(run)...)
		default:
			p.Tool, p.Argv = pythonTool(), pipCommand(module, d.requirements, d.project)
		}
	}
}

// pythonTool is the interpreter that runs the helper scripts below.
func pythonTool() string {
	if runtime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}

// pipHead is the Python source (run with `-c`, so no shell is involved and a
// Windows host can run it) that creates the venv, then installs each
// requirements file (the script arguments); pipRun ends it by running the
// module. Python's own venv and subprocess modules replace the shell.
const pipHead = `import os, subprocess, sys, venv
exe = os.path.join(".venv", "Scripts" if os.name == "nt" else "bin", "python")
if not os.path.isdir(".venv"):
    venv.create(".venv", with_pip=True)
def run(*args):
    code = subprocess.call([exe, *args])
    if code:
        sys.exit(code)
for f in sys.argv[1:]:
    run("-m", "pip", "install", "--quiet", "-r", f)
`

// pipProject installs the project itself (editable) when its manifest is
// pyproject.toml or setup.py (a setup.cfg alone cannot be installed), so a
// src-layout package imports.
const pipProject = `run("-m", "pip", "install", "--quiet", "-e", ".")
`

const pipRun = `sys.exit(subprocess.call([exe] + "`

func pipCommand(module string, requirements []string, project bool) []string {
	script := pipHead
	if project {
		script += pipProject
	}
	argv := []string{pythonTool(), "-c", script + pipRun + module + `".split()))`}
	return append(argv, requirements...)
}

// poetryScript runs `poetry install` then `poetry run <args>` (the script
// arguments) with the venv inside the project. The project installs itself
// (a src-layout package must import), except under package-mode = false or
// when the root cannot be installed (an application without a package
// directory), which fall back to --no-root.
const poetryScript = `import os, re, subprocess, sys
os.environ["POETRY_VIRTUALENVS_IN_PROJECT"] = "true"
try:
    text = open("pyproject.toml", encoding="utf-8").read()
except OSError:
    text = ""
code = 1
if not re.search(r"(?m)^\s*package-mode\s*=\s*false\b", text):
    code = subprocess.call(["poetry", "install", "--sync"])
if code:
    code = subprocess.call(["poetry", "install", "--sync", "--no-root"])
if code:
    sys.exit(code)
sys.exit(subprocess.call(["poetry", "run"] + sys.argv[1:]))
`

// requirementsInstalled reports whether a requirements file is one the checks
// install: requirements.txt and its dev/test variants (requirements-dev.txt,
// requirements_test.txt, ...). Docs, platform and other variants stay out.
func requirementsInstalled(base string) bool { return installedRequirements.MatchString(base) }

var installedRequirements = regexp.MustCompile(`^requirements([-_.](dev|test|tests|testing))?\.txt$`)

// pytestMissing reports output that shows pytest itself is not installed
// (not a failing test): the check is an unavailable tool, never a pass.
func pytestMissing(output string) bool {
	for _, marker := range []string{"No module named pytest", "Failed to spawn: `pytest`", "pytest: command not found", "Command not found: pytest"} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}
