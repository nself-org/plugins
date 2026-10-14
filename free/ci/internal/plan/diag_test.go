package plan

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// parseProblems runs the real loader (not a copy of its messages) so a
// wording change there breaks this test instead of silently turning E616
// back into E620.
func parseProblems(t *testing.T, yaml string) []Diagnostic {
	t.Helper()
	_, err := config.Parse("ci.yaml", []byte(yaml))
	var ce *config.Error
	if !errors.As(err, &ce) {
		t.Fatalf("expected a config.Error, got %v", err)
	}
	ds := make([]Diagnostic, len(ce.Problems))
	for i, p := range ce.Problems {
		ds[i] = Diagnostic{File: p.File, Line: p.Line, Pointer: p.Pointer, Message: p.Message}
	}
	return ds
}

const head = "version: 1\njobs:\n"

func TestDAGErrorCycleFromLoader(t *testing.T) {
	ds := parseProblems(t, head+"  a: {kind: test, run: [echo], deps: [b]}\n  b: {kind: test, run: [echo], deps: [c]}\n  c: {kind: test, run: [echo], deps: [a]}\n  d: {kind: test, run: [echo]}\n")
	e := DAGError(ds)
	if e == nil || e.Code != CodeCycle || !reflect.DeepEqual(e.Jobs, []string{"a", "b", "c"}) || !strings.Contains(e.Message, "a -> b -> c -> a") {
		t.Fatalf("cycle mapped to %+v", e)
	}
	if !strings.Contains(e.Message, "ci.yaml:") {
		t.Errorf("location lost: %s", e.Message)
	}
}

func TestDAGErrorDanglingFromLoader(t *testing.T) {
	ds := parseProblems(t, head+"  build: {kind: test, run: [echo]}\n  ship: {kind: test, run: [echo], deps: [build, ghost]}\n")
	e := DAGError(ds)
	if e == nil || e.Code != CodeCycle || !reflect.DeepEqual(e.Jobs, []string{"ship"}) {
		t.Fatalf("dangling mapped to %+v", e)
	}
	for _, want := range []string{"/jobs/ship/deps/1", "dependency 1 of job ship", "not defined"} {
		if !strings.Contains(e.Message, want) {
			t.Errorf("message lacks %q: %s", want, e.Message)
		}
	}
}

func TestDAGErrorMixedKeepsOtherProblems(t *testing.T) {
	ds := parseProblems(t, head+"  a: {kind: test, run: [echo], deps: [b]}\n  b: {kind: test, run: [echo], deps: [a], workdir: ../outside}\n")
	e := DAGError(ds)
	if e == nil || e.Code != CodeCycle || !reflect.DeepEqual(e.Jobs, []string{"a", "b"}) {
		t.Fatalf("mixed mapped to %+v", e)
	}
	if !strings.Contains(e.Message, "other config problems") || !strings.Contains(e.Message, "workdir must stay inside the repository") {
		t.Errorf("the non-DAG problem was dropped: %s", e.Message)
	}
}

func TestDAGErrorLeavesOtherConfigErrors(t *testing.T) {
	for name, yaml := range map[string]string{
		"escaping workdir": head + "  a: {kind: test, run: [echo], workdir: ../x}\n",
		"empty run":        head + "  a: {kind: test, run: []}\n",
		"bad timeout":      head + "  a: {kind: test, run: [echo], timeout: soon}\n",
	} {
		if e := DAGError(parseProblems(t, yaml)); e != nil {
			t.Errorf("%s: non-DAG error became %s", name, e.Code)
		}
	}
	// A wrongly typed dep is a schema error at the same pointer shape: not E616.
	if e := DAGError([]Diagnostic{{File: "f", Pointer: "/jobs/a/deps/0", Message: "expected string"}}); e != nil {
		t.Errorf("schema error at a deps pointer became %s", e.Code)
	}
	if DAGError(nil) != nil {
		t.Error("no problems must give nil")
	}
}

func TestDAGErrorUnescapesJobIDs(t *testing.T) {
	e := DAGError([]Diagnostic{{File: "f", Line: 3, Pointer: "/jobs/a~1b/deps/0", Message: `job "a/b" depends on a job that is not defined`}})
	if e == nil || !reflect.DeepEqual(e.Jobs, []string{"a/b"}) {
		t.Fatalf("got %+v", e)
	}
}

func TestWantedTools(t *testing.T) {
	cfg := cfgOf(map[string]model.Job{"x": {Requirements: &model.Requirements{Tools: []string{"xcode", "go"}}}})
	got := WantedTools(cfg, []Check{{Tool: "go"}, {Tool: "npm"}})
	if !reflect.DeepEqual(got, []string{"docker", "go", "npm", "xcode"}) {
		t.Errorf("tools = %v", got)
	}
}
