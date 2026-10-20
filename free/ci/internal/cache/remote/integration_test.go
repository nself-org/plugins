//go:build integration

package remote

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

type rig struct {
	t        *testing.T
	st       *store.Store
	blobs    *cas.Store
	lease    *Lease
	eng      *lease.Engine
	deadline time.Duration
	snk      *simSink
	mu       sync.Mutex
	now      time.Time
}

// newRig builds the coordinator side of one lease; mods change the cache plan before it is bound.
func newRig(t *testing.T, mods ...func(*rig, *protocol.LeaseCache)) *rig {
	t.Helper()
	r := &rig{t: t, now: time.Now()}
	clock := func() time.Time { r.mu.Lock(); defer r.mu.Unlock(); return r.now }
	var err error
	r.st, err = store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.st.Close() })
	ctx := context.Background()
	if err = r.st.CreatePipeline(ctx, store.PipelineRow{ID: "p1", Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err = r.st.CreateJob(ctx, store.JobRow{ID: "j1", PipelineID: "p1", Name: "build", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err = r.st.CreateAttempt(ctx, store.AttemptRow{ID: "a1", JobID: "j1", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	r.blobs, err = cas.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r.eng = lease.New(r.st, nil, lease.Config{Clock: clock})
	epoch, err := r.eng.Grant(ctx, "host", store.Demand{AttemptID: "a1", LeaseID: "lease-a1", RunnerID: "n-1", CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: r.eng.Config.TTL}, 1)
	if err != nil {
		t.Fatal(err)
	}
	facts := entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}
	ns, err := entries.NamespaceOf(facts)
	if err != nil {
		t.Fatal(err)
	}
	plan := &protocol.LeaseCache{Namespace: base64.RawURLEncoding.EncodeToString(ns.Canonical()), Access: "rw", Save: []protocol.CacheSave{{Spec: "go", Key: strings.Repeat("a", 64), Dirs: []string{"go/mod"}}}}
	for _, mod := range mods {
		mod(r, plan)
	}
	h := &Handler{Ledger: r.st, Entries: entries.New(r.st, r.blobs, entries.Options{DisableFloor: true}), Blobs: r.blobs, Limits: transfer.Limits{MaxFile: 4 << 20}}
	r.lease, err = h.Bind(Binding{AttemptID: "a1", LeaseID: "lease-a1", Node: "n-1", Epoch: epoch, Facts: facts, Plan: plan, Fence: func(ctx context.Context) error {
		err := r.eng.Fence(ctx, "lease-a1", epoch)
		var se *store.Error
		if errors.As(err, &se) && se.Code == "E662" {
			return fmt.Errorf("%w: %v", transfer.ErrStale, err)
		}
		return err
	}, Ready: func(ctx context.Context) error {
		state, cas, err := r.st.AttemptStateEpoch(ctx, "a1")
		if err == nil && state == "leased" {
			return r.st.Transition(ctx, "a1", "leased", "running", store.TransitionDetail{Epoch: cas})
		}
		return err
	}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *rig) expire() {
	r.mu.Lock()
	r.now = r.now.Add(10 * time.Minute)
	r.mu.Unlock()
	if n, err := r.eng.Sweep(context.Background()); err != nil || n != 1 {
		r.t.Fatalf("sweep=%d err=%v", n, err)
	}
}

type simSink struct{ r *rig }

func (r *rig) sink() *simSink { s := &simSink{r: r}; r.snk = s; return s }
func (s *simSink) declare(ctx context.Context) error {
	if len(s.r.lease.b.Plan.Save) == 0 { // a restore-only plan has nothing to declare; the attempt still starts running
		return s.r.lease.b.Ready(ctx)
	}
	return s.r.lease.Declare(ctx)
}
func (s *simSink) Log(ctx context.Context, _ protocol.Log) error { return s.declare(ctx) }
func (s *simSink) RestoreBlobs(ctx context.Context) ([]transport.RestoreBlob, error) {
	return s.r.lease.RestoreBlobs(ctx)
}
func (s *simSink) RestoreEvent(ctx context.Context, e transport.RestoreEvent) {
	s.r.lease.RestoreEvent(ctx, e)
}
func (s *simSink) Result(ctx context.Context, res protocol.Result) error {
	if err := s.declare(ctx); err != nil {
		return err
	}
	return s.r.eng.Result(ctx, res.LeaseID, int64(res.Epoch))
}
func (s *simSink) ArtifactRef(context.Context, protocol.ArtifactRef) error {
	return errors.New("unexpected artifact")
}
func (s *simSink) CacheRef(ctx context.Context, ref protocol.CacheRef) error {
	return s.r.lease.CacheRef(ctx, ref)
}
func (s *simSink) Blob(ctx context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	return s.r.lease.Blob(ctx, h)
}

type node struct {
	t      *testing.T
	data   []byte
	onSend func()
	once   sync.Once
	// tapFn and hooksFn replace the default carrier and job hooks (integration_restore_test.go).
	tapFn   func(session.Carrier) session.Carrier
	hooksFn func() []session.JobHook
}

func (n *node) tap(c session.Carrier) session.Carrier {
	if n.tapFn != nil {
		return n.tapFn(c)
	}
	return c
}
func (n *node) runner() session.Runner {
	return session.RunnerFunc(func(context.Context, session.JobRun) (session.JobResult, error) {
		return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: []byte(`{"schema":"test","ok":true}`)}, nil
	})
}
func (n *node) hooks() []session.JobHook {
	if n.hooksFn != nil {
		return n.hooksFn()
	}
	return []session.JobHook{{Name: "cache", After: func(ctx context.Context, l protocol.Lease, _ session.JobRoot, out session.Outcome, up session.Uploader) error {
		if out.State != "passed" {
			return nil
		}
		if n.onSend != nil {
			n.once.Do(n.onSend)
		}
		sender := transfer.Sender{Up: up, Offsets: up.(session.OffsetSource).Offsets(), WaitOffset: time.Second, MaxResumes: 1}
		digest := fmt.Sprintf("%x", sha256.Sum256(n.data))
		err := sender.Send(ctx, transfer.Blob{Stream: StreamCache, SHA256: digest, Size: int64(len(n.data)), Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(n.data)), nil }})
		if err != nil {
			return fmt.Errorf("%w: %v", session.ErrHookFatal, err)
		}
		return up.CacheRef(protocol.CacheRef{LeaseID: l.LeaseID, Spec: "go", Key: strings.Repeat("a", 64), Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: digest, Size: int64(len(n.data))}}})
	}}}
}

func (r *rig) assertCommit(data []byte) {
	r.t.Helper()
	ctx := context.Background()
	if err := r.lease.Finalize(ctx, time.Second); err != nil {
		r.t.Fatal(err)
	}
	list, err := r.st.CacheEntries(ctx, strings.Repeat("a", 64))
	if err != nil || len(list) != 1 {
		r.t.Fatalf("entries=%+v err=%v", list, err)
	}
	if list[0].Writer.AttemptID != "a1" || list[0].Writer.Epoch != r.lease.b.Epoch {
		r.t.Fatalf("writer not lease fenced: %+v", list[0].Writer)
	}
	cache, err := NewReadThrough(r.t.TempDir(), 4<<20)
	if err != nil {
		r.t.Fatal(err)
	}
	var fetches int
	for i := 0; i < 2; i++ {
		f, err := cache.Open(ctx, r.lease.b.Plan.Namespace, list[0].Blobs[0].SHA256, int64(len(data)), func(_ context.Context, w io.Writer) error {
			fetches++
			rc, err := r.blobs.Open(cacheRealm(r.lease.b.Plan.Namespace), list[0].Blobs[0].SHA256)
			if err != nil {
				return err
			}
			defer rc.Close()
			_, err = io.Copy(w, rc)
			return err
		})
		if err != nil {
			r.t.Fatal(err)
		}
		got, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil || !bytes.Equal(got, data) {
			r.t.Fatal("read-through bytes differ", err)
		}
	}
	if fetches != 1 {
		r.t.Fatalf("second read-through fetched again: %d", fetches)
	}
	rows, err := r.st.CacheLocality(ctx, "n-1", r.lease.b.Plan.Namespace, strings.Repeat("a", 64))
	if err != nil || len(rows) != 1 {
		r.t.Fatalf("locality=%+v err=%v", rows, err)
	}
}

// These exercise actual carrier upload and fenced commit. The node restore over
// the same carriers is covered by integration_restore_test.go.
func TestRemoteCacheSSH(t *testing.T) {
	r := newRig(t)
	data := carrierCacheArchive(t)
	if err := r.runSSH(&node{t: t, data: data}); err != nil {
		t.Fatal(err)
	}
	r.assertCommit(data)
}
func TestRemoteCacheAgent(t *testing.T) {
	r := newRig(t)
	data := carrierCacheArchive(t)
	if err := r.runAgent(&node{t: t, data: data}); err != nil {
		t.Fatal(err)
	}
	r.assertCommit(data)
}

func carrierCacheArchive(t *testing.T) []byte {
	t.Helper()
	data := bytes.Repeat([]byte("synthetic cache data"), 70000)
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	if err := tw.WriteHeader(&tar.Header{Name: "payload", Mode: 0600, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if out.Len() <= 512<<10 {
		t.Fatal("cache fixture must span multiple carrier chunks")
	}
	return out.Bytes()
}

func TestRemoteCacheZombie(t *testing.T) {
	r := newRig(t)
	_ = r.runSSH(&node{t: t, data: []byte("synthetic cache data"), onSend: r.expire})
	err := r.lease.CacheRef(context.Background(), protocol.CacheRef{LeaseID: r.lease.b.LeaseID, Spec: "go", Key: strings.Repeat("a", 64)})
	if !transfer.IsReason(err, transfer.ReasonStaleEpoch) {
		t.Fatalf("zombie: %v", err)
	}
	list, err := r.st.CacheEntries(context.Background(), "")
	if err != nil || len(list) != 0 {
		t.Fatalf("zombie committed %+v err=%v", list, err)
	}
	rows, err := r.st.AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Action == "cache.remote.refused" && row.Detail == "reason=stale_epoch" {
			return
		}
	}
	t.Fatal("missing stale_epoch audit")
}
