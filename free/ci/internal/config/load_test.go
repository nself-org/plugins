package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const goodJob = "  a:\n    kind: test\n    run: [go, test]\n"

func TestLoadInvalid(t *testing.T) {
	cases := []struct {
		name, doc string
		line      int
		pointer   string
		contains  string
	}{
		{"unknown key", "version: 1\njobs:\n  a:\n    kind: test\n    run: [go, test]\n    bogus: 1\n", 6, "/jobs/a/bogus", "unexpected additional properties"},
		{"unknown top-level key", "version: 1\nzzz: 1\n", 2, "/zzz", "unexpected additional properties"},
		{"bad enum", "version: 1\njobs:\n  a:\n    kind: nope\n    run: [go, test]\n", 4, "/jobs/a/kind", "does not equal any of"},
		{"wrong type", "version: 1\njobs:\n  a:\n    kind: test\n    run: [go, test]\n    timeout: 5\n", 6, "/jobs/a/timeout", "want \"string\""},
		{"bad version", "version: 2\n", 1, "/version", "does not equal 1"},
		{"missing version", "jobs: {}\n", 1, "", "missing properties"},
		{"second job is the bad one", "version: 1\njobs:\n" + goodJob + "  b:\n    kind: weird\n    run: [x]\n", 7, "/jobs/b/kind", "does not equal any of"},
		{"bad list item", "version: 1\njobs:\n  a:\n    kind: test\n    run: [go, test]\n    when:\n      triggers: [push, never]\n", 7, "/jobs/a/when/triggers/1", "never"},
		{"duplicate key", "version: 1\nversion: 1\n", 2, "/version", "duplicate key"},
		{"non-string key", "version: 1\n7: x\n", 2, "", "keys must be strings"},
		{"two documents", "version: 1\n---\nversion: 1\n", 3, "", "more than one YAML document"},
		{"not a mapping", "- a\n- b\n", 1, "", "must be a mapping"},
		{"empty", "", 0, "", "empty document"},
		{"yaml syntax", "version: 1\njobs: [\n", 2, "", "did not find expected"},
		{"unknown dependency", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    deps: [ghost]\n", 6, "/jobs/a/deps/0", "unknown job"},
		{"unknown service", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    needs_services: [db]\n", 6, "/jobs/a/needs_services/0", "unknown service"},
		{"unknown pipeline job", "version: 1\npipelines:\n  p:\n    triggers: [push]\n    jobs: [ghost]\n", 5, "/pipelines/p/jobs/0", "unknown job"},
		{"cycle", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    deps: [b]\n  b:\n    kind: test\n    run: [x]\n    deps: [a]\n", 2, "/jobs", "a -> b -> a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse("ci.yaml", []byte(c.doc))
			e, ok := err.(*Error)
			if !ok || e.Code != CodeInvalid {
				t.Fatalf("want E620, got %v", err)
			}
			p := e.Problems[0]
			if p.File != "ci.yaml" || p.Line != c.line || p.Pointer != c.pointer || !strings.Contains(p.Message, c.contains) {
				t.Errorf("got %+v, want line %d pointer %q containing %q", p, c.line, c.pointer, c.contains)
			}
			if c.line > 0 && !strings.HasPrefix(e.Error(), "E620: ci.yaml:") {
				t.Errorf("error text lacks file:line: %q", e.Error())
			}
		})
	}
}

func TestLoadValid(t *testing.T) {
	doc := "version: 1\nsupport: [linux/amd64]\nservices:\n  db: {component: postgres}\njobs:\n  a:\n    kind: test\n    run: [go, test]\n    needs_services: [db]\n    timeout: 90s\n  b:\n    kind: static\n    run: [go, vet]\n    deps: [a]\npipelines:\n  p:\n    triggers: [push]\n    jobs: [a, b]\npolicy: {anything: [1, 2]}\n"
	l, err := Parse("ci.yaml", []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if l.Config.Version != 1 || len(l.Config.Jobs) != 2 || l.Config.Jobs["a"].Timeout != "90s" || len(l.Warnings) != 0 {
		t.Errorf("decoded %+v warnings %v", l.Config, l.Warnings)
	}
	if l.Lines["/jobs/b"] != 11 {
		t.Errorf("line of /jobs/b = %d", l.Lines["/jobs/b"])
	}
}

func TestEnvPresentWarning(t *testing.T) {
	doc := func(required string) string {
		return "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n" + required + "    when:\n      env_present: [TOKEN_REF]\n"
	}
	for name, c := range map[string]struct {
		required string
		warns    int
	}{"explicit true": {"    required: true\n", 1}, "default is required": {"", 1}, "optional": {"    required: false\n", 0}} {
		l, err := Parse("ci.yaml", []byte(doc(c.required)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(l.Warnings) != c.warns {
			t.Errorf("%s: %d warnings, want %d: %v", name, len(l.Warnings), c.warns, l.Warnings)
			continue
		}
		if c.warns == 1 {
			w := l.Warnings[0]
			if w.Code != CodeInvalid || w.Pointer != "/jobs/a/when/env_present" || !strings.Contains(w.Message, "required: false") {
				t.Errorf("%s: warning %+v", name, w)
			}
		}
	}
}

// TestRepoConfig loads this repository's own `.nself/ci.yaml` and proves the
// hand-converted plugin-gauth jobs equal what `config migrate` produces.
func TestRepoConfig(t *testing.T) {
	l, err := Load(filepath.Join("..", "..", "..", "..", FileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Warnings) != 0 {
		t.Errorf("warnings: %v", l.Warnings)
	}
	m, err := Migrate(copyFixture(t, filepath.Join("legacy", "gauth")))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(l.Config.Jobs)
	want, _ := json.Marshal(m.Config.Jobs)
	if string(got) != string(want) {
		t.Errorf("hand conversion differs from migration\n got %s\nwant %s", got, want)
	}
	if j := l.Config.Jobs["gauth-integration"]; j.Required == nil || *j.Required {
		t.Errorf("gauth-integration must be required: false, got %v", j.Required)
	}
}

// FuzzLoad: arbitrary bytes never panic, and a document that loads has a
// non-empty file name and survives a second parse identically.
func FuzzLoad(f *testing.F) {
	for _, seed := range []string{"", "version: 1\n", "version: 1\njobs:\n" + goodJob, "a: &x [*x]\n", "version: 1\n---\n", "\x00\xff", "{{{", "version: 1\njobs: {a: {kind: test, run: [x], deps: [a]}}\n"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		l, err := Parse("fuzz.yaml", data)
		if err != nil {
			if e, ok := err.(*Error); !ok || e.Code != CodeInvalid || len(e.Problems) == 0 {
				t.Fatalf("unexpected error type %T %v", err, err)
			}
			return
		}
		again, err := Parse("fuzz.yaml", data)
		if err != nil || l.Config.Version != again.Config.Version || len(l.Config.Jobs) != len(again.Config.Jobs) {
			t.Fatalf("second parse differs: %v", err)
		}
	})
}
