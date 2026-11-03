package run

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trace"
)

func TestCoordinatorRunEmitsTraceSpans(t *testing.T) {
	const childEnv = "NSELF_CI_TEST_TRACE_CHILD"
	if os.Getenv(childEnv) == "1" {
		hygieneHooks.rt = noRuntime{}
		r := newRig(t, map[string]model.Job{"unit": sh("true"), "excluded": sh("true")}, gitFake{})
		r.C.Opts.Job = "unit"
		if res := r.run(t); res.Exit != 0 || outcome(res, "unit").State != StatePassed {
			t.Fatalf("run failed: %+v\n%s", res, r.Out)
		}
		return
	}
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	cmd := exec.Command(os.Args[0], "-test.run=^TestCoordinatorRunEmitsTraceSpans$")
	cmd.Env = append(os.Environ(), childEnv+"=1", "NSELF_CI_TRACE="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("trace child: %v\n%s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	spans := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var span trace.Span
		if err := json.Unmarshal([]byte(line), &span); err != nil {
			t.Fatalf("decode trace: %v", err)
		}
		spans[span.Name]++
	}
	for _, name := range []string{"run.classify", "run.dispatch", "run.step:unit", "run.finish.evidence", "run.finish.hygiene", "run.finish.status"} {
		if spans[name] != 1 {
			t.Errorf("span %q count = %d, want 1", name, spans[name])
		}
	}
	if spans["run.step:excluded"] != 0 {
		t.Error("excluded job emitted a step span")
	}
}
