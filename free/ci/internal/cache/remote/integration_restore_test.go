//go:build integration

package remote

// Purpose: the cold restore of a node from the coordinator over a real carrier
// (P7-CACHE-21): the real ssh driver pushes digest and chunk frames, a real agent
// session routes them to the Before hook, which restores through NodeRestorer.
// Carrier B (agent pull) cannot send coordinator blob frames yet (see the PR): its
// test pins the degrade path (no feed, a cold miss, no stall).

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// restoreNode is node B: it keeps its read-through copies between runs.
type restoreNode struct {
	t       *testing.T
	copies  *ReadThrough
	carrier bool // carrier A: the session offers the reverse blob feed
	flip    func(*protocol.Chunk)
	chunks  atomic.Int32
	noFeed  atomic.Bool
	atStart atomic.Bool // a verified copy was bound when the job started
	data    []byte
}

func newRestoreNode(t *testing.T, carrierA bool) *restoreNode {
	t.Helper()
	c, err := NewReadThrough(t.TempDir(), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	return &restoreNode{t: t, copies: c, carrier: carrierA}
}

type countingCarrier struct {
	session.Carrier
	n *restoreNode
}

func (c countingCarrier) Recv() (protocol.Message, error) {
	m, err := c.Carrier.Recv()
	if ch, ok := m.Body.(*protocol.Chunk); ok && err == nil && ch.Stream == StreamCache {
		c.n.chunks.Add(1)
		if c.n.flip != nil {
			c.n.flip(ch)
		}
	}
	return m, err
}

// carrierACounting keeps the stream carrier's restore capability.
type carrierACounting struct{ countingCarrier }

func (carrierACounting) BlobRestore() bool { return true }

func (n *restoreNode) tap(c session.Carrier) session.Carrier {
	if n.carrier {
		return carrierACounting{countingCarrier{c, n}}
	}
	return countingCarrier{c, n}
}

func (n *restoreNode) runner() session.Runner {
	return session.RunnerFunc(func(ctx context.Context, jr session.JobRun) (session.JobResult, error) {
		b := jr.Lease.Cache.Restore[0].Blobs[0]
		if f, err := n.copies.Open(ctx, jr.Lease.Cache.Namespace, b.SHA256, b.Size, nil); err == nil {
			_ = f.Close()
			n.atStart.Store(true)
		}
		return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: []byte(`{"schema":"test","ok":true}`)}, nil
	})
}

func (n *restoreNode) hooks() []session.JobHook {
	return []session.JobHook{{Name: "cache", Before: func(ctx context.Context, l protocol.Lease, _ session.JobRoot) (session.JobSetup, error) {
		feed, ok := session.FeedFrom(ctx)
		if !ok {
			n.noFeed.Store(true)
			return session.JobSetup{}, nil
		}
		defer feed.Close()
		var blobs []protocol.CacheBlob
		for _, r := range l.Cache.Restore {
			blobs = append(blobs, r.Blobs...)
		}
		(&NodeRestorer{Copies: n.copies, Feed: feed, Realm: l.Cache.Namespace, Wait: 10 * time.Second}).Fetch(ctx, blobs)
		return session.JobSetup{}, nil
	}}}
}

// planRestore stores data on the coordinator and plans it as the lease's restore.
func (r *rig) planRestore(data []byte) protocol.CacheBlob {
	r.t.Helper()
	sha, size, err := r.blobs.Put(context.Background(), cacheRealm(r.lease.b.Plan.Namespace), bytes.NewReader(data))
	if err != nil {
		r.t.Fatal(err)
	}
	b := protocol.CacheBlob{Path: "mod", SHA256: sha, Size: size}
	r.lease.b.Plan.Restore = []protocol.CacheRestore{{Spec: "go", Key: strings.Repeat("a", 64), Dir: "go", Blobs: []protocol.CacheBlob{b}}}
	r.lease.b.RestoreWait = 10 * time.Second
	return b
}

func (n *restoreNode) assertCopy(r *rig, b protocol.CacheBlob) {
	n.t.Helper()
	f, err := n.copies.Open(context.Background(), r.lease.b.Plan.Namespace, b.SHA256, b.Size, nil)
	if err != nil {
		n.t.Fatalf("node has no verified copy: %v", err)
	}
	_ = f.Close()
}

