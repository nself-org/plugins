//go:build integration

package remote

// Purpose: the cache restore over the real carriers (P7-CACHE-21): the node asks for a planned
// blob it lacks, the coordinator sends it over ssh stdio or the agent hub, the node re-hashes and
// publishes a copy, and the second run uses that copy with no chunk frame. Corrupt node copies,
// a flipped byte in transit, requests outside the plan and stale epochs run through the same path.

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

const restoreKey = "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0"

// restoreMod puts data in the coordinator's blob store and turns the lease plan into a restore plan.
func restoreMod(data []byte) func(*rig, *protocol.LeaseCache) {
	return func(r *rig, plan *protocol.LeaseCache) {
		sha, size, err := r.blobs.Put(context.Background(), cacheRealm(plan.Namespace), bytes.NewReader(data))
		if err != nil {
			r.t.Fatal(err)
		}
		plan.Save = nil
		plan.Restore = []protocol.CacheRestore{{Spec: "go", Key: restoreKey, Dir: "go", Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: sha, Size: size}}}}
	}
}

// probe watches the node side of one run: the frames it receives and what its hook obtained.
type probe struct {
	mu      sync.Mutex
	chunks  int
	digests int
	flip    int // chunk frames still to corrupt in transit
	got     []byte
	err     error
	strays  []string      // digests the hook asks for outside the plan
	skew    bool          // send the offsets with another epoch
	first   time.Duration // how long the hook waits for the coordinator's first announce (default 10 s)
}

func (p *probe) tap(c session.Carrier) session.Carrier {
	return &tapCarrier{Carrier: c, recv: func(m *protocol.Message) {
		p.mu.Lock()
		defer p.mu.Unlock()
		switch b := m.Body.(type) {
		case *protocol.Digest:
			p.digests++
		case *protocol.Chunk:
			p.chunks++
			if p.flip > 0 {
				p.flip--
				raw, _ := base64.StdEncoding.DecodeString(b.Data)
				raw[0] ^= 0xff
				b.Data = base64.StdEncoding.EncodeToString(raw)
			}
		}
	}, send: func(m *protocol.Message) {
		if _, ok := m.Body.(protocol.Offset); ok && p.skew && m.Epoch != nil {
			e := *m.Epoch + 1
			m.Epoch = &e
		}
	}}
}

type tapCarrier struct {
	session.Carrier
	recv, send func(*protocol.Message)
}

func (c *tapCarrier) Recv() (protocol.Message, error) {
	m, err := c.Carrier.Recv()
	if err == nil && c.recv != nil {
		c.recv(&m)
	}
	return m, err
}

func (c *tapCarrier) Send(m protocol.Message) error {
	if c.send != nil {
		c.send(&m)
	}
	return c.Carrier.Send(m)
}

// hooks restore every planned blob through the node copy store, as the agent's cache hook does.
func (p *probe) hooks(rt *ReadThrough) []session.JobHook {
	return []session.JobHook{{Name: "cache", BeforeTransfer: func(ctx context.Context, l protocol.Lease, _ session.JobRoot, tr session.Transfer) (session.JobSetup, error) {
		f := NewFetcher(Wire{Offset: tr.Up.Offset, In: tr.In}, l.Cache)
		f.First, f.Idle = 10*time.Second, 10*time.Second
		if p.first > 0 {
			f.First = p.first
		}
		for _, sha := range p.strays {
			_ = tr.Up.Offset(protocol.Offset{Stream: "cache", SHA256: sha})
		}
		for _, rs := range l.Cache.Restore {
			for _, b := range rs.Blobs {
				file, err := f.Open(ctx, rt, l.Cache.Namespace, b.SHA256, b.Size)
				p.mu.Lock()
				if err != nil {
					p.err = err
					p.mu.Unlock()
					continue
				}
				p.got, p.err = io.ReadAll(file)
				p.mu.Unlock()
				_ = file.Close()
			}
		}
		return session.JobSetup{}, nil
	}}}
}

type carrierRun func(*rig, *node) error

var carriers = map[string]carrierRun{
	"ssh":   func(r *rig, n *node) error { return r.runSSH(n) },
	"agent": func(r *rig, n *node) error { return r.runAgent(n) },
}

// restoreRun delivers a restore plan for data to a node whose copies live in rt.
func restoreRun(t *testing.T, run carrierRun, rt *ReadThrough, data []byte, p *probe) *rig {
	t.Helper()
	r := newRig(t, restoreMod(data))
	n := &node{t: t, data: data, tapFn: p.tap, hooksFn: func() []session.JobHook { return p.hooks(rt) }}
	if err := run(r, n); err != nil {
		t.Fatalf("delivery: %v", err)
	}
	return r
}

