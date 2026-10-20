//go:build cachesec

package cachesec

// Purpose: group g3 (remote attempts, TM-12..TM-15). The coordinator side of one remote lease: a real
// store with the audit sink of newWorld, the real lease engine, the real cache/remote and artifacts/remote
// handlers bound to one attempt, and a Sink that routes carrier frames to them exactly as the run's
// leaseSink does. The carriers (ssh driver, agent pull driver) deliver frames into this Sink.
// Inputs: facts, a cache plan, a job. Outputs: g3rig, g3sink, byte and audit probes.
// Constraints: production packages only; the handler code and the fences are never patched; helpers of
// the owning packages' tests are not importable, so their shape is rebuilt here.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	artremote "github.com/nself-org/plugins/free/ci/internal/artifacts/remote"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	cacheremote "github.com/nself-org/plugins/free/ci/internal/cache/remote"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

var (
	g3Main  = entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}
	g3Fork1 = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/example", PR: "1"}
	g3Fork2 = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/example", PR: "2"}
	g3Key   = strings.Repeat("a", 64)
)

// g3rig is one delivered remote lease on a world.
type g3rig struct {
	t       *testing.T
	w       *world
	eng     *lease.Engine
	id      string // attempt id
	leaseID string
	node    string
	epoch   int64
	facts   entries.Facts
	job     model.Job
	plan    *protocol.LeaseCache
	pol     records.Policy
	rev     string // checkout revision of the wire lease (the real agent refuses a lease without one)
	cache   *cacheremote.Lease
	art     *artremote.Lease
}

// g3NS is the wire encoding of the canonical namespace of f.
func g3NS(t *testing.T, f entries.Facts) string {
	t.Helper()
	ns, err := entries.NamespaceOf(f)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(ns.Canonical())
}

// g3Plan is a cache plan for the "go" spec in namespace ns with one planned dir (go/mod): the scripted
// nodes of the in-process carriers send one blob. g3PlanFull plans both dirs of the spec, as the real agent saves.
func g3Plan(ns, access, key string) *protocol.LeaseCache {
	return &protocol.LeaseCache{Namespace: ns, Access: access, Save: []protocol.CacheSave{{Spec: "go", Key: key, Dirs: []string{"go/mod"}}}}
}

func g3PlanFull(ns, access, key string) *protocol.LeaseCache {
	return &protocol.LeaseCache{Namespace: ns, Access: access, Save: []protocol.CacheSave{{Spec: "go", Key: key, Dirs: []string{"go/mod", "go/build"}}}}
}

// newG3Rig creates pipeline, job and attempt id for facts f, grants the lease and binds both handlers.
// plan may be nil (artifact-only lease); job may declare artifacts.
func newG3Rig(t *testing.T, w *world, id string, f entries.Facts, plan *protocol.LeaseCache, job model.Job) *g3rig {
	t.Helper()
	ctx, s := w.ctx, w.St
	now := func() time.Time { return time.Unix(0, w.clock.Load()) }
	for i, err := range []error{
		s.CreatePipeline(ctx, store.PipelineRow{ID: "p" + id, Revision: "rev", Project: f.Project, Trigger: "local", SourceTrust: f.Trust, PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}),
		s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: "p" + id, Name: "build", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""),
	} {
		if err != nil {
			t.Fatalf("g3 setup %s step %d: %v", id, i, err)
		}
	}
	r := &g3rig{t: t, w: w, id: id, leaseID: "lease-" + id, node: "n-" + id, facts: f, job: job, plan: plan}
	r.eng = lease.New(s, nil, lease.Config{Clock: now})
	epoch, err := r.eng.Grant(ctx, "host", store.Demand{AttemptID: id, LeaseID: r.leaseID, RunnerID: r.node, CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: r.eng.Config.TTL}, 1)
	if err != nil {
		t.Fatal(err)
	}
	r.epoch = epoch
	fence := func(ctx context.Context) error {
		err := r.eng.Fence(ctx, r.leaseID, epoch)
		var se *store.Error
		if errors.As(err, &se) && se.Code == "E662" {
			return fmt.Errorf("%w: %v", transfer.ErrStale, err)
		}
		return err
	}
	ready := func(ctx context.Context) error {
		state, cur, err := s.AttemptStateEpoch(ctx, id)
		if err == nil && state == "leased" {
			return s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: cur})
		}
		return err
	}
	ch := &cacheremote.Handler{Ledger: s, Entries: entries.New(s, w.Blobs, entries.Options{DisableFloor: true}), Blobs: w.Blobs, Limits: transfer.Limits{MaxFile: 4 << 20}}
	if r.cache, err = ch.Bind(cacheremote.Binding{AttemptID: id, LeaseID: r.leaseID, Node: r.node, Epoch: epoch, Facts: f, Plan: plan, Fence: fence, Ready: ready}); err != nil {
		t.Fatal(err)
	}
	pol := records.Policy{MaxFile: 4 << 20, MaxTotal: 64 << 20, ProjectShare: 50, StorePath: t.TempDir(),
		Statfs: func(string) (uint64, uint64, error) { return 100 << 30, 100 << 30, nil }}
	r.pol = pol
	ah := &artremote.Handler{Ledger: s, Blobs: w.Blobs, Policy: pol}
	r.art = ah.Bind(artremote.Binding{AttemptID: id, LeaseID: r.leaseID, Epoch: epoch, Job: job, Fence: fence, Ready: ready})
	return r
}

