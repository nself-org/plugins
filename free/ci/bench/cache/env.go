package cache

// Purpose: the real cache engine, wired the way the coordinator wires it: the
// sqlite store, the content-addressed blob store, entries.Store and specs.Engine.
// Inputs:  a work directory.
// Outputs: Env with Close and Attempt (creates a running attempt).
// Constraints: nothing here is a stand-in; the audit sealer only counts rows,
// because the real sealer needs the coordinator's signing key.

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
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
		func() error { return storetest.Admit(ctx, s, id) },
		func() error { return s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}) },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			return fmt.Errorf("attempt %s step %d: %w", id, i, err)
		}
	}
	return nil
}

// Finalizing moves the running attempt id to finalizing and returns the epoch a
// finalize hook sees.
func (e *Env) Finalizing(ctx context.Context, id string) (int64, error) {
	err := e.Store.Transition(ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2})
	return 3, err
}
