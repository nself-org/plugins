package remote

// Purpose: watch the node's offset answers while the coordinator pushes a restore
// blob, so the sender can tell "the node holds every byte" from "the node threw
// the bytes away" after the final chunk (P7-CACHE-21).
// Inputs:  the lease's offset channel.
// Outputs: a copy of every answer for the transfer.Sender, and the newest
// position per blob with a count that orders the answers.
// Constraints: forwarding never blocks (a full channel drops the oldest answer);
// the tee stops with its context or when the input closes.

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

type seenOffset struct {
	last int64
	n    int
}

type offsetTee struct {
	out     chan protocol.Offset
	mu      sync.Mutex
	seen    map[string]seenOffset
	changed chan struct{}
}

func newOffsetTee(ctx context.Context, in <-chan protocol.Offset) *offsetTee {
	t := &offsetTee{out: make(chan protocol.Offset, 64), seen: map[string]seenOffset{}, changed: make(chan struct{})}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case o, ok := <-in:
				if !ok {
					return
				}
				t.note(o)
				t.forward(o)
			}
		}
	}()
	return t
}

func (t *offsetTee) note(o protocol.Offset) {
	t.mu.Lock()
	s := t.seen[o.SHA256]
	t.seen[o.SHA256] = seenOffset{last: o.Offset, n: s.n + 1}
	close(t.changed)
	t.changed = make(chan struct{})
	t.mu.Unlock()
}

func (t *offsetTee) forward(o protocol.Offset) {
	for {
		select {
		case t.out <- o:
			return
		default:
		}
		select {
		case <-t.out:
		default:
		}
	}
}

func (t *offsetTee) count(sha string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.seen[sha].n
}

// after waits for an answer about sha newer than count since and returns its offset.
func (t *offsetTee) after(ctx context.Context, sha string, since int, wait time.Duration) (int64, error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		t.mu.Lock()
		s, ch := t.seen[sha], t.changed
		t.mu.Unlock()
		if s.n > since {
			return s.last, nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return 0, transfer.Failf(transfer.ReasonInterrupted, "cancelled while waiting for the node to confirm blob %s", sha[:12])
		case <-timer.C:
			return 0, transfer.Failf(transfer.ReasonInterrupted, "the node did not confirm blob %s within %s", sha[:12], wait)
		}
	}
}

// finalSnap wraps the RestoreLink as a transfer.Uploader and remembers how many
// answers the node had given when the final chunk left.
type finalSnap struct {
	link  transport.RestoreLink
	tee   *offsetTee
	mu    sync.Mutex
	since int
	sent  bool
}

func (f *finalSnap) reset() { f.mu.Lock(); f.sent = false; f.mu.Unlock() }

func (f *finalSnap) final() (int, bool) { f.mu.Lock(); defer f.mu.Unlock(); return f.since, f.sent }

func (f *finalSnap) Digest(d protocol.Digest) error { return f.link.Digest(d) }

func (f *finalSnap) Chunk(c protocol.Chunk) error {
	if c.Final {
		f.mu.Lock()
		f.since, f.sent = f.tee.count(c.Digest), true
		f.mu.Unlock()
	}
	return f.link.Chunk(c)
}