func newCopies(t *testing.T) *ReadThrough {
	t.Helper()
	rt, err := NewReadThrough(t.TempDir(), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func (p *probe) mustHave(t *testing.T, data []byte) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil || !bytes.Equal(p.got, data) {
		t.Fatalf("node did not restore the blob: err=%v, %d of %d B", p.err, len(p.got), len(data))
	}
}

func testRestore(t *testing.T, run carrierRun) {
	data := carrierCacheArchive(t)
	rt := newCopies(t)
	first := &probe{}
	r := restoreRun(t, run, rt, data, first)
	first.mustHave(t, data)
	if first.chunks < 2 {
		t.Fatalf("a cold node got %d chunk frames", first.chunks)
	}
	rows, err := r.st.CacheLocality(context.Background(), "n-1", r.lease.b.Plan.Namespace, restoreKey)
	if err != nil || len(rows) != 1 || rows[0].Bytes != int64(len(data)) {
		t.Fatalf("locality after the node confirmed: %+v err=%v", rows, err)
	}
	second := &probe{}
	restoreRun(t, run, rt, data, second) // a fresh coordinator store, the same node
	second.mustHave(t, data)
	if second.chunks != 0 || second.digests != 0 {
		t.Fatalf("a node with a valid copy got %d digest and %d chunk frames", second.digests, second.chunks)
	}
}

func TestRemoteCacheRestoreSSH(t *testing.T)   { testRestore(t, carriers["ssh"]) }
func TestRemoteCacheRestoreAgent(t *testing.T) { testRestore(t, carriers["agent"]) }

func TestRemoteCacheRestoreCorrupt(t *testing.T) {
	for name, run := range carriers {
		t.Run(name+"/node copy", func(t *testing.T) {
			data := carrierCacheArchive(t)
			rt := newCopies(t)
			first := &probe{}
			r := restoreRun(t, run, rt, data, first)
			sha := r.lease.b.Plan.Restore[0].Blobs[0].SHA256
			path, err := rt.name(r.lease.b.Plan.Namespace, sha)
			if err != nil {
				t.Fatal(err)
			}
			bad := append([]byte(nil), data...)
			bad[len(bad)/2] ^= 0xff
			if err = os.WriteFile(path, bad, 0o600); err != nil {
				t.Fatal(err)
			}
			again := &probe{}
			restoreRun(t, run, rt, data, again)
			again.mustHave(t, data)
			if again.chunks < 2 {
				t.Fatalf("a corrupt node copy was not fetched again (%d chunks)", again.chunks)
			}
			if disk, err := os.ReadFile(path); err != nil || !bytes.Equal(disk, data) {
				t.Fatalf("the replaced copy is not the coordinator blob: %v", err)
			}
		})
		t.Run(name+"/flipped byte in transit", func(t *testing.T) {
			data := carrierCacheArchive(t)
			p := &probe{flip: 1}
			restoreRun(t, run, newCopies(t), data, p)
			p.mustHave(t, data)
			if p.digests < 2 || p.chunks < 4 {
				t.Fatalf("a corrupt transfer was not sent again: %d digest, %d chunk frames", p.digests, p.chunks)
			}
		})
	}
}

func auditHas(t *testing.T, r *rig, detail string) bool {
	t.Helper()
	rows, err := r.st.AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Action == "cache.remote.refused" && row.Detail == detail {
			return true
		}
	}
	return false
}

func TestRemoteCacheRestoreRefusesADigestOutsideThePlan(t *testing.T) {
	for name, run := range carriers {
		t.Run(name, func(t *testing.T) {
			data := carrierCacheArchive(t)
			p := &probe{strays: []string{strings.Repeat("ab", 32)}}
			r := restoreRun(t, run, newCopies(t), data, p)
			p.mustHave(t, data) // the planned blob still arrives
			if !auditHas(t, r, "reason=restore_not_planned") {
				t.Fatal("no audit row for a request outside the plan")
			}
		})
	}
}

func TestRemoteCacheRestoreRefusesAStaleEpochSSH(t *testing.T) {
	data := carrierCacheArchive(t)
	p := &probe{skew: true}
	r := newRig(t, restoreMod(data))
	n := &node{t: t, data: data, tapFn: p.tap, hooksFn: func() []session.JobHook {
		return []session.JobHook{{Name: "cache", BeforeTransfer: func(_ context.Context, l protocol.Lease, _ session.JobRoot, tr session.Transfer) (session.JobSetup, error) {
			// ask for the planned blob under another epoch: nothing may be sent and an audit row is written
			_ = tr.Up.Offset(protocol.Offset{Stream: "cache", SHA256: l.Cache.Restore[0].Blobs[0].SHA256})
			return session.JobSetup{}, nil
		}}}
	}}
	if err := r.runSSH(n); err != nil {
		t.Fatal(err)
	}
	if !auditHas(t, r, "reason=restore_stale_epoch") {
		t.Fatal("no audit row for a stale epoch request")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.chunks != 0 || p.digests != 0 {
		t.Fatalf("a stale epoch request was served: %d digest, %d chunk frames", p.digests, p.chunks)
	}
}

// A blob the coordinator cannot read is a cold miss for the node, never a failed lease.
func TestRemoteCacheRestoreCoordinatorFailureDegradesToAColdMiss(t *testing.T) {
	for name, run := range carriers {
		t.Run(name, func(t *testing.T) {
			data := carrierCacheArchive(t)
			p := &probe{first: 400 * time.Millisecond}
			r := newRig(t, func(r *rig, plan *protocol.LeaseCache) {
				restoreMod(data)(r, plan)
				plan.Restore[0].Blobs[0].SHA256 = strings.Repeat("cd", 32) // planned, but absent from the coordinator store
			})
			n := &node{t: t, data: data, tapFn: p.tap, hooksFn: func() []session.JobHook { return p.hooks(newCopies(t)) }}
			if err := run(r, n); err != nil {
				t.Fatalf("a failed restore must not fail the lease: %v", err)
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			if !errors.Is(p.err, ErrNoRestore) || len(p.got) != 0 {
				t.Fatalf("node err=%v, bound %d B", p.err, len(p.got))
			}
		})
	}
}
