package session

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// restoreCar is a fake carrier that, like carrier A, delivers coordinator blob frames.
type restoreCar struct{ *fakeCarrier }

func (c restoreCar) fake() *fakeCarrier { return c.fakeCarrier }
func (restoreCar) BlobRestore() bool    { return true }

func restoreLease(id string, epoch uint64) protocol.Lease {
	l := testLease(id, epoch)
	l.Cache = &protocol.LeaseCache{Namespace: "ns", Access: "ro", Restore: []protocol.CacheRestore{{Spec: "go", Key: "k", Dir: "go"}}}
	return l
}

// feedHook records what its Before hook sees of the reverse blob path.
type feedHook struct {
	got   chan []protocol.Digest
	noOne chan bool
}

func newFeedHook() (*feedHook, JobHook) {
	f := &feedHook{got: make(chan []protocol.Digest, 1), noOne: make(chan bool, 1)}
	return f, JobHook{Name: "feed", Before: func(ctx context.Context, l protocol.Lease, _ JobRoot) (JobSetup, error) {
		feed, ok := FeedFrom(ctx)
		if !ok {
			f.noOne <- true
			return JobSetup{}, nil
		}
		defer feed.Close()
		var seen []protocol.Digest
		for len(seen) < 2 {
			select {
			case fr := <-feed.Frames():
				if fr.Digest != nil {
					seen = append(seen, *fr.Digest)
					_ = feed.Offset(protocol.Offset{Stream: fr.Digest.Stream, SHA256: fr.Digest.SHA256, Offset: 0})
				}
			case <-time.After(5 * time.Second):
				seen = append(seen, protocol.Digest{SHA256: "timeout"})
			}
		}
		f.got <- seen
		return JobSetup{}, nil
	}}
}

func TestInboundBlobFramesReachTheHookFenced(t *testing.T) {
	f, hook := newFeedHook()
	car := restoreCar{newFakeCarrier()}
	r := startRigWith(t, t.TempDir(), car, okRunner(`{}`), func(c *Config) { c.Hooks = []JobHook{hook} })
	r.handshake()
	r.lease(restoreLease("l1", 5))
	dig := func(sha string) protocol.Digest { return protocol.Digest{Stream: "cache", SHA256: sha, Size: 3} }
	// none of these may reach the hook: another epoch, another lease
	r.car.pushFenced("digest", dig("stale"), 4, "l1")
	r.car.pushFenced("digest", dig("other"), 5, "l2")
	r.car.pushFenced("digest", dig("one"), 5, "l1")
	r.car.pushFenced("chunk", protocol.Chunk{Stream: "cache", Offset: 0, Data: "AAAA", Digest: "one", Final: true}, 5, "l1")
	r.car.pushFenced("digest", dig("two"), 5, "l1")
	select {
	case seen := <-f.got:
		if len(seen) != 2 || seen[0].SHA256 != "one" || seen[1].SHA256 != "two" {
			t.Fatalf("hook saw %+v", seen)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("hook never saw the frames")
	}
	off := r.car.next(t, "offset").Body.(protocol.Offset)
	if off.SHA256 != "one" {
		t.Fatalf("offset answer %+v", off)
	}
	if m := r.car.next(t, "offset"); m.LeaseID != "l1" || m.Epoch == nil || *m.Epoch != 5 {
		t.Fatalf("offset not fenced to the lease: %+v", m)
	}
}

func TestInboundNoFeedWithoutCarrierSupportOrPlan(t *testing.T) {
	f, hook := newFeedHook()
	r := startRig(t, okRunner(`{}`), func(c *Config) { c.Hooks = []JobHook{hook} }) // a carrier that cannot deliver blobs
	r.handshake()
	r.lease(restoreLease("l1", 1))
	if !<-f.noOne {
		t.Fatal("a feed was offered on a carrier without blob delivery")
	}
	f2, hook2 := newFeedHook()
	r2 := startRigWith(t, t.TempDir(), restoreCar{newFakeCarrier()}, okRunner(`{}`), func(c *Config) { c.Hooks = []JobHook{hook2} })
	r2.handshake()
	r2.lease(testLease("l2", 1)) // no cache plan
	if !<-f2.noOne {
		t.Fatal("a feed was offered to a lease without restore blobs")
	}
}
