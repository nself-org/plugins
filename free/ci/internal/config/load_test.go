package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
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
		{"bad enum", "version: 1\njobs:\n  a:\n    kind: nope\n    run: [go, test]\n", 4, "/jobs/a/kind", "must be one of"},
		{"wrong type", "version: 1\njobs:\n  a:\n    kind: test\n    run: [go, test]\n    timeout: 5\n", 6, "/jobs/a/timeout", "wrong type"},
		{"bad version", "version: 2\n", 1, "/version", "must equal 1"},
		{"missing version", "jobs: {}\n", 1, "", "missing properties"},
		{"second job is the bad one", "version: 1\njobs:\n" + goodJob + "  b:\n    kind: weird\n    run: [x]\n", 7, "/jobs/b/kind", "must be one of"},
		{"bad list item", "version: 1\njobs:\n  a:\n    kind: test\n    run: [go, test]\n    when:\n      triggers: [push, never]\n", 7, "/jobs/a/when/triggers/1", "must be one of"},
		{"duplicate key", "version: 1\nversion: 1\n", 2, "/version", "duplicate key"},
		{"non-string key", "version: 1\n7: x\n", 2, "", "keys must be strings"},
		{"two documents", "version: 1\n---\nversion: 1\n", 3, "", "more than one YAML document"},
		{"not a mapping", "- a\n- b\n", 1, "", "must be a mapping"},
		{"empty", "", 0, "", "empty document"},
		{"yaml syntax", "version: 1\njobs: [\n", 2, "", "did not find expected"},
		{"unknown dependency", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    deps: [ghost]\n", 6, "/jobs/a/deps/0", "not defined"},
		{"unknown service", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    needs_services: [db]\n", 6, "/jobs/a/needs_services/0", "not defined"},
		{"unknown pipeline job", "version: 1\npipelines:\n  p:\n    triggers: [push]\n    jobs: [ghost]\n", 5, "/pipelines/p/jobs/0", "not defined"},
		{"pattern error names the bad job", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    timeout: 30s\n  b:\n    kind: test\n    run: [x]\n    timeout: \"30\"\n", 10, "/jobs/b/timeout", "must match the pattern"},
		{"pattern error in a list", "version: 1\nsupport:\n  - linux/amd64\n  - invalid\n", 4, "/support/1", "must match the pattern"},
		{"null run", "version: 1\njobs:\n  a:\n    kind: test\n    run:\n", 5, "/jobs/a/run", "null is not allowed"},
		{"null policy is fine only inside policy", "version: 1\ndetect:\n", 2, "/detect", "null is not allowed"},
		{"timeout overflow", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    timeout: 999999999999999999999h\n", 6, "/jobs/a/timeout", "positive duration"},
		{"negative timeout", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    timeout: -5s\n", 6, "/jobs/a/timeout", "positive duration"},
		{"infinite number", "version: 1\npolicy: {a: 1}\njobs:\n  a:\n    kind: test\n    run: [x]\n    requires: {cpu: .inf}\n", 7, "/jobs/a/requires/cpu", "finite"},
		{"integer too large", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    requires: {cpu: 9223372036854775808}\n", 6, "/jobs/a/requires/cpu", "out of range"},
		{"workdir escapes", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    workdir: ../../outside\n", 6, "/jobs/a/workdir", "inside the repository"},
		{"absolute path glob", "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    when:\n      paths: [src/**, /etc/**]\n", 7, "/jobs/a/when/paths/1", "inside the repository"},
		{"unsafe job id", "version: 1\njobs:\n  ../x:\n    kind: test\n    run: [x]\n", 3, "/jobs/..~1x", "job id must match"},
		{"secret-looking string with message words", "version: 1\njobs:\n  a: {kind: test, run: [echo], secrets: [project/ok]}\n  b: {kind: test, run: [echo], secrets: [\"ghp_FAKE does not match\"]}\n", 4, "/jobs/b/secrets/0", "must match the pattern"},
		{"composite value", "version: 1\njobs:\n  a: {kind: test, run: [echo], secrets: [project/ok]}\n  b: {kind: test, run: [echo], secrets: [[ghp_FAKE]]}\n", 4, "/jobs/b/secrets/0", "wrong type"},
		{"float where integer", "version: 1\njobs: {a: {kind: test, run: [echo], requires: {cpu: 18446744073709551616}}}\n", 2, "/jobs/a/requires/cpu", "out of range"},
		{"conditional schema picks the failing artifact", "version: 1\njobs: {a: {kind: test, run: [echo], artifacts: [{path: x, kind: package}, {path: y, image: {ref: x}, kind: package}]}}\n", 2, "/jobs/a/artifacts/1/kind", "must equal"},
		{"float spelling of an integer", "version: 1\njobs: {a: {kind: test, run: [echo], requires: {cpu: 1e30}}}\n", 2, "/jobs/a/requires/cpu", "out of range"},
		{"dotted key in typed error", "version: 1\njobs:\n  a.b: {kind: test, run: [echo], requires: {cpu: 1e30}}\n", 3, "/jobs/a.b/requires/cpu", "out of range"},
		{"env_present holds a value", "version: 1\njobs: {a: {kind: test, run: [x], when: {env_present: [\"TOKEN=secretvalue\"]}}}\n", 2, "/jobs/a/when/env_present/0", "variable names"},
		{"env key is not a name", "version: 1\njobs: {a: {kind: test, run: [x], env: {\"bad key\": v}}}\n", 2, "/jobs/a/env", "variable names"},
		{"unpinned container image", "version: 1\njobs: {a: {kind: test, run: [x], container: {image: \"alpine:latest\"}}}\n", 2, "/jobs/a/container/image", "pinned by digest"},
		{"artifact path escapes", "version: 1\njobs: {a: {kind: test, run: [x], artifacts: [{path: /etc/passwd}]}}\n", 2, "/jobs/a/artifacts/0/path", "inside the repository"},
		{"cache path escapes", "version: 1\njobs: {a: {kind: test, run: [x], cache: {paths: [../../x]}}}\n", 2, "/jobs/a/cache/paths/0", "inside the repository"},
		{"detect root escapes", "version: 1\ndetect: {roots: [/etc]}\n", 2, "/detect/roots/0", "inside the repository"},
		{"image with no reference", "version: 1\njobs: {a: {kind: test, run: [x], container: {image: \"@sha256:" + strings.Repeat("a", 64) + "\"}}}\n", 2, "/jobs/a/container/image", "pinned by digest"},
		{"image with a bad repository", "version: 1\njobs: {a: {kind: test, run: [x], container: {image: \"!!!@sha256:" + strings.Repeat("a", 64) + "\"}}}\n", 2, "/jobs/a/container/image", "pinned by digest"},
		{"image with a leading slash", "version: 1\njobs: {a: {kind: test, run: [x], container: {image: \"/alpine@sha256:" + strings.Repeat("a", 64) + "\"}}}\n", 2, "/jobs/a/container/image", "pinned by digest"},
		{"image with empty dns label", "version: 1\njobs: {a: {kind: test, run: [x], container: {image: \"foo..bar/alpine@sha256:" + strings.Repeat("a", 64) + "\"}}}\n", 2, "/jobs/a/container/image", "pinned by digest"},
		{"image with a hyphen label", "version: 1\njobs: {a: {kind: test, run: [x], container: {image: \"foo.-bar/alpine@sha256:" + strings.Repeat("a", 64) + "\"}}}\n", 2, "/jobs/a/container/image", "pinned by digest"},
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
// hand-converted plugin-gauth jobs (the gauth-* ones) equal what `config
// migrate` produces. The file also declares the repository's gate jobs
// (P7-CI-07), which the migration knows nothing about.
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
	gauth := map[string]model.Job{}
	for id, j := range l.Config.Jobs {
		if strings.HasPrefix(id, "gauth-") {
			gauth[id] = j
		}
	}
	got, _ := json.Marshal(gauth)
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

func TestErrorNeverEchoesTheValue(t *testing.T) {
	doc := "version: 1\njobs:\n  a:\n    kind: test\n    run: [x]\n    secrets: [project/ok, ghp_FAKEFAKEFAKEFAKEFAKE]\n"
	_, err := Parse("ci.yaml", []byte(doc))
	if err == nil || strings.Contains(err.Error(), "FAKEFAKE") || strings.Contains(err.Error(), "ghp_") {
		t.Fatalf("value echoed or no error: %v", err)
	}
	if e := err.(*Error); e.Problems[0].Pointer != "/jobs/a/secrets/1" || e.Problems[0].Line != 6 {
		t.Errorf("located %+v", e.Problems[0])
	}
}

func TestAliasBombIsRefused(t *testing.T) {
	var b strings.Builder
	b.WriteString("version: 1\nsupport: &s [" + strings.Repeat("x", 1<<16) + "]\nx:\n")
	b.WriteString("  - &big " + strings.Repeat("y", 1<<16) + "\n")
	for i := 0; i < 200; i++ {
		b.WriteString("  - *big\n")
	}
	start := time.Now()
	_, err := Parse("bomb.yaml", []byte(b.String()))
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("alias bomb accepted: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %v", time.Since(start))
	}
}

func TestErrorTextHasNoControlCharacters(t *testing.T) {
	doc := "version: 1\njobs:\n  \"a\\nE620: fake\":\n    kind: test\n    run: [x]\n"
	_, err := Parse("ci.yaml", []byte(doc))
	if err == nil || strings.ContainsAny(err.Error(), "\x1b\r") || strings.Count(err.Error(), "\n") != 0 {
		t.Fatalf("control characters survived: %q", err)
	}
}

func TestNeverEchoesAnyValue(t *testing.T) {
	for _, doc := range []string{
		"version: 1\njobs:\n  a: {kind: test, run: [echo], secrets: [project/ok]}\n  b: {kind: test, run: [echo], secrets: [\"ghp_FAKE does not match\"]}\n",
		"version: 1\njobs:\n  b: {kind: test, run: [echo], secrets: [[ghp_FAKE]]}\n",
		"version: 1\njobs:\n  b: {kind: ghp_FAKE, run: [echo]}\n",
		"version: 1\njobs:\n  b: {kind: test, run: [echo], timeout: ghp_FAKE}\n",
		"version: 1\njobs:\n  b: {kind: test, run: [echo], when: {triggers: [ghp_FAKE]}}\n",
		"version: 1\njobs:\n  a: {kind: test, run: [echo], secrets: [project/ok]}\n  b: {kind: test, run: [echo], secrets: [\"bad validating /properties/jobs: ghp_FAKE\"]}\n",
		"version: 1\njobs:\n  a: {kind: test, run: [echo], secrets: [!!int ghp_FAKE]}\n",
		"version: 1\njobs: {a: {kind: test, run: [echo], secrets: [*ghp_FAKE]}}\n",
		"version: 1\njobs: {a: {kind: test, run: [echo], secrets: [\"ghp_FAKE\\q\"]}}\n",
		"version: 1\njobs:\n  a: {kind: test, run: [echo], secrets: [\"enum: x unexpected additional properties [\\\"a\\\"] ghp_FAKE\"]}\n",
	} {
		_, err := Parse("ci.yaml", []byte(doc))
		if err == nil || strings.Contains(err.Error(), "ghp_FAKE") {
			t.Errorf("%q: %v", doc, err)
		}
	}
}

func TestAliasBombViaTaggedScalarsAndKeys(t *testing.T) {
	big := strings.Repeat("A", 1<<16)
	var tagged, keys strings.Builder
	tagged.WriteString("version: 1\npolicy:\n  seed: &s !!binary " + big + "\n  copies: [")
	keys.WriteString("version: 1\npolicy:\n  base: &m\n    ? " + big + "\n    : 1\n  copies: [")
	for i := 0; i < 100; i++ {
		tagged.WriteString("*s,")
		keys.WriteString("*m,")
	}
	tagged.WriteString("]\n")
	keys.WriteString("]\n")
	for name, doc := range map[string]string{"tagged": tagged.String(), "keys": keys.String()} {
		if _, err := Parse("bomb.yaml", []byte(doc)); err == nil || !strings.Contains(err.Error(), "too large") {
			t.Errorf("%s bomb accepted: %v", name, err)
		}
	}
}

func TestPolicyKeepsLargeIntegersAndNulls(t *testing.T) {
	if _, err := Parse("ci.yaml", []byte("version: 1\npolicy: {value: 9223372036854775808, nothing: ~}\n")); err != nil {
		t.Fatalf("opaque policy rejected: %v", err)
	}
}

func TestPathsAreNormalisedBeforeContainment(t *testing.T) {
	for p, escapes := range map[string]bool{"src/../lib": false, "./a/b": false, "a/**": false, "..": true, "a/../../x": true, "../x": true, "/etc": true, "~/x": true, "a\\..\\..\\x": true} {
		if escapesRepo(p) != escapes {
			t.Errorf("escapesRepo(%q) = %v", p, !escapes)
		}
	}
}

func TestPinnedImageReferences(t *testing.T) {
	d := "@sha256:" + strings.Repeat("a", 64)
	for ref, ok := range map[string]bool{
		"alpine" + d: true, "ghcr.io/nself-org/ci:1.2" + d: true, "localhost:5000/a/b" + d: true, "[::1]:5000/alpine" + d: true,
		"registry.example.com:443/team/app_x-y" + d: true,
		"/alpine" + d: false, "!!!" + d: false, "foo..bar/alpine" + d: false, "UPPER/case" + d: false, "a:b:c" + d: false,
		"alpine@sha256:abc": false, "host:99999/x" + d: false, "": false,
	} {
		if pinnedImageRef(ref) != ok {
			t.Errorf("pinnedImageRef(%q) = %v", ref, !ok)
		}
	}
}

func TestPinnedImageAndNamesAccepted(t *testing.T) {
	doc := "version: 1\njobs: {a: {kind: test, run: [x], env: {LOG_LEVEL: debug}, when: {env_present: [DB_URL]}, container: {image: \"ghcr.io/nself-org/ci-toolchain:1.2@sha256:" + strings.Repeat("a", 64) + "\"}}}\n"
	if _, err := Parse("ci.yaml", []byte(doc)); err != nil {
		t.Fatal(err)
	}
}

func TestWarningNeverContainsAValue(t *testing.T) {
	// env_present is refused unless it is a variable name, so the D28 warning
	// can only ever print names.
	_, err := Parse("ci.yaml", []byte("version: 1\njobs: {a: {kind: test, run: [x], when: {env_present: [\"ghp_FAKEENVP=secretvalue\"]}}}\n"))
	if err == nil || strings.Contains(err.Error(), "secretvalue") || strings.Contains(err.Error(), "ghp_") {
		t.Fatalf("value echoed or accepted: %v", err)
	}
}
