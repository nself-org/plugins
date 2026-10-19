//go:build !windows

package exec

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func runCapture(t *testing.T, e *Executor, s JobSpec) (AttemptResult, string) {
	t.Helper()
	var mu sync.Mutex
	var out strings.Builder
	s.Output = func(p []byte) { mu.Lock(); defer mu.Unlock(); out.Write(p) }
	res := e.Run(context.Background(), s)
	mu.Lock()
	defer mu.Unlock()
	return res, out.String()
}

// TestStdinReachesAProcessJobOnly proves D-1001: a JobSpec Stdin is the job's
// standard input, not argv or the environment, and a job without one reads EOF.
func TestStdinReachesAProcessJobOnly(t *testing.T) {
	script := `read line; printf 'got=%s\n' "$line"; printf 'argv=%s env=%s\n' "$*" "$(env | grep -c STDIN-SECRET-1 || true)"`
	spec := JobSpec{AttemptID: "stdin-ok", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", script, "job"}, Stdin: strings.NewReader("STDIN-SECRET-1\n")}
	res, out := runCapture(t, &Executor{}, spec)
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run %+v", res)
	}
	if !strings.Contains(out, "got=STDIN-SECRET-1\n") || !strings.Contains(out, "argv= env=0") {
		t.Fatalf("stdin not delivered, or leaked to argv/env:\n%s", out)
	}
	spec = JobSpec{AttemptID: "stdin-none", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", `read line; echo "got=[$line]"`}}
	if _, out = runCapture(t, &Executor{}, spec); !strings.Contains(out, "got=[]") {
		t.Fatalf("a job without Stdin must read EOF:\n%s", out)
	}
}

// TestMasksRedactTheValueEvenSplitAcrossChunks proves a value listed in Masks never
// reaches the output sink, in one write, split across writes, or left in a final partial line.
func TestMasksRedactTheValueEvenSplitAcrossChunks(t *testing.T) {
	const value = "JITCONFIG-synthetic-0123456789"
	script := `printf 'a ` + value + ` b\n'; printf 'JITCONFIG-syn'; sleep .2; printf 'thetic-0123456789 tail'`
	spec := JobSpec{AttemptID: "mask-split", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", script}, Masks: []string{value}}
	spec.Job = model.Job{}
	res, out := runCapture(t, &Executor{NewRedactor: NewOutputRedactor}, spec)
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run %+v", res)
	}
	if strings.Contains(out, value) || strings.Contains(out, "JITCONFIG") || strings.Count(out, maskText) != 2 || !strings.Contains(out, "a [redacted] b\n") || !strings.HasSuffix(out, "[redacted] tail") {
		t.Fatalf("value not masked:\n%q", out)
	}
}

// TestPlannedContainerRefusesStdin: the planned path already spends stdin on the env file, so a
// Stdin there must fail closed instead of being dropped.
func TestPlannedContainerRefusesStdin(t *testing.T) {
	calls := fakeRuntime(t, "ok\n", "0")
	spec := plannedSpec(t, "plan-stdin", hardPlanner{})
	spec.Stdin = strings.NewReader("STDIN-SECRET-1\n")
	res := (&Executor{}).Run(context.Background(), spec)
	var e *Error
	if !asError(res.Err, &e) || e.Code != "E613" {
		t.Fatalf("want E613, got %+v", res)
	}
	if got := calls(); len(got) > 1 || (len(got) == 1 && got[0] != "") {
		t.Fatalf("the runtime ran despite the refusal: %q", got)
	}
}

// TestBaselineContainerForwardsStdin: the baseline docker run gets --interactive only
// when a Stdin is given, and the value arrives on the CLI's stdin, never argv.
func TestBaselineContainerForwardsStdin(t *testing.T) {
	calls, stdin := fakeRuntimeStdin(t, "ok\n", "0")
	spec := JobSpec{AttemptID: "base-stdin", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"true"},
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}, Stdin: strings.NewReader("STDIN-SECRET-1\n")}
	if res := (&Executor{}).Run(context.Background(), spec); res.Err != nil {
		t.Fatal(res.Err)
	}
	if stdin() != "STDIN-SECRET-1\n" {
		t.Fatalf("stdin %q", stdin())
	}
	for _, c := range calls() {
		if strings.Contains(c, "STDIN-SECRET-1") || (strings.HasPrefix(c, "run ") && !strings.Contains(c, " --interactive ")) {
			t.Fatalf("run call %q", c)
		}
	}
	calls, _ = fakeRuntimeStdin(t, "ok\n", "0")
	spec = JobSpec{AttemptID: "base-nostdin", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"true"},
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}}
	if res := (&Executor{}).Run(context.Background(), spec); res.Err != nil {
		t.Fatal(res.Err)
	}
	for _, c := range calls() {
		if strings.Contains(c, "--interactive") {
			t.Fatalf("no Stdin but run is interactive: %q", c)
		}
	}
}
