package run

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// refusing is an Isolator that refuses while refuse(slot, call) says so.
type refusing struct {
	calls  atomic.Int32
	refuse func(slot, call int) error
}

func (r *refusing) Prepare(_ context.Context, s *ciexec.JobSpec) error {
	return r.refuse(s.Slot, int(r.calls.Add(1)))
}

func waitRun(t *testing.T, iso ciexec.Isolator) (*runState, ciexec.JobSpec) {
	t.Helper()
	privateSlots(t)
	home := t.TempDir()
	rs := &runState{c: &Coordinator{exec: &ciexec.Executor{Isolation: iso}}, sess: &Session{Home: home}}
	t.Cleanup(func() { releaseAll(&rs.sess.slots) })
	return rs, ciexec.JobSpec{AttemptID: "wait-1", CoordinatorID: "coord", Home: home, Command: []string{"true"}, Job: model.Job{Isolation: "process"}}
}

func fastWaits(t *testing.T, slot, runtime time.Duration) {
	t.Helper()
	s, r, a, b, n := slotWaitMax, runtimeWaitMax, waitStepFirst, waitStepMax, waitNoteEvery
	slotWaitMax, runtimeWaitMax, waitStepFirst, waitStepMax = slot, runtime, 5*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { slotWaitMax, runtimeWaitMax, waitStepFirst, waitStepMax, waitNoteEvery = s, r, a, b, n })
}

// A refusal before anything ran is waited out while its budget lasts; other errors and reasons that waiting cannot
// cure end the attempt at once, as before.
func TestRunWaitingByReason(t *testing.T) {
	cases := []struct {
		name      string
		refuseFor int   // refusals before the isolator accepts (-1: always)
		err       error // the refusal
		wantState string
		wantCalls int // -1: more than one
		wantNote  bool
	}{
		{"probe failed, then ready", 2, isolation.Unavailable(isolation.ReasonProbeFailed, "daemon restarting"), "passed", 3, true},
		{"runtime missing, then installed", 1, isolation.Unavailable(isolation.ReasonRuntimeMissing, "no docker"), "passed", 2, true},
		{"limits unenforceable, never fixed", -1, isolation.Unavailable(isolation.ReasonLimits, "no cgroups"), "failed", -1, true},
		{"rootful runtime is not waited for", -1, isolation.Unavailable(isolation.ReasonRootful, "rootful"), "failed", 1, false},
		{"network scope is not waited for", -1, isolation.ScopeUnenforceable("no proxy"), "failed", 1, false},
		{"plain infra error is not waited for", -1, errors.New("disk full"), "failed", 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastWaits(t, time.Second, 150*time.Millisecond)
			iso := &refusing{refuse: func(_, call int) error {
				if tc.refuseFor < 0 || call <= tc.refuseFor {
					return tc.err
				}
				return nil
			}}
			rs, spec := waitRun(t, iso)
			var out strings.Builder
			spec.Output = func(b []byte) { out.Write(b) }
			res := rs.runWaiting(context.Background(), spec)
			if string(res.Attempt.State) != tc.wantState {
				t.Fatalf("state %q (err %v), want %q", res.Attempt.State, res.Err, tc.wantState)
			}
			if n := int(iso.calls.Load()); (tc.wantCalls > 0 && n != tc.wantCalls) || (tc.wantCalls < 0 && n < 2) {
				t.Errorf("isolator called %d times, want %d", n, tc.wantCalls)
			}
			if got := strings.Contains(out.String(), "waiting for process isolation"); got != tc.wantNote {
				t.Errorf("wait note = %v, want %v (output %q)", got, tc.wantNote, out.String())
			}
		})
	}
}

