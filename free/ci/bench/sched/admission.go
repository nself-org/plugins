package sched

// Purpose: one timed scheduling decision for the schedbench harness (P7-SCHED-14):
// the same path the V15 dispatcher takes for one local job, world build, Place and
// the admission transaction, against a fresh on-disk store.
// Inputs:  a scratch directory.
// Outputs: the Place time and the Build+Place+Admit time of one decision, in ms.
// Constraints: the store and the queued attempt are set up untimed and the store
// is removed afterwards, so the snapshot cardinality is the same every iteration
// (it does not grow with the number of runs); nothing here reads the operator's state.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	isched "github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

const (
	hostCPU   = 8
	hostMemMB = 8192
)

// Decision is the timing of one scheduling decision.
type Decision struct{ PlaceMS, AdmitMS float64 }

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func job(attempt string) isched.Job {
	return isched.Job{Key: attempt, ID: attempt, Name: attempt, Pipeline: isched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan",
		TimeoutMs: 60000, PersonalOwnRun: true, Project: "project"}
}

// queue creates the pipeline, job and first attempt a decision is made for.
func queue(ctx context.Context, st *store.Store) (string, error) {
	err := st.CreatePipeline(ctx, store.PipelineRow{ID: "pipe", Revision: "rev", Project: "project", Trigger: "local", SourceTrust: "owner",
		PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"})
	if err != nil {
		return "", err
	}
	const id = "pipe.noop.1"
	if err = st.CreateJob(ctx, store.JobRow{ID: "job-" + id, PipelineID: "pipe", Name: "noop", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		return "", err
	}
	return id, st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "job-" + id, N: 1}, "")
}

// Decide makes one decision in a fresh store under dir.
func Decide(ctx context.Context, dir string) (Decision, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Decision{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		return Decision{}, err
	}
	defer func() { _ = st.Close() }()
	id, err := queue(ctx, st)
	if err != nil {
		return Decision{}, err
	}
	capability, err := planview.LocalCapability(planview.LocalFacts{Project: "project", Platform: "linux/amd64", CPUs: hostCPU, MemMB: hostMemMB, Now: time.Now()})
	if err != nil {
		return Decision{}, err
	}
	j := job(id)
	deps := world.Deps{Store: st, Local: &capability, Jobs: []isched.Job{j},
		Policy: func(context.Context, string) (isched.Policy, error) { return isched.Policy{Preset: "balanced"}, nil }}
	t0 := time.Now()
	w, digest, err := world.Build(ctx, deps, j.Pipeline)
	if err != nil {
		return Decision{}, err
	}
	t1 := time.Now()
	placed := isched.Place(w, []isched.Job{j})
	t2 := time.Now()
	if len(placed.Placements) != 1 || len(w.Runners) != 1 {
		return Decision{}, fmt.Errorf("the local fixture did not place exactly one job")
	}
	_, err = dispatch.Admit(ctx, st, dispatch.Request{AttemptID: id, Placement: placed.Placements[0], Runner: w.Runners[0], Job: j,
		Local: true, Host: "bench-host", CapacityCPU: hostCPU, CapacityMemMB: hostMemMB, WorldDigest: digest})
	t3 := time.Now()
	if err != nil {
		return Decision{}, err
	}
	return Decision{PlaceMS: ms(t2.Sub(t1)), AdmitMS: ms(t3.Sub(t0))}, nil
}
