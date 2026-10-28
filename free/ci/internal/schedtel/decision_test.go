package schedtel

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func (e *env) source() Source {
	return Source{Open: func(context.Context) (*store.Store, func(), error) { return e.st, func() {}, nil }}
}

func TestDecisionRecordKeepsTheDispatchersAlternatives(t *testing.T) {
	e := newEnv(t)
	sink := NewSink(e.st, e.ms)
	run, attempt := e.runOnce(sink, 0, 1000)
	want := decision(run, attempt, run+".build", "local", dispatch.OutcomeAdmitted).Explanation.Alternatives
	planview.Register(e.source())
	t.Cleanup(func() { planview.Register(nil) })
	for _, job := range []string{"build", run + ".build"} { // by name and by job id
		doc, err := planview.Lookup(context.Background(), run, job)
		if err != nil {
			t.Fatal(err)
		}
		if len(doc.Jobs) != 1 || doc.Jobs[0].Status != "placed" || doc.Jobs[0].Name != "build" || doc.Jobs[0].Placement == nil || doc.Jobs[0].Placement.RunnerID != "local" {
			t.Fatalf("%s: %+v", job, doc.Jobs)
		}
		if mustJSON(t, doc.Jobs[0].Alternatives) != mustJSON(t, want) {
			t.Fatalf("alternatives differ:\n got %s\nwant %s", mustJSON(t, doc.Jobs[0].Alternatives), mustJSON(t, want))
		}
		if doc.Pipeline.WorldDigest != "digest-"+run || doc.Pipeline.Revision != "rev" || doc.Pipeline.PolicyDigest != "pol" {
			t.Fatalf("pipeline: %+v", doc.Pipeline)
		}
	}
	lines := strings.Join(planview.ExplainLines(mustLookup(t, run, "build"), planview.SourceDecisionRecord), "\n")
	if !strings.Contains(lines, "decision-record") || !strings.Contains(lines, "chosen  local") {
		t.Fatalf("explain lines:\n%s", lines)
	}
}

func mustLookup(t *testing.T, run, job string) model.PlacementPlan {
	t.Helper()
	doc, err := planview.Lookup(context.Background(), run, job)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestDecisionWaitingThenAdmittedIsOneRecord(t *testing.T) {
	e := newEnv(t)
	sink := NewSink(e.st, e.ms)
	ctx := context.Background()
	attempt := e.attempt("run-w", "lint")
	for i := 0; i < 3; i++ { // the same word every tick is stored once
		sink.Record(ctx, decision("run-w", attempt, "run-w.lint", "local", dispatch.OutcomeWaiting))
		e.advance(time.Second)
	}
	d, _, _ := e.st.TelDecision(ctx, attempt)
	if d.Outcome != dispatch.OutcomeWaiting || d.TS != time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("repeat waiting rewrote the record: %+v", d)
	}
	var doc model.PlacementPlan
	if err := json.Unmarshal(d.Doc, &doc); err != nil || doc.Jobs[0].Status != "waiting" || doc.Jobs[0].Reasons[0].Code != "capacity.busy" {
		t.Fatalf("waiting doc: %+v %v", doc, err)
	}
	sink.Record(ctx, decision("run-w", attempt, "run-w.lint", "local", dispatch.OutcomeAdmitted))
	d, _, _ = e.st.TelDecision(ctx, attempt)
	if d.Outcome != dispatch.OutcomeAdmitted || d.WaitMs != 3000 {
		t.Fatalf("admitted after 3 s of waiting: %+v", d)
	}
	if all, _ := e.st.TelDecisions(ctx, "run-w", 0); len(all) != 1 {
		t.Fatalf("one record per placed job: %d", len(all))
	}
}

func TestDecisionTimeoutHasNoWorld(t *testing.T) {
	e := newEnv(t)
	sink := NewSink(e.st, e.ms)
	attempt := e.attempt("run-t", "slow")
	d := dispatch.Decision{WorldDigest: "dg", Job: decision("run-t", attempt, "run-t.slow", "x", "").Job, Outcome: dispatch.OutcomeTimeout, Reason: model.QueueTimeout}
	sink.Record(context.Background(), d)
	doc, err := e.source().Decision(context.Background(), "run-t", "slow")
	if err != nil || doc.Jobs[0].Status != "unplaceable" || doc.Jobs[0].Reasons[0].Code != string(model.QueueTimeout) {
		t.Fatalf("%+v %v", doc.Jobs, err)
	}
}

func TestDecisionUnknownJobIsE692(t *testing.T) {
	e := newEnv(t)
	_, err := e.source().Decision(context.Background(), "nope", "build")
	var pe *planview.Error
	if err == nil || !asPlanviewError(err, &pe) || pe.Code != planview.CodeExplainTarget || !strings.Contains(pe.Message, "nself ci runs decisions") {
		t.Fatalf("%v", err)
	}
}

func TestDecisionUnknownAttemptIsReportedByFinalizer(t *testing.T) {
	e := newEnv(t)
	sink := NewSink(e.st, e.ms)
	for i := 0; i < 50; i++ { // a failing store fails on every tick
		sink.Record(context.Background(), decision("run-x", "ghost.a1", "ghost", "local", dispatch.OutcomeAdmitted))
	}
	err := (finalizer{}).Finalize(context.Background(), cirun.RunInfo{RunID: "run-x"})
	if err == nil || !strings.Contains(err.Error(), "ghost.a1") || !strings.Contains(err.Error(), "45 more failures") {
		t.Fatalf("a failure before any write reaches the run, bounded: %v", err)
	}
	if sinkFor("run-x") != nil {
		t.Fatal("finalizer left the binding")
	}
}