func TestRemoteCacheRestoreSSH(t *testing.T) {
	data := carrierCacheArchive(t)
	node := newRestoreNode(t, true)
	r1 := newRig(t)
	b := r1.planRestore(data)
	if err := r1.runSSH(node); err != nil {
		t.Fatal(err)
	}
	if node.noFeed.Load() || !node.atStart.Load() {
		t.Fatalf("node B did not hold a verified copy when the job started (noFeed=%v)", node.noFeed.Load())
	}
	if node.chunks.Load() < 2 {
		t.Fatalf("cold restore moved %d chunk frames", node.chunks.Load())
	}
	node.assertCopy(r1, b)
	rows, err := r1.st.CacheLocality(context.Background(), "n-1", r1.lease.b.Plan.Namespace, strings.Repeat("a", 64))
	if err != nil || len(rows) != 1 {
		t.Fatalf("locality=%+v err=%v", rows, err)
	}
	// The second run on node B, against a fresh coordinator state, uses its own copy.
	before := node.chunks.Load()
	node.atStart.Store(false)
	r2 := newRig(t)
	r2.planRestore(data)
	if err = r2.runSSH(node); err != nil {
		t.Fatal(err)
	}
	if !node.atStart.Load() || node.chunks.Load() != before {
		t.Fatalf("second run fetched again: %d chunk frames since the first", node.chunks.Load()-before)
	}
}

func TestRemoteCacheRestoreCorrupt(t *testing.T) {
	data := carrierCacheArchive(t)
	node := newRestoreNode(t, true)
	r1 := newRig(t)
	b := r1.planRestore(data)
	if err := r1.runSSH(node); err != nil {
		t.Fatal(err)
	}
	// A copy that rots on the node is discarded and fetched again.
	name, err := node.copies.name(r1.lease.b.Plan.Namespace, b.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(name)
	raw[11] ^= 0xff
	if err = os.WriteFile(name, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	before := node.chunks.Load()
	node.atStart.Store(false)
	r2 := newRig(t)
	r2.planRestore(data)
	if err = r2.runSSH(node); err != nil {
		t.Fatal(err)
	}
	if !node.atStart.Load() || node.chunks.Load() <= before {
		t.Fatalf("corrupt copy was not refetched (chunks %d -> %d)", before, node.chunks.Load())
	}
	// A byte flipped in transit fails the re-hash and the blob is sent again.
	if err = os.Remove(name); err != nil {
		t.Fatal(err)
	}
	node.flip = flipOnce()
	before = node.chunks.Load()
	node.atStart.Store(false)
	r3 := newRig(t)
	r3.planRestore(data)
	if err = r3.runSSH(node); err != nil {
		t.Fatal(err)
	}
	if !node.atStart.Load() {
		t.Fatal("job started without a verified copy after a flipped byte")
	}
	if got := node.chunks.Load() - before; got < 4 {
		t.Fatalf("a flipped byte did not cause a second pass: %d chunk frames", got)
	}
	node.assertCopy(r3, b)
	// Always flipped: the job still runs, cold, with nothing bound.
	if err = os.Remove(name); err != nil {
		t.Fatal(err)
	}
	node.flip = func(c *protocol.Chunk) {
		if c.Final {
			raw, _ := base64.StdEncoding.DecodeString(c.Data)
			raw[0] ^= 1
			c.Data = base64.StdEncoding.EncodeToString(raw)
		}
	}
	node.atStart.Store(false)
	r4 := newRig(t)
	r4.planRestore(data)
	if err = r4.runSSH(node); err != nil {
		t.Fatal(err)
	}
	if node.atStart.Load() {
		t.Fatal("a corrupt transfer was bound")
	}
}

// TODO: a real carrier-B restore test must replace this once agentapi can carry the
// reverse blob frames; until then this pins only the degrade path.
func TestRemoteCacheRestoreAgentDegradesToColdMiss(t *testing.T) {
	data := carrierCacheArchive(t)
	node := newRestoreNode(t, false)
	r := newRig(t)
	r.planRestore(data)
	start := time.Now()
	if err := r.runAgent(node); err != nil {
		t.Fatal(err)
	}
	if !node.noFeed.Load() || node.atStart.Load() || node.chunks.Load() != 0 {
		t.Fatalf("carrier B must degrade to a cold miss without a feed: noFeed=%v bound=%v chunks=%d", node.noFeed.Load(), node.atStart.Load(), node.chunks.Load())
	}
	if time.Since(start) > 20*time.Second {
		t.Fatal("the cold miss stalled the job")
	}
}
