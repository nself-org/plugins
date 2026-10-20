package session

// Purpose: the coordinator's digest and chunk frames reach the BeforeTransfer hook of their lease
// and epoch only, the hook answers through the Uploader, and a hook that falls behind never
// blocks the session loop.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

var sha64 = strings.Repeat("ab", 32)

func transferHook(in chan<- []protocol.Message, ready chan<- struct{}, release <-chan struct{}, want int) JobHook {
	return JobHook{Name: "transfer", BeforeTransfer: func(ctx context.Context, l protocol.Lease, _ JobRoot, tr Transfer) (JobSetup, error) {
		if err := tr.Up.Offset(protocol.Offset{Stream: "cache", SHA256: sha64, Offset: 0}); err != nil { // the request
			return JobSetup{}, err
		}
		close(ready)
		<-release
		var got []protocol.Message
		for len(got) < want {
			select {
			case m := <-tr.In:
				got = append(got, m)
			case <-time.After(300 * time.Millisecond):
				in <- got
				return JobSetup{}, nil
			}
		}
		in <- got
		return JobSetup{}, nil
	}}
}

func TestInboundFramesReachTheHookOfTheirLeaseAndEpochOnly(t *testing.T) {
	in, ready, release := make(chan []protocol.Message, 1), make(chan struct{}), make(chan struct{})
	r := startRig(t, passRunner(), func(c *Config) { c.Hooks = []JobHook{transferHook(in, ready, release, 2)} })
	r.handshake()
	r.lease(testLease("l1", 3))
	<-ready
	if o := r.car.next(t, "offset").Body.(protocol.Offset); o.SHA256 != sha64 || o.Stream != "cache" {
		t.Fatalf("hook request %+v", o)
	}
	d := protocol.Digest{Stream: "cache", SHA256: sha64, Size: 4}
	c := protocol.Chunk{Stream: "cache", Offset: 0, Data: "AAAA", Digest: sha64, Final: true}
	r.car.pushFenced("digest", d, 2, "l1")                                                           // stale epoch
	r.car.pushFenced("digest", d, 3, "other")                                                        // another lease
	r.car.pushFenced("digest", protocol.Digest{Stream: "artifact", SHA256: sha64, Size: 4}, 3, "l1") // another stream
	r.car.pushFenced("chunk", protocol.Chunk{Stream: "artifact", Offset: 0, Data: "AAAA", Digest: sha64}, 3, "l1")
	r.car.pushFenced("digest", d, 3, "l1")
	r.car.pushFenced("chunk", c, 3, "l1")
	r.car.push("heartbeat", protocol.Heartbeat{Leases: []protocol.LeaseState{}})
	time.Sleep(100 * time.Millisecond)
	close(release)
	got := <-in
	if len(got) != 2 {
		t.Fatalf("hook received %d frames, want the 2 of its lease", len(got))
	}
	if _, ok := got[0].Body.(*protocol.Digest); !ok {
		t.Fatalf("first frame %T", got[0].Body)
	}
	if ch, ok := got[1].Body.(*protocol.Chunk); !ok || !ch.Final {
		t.Fatalf("second frame %T", got[1].Body)
	}
}

func TestInboundOverflowDropsWithoutBlockingTheLoop(t *testing.T) {
	in, ready, release := make(chan []protocol.Message, 1), make(chan struct{}), make(chan struct{})
	r := startRig(t, passRunner(), func(c *Config) { c.Hooks = []JobHook{transferHook(in, ready, release, 1000)} })
	r.handshake()
	r.lease(testLease("l1", 3))
	<-ready
	for i := 0; i < 3*inboundBuffer; i++ { // the hook is parked: nothing reads
		r.car.pushFenced("chunk", protocol.Chunk{Stream: "cache", Offset: int64(i), Data: "AAAA", Digest: sha64}, 3, "l1")
	}
	r.car.next(t, "offset")           // the hook's own request
	r.clock.advance(11 * time.Second) // a heartbeat tick proves the loop still runs behind the parked hook
	deadline := time.After(5 * time.Second)
	for beat := false; !beat; {
		select {
		case m := <-r.car.out:
			beat = m.Type == "heartbeat"
		case <-deadline:
			close(release)
			t.Fatal("the session loop stopped answering behind a parked hook")
		}
	}
	close(release)
	if got := <-in; len(got) != inboundBuffer {
		t.Fatalf("hook received %d frames, want the buffer of %d", len(got), inboundBuffer)
	}
}
