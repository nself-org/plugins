package main

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// countingFacts is a FactStore that counts reads, to see whether a finalizer looks at the ledger at all.
type countingFacts struct{ reads atomic.Int32 }

func (*countingFacts) ProviderFactPut(context.Context, store.ProviderFact) (bool, error) {
	return true, nil
}
func (c *countingFacts) ProviderFactGet(context.Context, string, string, string) (store.ProviderFact, bool, error) {
	c.reads.Add(1)
	return store.ProviderFact{}, false, nil
}
func (c *countingFacts) ProviderFactList(context.Context, string, string, string) ([]store.ProviderFact, error) {
	c.reads.Add(1)
	return nil, nil
}

// TestRunnerFinalizerSkipsOrdinaryRuns: every run is finalized, but only a provider-runner pipeline has
// registrations; an ordinary pipeline costs the ledger nothing.
func TestRunnerFinalizerSkipsOrdinaryRuns(t *testing.T) {
	facts := &countingFacts{}
	runnerjob.Use(runnerjob.NewLedger(facts, nil))
	t.Cleanup(func() { runnerjob.Use(nil) })
	var fin cirun.Finalizer
	for _, f := range cirun.Registered.Finalizers {
		if f.Name() == "runnerjob" {
			fin = f
		}
	}
	jobs := []cirun.JobOutcome{{AttemptID: "a1"}}
	var ordinary cirun.RunInfo
	ordinary.Jobs, ordinary.Evidence.Pipeline = jobs, model.PipelineRef{Name: "default", Kind: "default"}
	if err := fin.Finalize(context.Background(), ordinary); err != nil || facts.reads.Load() != 0 {
		t.Fatalf("an ordinary run read the ledger %d times (err %v)", facts.reads.Load(), err)
	}
	var runner cirun.RunInfo
	runner.Jobs, runner.Evidence.Pipeline = jobs, model.PipelineRef{Name: runnerjob.PipelineName, Kind: "adhoc"}
	if err := fin.Finalize(context.Background(), runner); err != nil || facts.reads.Load() == 0 {
		t.Fatalf("a provider-runner run must be looked up (reads %d, err %v)", facts.reads.Load(), err)
	}
}

// TestRunnerFinalizerIsRegisteredAndReadsEveryAttempt: a closed run reaches runnerjob.Finalize through the
// run finalizer registered here; with no ledger bound it is a no-op that succeeds.
func TestRunnerFinalizerIsRegisteredAndReadsEveryAttempt(t *testing.T) {
	var found cirun.Finalizer
	for _, f := range cirun.Registered.Finalizers {
		if f.Name() == "runnerjob" {
			found = f
		}
	}
	if found == nil {
		t.Fatal("the runnerjob finalizer is not registered")
	}
	info := cirun.RunInfo{RunID: "r", Jobs: []cirun.JobOutcome{{AttemptID: "a1"}, {AttemptID: ""}, {AttemptID: "a2"}}}
	if err := found.Finalize(context.Background(), info); err != nil {
		t.Fatalf("an unbound launcher must not fail a run: %v", err)
	}
}
