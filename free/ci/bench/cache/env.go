package cache

// Purpose: the real cache engine, wired the way the coordinator wires it: the
// sqlite store, the content-addressed blob store, entries.Store and specs.Engine.
// Inputs:  a work directory.
// Outputs: Env with Close and Attempt (creates a running attempt).
// Constraints: nothing here is a stand-in; the audit sealer only counts rows,
// because the real sealer needs the coordinator's signing key. Attempts are
// leased through dispatch.Admit, the production admission chokepoint (R-SEC-22),
// never through a test fixture: this is a non-test package and must not import
// internal/storetest (TestStoretestIsTestOnly).

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	isched "github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// Host figures of the admission fixture: ample, so admission never refuses.
const (
	hostCPU   = 8
	hostMemMB = 8192
)

// Env is one engine instance over a work directory.
type Env struct {
	Dir     string
	Store   *store.Store
	Blobs   *cas.Store
	Entries *entries.Store
	Engine  *specs.Engine
	Facts   entries.Facts
}

// NewEnv opens the store and builds the engine under dir.
func NewEnv(dir string) (*Env, error) {
	e := &Env{Dir: dir, Facts: entries.Facts{Project: "/work/bench", Trust: "owner", Protection: "protected"}}
	var err error
	if e.Store, err = store.Open(filepath.Join(dir, "state.db"), store.Options{}); err != nil {
		return nil, err
	}
	e.Store.SetAuditSealer(func([]byte, store.AuditRow) (store.AuditChain, error) {
		return store.AuditChain{Head: []byte("h"), Signature: []byte("s")}, nil
	})
	if e.Blobs, err = cas.New(dir); err != nil {
		_ = e.Store.Close()
		return nil, err
	}
	e.Entries = entries.New(e.Store, e.Blobs, entries.Options{Home: dir,
		Free:    func() (int64, int64, error) { return 1 << 40, 1 << 41, nil },
		Project: func(string) authz.ProjectFacts { return authz.ProjectFacts{NoPrivateDeps: true} }})
	e.Engine = &specs.Engine{Entries: e.Entries, Blobs: e.Blobs, Scanner: redact.New(), Backend: e.Store}
	return e, nil
}

// Close releases the store.
func (e *Env) Close() error { return e.Store.Close() }

// Attempt creates attempt id (running, launch epoch 2) of a fresh pipeline.
func (e *Env) Attempt(ctx context.Context, id string) error {
	s, f := e.Store, e.Facts
	steps := []func() error{
		func() error {
			return s.CreatePipeline(ctx, store.PipelineRow{ID: "p" + id, Revision: "rev", Project: f.Project, Trigger: "local", SourceTrust: f.Trust,
				PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"})
		},
		func() error {
			return s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: "p" + id, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"})
		},
		func() error { return s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, "") },
		func() error { return e.admit(ctx, id) },
		func() error { return s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}) },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			return fmt.Errorf("attempt %s step %d: %w", id, i, err)
		}
	}
	return nil
}

// admit leases the queued attempt id the way the V15 dispatcher does for one
// local job: build the world, place the job, admit the placement. It is setup,
// outside every timed span (save, restore, finalize).
func (e *Env) admit(ctx context.Context, id string) error {
	j := isched.Job{Key: id, ID: id, Name: id, Pipeline: isched.Pipeline{ID: "p" + id, Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan",
		TimeoutMs: 60000, PersonalOwnRun: true, Project: e.Facts.Project}
	capability, err := planview.LocalCapability(planview.LocalFacts{Project: e.Facts.Project, Platform: "linux/amd64", CPUs: hostCPU, MemMB: hostMemMB, Now: time.Now()})
	if err != nil {
		return err
	}
	deps := world.Deps{Store: e.Store, Local: &capability, Jobs: []isched.Job{j},
		Policy: func(context.Context, string) (isched.Policy, error) { return isched.Policy{Preset: "balanced"}, nil }}
	w, digest, err := world.Build(ctx, deps, j.Pipeline)
	if err != nil {
		return err
	}
	placed := isched.Place(w, []isched.Job{j})
	if len(placed.Placements) != 1 || len(w.Runners) != 1 {
		return fmt.Errorf("the local fixture did not place exactly one job: %d placements, %d runners, explanations %+v", len(placed.Placements), len(w.Runners), placed.Explanations)
	}
	_, err = dispatch.Admit(ctx, e.Store, dispatch.Request{AttemptID: id, Placement: placed.Placements[0], Runner: w.Runners[0], Job: j,
		Local: true, Host: "bench-host", CapacityCPU: hostCPU, CapacityMemMB: hostMemMB, WorldDigest: digest})
	return err
}

// Finalizing moves the running attempt id to finalizing and returns the epoch a
// finalize hook sees.
func (e *Env) Finalizing(ctx context.Context, id string) (int64, error) {
	err := e.Store.Transition(ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2})
	return 3, err
}

// Release finishes the running attempt id (running, finalizing, passed) so the
// host reservation its lease holds is returned. Real admission counts the slots
// a lease holds, so a cycle that leaves its attempt running starves the next
// admission; this is setup, outside every timed span.
func (e *Env) Release(ctx context.Context, id string) error {
	epoch, err := e.Finalizing(ctx, id)
	if err != nil {
		return err
	}
	return e.Store.Transition(ctx, id, "finalizing", "passed", store.TransitionDetail{Epoch: epoch})
}
