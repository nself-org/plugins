package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*13 + i/97)
	}
	return b
}

// rig is a real store with one running attempt and a handler over it.
type rig struct {
	t     *testing.T
	st    *store.Store
	blobs *cas.Store
	h     *Handler
	lease *Lease
	eng   *lease.Engine
	clk   *time.Time
	pol   records.Policy
}

func newRig(t *testing.T, job model.Job) *rig {
	t.Helper()
	ctx := context.Background()
	clk := new(time.Time)
	*clk = time.Unix(1_000_000, 0)
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: func() time.Time { return *clk }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err = st.CreatePipeline(ctx, store.PipelineRow{ID: "p1", Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateJob(ctx, store.JobRow{ID: "j1", PipelineID: "p1", Name: "build", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAttempt(ctx, store.AttemptRow{ID: "a1", JobID: "j1", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	blobs, err := cas.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, st: st, blobs: blobs, clk: clk, pol: records.Policy{MaxFile: 1 << 20, MaxTotal: 8 << 20, ProjectShare: 50, StorePath: t.TempDir(),
		Statfs: func(string) (uint64, uint64, error) { return 100 << 30, 100 << 30, nil }}}
	r.eng = lease.New(st, nil, lease.Config{Clock: func() time.Time { return *clk }})
	d := store.Demand{AttemptID: "a1", LeaseID: "lease-a1", RunnerID: "r1", CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: r.eng.Config.TTL}
	epoch, err := r.eng.Grant(ctx, "host", d, 1)
	if err != nil {
		t.Fatal(err)
	}
	r.h = &Handler{Ledger: st, Blobs: blobs, Policy: r.pol}
	r.lease = r.h.Bind(Binding{AttemptID: "a1", LeaseID: "lease-a1", Epoch: epoch, Job: job,
		Fence: func(ctx context.Context) error { return r.eng.Fence(ctx, "lease-a1", epoch) },
		Ready: func(ctx context.Context) error {
			state, epoch, err := st.AttemptStateEpoch(ctx, "a1")
			if err == nil && state == "leased" {
				err = st.Transition(ctx, "a1", "leased", "running", store.TransitionDetail{Epoch: epoch})
			}
			return err
		}})
	return r
}

// expire lets the lease lapse without a heartbeat: the engine fences the epoch and loses the attempt.
func (r *rig) expire() {
	r.t.Helper()
	*r.clk = r.clk.Add(10 * time.Minute)
	if n, err := r.eng.Sweep(context.Background()); err != nil || n != 1 {
		r.t.Fatalf("sweep = %d, %v", n, err)
	}
}

func jobWith(paths ...string) model.Job {
	j := model.Job{Run: []string{"true"}}
	for _, p := range paths {
		j.Artifacts = append(j.Artifacts, model.ArtifactDecl{Path: p})
	}
	return j
}

// upload plays the driver: announce, stream every byte, commit.
func (r *rig) upload(data []byte) error {
	r.t.Helper()
	w, err := r.lease.Blob(context.Background(), transport.BlobHeader{Stream: "artifact", SHA256: sum(data), Size: int64(len(data))})
	if err != nil {
		return err
	}
	if _, err = w.Write(data[w.Offset():]); err != nil {
		return err
	}
	return w.Close()
}

func (r *rig) audits(action string) int {
	r.t.Helper()
	rows, err := r.st.AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		r.t.Fatal(err)
	}
	n := 0
	for _, a := range rows {
		if a.Action == action {
			n++
		}
	}
	return n
}

func (r *rig) has(data []byte) bool {
	rc, err := r.blobs.Open(cas.ArtifactRealm("proj"), sum(data))
	if err != nil {
		return false
	}
	defer func() { _ = rc.Close() }()
	var b bytes.Buffer
	_, _ = b.ReadFrom(rc)
	return bytes.Equal(b.Bytes(), data)
}
