package schedtel

// Fixtures: a store with an injected clock, runs with one queued attempt each,
// and dispatcher decisions for them. No sleeps, no network, no fixed ports.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// env is one test store whose clock the test moves.
type env struct {
	t   *testing.T
	st  *store.Store
	now time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	st, err := store.Open(t.TempDir()+"/state.db", store.Options{Clock: func() time.Time { return e.now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e.st = st
	return e
}

func (e *env) ms() int64 { return e.now.UnixMilli() }

func (e *env) advance(d time.Duration) { e.now = e.now.Add(d) }

// attempt queues attempt "<run>.a1" of job name in run, and returns its id.
func (e *env) attempt(run, name string) string {
	e.t.Helper()
	ctx := context.Background()
	if _, ok, _ := e.st.PipelineByID(ctx, run); !ok {
		p := store.PipelineRow{ID: run, Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "pol", InputDigest: "i", SelectionMode: "full", Status: "queued"}
		if err := e.st.CreatePipeline(ctx, p); err != nil {
			e.t.Fatal(err)
		}
	}
	row := run + "." + name
	if err := e.st.CreateJob(ctx, store.JobRow{ID: row, PipelineID: run, Name: name, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		e.t.Fatal(err)
	}
	id := row + ".a1"
	if err := e.st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: row, N: 1}, ""); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// finish moves the attempt to passed (or the given terminal state) at the current
// clock; running is the hook called once it is running (the job's own runtime).
func (e *env) finish(id string, to model.JobState, running func()) {
	e.t.Helper()
	ctx := context.Background()
	steps := []model.JobState{"leased", "running", "finalizing"}
	from := model.JobState("queued")
	if to == "failed" {
		steps = []model.JobState{"leased", "running"}
	}
	for epoch, next := range append(steps, to) { // every transition bumps the attempt epoch
		if err := e.st.Transition(ctx, id, from, next, store.TransitionDetail{Epoch: int64(epoch)}); err != nil {
			e.t.Fatal(err)
		}
		from = next
		if next == "running" && running != nil {
			running()
		}
	}
}

// decision is the dispatcher's word on the attempt: placed on runner (or waiting).
func decision(run, attempt, job, runner, outcome string) dispatch.Decision {
	sj := sched.Job{Key: attempt, ID: job, Name: job, Pipeline: sched.Pipeline{ID: run}, Project: "proj",
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner"}
	score := int64(42)
	alts := []model.Alternative{{RunnerID: runner, Provider: "operator", Eligible: true, Reasons: []model.PlacementReasonDetail{}, Score: &score,
		Components: map[string]model.Component{"runtime": {ValueMs: 1000, WeightPermille: 500, Confidence: "unknown"}, "wait": {ValueMs: 0, WeightPermille: 500, Confidence: "known"}}},
		{RunnerID: "lan-1", Provider: "operator", Eligible: false, Reasons: []model.PlacementReasonDetail{{Code: "runner.offline", Detail: "not online"}}}}
	d := dispatch.Decision{WorldDigest: "digest-" + run, Job: sj, Outcome: outcome}
	w := sched.World{Policy: sched.Policy{Preset: "balanced"}, Runners: []sched.Runner{{ID: runner, Class: "local", Location: "local", Provider: "operator"}}}
	d.World = &w
	if outcome == dispatch.OutcomeWaiting {
		d.Explanation = sched.Explanation{JobKey: attempt, Status: "waiting", Alternatives: alts,
			Reasons: []model.PlacementReasonDetail{{Code: "capacity.busy", Detail: "no free slot"}}}
		return d
	}
	d.Explanation = sched.Explanation{JobKey: attempt, Status: "placed", Alternatives: alts}
	d.Placement = &sched.Placement{JobKey: attempt, RunnerID: runner, CostMicro: 5}
	return d
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// runOnce plays one run of the fixture job: waiting, admitted 40 ms later, then
// a runtime of runtimeMs and 500 ms of overhead before the attempt ends.
func (e *env) runOnce(sink *Sink, i int, runtimeMs int64) (run, attempt string) {
	e.t.Helper()
	run = fmt.Sprintf("run-%d", i)
	attempt = e.attempt(run, "build")
	ctx := context.Background()
	sink.Record(ctx, decision(run, attempt, run+".build", "local", dispatch.OutcomeWaiting))
	e.advance(40 * time.Millisecond)
	sink.Record(ctx, decision(run, attempt, run+".build", "local", dispatch.OutcomeAdmitted))
	e.advance(500 * time.Millisecond) // lease to start
	e.finish(attempt, "passed", func() { e.advance(time.Duration(runtimeMs) * time.Millisecond) })
	return run, attempt
}
