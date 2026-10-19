package dispatch

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type who struct{ pipe, proj, name, runner, provider, user, team string }

// occupy admits an attempt with the given identity on its own host, so only the
// counters (never the host reservation) see it.
func occupy(t *testing.T, st *store.Store, o who) string {
	t.Helper()
	id := queueAttempt(t, st, o.pipe, o.proj, o.name)
	r := testRunner(o.runner, "lan", 8)
	r.Provider = o.provider
	job := testJob(id)
	job.User, job.Team = o.user, o.team
	if _, err := Admit(context.Background(), st, Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: o.runner, CPU: 1, MemMB: 1}, Runner: r, Job: job, Local: true, Host: "h-" + id, CapacityCPU: 4, CapacityMemMB: 4096}); err != nil {
		t.Fatalf("occupant %+v: %v", o, err)
	}
	return id
}

func probe(t *testing.T, st *store.Store, limits sched.Limits) (Admitted, error, string) {
	t.Helper()
	id := queueAttempt(t, st, "P", "J", "N")
	a, err := tryProbe(st, id, limits)
	return a, err, id
}

// tryProbe admits the queued probe attempt id under limits.
func tryProbe(st *store.Store, id string, limits sched.Limits) (Admitted, error) {
	r := testRunner("R", "lan", 8)
	r.Provider = "V"
	job := testJob(id)
	job.User, job.Team = "U", "T"
	return Admit(context.Background(), st, Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: "R", CPU: 1, MemMB: 1}, Runner: r, Job: job, Local: true, Host: "h-" + id, CapacityCPU: 4, CapacityMemMB: 4096, Limits: limits})
}

// TestDispatchLimits: each of the eight levels refuses with limit.<level>, the
// refusal is stored for `ci explain`, and the same job passes under a higher limit.
func TestDispatchLimits(t *testing.T) {
	cases := []struct {
		level  model.PlacementReason
		set    func(*sched.Limits, int)
		occupy who
	}{
		{model.LimitPipeline, func(l *sched.Limits, n int) { l.Pipeline = n }, who{"P", "o", "x", "r2", "v2", "u2", "t2"}},
		{model.LimitJob, func(l *sched.Limits, n int) { l.Job = n }, who{"P", "o", "N", "r2", "v2", "u2", "t2"}},
		{model.LimitRunner, func(l *sched.Limits, n int) { l.Runner = n }, who{"p2", "o", "x", "R", "v2", "u2", "t2"}},
		{model.LimitProvider, func(l *sched.Limits, n int) { l.Provider = n }, who{"p2", "o", "x", "r2", "V", "u2", "t2"}},
		{model.LimitProject, func(l *sched.Limits, n int) { l.Project = n }, who{"p2", "J", "x", "r2", "v2", "u2", "t2"}},
		{model.LimitUser, func(l *sched.Limits, n int) { l.User = n }, who{"p2", "o", "x", "r2", "v2", "U", "t2"}},
		{model.LimitTeam, func(l *sched.Limits, n int) { l.Team = n }, who{"p2", "o", "x", "r2", "v2", "u2", "T"}},
		{model.LimitGlobal, func(l *sched.Limits, n int) { l.Global = n }, who{"p2", "o", "x", "r2", "v2", "u2", "t2"}},
	}
	for _, c := range cases {
		t.Run(string(c.level), func(t *testing.T) {
			st := openStore(t)
			occupy(t, st, c.occupy)
			var limits sched.Limits
			c.set(&limits, 1)
			_, err, id := probe(t, st, limits)
			var ref *Refusal
			if !errors.As(err, &ref) || ref.Reason != c.level {
				t.Fatalf("limit 1 with one active: %v, want %s", err, c.level)
			}
			if reason, _, ok, e := st.Refusal(context.Background(), id); e != nil || !ok || reason != string(c.level) {
				t.Fatalf("stored refusal = %q %v %v", reason, ok, e)
			}
			if state, _, _ := st.AttemptStateEpoch(context.Background(), id); state != "queued" {
				t.Fatalf("refused attempt is %s, want queued", state)
			}
			c.set(&limits, 2)
			if _, err = tryProbe(st, id, limits); err != nil {
				t.Fatalf("the same attempt under limit 2: %v", err)
			}
			if _, _, ok, _ := st.Refusal(context.Background(), id); ok {
				t.Fatal("admission kept the refusal")
			}
		})
	}
}

// TestDispatchLimitsRaised: a limit above the active count admits.
func TestDispatchLimitsRaised(t *testing.T) {
	st := openStore(t)
	occupy(t, st, who{"P", "J", "N", "R", "V", "U", "T"})
	id := queueAttempt(t, st, "P", "J", "M")
	r := testRunner("R", "lan", 8)
	r.Provider = "V"
	job := testJob(id)
	job.User, job.Team = "U", "T"
	all := sched.Limits{Pipeline: 2, Job: 2, Runner: 2, Provider: 2, Project: 2, User: 2, Team: 2, Global: 2}
	if _, err := Admit(context.Background(), st, Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: "R", CPU: 1, MemMB: 1}, Runner: r, Job: job, Local: true, Host: "h", CapacityCPU: 4, CapacityMemMB: 4096, Limits: all}); err != nil {
		t.Fatalf("one below every limit must admit: %v", err)
	}
}