// Five concurrent container attempts on a rootless runtime that maps four slots: the fifth waits for a slot and runs.
func TestRunWaitingTakesAFreedSlot(t *testing.T) {
	fastWaits(t, 5*time.Second, time.Second)
	const mapped = 4
	iso := &refusing{refuse: func(slot, _ int) error {
		if slot >= mapped {
			return isolation.Unavailable(isolation.ReasonSlotUID, "slot %d is above the %d the runtime maps", slot, mapped)
		}
		return nil
	}}
	rs, spec := waitRun(t, iso)
	held := make([]int, mapped)
	for i := range held {
		held[i] = rs.sess.slots.take(slotDir(rs.sess.Home))
	}
	time.AfterFunc(80*time.Millisecond, func() { rs.sess.slots.put(held[2]) })
	res := rs.runWaiting(context.Background(), spec)
	if string(res.Attempt.State) != "passed" || res.Err != nil {
		t.Fatalf("fifth attempt: %+v err %v", res.Attempt, res.Err)
	}
	if n := iso.calls.Load(); n < 2 {
		t.Errorf("isolator called %d times: the attempt never had to wait", n)
	}
	if again := rs.sess.slots.take(slotDir(rs.sess.Home)); again != held[2] {
		t.Errorf("the attempt kept its slot after finishing: next free slot %d, want %d", again, held[2])
	}
}

// A cancel ends the wait at once and the attempt is cancelled, not failed.
func TestRunWaitingStopsOnCancel(t *testing.T) {
	fastWaits(t, time.Minute, time.Minute)
	iso := &refusing{refuse: func(_, _ int) error { return isolation.Unavailable(isolation.ReasonSlotUID, "no slot") }}
	rs, spec := waitRun(t, iso)
	rs.sess.slots.take(slotDir(rs.sess.Home)) // slot 0 is held elsewhere, so the attempt waits on slot 1
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	res := rs.runWaiting(ctx, spec)
	if string(res.Attempt.State) != "cancelled" || time.Since(start) > 5*time.Second {
		t.Fatalf("state %q after %s", res.Attempt.State, time.Since(start))
	}
}

// A refusal for the lowest slot cannot be cured by another attempt ending, so it is not waited for.
func TestRunWaitingSlotZeroFailsAtOnce(t *testing.T) {
	fastWaits(t, time.Minute, time.Minute)
	iso := &refusing{refuse: func(_, _ int) error { return isolation.Unavailable(isolation.ReasonSlotUID, "runtime maps no job UID") }}
	rs, spec := waitRun(t, iso)
	res := rs.runWaiting(context.Background(), spec)
	if string(res.Attempt.State) != "failed" || iso.calls.Load() != 1 {
		t.Fatalf("state %q after %d calls", res.Attempt.State, iso.calls.Load())
	}
}

// planRefusing is an Isolator that accepts and sets a planner whose Plan raises E678.
type planRefusing struct{ calls atomic.Int32 }

type e678Planner struct{ calls *atomic.Int32 }

func (p e678Planner) Plan(ciexec.ContainerRun) (ciexec.ContainerCommand, error) {
	p.calls.Add(1)
	return ciexec.ContainerCommand{}, isolation.Unavailable(isolation.ReasonRuntimeMissing, "runtime gone")
}
func (e678Planner) Cleanup(ciexec.ContainerRun) (*ciexec.ContainerCommand, error) { return nil, nil }

func (p *planRefusing) Prepare(_ context.Context, s *ciexec.JobSpec) error {
	s.Plan = e678Planner{calls: &p.calls}
	return nil
}

// An E678 that arrives after the attempt started (a planner's) is not waited for: its hooks and secrets already ran.
func TestRunWaitingDoesNotRepeatAnAttemptThatStarted(t *testing.T) {
	fastWaits(t, time.Minute, time.Minute)
	iso := &planRefusing{}
	rs, spec := waitRun(t, iso)
	spec.Job = model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}
	res := rs.runWaiting(context.Background(), spec)
	if string(res.Attempt.State) != "failed" || iso.calls.Load() != 1 {
		t.Fatalf("state %q (err %v) after %d plans: the attempt was run again", res.Attempt.State, res.Err, iso.calls.Load())
	}
}
