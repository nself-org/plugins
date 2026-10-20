package restore

// Purpose: stream planned cache blobs and wait for verified node confirmation.

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// countingUp counts the chunk frames sent, which tells a blob the node already held (none) from one that was sent.
type countingUp struct {
	transfer.Uploader
	n *atomic.Int64
}

func (c countingUp) Chunk(b protocol.Chunk) error {
	c.n.Add(1)
	return c.Uploader.Chunk(b)
}

func (p *Pusher) resumes() int {
	if p.Resumes > 0 {
		return p.Resumes
	}
	return transfer.MaxResumes
}

// maxAsks is how many times one blob may be requested: every send of it, with the node's own retries.
func (p *Pusher) maxAsks() int { return 2 * (p.resumes() + 1) }

// confirm is how long the node gets to hash and publish a blob of size bytes.
func (p *Pusher) confirm(size int64) time.Duration {
	if p.Confirm > 0 {
		return p.Confirm
	}
	return DefaultConfirm + time.Duration(size/confirmRate)*time.Second
}

// push sends b until the node confirms a verified copy or the attempts run out.
func (p *Pusher) push(ctx context.Context, b transport.RestoreBlob, answers chan protocol.Offset) {
	up := p.up
	if cu, ok := up.(interface {
		WithContext(context.Context) transfer.Uploader
	}); ok {
		up = cu.WithContext(ctx) // a send that waits for room ends with this transfer, not with the run
	}
	s := transfer.Sender{Up: countingUp{up, &p.chunks}, Offsets: answers, WaitOffset: p.Wait, MaxResumes: p.Resumes}
	ev := context.WithoutCancel(ctx) // an event about a cancelled transfer is still recorded
	fail := func(why string) {
		p.src.RestoreEvent(ev, transport.RestoreEvent{Kind: transport.RestoreFailed, SHA256: b.SHA256, Reason: why})
	}
	for try := 0; try <= p.resumes(); try++ {
		before := p.chunks.Load()
		err := s.Send(ctx, transfer.Blob{Stream: Stream, SHA256: b.SHA256, Size: b.Size, Open: b.Open})
		if err != nil {
			fail(err.Error())
			return
		}
		if p.chunks.Load() == before { // the node already held every byte
			p.src.RestoreEvent(ev, transport.RestoreEvent{Kind: transport.RestoreCopied, SHA256: b.SHA256, Bytes: b.Size})
			return
		}
		switch p.awaitConfirm(ctx, answers, b) {
		case confirmed:
			p.src.RestoreEvent(ev, transport.RestoreEvent{Kind: transport.RestoreCopied, SHA256: b.SHA256, Bytes: b.Size})
			return
		case again:
			continue
		default:
			fail("the node did not confirm the copy")
			return
		}
	}
	fail("the node rejected the blob every time")
}

// verdict is the outcome of waiting for the node's confirmation.
type verdict int

const (
	gaveUp verdict = iota
	confirmed
	again
)

// lastAnswer looks once at the answers already queued for a confirmation.
func lastAnswer(answers <-chan protocol.Offset, b transport.RestoreBlob) verdict {
	for {
		select {
		case o := <-answers:
			if o.SHA256 == b.SHA256 && o.Offset == b.Size {
				return confirmed
			}
		default:
			return gaveUp
		}
	}
}

// awaitConfirm waits for the node's position after the final chunk: the size means it published
// a verified copy, anything less means its re-hash failed and it wants the blob again.
func (p *Pusher) awaitConfirm(ctx context.Context, answers <-chan protocol.Offset, b transport.RestoreBlob) verdict {
	timer := time.NewTimer(p.confirm(b.Size))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return lastAnswer(answers, b) // a confirmation that arrived just before the cancel still counts
		case <-timer.C:
			return gaveUp
		case o := <-answers:
			if o.SHA256 != b.SHA256 {
				continue
			}
			if o.Offset == b.Size {
				return confirmed
			}
			if lastAnswer(answers, b) == confirmed { // a position short of the size, then the confirmation: the copy is there
				return confirmed
			}
			return again
		}
	}
}
