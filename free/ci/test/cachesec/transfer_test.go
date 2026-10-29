//go:build cachesec

package cachesec

import (
	"bytes"
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	cacheRemote "github.com/nself-org/plugins/free/ci/internal/cache/remote"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// bindRemote binds the real coordinator receiver (cacheRemote.Handler) to a durable remote lease of
// a fresh attempt. The agent carrier permits unordered refs; SSH preserves stream order. restore, when
// set, is the lease plan's restore blob set.
func (r *rig) bindRemote(unordered bool, restore ...protocol.CacheBlob) (*cacheRemote.Lease, string) {
	f := owner()
	eng := lease.New(r.st, nil, lease.Config{})
	// Grant needs a queued attempt, so use a separate attempt from the local rig.
	must(r.t, r.st.CreatePipeline(r.ctx, store.PipelineRow{ID: "pwire", Revision: strings.Repeat("a", 40), Project: f.Project, Trigger: "local", SourceTrust: f.Trust, PrivacyZone: "local-only", PolicyDigest: "policy", InputDigest: "input", SelectionMode: "full", Status: "queued"}))
	must(r.t, r.st.CreateJob(r.ctx, store.JobRow{ID: "jwire", PipelineID: "pwire", Name: "wire", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "input"}))
	must(r.t, r.st.CreateAttempt(r.ctx, store.AttemptRow{ID: "remote-wire", JobID: "jwire", N: 1}, ""))
	epoch, err := eng.Grant(r.ctx, "host", store.Demand{AttemptID: "remote-wire", LeaseID: "wire", RunnerID: "node", CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: eng.Config.TTL}, 1)
	must(r.t, err)
	ns := namespace(r.t, f)
	plan := &protocol.LeaseCache{Namespace: base64.RawURLEncoding.EncodeToString(ns.Canonical()), Access: "rw", Save: []protocol.CacheSave{{Spec: "go", Key: key(r.t, f), Dirs: []string{"go/data"}}}}
	if len(restore) > 0 {
		plan.Restore = []protocol.CacheRestore{{Spec: "go", Key: key(r.t, f), Dir: "go", Blobs: restore}}
	}
	h := &cacheRemote.Handler{Ledger: r.st, Entries: r.entries, Blobs: r.blobs}
	l, err := h.Bind(cacheRemote.Binding{AttemptID: "remote-wire", LeaseID: "wire", Node: "node", Epoch: epoch, Facts: f, Plan: plan, Unordered: unordered, RestoreWait: 5 * time.Second, Fence: func(ctx context.Context) error {
		if e := eng.Fence(ctx, "wire", epoch); e != nil {
			return transfer.ErrStale
		}
		return nil
	}})
	must(r.t, err)
	return l, plan.Namespace
}

func (r *rig) remoteWrite(id string) error {
	r.running("remote", owner())
	l, _ := r.bindRemote(strings.HasSuffix(id, "agent"))
	if strings.HasPrefix(id, "ended-") {
		state, cas, e := r.st.AttemptStateEpoch(r.ctx, "remote-wire")
		must(r.t, e)
		must(r.t, r.st.Transition(r.ctx, "remote-wire", state, "cancelled", store.TransitionDetail{Epoch: cas}))
	} else {
		// The binding retains the old epoch after the durable lease is lost.
		durable, e := r.st.LeaseGet(r.ctx, "wire")
		must(r.t, e)
		must(r.t, r.st.LeaseFail(r.ctx, durable, time.Now(), "test fence moved", false))
	}
	w, err := l.Blob(r.ctx, transport.BlobHeader{Stream: "cache", SHA256: digest([]byte("payload")), Size: 7})
	if w != nil {
		r.t.Error("zombie obtained a blob writer")
	}
	r.noEntries()
	if len(tree(r.t, filepath.Join(r.home, "blobs"))) != 0 {
		r.t.Error("zombie left blob bytes")
	}
	return err
}

// transferBlob sends a corrupted or truncated blob to the coordinator's cache receiver of a live lease.
func (r *rig) transferBlob(id string) error {
	l, _ := r.bindRemote(false)
	state, cas, err := r.st.AttemptStateEpoch(r.ctx, "remote-wire")
	must(r.t, err)
	must(r.t, r.st.Transition(r.ctx, "remote-wire", state, "running", store.TransitionDetail{Epoch: cas}))
	realm := cacheRealm(namespace(r.t, owner()).Canonical())
	want := []byte("intact payload")
	bw, err := l.Blob(r.ctx, transport.BlobHeader{Stream: "cache", SHA256: digest(want), Size: int64(len(want))})
	must(r.t, err)
	w, ok := bw.(*transfer.Writer)
	if !ok {
		r.t.Fatalf("the cache receiver handed out a %T, not a transfer writer", bw)
	}
	got := bytes.Clone(want)
	if id == "partial-transit" {
		got = got[:len(got)/2]
	} else {
		got[0] ^= 1
	}
	_, err = w.Write(got)
	must(r.t, err)
	err = w.Close()
	defer w.Abort()
	if w.Published() {
		r.t.Error("invalid transfer published bytes")
	}
	if f, e := r.blobs.Open(realm, digest(want)); e == nil {
		f.Close()
		r.t.Error("invalid transfer became readable")
	}
	r.noEntries()
	return err
}
