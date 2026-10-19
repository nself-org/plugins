package background

// Admission-level tests against a real store: the step runs inside
// dispatch.Admit's transaction, so a refusal leaves the attempt queued.

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

var rowSeq atomic.Int64

// queueAttempt creates a pipeline (once), a job named name and its queued attempt.
func queueAttempt(t testing.TB, st *store.Store, name string) string {
	t.Helper()
	ctx := context.Background()
	err := st.CreatePipeline(ctx, store.PipelineRow{ID: "pipe", Revision: "rev", Project: "project", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"})
	if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatal(err)
	}
	id := name + "." + strconv.FormatInt(rowSeq.Add(1), 10)
	if err = st.CreateJob(ctx, store.JobRow{ID: "job-" + id, PipelineID: "pipe", Name: name, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "job-" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	return id
}

func admit(st *store.Store, id string, r sched.Runner, j sched.Job) (dispatch.Admitted, error) {
	return dispatch.Admit(context.Background(), st, dispatch.Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: r.ID, CPU: 1, MemMB: 1},
		Runner: r, Job: j, Local: true, Host: "host", CapacityCPU: 4, CapacityMemMB: 4096})
}

func state(t testing.TB, st *store.Store, id string) model.JobState {
	t.Helper()
	s, _, err := st.AttemptStateEpoch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// withProbe installs a queue probe for one test.
func withProbe(t *testing.T, p QueueProbe) {
	t.Helper()
	old := SetQueueProbe(p)
	t.Cleanup(func() { SetQueueProbe(old) })
}

// TestBackgroundAdmissionChokepoint: the registered step refuses through the
// real admission, stores the reason and leaves the attempt queued.
func TestBackgroundAdmissionChokepoint(t *testing.T) {
	st := openStore(t)
	battery := runner("r", signals{Plugged: ptr(false), Interactive: ptr("idle"), Operator: true})
	id := queueAttempt(t, st, "bg")
	_, err := admit(st, id, battery, job(id, BandName))
	var ref *dispatch.Refusal
	if !asRefusal(err, &ref) || ref.Reason != model.BackgroundContention {
		t.Fatalf("on battery: err = %v", err)
	}
	if reason, _, ok, _ := st.Refusal(context.Background(), id); !ok || reason != string(model.BackgroundContention) {
		t.Fatalf("stored reason = %q %v", reason, ok)
	}
	if s := state(t, st, id); s != "queued" {
		t.Fatalf("a refused job must stay queued, got %s", s)
	}
	if _, err = admit(st, id, runner("r", idleDesk), job(id, BandName)); err != nil {
		t.Fatalf("idle desk: %v", err)
	}
	if s := state(t, st, id); s != "leased" {
		t.Fatalf("state = %s", s)
	}
}

func asRefusal(err error, out **dispatch.Refusal) bool {
	r, ok := err.(*dispatch.Refusal)
	*out = r
	return ok
}

// TestInteractiveFirst: (1) a background flood never keeps an interactive job
// from the slot that frees (pure plan, the dispatcher's order and placement);
// (2) at the admission, a background job is refused while an interactive job is
// queued for the runner and runs once that job holds its lease. Nothing
// preempts: the running background attempt stays leased throughout.
func TestInteractiveFirst(t *testing.T) {
	st := openStore(t)
	clock := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	d, err := dispatch.New(dispatch.Config{Store: st, Host: "host", Clock: func() time.Time { return clock },
		World: func(context.Context, sched.Pipeline) (sched.World, string, error) { return sched.World{}, "d", nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)

	world := func(slots int) sched.World {
		r := runner("r", idleDesk)
		r.Avail.Slots = slots
		return sched.World{Policy: sched.Policy{Preset: "balanced"}, Runners: []sched.Runner{r}, Costs: map[string]sched.Cost{}, Previous: map[string]string{},
			Counts: sched.Counts{Runners: map[string]int{}, Providers: map[string]int{}}, InteractiveQueued: map[string]bool{"r": true}}
	}
	var queued []dispatch.Queued
	for i := 0; i < 50; i++ {
		key := "bg" + strconv.Itoa(i)
		queued = append(queued, dispatch.Queued{Job: dispatch.Job{AttemptID: key, Sched: job(key, BandName)}, EnqueuedAt: clock.Add(time.Duration(i) * time.Second), Seq: uint64(i + 1)})
	}
	fg := job("fg", "")
	fg.Pipeline.Band = "interactive"
	queued = append(queued, dispatch.Queued{Job: dispatch.Job{AttemptID: "fg", Sched: fg, Foreground: true}, EnqueuedAt: clock.Add(time.Hour), Seq: 51})

	if _, res := d.Plan(world(0), queued); len(res.Placements) != 0 {
		t.Fatalf("no free slot, yet placed %+v", res.Placements)
	}
	_, res := d.Plan(world(1), queued) // the slot frees
	if len(res.Placements) != 1 || res.Placements[0].JobKey != "fg" {
		t.Fatalf("the freed slot must go to the interactive job: %+v", res.Placements)
	}

	// admission re-check: a stale plan that offers the slot to a background job
	runningID := queueAttempt(t, st, "running-bg")
	desk := runner("r", idleDesk)
	if _, err = admit(st, runningID, desk, job(runningID, BandName)); err != nil {
		t.Fatalf("background job on an idle desk: %v", err)
	}
	waiting := queueAttempt(t, st, "bg2")
	fgID := queueAttempt(t, st, "fg")
	interactiveQueued := true
	withProbe(t, func(id string) bool { return id == "r" && interactiveQueued })
	_, err = admit(st, waiting, desk, job(waiting, BandName))
	var ref *dispatch.Refusal
	if !asRefusal(err, &ref) || ref.Reason != model.BackgroundInteractiveQueued {
		t.Fatalf("background while interactive queued: %v", err)
	}
	if _, err = admit(st, fgID, desk, fg); err != nil {
		t.Fatalf("interactive job refused: %v", err)
	}
	interactiveQueued = false
	if _, err = admit(st, waiting, desk, job(waiting, BandName)); err != nil {
		t.Fatalf("background after the interactive job started: %v", err)
	}
	if s := state(t, st, runningID); s != "leased" {
		t.Fatalf("the running background attempt must not be touched, got %s", s)
	}
}

// TestForegroundFloor: a node on battery refuses background work only. The
// operator's own foreground run still gets its slot, and so does a normal job
// (X17: the foreground floor is unaffected).
func TestForegroundFloor(t *testing.T) {
	st := openStore(t)
	withProbe(t, func(string) bool { return true }) // even with interactive work queued
	battery := runner("laptop", signals{Plugged: ptr(false), BatteryPct: ptr(41), Interactive: ptr("active"), Operator: true, Active: true})

	fg := job("fg", "interactive")
	fgID := queueAttempt(t, st, "fg")
	a, err := admit(st, fgID, battery, fg)
	if err != nil {
		t.Fatalf("foreground run on battery: %v", err)
	}
	if a.RunnerID != "laptop" || !a.Local {
		t.Fatalf("admitted = %+v", a)
	}
	normalID := queueAttempt(t, st, "normal")
	if _, err = admit(st, normalID, battery, job(normalID, "normal")); err != nil {
		t.Fatalf("normal job on battery: %v", err)
	}
	bgID := queueAttempt(t, st, "bg")
	if _, err = admit(st, bgID, battery, job(bgID, BandName)); err == nil {
		t.Fatal("a background job must not start on a battery laptop")
	}
}
