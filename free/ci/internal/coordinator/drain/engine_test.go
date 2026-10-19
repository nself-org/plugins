package drain

// Purpose: TestDrainEngineLeases: the drain over the real lease engine and store (P7-NODE-09): the
//          idempotent job is requeued after the ack, the release job and the idempotent:false job end in
//          needs-operator with no new attempt, and an expired lease is handed back through Sweep.
// Inputs:  a SQLite store in t.TempDir() with the fake clock. Outputs: pass/fail.
// Constraints: fake values only; one lease per job as in internal/lease's own fixture.

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type engineRig struct {
	c     *fakeClock
	eng   *lease.Engine
	st    *store.Store
	coord string
}

func newEngineRig(t *testing.T, ttl time.Duration) *engineRig {
	t.Helper()
	c := newClock()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := lease.New(st, nil, lease.Config{Clock: c.Now, TTL: ttl})
	co, err := st.RegisterCoordinator(context.Background(), 4242, "start-1", "boot-1")
	if err != nil {
		t.Fatal(err)
	}
	return &engineRig{c: c, eng: e, st: st, coord: co.ID}
}

// job creates a running attempt "a<id>" with lease "l<id>" of the given kind.
func (r *engineRig) job(t *testing.T, id, kind string, idempotent bool) {
	t.Helper()
	ctx := context.Background()
	p := store.PipelineRow{ID: "p" + id, Project: "p", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	if err := r.st.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := r.st.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: p.ID, Name: id, Kind: kind, Idempotent: idempotent, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := r.st.CreateAttempt(ctx, store.AttemptRow{ID: "a" + id, JobID: "j" + id, N: 1, CoordinatorID: r.coord}, ""); err != nil {
		t.Fatal(err)
	}
	d := store.Demand{AttemptID: "a" + id, LeaseID: "l" + id, RunnerID: "r" + id, CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: r.eng.Config.TTL}
	if _, err := r.eng.Grant(ctx, "host", d, 1); err != nil {
		t.Fatal(err)
	}
	if err := r.st.Transition(ctx, "a"+id, "leased", "running", store.TransitionDetail{Epoch: 1}); err != nil {
		t.Fatal(err)
	}
}

func (r *engineRig) state(t *testing.T, attempt string) string {
	t.Helper()
	s, _, err := r.st.AttemptStateEpoch(context.Background(), attempt)
	if err != nil {
		t.Fatalf("%s: %v", attempt, err)
	}
	return string(s)
}

type acker struct {
	d     **Drainer
	count int
}

func (a *acker) Cancel(_, leaseID, _ string) error {
	a.count++
	(*a.d).Ack(leaseID)
	return nil
}

func TestDrainEngineLeases(t *testing.T) {
	kinds := map[string]struct {
		kind string
		idem bool
	}{"aok": {"test", true}, "arel": {"release", true}, "aonce": {"test", false}}
	info := func(_ context.Context, attempt string) (string, bool, error) {
		k := kinds[attempt]
		return k.kind, k.idem, nil
	}

	t.Run("ack hands back the idempotent job; release and idempotent:false go to needs-operator", func(t *testing.T) {
		noLeak(t)
		r := newEngineRig(t, 12*time.Minute)
		r.job(t, "ok", "test", true)
		r.job(t, "rel", "release", true)
		r.job(t, "once", "test", false)
		var d *Drainer
		cancel := &acker{d: &d}
		leases := &EngineLeases{Engine: r.eng, CoordinatorID: r.coord, JobInfo: info}
		world := newWorld(r.c)
		var err error
		d, err = New(Config{Intake: world, Leases: leases, Canceller: cancel, Clock: r.c})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		var rep Report
		go func() { defer close(done); rep, err = d.Run(context.Background()) }()
		r.c.settleOn(t, 1, done)
		r.c.Advance(10 * time.Minute)
		<-done
		if err != nil || rep.ExitCode() != 0 {
			t.Fatalf("err %v exit %d results %+v", err, rep.ExitCode(), rep.Results)
		}
		if got := r.state(t, "aok"); got != "retryable" {
			t.Errorf("idempotent attempt is %s, want retryable", got)
		}
		if got := r.state(t, "aok-retry-2"); got != "queued" {
			t.Errorf("the requeued attempt is %s, want queued", got)
		}
		for _, a := range []string{"arel", "aonce"} {
			if got := r.state(t, a); got != "needs-operator" {
				t.Errorf("%s is %s, want needs-operator", a, got)
			}
			if _, _, err := r.st.AttemptStateEpoch(context.Background(), a+"-retry-2"); err == nil {
				t.Errorf("%s was requeued", a)
			}
		}
		if cancel.count != 1 {
			t.Errorf("%d cancels were sent, want 1 (only the idempotent job)", cancel.count)
		}
	})

	t.Run("expiry hands back through the engine sweep", func(t *testing.T) {
		noLeak(t)
		r := newEngineRig(t, 10*time.Minute+10*time.Second)
		r.job(t, "ok", "test", true)
		leases := &EngineLeases{Engine: r.eng, CoordinatorID: r.coord, JobInfo: info}
		world := newWorld(r.c)
		d, err := New(Config{Intake: world, Leases: leases, Canceller: world, Clock: r.c})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		var rep Report
		go func() { defer close(done); rep, err = d.Run(context.Background()) }()
		r.c.settleOn(t, 1, done)
		r.c.Advance(10 * time.Minute)
		r.c.run(t, 1, time.Second, done, func() {
			if got := r.state(t, "aok"); got != "running" && r.c.Now().Before(time.Unix(1_000_000, 0).Add(10*time.Minute+10*time.Second)) {
				t.Fatalf("attempt left running before its lease expired: %s", got)
			}
		})
		<-done
		if err != nil || outcome(t, rep, "lok") != OutcomeExpired || r.state(t, "aok") != "retryable" {
			t.Fatalf("err %v results %+v state %s", err, rep.Results, r.state(t, "aok"))
		}
	})

	t.Run("without JobInfo the drain refuses to guess", func(t *testing.T) {
		noLeak(t)
		r := newEngineRig(t, 12*time.Minute)
		r.job(t, "ok", "test", true)
		world := newWorld(r.c)
		d, _ := New(Config{Intake: world, Leases: &EngineLeases{Engine: r.eng, CoordinatorID: r.coord}, Canceller: world, Clock: r.c})
		if _, err := d.Run(context.Background()); err == nil {
			t.Fatal("a drain without JobInfo ran")
		}
		if got := r.state(t, "aok"); got != "running" {
			t.Fatalf("attempt is %s, want it untouched", got)
		}
	})

	t.Run("a job class that cannot be read is left to restart grace", func(t *testing.T) {
		noLeak(t)
		r := newEngineRig(t, 12*time.Minute)
		r.job(t, "ok", "test", true)
		world := newWorld(r.c)
		broken := func(context.Context, string) (string, bool, error) {
			return "", false, errors.New("job table unreadable")
		}
		d, _ := New(Config{Intake: world, Leases: &EngineLeases{Engine: r.eng, CoordinatorID: r.coord, JobInfo: broken}, Canceller: world, Clock: r.c})
		done := make(chan struct{})
		var rep Report
		go func() { defer close(done); rep, _ = d.Run(context.Background()) }()
		r.c.settleOn(t, 1, done)
		r.c.Advance(10 * time.Minute)
		<-done
		if outcome(t, rep, "lok") != OutcomeError || rep.ExitCode() != 1 || r.state(t, "aok") != "running" {
			t.Fatalf("results %+v state %s", rep.Results, r.state(t, "aok"))
		}
		world.never(t, "cancel lok node=rok")
	})
}
