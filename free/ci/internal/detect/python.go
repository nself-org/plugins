package detect

// Purpose: detector `python` (P7-CI-64, Epic D33): Python projects, their
// package manager and the test, lint and type tools they configure.
// Inputs:  the snapshot; per directory pyproject.toml, setup.cfg, setup.py,
// requirements*.txt, uv.lock, poetry.lock and the tool config files.
// Outputs: stack python (Path = the primary manifest: pyproject.toml, then
// setup.py, setup.cfg, requirements.txt, other requirements*.txt),
// package_manager uv | poetry | pip, and when configured test_framework
// pytest, lint_tool ruff, type_checker mypy. Lockfiles (uv.lock, poetry.lock,
// requirements*.txt) are digested by the lockfiles detector.
// Constraints: pure over the snapshot; manager precedence is uv when uv.lock
// exists, else poetry when poetry.lock exists, else pip; pytest counts as
// configured by its config or by a tests/ directory holding Python files;
// ruff and mypy only by their config (no TOML parser: section headers only).

import (
	"path"
	"sort"
	"strings"
)

// pyDir collects the Python manifests of one directory.
type pyDir struct {
	pyproject, setupCfg, setupPy string
	requirements                 []string
}

func detectPython(s Snapshot) []Fact {
	dirs := map[string]*pyDir{}
	var names []string
	for p := range s.files {
		base := path.Base(p)
		var set func(*pyDir)
		switch {
		case base == "pyproject.toml":
			set = func(d *pyDir) { d.pyproject = p }
		case base == "setup.cfg":
			set = func(d *pyDir) { d.setupCfg = p }
		case base == "setup.py":
			set = func(d *pyDir) { d.setupPy = p }
		case strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt"):
			set = func(d *pyDir) { d.requirements = append(d.requirements, p) }
		default:
			continue
		}
		dir := path.Dir(p)
		if dirs[dir] == nil {
			dirs[dir] = &pyDir{}
			names = append(names, dir)
		}
		set(dirs[dir])
	}
	sort.Strings(names)
	var out []Fact
	for _, dir := range names {
		d := dirs[dir]
		sort.Strings(d.requirements)
		out = append(out, detectPythonDir(s, dir, d)...)
	}
	return out
}

func detectPythonDir(s Snapshot, dir string, d *pyDir) []Fact {
	var pyproject, setupCfg string
	if d.pyproject != "" {
		b, err := s.read(d.pyproject)
		if err != nil {
			return []Fact{unknown("manifest", d.pyproject, "unreadable pyproject.toml")}
		}
		pyproject = string(b)
	}
	if d.setupCfg != "" {
		if b, err := s.read(d.setupCfg); err == nil {
			setupCfg = string(b)
		}
	}
	primary := ""
	switch {
	case d.pyproject != "" && pyprojectIsProject(pyproject):
		primary = d.pyproject
	case d.setupPy != "":
		primary = d.setupPy
	case d.setupCfg != "" && hasAnySection(setupCfg, "[metadata]", "[options]", "[tool:pytest]"):
		primary = d.setupCfg
	case len(d.requirements) > 0:
		primary = d.requirements[0]
		for _, r := range d.requirements {
			if path.Base(r) == "requirements.txt" {
				primary = r
			}
		}
	}
	if primary == "" {
		return nil // a pyproject.toml that only configures tools is not a Python project
	}
	out := []Fact{fact("stack", "python", primary)}
	in := func(name string) string { return path.Join(dir, name) }
	switch {
	case s.has(in("uv.lock")):
		out = append(out, fact("package_manager", "uv", in("uv.lock")))
	case s.has(in("poetry.lock")):
		out = append(out, fact("package_manager", "poetry", in("poetry.lock")))
	default:
		out = append(out, fact("package_manager", "pip", primary))
	}
	if ev := pytestEvidence(s, dir, pyproject, setupCfg); ev != "" {
		out = append(out, fact("test_framework", "pytest", ev))
	}
	switch {
	case s.has(in("ruff.toml")):
		out = append(out, fact("lint_tool", "ruff", in("ruff.toml")))
	case s.has(in(".ruff.toml")):
		out = append(out, fact("lint_tool", "ruff", in(".ruff.toml")))
	case hasSectionPrefix(pyproject, "[tool.ruff"):
		out = append(out, fact("lint_tool", "ruff", d.pyproject))
	}
	switch {
	case s.has(in("mypy.ini")):
		out = append(out, fact("type_checker", "mypy", in("mypy.ini")))
	case s.has(in(".mypy.ini")):
		out = append(out, fact("type_checker", "mypy", in(".mypy.ini")))
	case hasSectionPrefix(pyproject, "[tool.mypy"):
		out = append(out, fact("type_checker", "mypy", d.pyproject))
	case hasAnySection(setupCfg, "[mypy]"):
		out = append(out, fact("type_checker", "mypy", d.setupCfg))
	}
	return out
}

// pytestEvidence names the file that proves pytest is wanted, or "".
func pytestEvidence(s Snapshot, dir, pyproject, setupCfg string) string {
	in := func(name string) string { return path.Join(dir, name) }
	switch {
	case s.has(in("pytest.ini")):
		return in("pytest.ini")
	case hasAnySection(pyproject, "[tool.pytest.ini_options]", "[tool.pytest]"):
		return in("pyproject.toml")
	case hasAnySection(setupCfg, "[tool:pytest]"):
		return in("setup.cfg")
	case s.has(in("tox.ini")) && fileHasSection(s, in("tox.ini"), "[pytest]"):
		return in("tox.ini")
	}
	prefix := in("tests") + "/"
	var found []string
	for p := range s.files {
		if strings.HasPrefix(p, prefix) && strings.HasSuffix(p, ".py") {
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		return ""
	}
	sort.Strings(found)
	return found[0]
}

func pyprojectIsProject(text string) bool {
	return hasAnySection(text, "[project]", "[build-system]", "[dependency-groups]") || hasSectionPrefix(text, "[tool.poetry") || hasSectionPrefix(text, "[tool.uv")
}

func fileHasSection(s Snapshot, p, section string) bool {
	b, err := s.read(p)
	return err == nil && hasAnySection(string(b), section)
}

// tableHeader returns the TOML table header on a line, without comment or
// blanks (`[project] # note` is `[project]`), or "" when the line is no header.
func tableHeader(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") {
		return ""
	}
	end := strings.Index(line, "]")
	if end < 0 {
		return ""
	}
	return strings.ReplaceAll(line[:end+1], " ", "")
}

// hasAnySection reports whether a line of text is exactly one of the section headers.
func hasAnySection(text string, headers ...string) bool {
	for _, line := range strings.Split(text, "\n") {
		h := tableHeader(line)
		for _, want := range headers {
			if h != "" && h == want {
				return true
			}
		}
	}
	return false
}

// hasSectionPrefix reports whether a line of text opens a section that starts with prefix.
func hasSectionPrefix(text, prefix string) bool {
	for _, line := range strings.Split(text, "\n") {
		if h := tableHeader(line); h != "" && strings.HasPrefix(h, prefix) {
			return true
		}
	}
	return false
}
