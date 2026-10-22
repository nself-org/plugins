package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// A tree that changes while the jobs run is not bound to the start commit.
func TestTreeChangedDuringRunNeverPostsSuccess(t *testing.T) {
	t.Parallel()
	n := 0
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{calls: &n, dirtyAfter: 2})
	res := r.run(t)
	if res.Exit != 0 || len(r.Rep.all()) != 0 || !strings.Contains(res.Summary.Status, "status not posted: worktree is dirty") {
		t.Fatalf("status %q posted %+v", res.Summary.Status, r.Rep.all())
	}
}

func TestStatusNotPostedForPartialCancelledOrOutsideConfig(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		set  func(*rig)
		want string
	}{
		"job":     {func(r *rig) { r.C.Opts.Job = "a" }, report.ReasonPartial},
		"outside": {func(r *rig) { r.C.ConfigOutside = true }, "worktree is dirty"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
			c.set(r)
			res := r.run(t)
			if len(r.Rep.all()) != 0 || !strings.Contains(res.Summary.Status, c.want) {
				t.Fatalf("status %q posted %+v", res.Summary.Status, r.Rep.all())
			}
		})
	}
	// Ctrl-C after admission: cancellation never posts a status. Synchronize on attempt
	// startup so a loaded test host cannot cancel during coordinator initialization.
	r := newRig(t, map[string]model.Job{"a": sh("sleep 60")}, gitFake{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.C.Hooks.AttemptStarts = []AttemptStartHook{startFn(func(AttemptInfo) error { cancel(); return nil })}
	res, err := r.C.Run(ctx)
	if err != nil || res.Exit == 0 || len(r.Rep.all()) != 0 || !strings.Contains(res.Summary.Status, report.ReasonCancelled) {
		t.Fatalf("exit %d err %v status %q posted %+v", res.Exit, err, res.Summary.Status, r.Rep.all())
	}
}

type panicPublisher struct{}

func (panicPublisher) Publish(context.Context, PublishRequest) (report.Status, error) {
	panic("boom")
}

// A panicking hook does not kill the coordinator; it posts nothing.
func TestPanickingPublisherPostsNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.Publisher = panicPublisher{}
	res := r.run(t)
	if p := r.Rep.all(); len(p) != 0 {
		t.Fatalf("posted %+v", p)
	}
	if !strings.Contains(strings.Join(res.Summary.Notes, "|"), "publisher failed") {
		t.Fatalf("no note: %v", res.Summary.Notes)
	}
}

func TestDispatchHookMisuse(t *testing.T) {
	t.Cleanup(func() { Dispatch = nil })
	Dispatch = func(ctx context.Context, s AttemptSpec) (Admitted, error) {
		adm, err := LocalAdmit(ctx, s)
		adm.AttemptID = "someone-elses.a1"
		return adm, err
	}
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	res := r.run(t)
	if o := outcome(res, "a"); o.State != StateFailed || o.Code != CodeOptions {
		t.Fatalf("a: %+v", o)
	}
	Dispatch = func(ctx context.Context, s AttemptSpec) (Admitted, error) {
		adm, err := LocalAdmit(ctx, s)
		adm.Local = false
		return adm, err
	}
	r = newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	res = r.run(t)
	if o := outcome(res, "a"); o.State != StateFailed {
		t.Fatalf("remote placement: %+v", o)
	}
	st := r.store(t)
	page, _ := st.History(context.Background(), 0, 5)
	at, _ := st.PipelineAttempts(context.Background(), page.Pipelines[0].ID)
	if len(at) != 1 || at[0].State != "cancelled" {
		t.Fatalf("the leased attempt of a remote placement must be cancelled: %+v", at)
	}
	Dispatch = func(context.Context, AttemptSpec) (Admitted, error) { panic("dispatch exploded") }
	r = newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	if res = r.run(t); outcome(res, "a").State != StateFailed {
		t.Fatalf("panicking dispatch: %+v", res.Summary.Jobs)
	}
}

func TestRowIDsNeverCollide(t *testing.T) {
	if rowID("r1", "test@linux") == rowID("r1", "test-linux") {
		t.Fatal("two job ids that sanitise alike share a row id")
	}
	if got := rowID("r1", strings.Repeat("x", 300)); len(got) > 100 {
		t.Fatalf("row id too long: %d", len(got))
	}
}

// Queued attempts and the pipeline row of a dead coordinator do not stay
// stuck.
func TestRecoveryEndsStrandedQueuedAttempts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	dead, err := st.Register(ctx, 2147483000, strconv.Itoa(1), bootID())
	if err != nil {
		t.Fatal(err)
	}
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(st.CreatePipeline(ctx, store.PipelineRow{ID: "p1", Project: "x", Status: "running"}))
	must(st.CreateJob(ctx, store.JobRow{ID: "p1.j", PipelineID: "p1", Name: "j", Kind: "test", Idempotent: true, Required: true}))
	must(st.CreateAttempt(ctx, store.AttemptRow{ID: "p1.j.a1", JobID: "p1.j", CoordinatorID: dead.ID, N: 1}, ""))
	rep := Recover(ctx, st, "", home)
	if len(rep.Errors) != 0 || rep.Stranded != 1 {
		t.Fatalf("report %+v", rep)
	}
	if state, _, _ := st.AttemptStateEpoch(ctx, "p1.j.a1"); state != "cancelled" {
		t.Fatalf("attempt state %s", state)
	}
	if row, _, _ := st.PipelineByID(ctx, "p1"); row.Status != "lost" {
		t.Fatalf("pipeline status %s", row.Status)
	}
	_ = errors.New
	_ = os.Getpid
}