// The first world of a run is built before its first decision: the sink the
// factory returns is already the contributor's store.
func TestTelemetryFirstWorldSeesTheHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		o := store.TelOutcome{AttemptID: "old" + string(rune('a'+i)), Project: "proj", JobKey: "build", Runner: "local", RunnerClass: "local", Result: "passed", RuntimeMs: 4000, TS: int64(i)}
		if err := e.st.TelRecordOutcome(ctx, o, fold(4000, -1, int64(i))); err != nil {
			t.Fatal(err)
		}
	}
	q := e.attempt("fresh", "build")
	bindLatest(NewSink(e.st, e.ms)) // what the dispatcher's sink factory does at New
	t.Cleanup(func() { bindLatest(nil) })
	w := sched.World{Runners: []sched.Runner{{ID: "local", Class: "local", Avail: sched.Avail{Slots: 1}}}}
	if err := contribute(context.Background(), &w); err != nil {
		t.Fatal(err)
	}
	if got := ptrVal(w.Estimates["local"][q].RuntimeMs); got != int64(4000) {
		t.Fatalf("first tick estimate %v, want 4000", got)
	}
}

func TestDecisionWaitStartsAtEnqueue(t *testing.T) {
	e := newEnv(t)
	sink := NewSink(e.st, e.ms)
	attempt := e.attempt("run-q", "slow")
	d := decision("run-q", attempt, "run-q.slow", "local", dispatch.OutcomeAdmitted)
	d.EnqueuedAt = e.now.Add(-7 * time.Second)
	sink.Record(context.Background(), d)
	if got, _, _ := e.st.TelDecision(context.Background(), attempt); got.WaitMs != 7000 {
		t.Fatalf("wait %d, want 7000 from the enqueue time", got.WaitMs)
	}
}

func TestPlacementRefFollowsTheAdmittedDecision(t *testing.T) {
	e := newEnv(t)
	sink := NewSink(e.st, e.ms)
	ctx := context.Background()
	waiting := e.attempt("run-p", "wait")
	sink.Record(ctx, decision("run-p", waiting, "run-p.wait", "local", dispatch.OutcomeWaiting))
	if _, ok := cirun.PlacementRef(ctx, waiting); ok {
		t.Fatal("a waiting attempt has no placement yet")
	}
	run, attempt := e.runOnce(sink, 1, 1000)
	ref, ok := cirun.PlacementRef(ctx, attempt)
	if !ok || ref != "sched:decision/"+attempt {
		t.Fatalf("ref %q %v", ref, ok)
	}
	if d, found, _ := e.st.TelDecision(ctx, strings.TrimPrefix(ref, RefPrefix)); !found || d.Run != run {
		t.Fatalf("the ref resolves to its decision: %+v", d)
	}
	finalize(t, run, attempt, 1000)
	if _, ok = cirun.PlacementRef(ctx, attempt); ok {
		t.Fatal("the finalizer forgets the ref once evidence is built")
	}
	if _, ok = cirun.PlacementRef(ctx, "never-placed"); ok {
		t.Fatal("no decision, no ref")
	}
}

// D-1069: the contributor samples the queued attempts of the pipeline being built and no other, so an
// unrelated pipeline queueing a job changes neither the estimates nor the world digest.
func TestEstimatesCoverOnlyThePipelineBeingBuilt(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 3; i++ {
		o := store.TelOutcome{AttemptID: "old" + string(rune('a'+i)), Project: "proj", JobKey: "build", Runner: "local", RunnerClass: "local", Result: "passed", RuntimeMs: 4000, TS: int64(i)}
		if err := e.st.TelRecordOutcome(context.Background(), o, fold(4000, -1, int64(i))); err != nil {
			t.Fatal(err)
		}
	}
	mine := e.attempt("mine", "build")
	build := func(ctx context.Context) sched.World {
		t.Helper()
		w := sched.World{Runners: []sched.Runner{{ID: "local", Class: "local", Avail: sched.Avail{Slots: 1}}}}
		if err := contribute(WithStore(ctx, e.st), &w); err != nil {
			t.Fatal(err)
		}
		return w
	}
	forMine := world.WithPipeline(context.Background(), sched.Pipeline{ID: "mine"})
	before := build(forMine)
	other := e.attempt("other", "build")
	after := build(forMine)

	if _, ok := after.Estimates["local"][other]; ok {
		t.Fatal("the estimates sample an attempt of another pipeline")
	}
	if !reflect.DeepEqual(before.Estimates, after.Estimates) || ptrVal(after.Estimates["local"][mine].RuntimeMs) != int64(4000) {
		t.Fatalf("an unrelated pipeline changed the estimates: before %+v after %+v", before.Estimates, after.Estimates)
	}
	// without a pipeline in the context the contributor still samples every queued attempt
	if all := build(context.Background()); len(all.Estimates["local"]) != 2 {
		t.Fatalf("no pipeline: %d samples, want 2", len(all.Estimates["local"]))
	}
}
