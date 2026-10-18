package run

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// D-0948: a job must not inherit the operator's whole environment, and a
// secret a job prints must not reach the tail, the stream or the evidence.
func TestJobEnvironmentIsAllowlistedAndOutputRedacted(t *testing.T) {
	t.Setenv("NSELF_TEST_OPERATOR_TOKEN", "operator-token-value-1234")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws-secret-value-5678")
	ghp := "ghp_" + strings.Repeat("a1B2", 9)
	script := `echo "inherited=[$NSELF_TEST_OPERATOR_TOKEN$AWS_SECRET_ACCESS_KEY]"; echo "path=${PATH:+set}";` +
		` echo "DB_PASSWORD=hunter2hunter2"; echo "pushed with ` + ghp + `"; exit 3`
	r := newRig(t, map[string]model.Job{"a": sh(script)}, gitFake{})
	r.C.Opts.Stream = true
	var excerpt string
	r.C.Hooks.AfterRuns = []AfterRunHook{afterFn(func(ri RunInfo) error {
		for _, c := range ri.Evidence.Checks {
			excerpt += c.Excerpt
		}
		return nil
	})}
	res := r.run(t)
	o := outcome(res, "a")
	if o.State != StateFailed || o.ExitCode != 3 {
		t.Fatalf("a: %+v\n%s", o, r.Out)
	}
	for name, got := range map[string]string{"tail": o.Tail, "stream": r.Out.String(), "evidence": excerpt} {
		for _, leak := range []string{"operator-token-value-1234", "aws-secret-value-5678", "hunter2hunter2", ghp} {
			if strings.Contains(got, leak) {
				t.Errorf("%s leaks %q:\n%s", name, leak, got)
			}
		}
	}
	for _, want := range []string{"inherited=[]", "path=set", "[redacted]"} {
		if !strings.Contains(o.Tail, want) {
			t.Errorf("tail lacks %q:\n%s", want, o.Tail)
		}
	}
	if excerpt == "" {
		t.Error("failed job has no evidence excerpt")
	}
}
