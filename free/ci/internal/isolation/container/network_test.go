package container

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// attemptPlanner is a fake isolation.AttemptPlanner that records what Wrap hands it.
type attemptPlanner struct {
	binary   string
	attempt  isolation.Attempt
	scope    model.NetworkScope
	err      error
	released int
}

func (p *attemptPlanner) Plan(model.NetworkScope) (isolation.NetworkPlan, error) {
	return isolation.NetworkPlan{}, isolation.ScopeUnenforceable("stateless plan refused")
}

func (p *attemptPlanner) PlanAttempt(_ context.Context, binary string, a isolation.Attempt, scope model.NetworkScope) (isolation.NetworkPlan, error) {
	p.binary, p.attempt, p.scope = binary, a, scope
	plan := isolation.NetworkPlan{Network: "nself-ci-" + a.ID + "-net", DNS: []string{"127.0.0.1"}, Env: []string{"HTTPS_PROXY=http://172.30.0.2:3128"},
		Release: func(context.Context) { p.released++ }}
	if p.err != nil {
		return plan, p.err
	}
	return plan, nil
}

func wrapWith(t *testing.T, pl isolation.NetworkPlanner, s exec.JobSpec) (exec.JobSpec, error) {
	t.Helper()
	p := New(Options{Runner: &fakeRunner{info: map[string]string{"docker": dockerRootful}}, Network: pl, UIDs: func() []int { return []int{501} }})
	return p.Wrap(s)
}

func TestWrapCreatesPerAttemptNetworkAndChainsRelease(t *testing.T) {
	ap := &attemptPlanner{}
	prev := 0
	s, err := wrapWith(t, ap, exec.JobSpec{AttemptID: "a1", CoordinatorID: "c1", Slot: 2, Network: "restricted", Release: func(context.Context) { prev++ }})
	if err != nil {
		t.Fatal(err)
	}
	if ap.binary != "docker" || ap.attempt.ID != "a1" || ap.attempt.CoordinatorID != "c1" || ap.attempt.Slot != 2 || ap.scope != "restricted" {
		t.Errorf("planner saw %+v %+v %s", ap.binary, ap.attempt, ap.scope)
	}
	if len(s.NetEnv) != 1 || !strings.HasPrefix(s.NetEnv[0], "HTTPS_PROXY=") {
		t.Errorf("NetEnv %v", s.NetEnv)
	}
	cmd, err := s.Plan.Plan(exec.ContainerRun{Name: "nself-ci-a1", Image: "x@sha256:" + strings.Repeat("a", 64), Workspace: "/work/a1", Command: []string{"true"}, AttemptID: "a1", CoordinatorID: "c1", Slot: 2, CPU: 1, MemoryMB: 256})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--network=nself-ci-a1-net") || !strings.Contains(joined, "--dns 127.0.0.1") || strings.Contains(joined, "HTTPS_PROXY") {
		t.Errorf("argv %s", joined)
	}
	s.Release(context.Background())
	if ap.released != 1 || prev != 1 {
		t.Errorf("released %d, previous release ran %d times", ap.released, prev)
	}
}

// A refused scope leaves nothing behind and no Plan: the job never starts.
func TestWrapRefusedScopeReleasesAndFails(t *testing.T) {
	ap := &attemptPlanner{err: isolation.ScopeUnenforceable("the sidecar did not start")}
	s, err := wrapWith(t, ap, exec.JobSpec{AttemptID: "a1", Network: "restricted"})
	if err == nil || !strings.Contains(err.Error(), "E679") || s.Plan != nil || s.Release != nil || len(s.NetEnv) != 0 {
		t.Fatalf("plan=%v release=%v env=%v err=%v", s.Plan, s.Release != nil, s.NetEnv, err)
	}
	if ap.released != 1 {
		t.Errorf("released %d times, want the partial resources removed once", ap.released)
	}
	// a non-coded failure from the planner is reported as it is
	ap2 := &attemptPlanner{err: errors.New("boom")}
	if _, err := wrapWith(t, ap2, exec.JobSpec{AttemptID: "a1", Network: "internet"}); err == nil {
		t.Error("planner error swallowed")
	}
}

// A planner without per-attempt resources keeps the stateless path (none, lan).
func TestWrapStatelessPlannerKeepsOldBehaviour(t *testing.T) {
	s, err := wrapWith(t, isolation.DefaultNetworkPlanner(), exec.JobSpec{AttemptID: "a1", Network: "none"})
	if err != nil || s.Plan == nil || s.Release != nil || len(s.NetEnv) != 0 {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := wrapWith(t, isolation.DefaultNetworkPlanner(), exec.JobSpec{AttemptID: "a1", Network: "restricted"}); err == nil || !strings.Contains(err.Error(), "E679") {
		t.Errorf("restricted without a scope planner: %v", err)
	}
}
