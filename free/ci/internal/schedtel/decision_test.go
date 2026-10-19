package schedtel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
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
	sink.Record(context.Background(), decision("run-x", "ghost.a1", "ghost", "local", dispatch.OutcomeAdmitted))
	if err := sink.takeErr(); err == nil || !strings.Contains(err.Error(), "ghost.a1") {
		t.Fatalf("a decision for a missing attempt is an error, not a silent drop: %v", err)
	}
	if err := sink.takeErr(); err != nil {
		t.Fatalf("takeErr clears: %v", err)
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
