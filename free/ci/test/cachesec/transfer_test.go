//go:build cachesec

package cachesec

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
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

func (r *rig) remoteWrite(id string) error {
	// Bind the real coordinator receiver to a durable remote lease. The agent
	// carrier permits unordered refs; SSH preserves stream order.
	f := owner()
	r.running("remote", f)
	eng := lease.New(r.st, nil, lease.Config{})
	// Grant needs a queued attempt, so use a separate attempt from the local rig.
	must(r.t, r.st.CreateAttempt(r.ctx, store.AttemptRow{ID: "remote-wire", JobID: "jremote", N: 2}, ""))
	epoch, err := eng.Grant(r.ctx, "host", store.Demand{AttemptID: "remote-wire", LeaseID: "wire", RunnerID: "node", CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: eng.Config.TTL}, 1)
	must(r.t, err)
	ns := namespace(r.t, f)
	plan := &protocol.LeaseCache{Namespace: base64.RawURLEncoding.EncodeToString(ns.Canonical()), Access: "rw", Save: []protocol.CacheSave{{Spec: "go", Key: key(r.t, f), Dirs: []string{"go/data"}}}}
	h := cacheRemote.Handler{Ledger: r.st, Entries: r.entries, Blobs: r.blobs}
	l, err := h.Bind(cacheRemote.Binding{AttemptID: "remote-wire", LeaseID: "wire", Node: "node", Epoch: epoch, Facts: f, Plan: plan, Unordered: strings.HasSuffix(id, "agent"), Fence: func(ctx context.Context) error {
		if e := eng.Fence(ctx, "wire", epoch); e != nil {
			return transfer.ErrStale
		}
		return nil
	}})
	must(r.t, err)
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

func (r *rig) transferBlob(id string) error {
	realm := cacheRealm(namespace(r.t, owner()).Canonical())
	want := []byte("intact payload")
	rcv := transfer.Receiver{Store: r.blobs, Realm: realm, Lease: "wire", Live: func(context.Context) error { return nil }}
	w, err := rcv.Open(r.ctx, transport.BlobHeader{Stream: transfer.StreamArtifact, SHA256: digest(want), Size: int64(len(want))})
	must(r.t, err)
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
	return err
}

func (r *rig) readThrough(id string) error {
	dir := r.t.TempDir()
	c, err := cacheRemote.NewReadThrough(dir, 4<<20)
	must(r.t, err)
	want := []byte("intact payload")
	got := bytes.Clone(want)
	if id == "partial-readthrough" {
		got = got[:len(got)/2]
	} else {
		got[0] ^= 1
	}
	f, err := c.Open(r.ctx, "realm", digest(want), int64(len(want)), func(_ context.Context, w io.Writer) error { _, e := w.Write(got); return e })
	if f != nil {
		f.Close()
		r.t.Error("invalid read-through copy was served")
	}
	if len(tree(r.t, dir)) != 0 {
		r.t.Error("read-through published invalid bytes")
	}
	return err
}