// TestDispatchLimitsAllLevels: every full level is named, the first is the reason.
func TestDispatchLimitsAllLevels(t *testing.T) {
	st := openStore(t)
	occupy(t, st, who{"P", "J", "N", "R", "V", "U", "T"})
	_, err, _ := probe(t, st, sched.Limits{Pipeline: 1, Project: 1, Global: 1})
	var ref *Refusal
	if !errors.As(err, &ref) || ref.Reason != model.LimitPipeline {
		t.Fatalf("first full level: %v", err)
	}
	for _, level := range []string{"limit.pipeline", "limit.project", "limit.global"} {
		if !strings.Contains(ref.Detail, level) {
			t.Fatalf("detail %q misses %s", ref.Detail, level)
		}
	}
}

// TestDispatchForegroundFloor: one foreground job always gets an idle local runner.
func TestDispatchForegroundFloor(t *testing.T) {
	st := openStore(t)
	occupy(t, st, who{"p2", "o", "x", "r2", "v2", "u2", "t2"})
	admit := func(name string, foreground bool) error {
		id := queueAttempt(t, st, "P", "J", name)
		_, err := Admit(context.Background(), st, Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: "local", CPU: 1, MemMB: 1}, Runner: testRunner("local", "local", 4), Job: testJob(id), Local: true,
			Host: "h-" + id, CapacityCPU: 4, CapacityMemMB: 4096, Limits: sched.Limits{Global: 1}, Foreground: foreground})
		return err
	}
	var ref *Refusal
	if err := admit("background", false); !errors.As(err, &ref) || ref.Reason != model.LimitGlobal {
		t.Fatalf("background job at the global limit: %v", err)
	}
	if err := admit("first", true); err != nil {
		t.Fatalf("foreground floor: %v", err)
	}
	if err := admit("second", true); !errors.As(err, &ref) {
		t.Fatalf("the floor is one job, not a bypass: %v", err)
	}
}

// withSteps reopens registration with only `limits` plus the given steps.
func withSteps(t *testing.T, extra map[string]Step) {
	t.Helper()
	resetSteps("limits")
	for name, s := range extra {
		if err := RegisterStep(name, s); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { resetSteps("limits") })
}

// TestDispatchStepOrder: steps run in name order inside one admission, a
// refusing step rolls everything back, and registration closes at the first Admit.
func TestDispatchStepOrder(t *testing.T) {
	var mu sync.Mutex
	var order []string
	rec := func(name string, err error) Step {
		return func(context.Context, View) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, name)
			return err
		}
	}
	withSteps(t, map[string]Step{"release": rec("release", nil), "a-first": rec("a-first", nil), "z-last": rec("z-last", nil)})
	st := openStore(t)
	occupy(t, st, who{"P", "J", "N", "R", "V", "U", "T"})
	mu.Lock()
	want := []string{"a-first", "release", "z-last"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("step order %v, want %v", order, want)
	}
	order = nil
	mu.Unlock()
	if err := RegisterStep("late", rec("late", nil)); err == nil {
		t.Fatal("registration after the first Admit must fail")
	}
	if err := RegisterStep("", rec("x", nil)); err == nil {
		t.Fatal("empty step name accepted")
	}
	if err := RegisterStep("release", rec("x", nil)); err == nil {
		t.Fatal("duplicate step accepted")
	}

	withSteps(t, map[string]Step{"release": rec("release", &Refusal{Reason: model.BudgetExceeded, Detail: "fake"})})
	id := queueAttempt(t, st, "P2", "J", "N2")
	_, err := Admit(context.Background(), st, Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: "R", CPU: 1, MemMB: 1}, Runner: testRunner("R", "lan", 8), Job: testJob(id), Local: true, Host: "h2", CapacityCPU: 4, CapacityMemMB: 4096})
	var ref *Refusal
	if !errors.As(err, &ref) || ref.Reason != model.BudgetExceeded {
		t.Fatalf("refusing step: %v", err)
	}
	snap, e := st.WorldSnapshot(context.Background())
	if e != nil || len(snap.Active) != 1 || snap.Reservations["R"].Slots != 1 {
		t.Fatalf("refused admission left state behind: %+v %v", snap, e)
	}
	if state, _, _ := st.AttemptStateEpoch(context.Background(), id); state != "queued" {
		t.Fatalf("refused attempt is %s", state)
	}
}

// TestDispatchCapacityRefusal: a full host reservation is capacity.busy, not an error.
func TestDispatchCapacityRefusal(t *testing.T) {
	st := openStore(t)
	for i, name := range []string{"one", "two"} {
		id := queueAttempt(t, st, "P", "J", name)
		_, err := Admit(context.Background(), st, Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: "local", CPU: 1, MemMB: 1}, Runner: testRunner("local", "local", 1), Job: testJob(id), Local: true, Host: "same", CapacityCPU: 1, CapacityMemMB: 4096})
		var ref *Refusal
		if i == 0 && err != nil || i == 1 && (!errors.As(err, &ref) || ref.Reason != model.CapacityBusy) {
			t.Fatalf("admission %d: %v", i, err)
		}
	}
}

// TestDispatchStateConflict: an attempt that is no longer queued is a conflict.
func TestDispatchStateConflict(t *testing.T) {
	st := openStore(t)
	id := queueAttempt(t, st, "P", "J", "N")
	req := Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: "R", CPU: 1, MemMB: 1}, Runner: testRunner("R", "lan", 8), Job: testJob(id), Local: true, Host: "h", CapacityCPU: 4, CapacityMemMB: 4096}
	if _, err := Admit(context.Background(), st, req); err != nil {
		t.Fatal(err)
	}
	if _, err := Admit(context.Background(), st, req); !errors.Is(err, ErrConflict) {
		t.Fatalf("second admission: %v", err)
	}
	if _, err := Admit(context.Background(), nil, req); err == nil {
		t.Fatal("nil store accepted")
	}
}
