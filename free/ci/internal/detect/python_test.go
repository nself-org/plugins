package detect

import (
	"testing"
	"testing/fstest"
)

func TestPythonFixtures(t *testing.T) {
	for _, name := range []string{"uv", "poetry", "pip", "no-tests"} {
		t.Run(name, func(t *testing.T) {
			facts := checkGolden(t, "python", name)
			manager := map[string]string{"uv": "uv", "poetry": "poetry", "pip": "pip", "no-tests": "pip"}[name]
			found := false
			for _, f := range facts {
				found = found || f.Kind == "package_manager" && f.Name == manager
			}
			if !found {
				t.Fatalf("package manager %s missing: %+v", manager, facts)
			}
			pytest := false
			for _, f := range facts {
				pytest = pytest || f.Kind == "test_framework" && f.Name == "pytest"
			}
			if pytest != (name != "no-tests") {
				t.Fatalf("pytest fact = %v for %s", pytest, name)
			}
		})
	}
}

// pyFacts names the Python facts of an in-memory tree as "kind:name@path".
func pyFacts(t *testing.T, files fstest.MapFS) map[string]bool {
	t.Helper()
	facts, err := DefaultRegistry().Detect(files, allChecks)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, f := range facts {
		out[f.Kind+":"+f.Name+"@"+f.Path] = true
	}
	return out
}

func TestPythonManagerPrecedence(t *testing.T) {
	base := func(extra ...string) fstest.MapFS {
		m := fstest.MapFS{"pyproject.toml": file("[project]\nname = 'x'\n")}
		for _, e := range extra {
			m[e] = file("lock")
		}
		return m
	}
	cases := []struct {
		name  string
		files fstest.MapFS
		want  string
	}{
		{"uv lock wins over poetry lock", base("uv.lock", "poetry.lock"), "package_manager:uv@uv.lock"},
		{"poetry lock without uv", base("poetry.lock"), "package_manager:poetry@poetry.lock"},
		{"no lock is pip", base(), "package_manager:pip@pyproject.toml"},
		{"requirements only is pip", fstest.MapFS{"requirements.txt": file("pytest\n")}, "package_manager:pip@requirements.txt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pyFacts(t, c.files); !got[c.want] {
				t.Fatalf("want %s in %v", c.want, got)
			}
		})
	}
}

func TestPythonToolEvidence(t *testing.T) {
	proj := "[project]\nname = 'x'\n"
	cases := []struct {
		name  string
		files fstest.MapFS
		want  []string
		deny  []string
	}{
		{"pytest by tests directory", fstest.MapFS{"pyproject.toml": file(proj), "tests/test_a.py": file("")}, []string{"test_framework:pytest@tests/test_a.py"}, nil},
		{"pytest by config", fstest.MapFS{"pyproject.toml": file(proj + "[tool.pytest.ini_options]\n")}, []string{"test_framework:pytest@pyproject.toml"}, nil},
		{"pytest.ini", fstest.MapFS{"setup.py": file(""), "pytest.ini": file("[pytest]\n")}, []string{"test_framework:pytest@pytest.ini"}, nil},
		{"tests directory without python files", fstest.MapFS{"pyproject.toml": file(proj), "tests/data.json": file("{}")}, nil, []string{"test_framework:pytest@tests/data.json"}},
		{"ruff and mypy by pyproject", fstest.MapFS{"pyproject.toml": file(proj + "[tool.ruff.lint]\n[tool.mypy]\n")}, []string{"lint_tool:ruff@pyproject.toml", "type_checker:mypy@pyproject.toml"}, nil},
		{"ruff.toml and mypy.ini", fstest.MapFS{"setup.py": file(""), "ruff.toml": file(""), "mypy.ini": file("[mypy]\n")}, []string{"lint_tool:ruff@ruff.toml", "type_checker:mypy@mypy.ini"}, nil},
		{"mypy in setup.cfg", fstest.MapFS{"setup.cfg": file("[metadata]\nname = x\n[mypy]\nstrict = True\n")}, []string{"type_checker:mypy@setup.cfg", "stack:python@setup.cfg"}, nil},
		{"unconfigured has no ruff or mypy", fstest.MapFS{"pyproject.toml": file(proj)}, nil, []string{"lint_tool:ruff@pyproject.toml", "type_checker:mypy@pyproject.toml"}},
		{"project below a directory", fstest.MapFS{"svc/pyproject.toml": file(proj), "svc/tests/test_a.py": file("")}, []string{"stack:python@svc/pyproject.toml", "test_framework:pytest@svc/tests/test_a.py"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pyFacts(t, c.files)
			for _, w := range c.want {
				if !got[w] {
					t.Errorf("want %s in %v", w, got)
				}
			}
			for _, d := range c.deny {
				if got[d] {
					t.Errorf("unexpected %s", d)
				}
			}
		})
	}
}

func TestPythonNotAProject(t *testing.T) {
	// A pyproject.toml that only configures a tool is not a Python project.
	got := pyFacts(t, fstest.MapFS{"pyproject.toml": file("[tool.black]\nline-length = 100\n")})
	for k := range got {
		if len(k) > 6 && k[:6] == "stack:" && k != "stack:no_stack@." {
			t.Fatalf("tool-only pyproject became a stack: %s", k)
		}
	}
}

func TestPythonIgnoresEnvironments(t *testing.T) {
	got := pyFacts(t, fstest.MapFS{
		"pyproject.toml":                     file("[project]\nname = 'x'\n"),
		".venv/lib/site-packages/a/setup.py": file(""),
		"venv/requirements.txt":              file("x\n"),
	})
	for k := range got {
		if k == "stack:python@.venv/lib/site-packages/a/setup.py" || k == "stack:python@venv/requirements.txt" {
			t.Fatalf("environment directory scanned: %s", k)
		}
	}
}

// TestPythonLockfilesEnterDigests: the lockfiles detector (the one the
// evidence input digests read) carries the Swift and Python lock names.
func TestPythonLockfilesEnterDigests(t *testing.T) {
	got := pyFacts(t, fstest.MapFS{
		"uv.lock": file("a"), "poetry.lock": file("b"), "Package.resolved": file("c"),
		"requirements.txt": file("d"), "requirements-dev.txt": file("e"), "sub/requirements/base.txt": file("f"),
	})
	for _, want := range []string{"lockfile:uv@uv.lock", "lockfile:poetry@poetry.lock", "lockfile:swiftpm@Package.resolved", "lockfile:pip@requirements.txt", "lockfile:pip@requirements-dev.txt"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	if got["lockfile:pip@sub/requirements/base.txt"] {
		t.Error("requirements/base.txt is not a requirements*.txt lockfile")
	}
}