// Proto is the wire lease of the rig.
func (r *g3rig) Proto() protocol.Lease {
	l := protocol.Lease{LeaseID: r.leaseID, AttemptID: r.id, Epoch: uint64(r.epoch), Token: "opaque", CompatMode: protocol.Compat15,
		Job: r.job, Cache: r.plan, Secrets: []protocol.SecretRef{}, ExpiresAt: "2099-01-01T00:00:00Z"}
	l.Checkout.Revision = r.rev
	return l
}

// Expire lets the lease lapse without a heartbeat: the engine fences the epoch and loses the attempt.
func (r *g3rig) Expire() {
	r.t.Helper()
	r.w.clock.Add(int64(10 * time.Minute))
	if _, err := r.eng.Sweep(r.w.ctx); err != nil {
		r.t.Fatal(err)
	}
}

// Landed is the bytes of this attempt that are reachable in the attacked stores: every cache entry, every
// artifact record staged or committed for the attempt and every artifact blob visible in the project realm
// for the digests in extra (a published blob is reachable by digest even before a record names it).
func (r *g3rig) Landed(extra ...string) (n int64) {
	r.t.Helper()
	n = r.w.EntryBytes()
	staged, err := r.w.St.ArtifactStaged(r.w.ctx, r.id)
	if err != nil {
		r.t.Fatal(err)
	}
	for _, rec := range staged {
		n += rec.Size
	}
	for _, d := range extra {
		if fi, err := r.w.Blobs.Stat(cas.ArtifactRealm(r.facts.Project), d); err == nil {
			n += fi.Size()
		}
	}
	return n
}

// g3Sink routes carrier frames to the handlers (the run's leaseSink). Refusals are remembered because a
// driver reports a rejected blob only as a generic failure.
type g3Sink struct {
	r        *g3rig
	mu       sync.Mutex
	logs     []string
	errs     []error
	onBlob   func() // runs once when the first blob frame reaches the sink, before the handler sees it
	blobOnce sync.Once
	wrapW    func(transport.BlobWriter) transport.BlobWriter // wraps an admitted blob writer (fault injection at the transfer boundary)
	onLog    func(string)                                    // runs on each log line before the handler
	result   int
}

func (r *g3rig) Sink() *g3Sink { return &g3Sink{r: r} }

func (s *g3Sink) note(err error) error {
	if err != nil {
		s.mu.Lock()
		s.errs = append(s.errs, err)
		s.mu.Unlock()
	}
	return err
}

// Logs returns the job output lines seen so far.
func (s *g3Sink) Logs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.logs...)
}

// Errs returns every handler refusal seen so far.
func (s *g3Sink) Errs() []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]error(nil), s.errs...)
}

func (s *g3Sink) Log(ctx context.Context, l protocol.Log) error {
	s.mu.Lock()
	s.logs = append(s.logs, l.Text)
	s.mu.Unlock()
	if s.onLog != nil {
		s.onLog(l.Text)
	}
	if s.r.plan != nil {
		return s.note(s.r.cache.Declare(ctx))
	}
	return s.note(s.r.art.Declare(ctx))
}

func (s *g3Sink) Result(ctx context.Context, res protocol.Result) error {
	if s.r.plan != nil {
		if err := s.note(s.r.cache.Declare(ctx)); err != nil {
			return err
		}
	}
	if err := s.note(s.r.art.Declare(ctx)); err != nil {
		return err
	}
	if err := s.r.eng.Result(ctx, res.LeaseID, int64(res.Epoch)); err != nil {
		return s.note(err)
	}
	s.mu.Lock()
	s.result++
	s.mu.Unlock()
	return nil
}

func (s *g3Sink) ArtifactRef(ctx context.Context, a protocol.ArtifactRef) error {
	return s.note(s.r.art.ArtifactRef(ctx, a))
}

func (s *g3Sink) CacheRef(ctx context.Context, c protocol.CacheRef) error {
	return s.note(s.r.cache.CacheRef(ctx, c))
}

func (s *g3Sink) Blob(ctx context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	if s.onBlob != nil {
		s.blobOnce.Do(s.onBlob)
	}
	var w transport.BlobWriter
	var err error
	if h.Stream == cacheremote.StreamCache {
		w, err = s.r.cache.Blob(ctx, h)
	} else {
		w, err = s.r.art.Blob(ctx, h)
	}
	if err != nil {
		return nil, s.note(err)
	}
	if s.wrapW != nil {
		w = s.wrapW(w)
	}
	return w, nil
}
