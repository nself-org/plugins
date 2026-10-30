package run

// Purpose: the platform a foreground attempt asks the scheduler for (P7-NODE-17). An explicit platform on
// the job's matrix entry steers placement to a node of that platform; with none, the invoking host's own
// platform stays the default.
// Inputs:  AttemptSpec with and without Platform, a Session with and without a plan.
// Outputs: the sched.Job the dispatch hook hands the scheduler.

import (
	"runtime"
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

func platformJob(t *testing.T, explicit string, sess *Session) (platformGot string, support []string, matrixKey string) {
	t.Helper()
	rd := &runDispatch{sess: sess}
	j := rd.schedJob(AttemptSpec{Job: &model.Job{Kind: "test", Run: []string{"true"}}, N: 1, Foreground: true, Platform: explicit}, "a1", "row1", 1, 64)
	return j.Requirements.Platform, j.Pipeline.Support, j.MatrixKey
}

func TestWantPlatform(t *testing.T) {
	host := runtime.GOOS + "/" + runtime.GOARCH
	if got := wantPlatform(""); got != host {
		t.Fatalf("no explicit platform must ask for the host platform %q, got %q", host, got)
	}
	if got := wantPlatform("linux/riscv64"); got != "linux/riscv64" {
		t.Fatalf("an explicit platform must be kept, got %q", got)
	}
}

func TestSchedJobDefaultsToHostPlatform(t *testing.T) {
	host := runtime.GOOS + "/" + runtime.GOARCH
	got, support, key := platformJob(t, "", &Session{})
	if got != host || key != "" || !slices.Equal(support, []string{host}) {
		t.Fatalf("platform %q matrix key %q support %v, want host %q, no key, support [%s]", got, key, support, host, host)
	}
}

func TestSchedJobExplicitPlatformSteers(t *testing.T) {
	const other = "linux/riscv64" // never the host of a developer or CI machine
	got, support, key := platformJob(t, other, &Session{})
	if got != other || key != other {
		t.Fatalf("platform %q matrix key %q, want %q for both", got, key, other)
	}
	if !slices.Contains(support, other) {
		t.Fatalf("support %v lacks the explicit platform: the scheduler would refuse it with E690", support)
	}
}

func TestSchedJobExplicitPlatformKeepsPlanSupport(t *testing.T) {
	sess := &Session{Plan: &plan.Plan{Support: []string{"linux/amd64", "linux/arm64"}}}
	got, support, _ := platformJob(t, "linux/arm64", sess)
	if got != "linux/arm64" || !slices.Equal(support, []string{"linux/amd64", "linux/arm64"}) {
		t.Fatalf("platform %q support %v, want linux/arm64 with the plan's support unchanged", got, support)
	}
}
