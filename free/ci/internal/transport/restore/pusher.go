// Package restore is the coordinator side of the reverse blob path (Epic CACHE D7, P7-CACHE-21).
package restore

// Purpose: send the cache blobs of a lease plan to its node, carrier neutral. The
// node asks with an `offset` frame for a blob it lacks (the request names a digest of
// the plan and the byte it wants next); the pusher then announces the blob (`digest`),
// learns the node's position from its `offset` answer, streams `chunk{stream: cache}`
// frames and waits for the node's confirmation, an `offset` equal to the size sent
// after the node re-hashed and published its copy.
// Inputs:  a transport.Restorer (the plan), a transfer.Uploader (the carrier's frame
// sender) and the node's offset frames, fenced to the lease and epoch by the driver.
// Outputs: digest and chunk frames; transport.RestoreEvent calls.
// Constraints: only digests of the plan are ever sent (anything else is refused with
// an event, never answered); one blob at a time; a node that reports a position short
// of the size after the final chunk (its re-hash failed) gets the blob again, up to
// MaxResumes times; a failure never fails the lease: the node degrades to a cold miss;
// after Close every offer is refused as cancelled.

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Stream is the protocol stream of cache blobs.
const Stream = "cache"

// Refusal reasons carried by RestoreRefused events.
const (
	ReasonNotPlanned = "not_planned"
	ReasonStaleEpoch = "stale_epoch"
	ReasonCancelled  = "cancelled"
	ReasonBadOffset  = "bad_offset"
	ReasonDenied     = "denied"
)

// Defaults of the pusher.
const (
	DefaultConfirm = 20 * time.Second
	// confirmRate is the node's assumed hash and publish speed, which stretches the confirmation wait of a big blob.
	confirmRate = 20 << 20
	queueLen    = 64
	answerLen   = 64
	// ReasonRepeated refuses a node that asks for the same blob again and again.
	ReasonRepeated = "repeated"
)

// Pusher serves the restore requests of one lease.
type Pusher struct {
	src   transport.Restorer
	up    transfer.Uploader
	blobs map[string]transport.RestoreBlob

	// Wait bounds the node's answer to an announce; Confirm the node's confirmation after the
	// final chunk; Resumes how often one blob is sent again. Zero values use the defaults.
	Wait    time.Duration
	Confirm time.Duration
	Resumes int

	mu       sync.Mutex
	active   map[string]chan protocol.Offset // digest -> answers of the running transfer
	queued   map[string]bool
	refused  map[string]bool    // reason+digest pairs already reported: a flood costs one event each
	asked    map[string]int     // requests accepted per digest
	byReason map[string]int     // refusals remembered per reason
	stop     context.CancelFunc // ends the running transfer
	closed   bool
	queue    chan string
	chunks   atomic.Int64
}

// maxRefused bounds the refusals remembered per reason: a flood of one reason cannot hide another.
const maxRefused = 16

// New loads the plan of src and returns a pusher for it.
func New(ctx context.Context, src transport.Restorer, up transfer.Uploader) (*Pusher, error) {
	list, err := src.RestoreBlobs(ctx)
	if err != nil {
		return nil, err
	}
	p := &Pusher{src: src, up: up, blobs: map[string]transport.RestoreBlob{}, active: map[string]chan protocol.Offset{},
		queued: map[string]bool{}, refused: map[string]bool{}, byReason: map[string]int{}, asked: map[string]int{}, queue: make(chan string, queueLen)}
	for _, b := range list {
		if b.Open != nil && b.Size > 0 {
			p.blobs[strings.ToLower(b.SHA256)] = b
		}
	}
	return p, nil
}

// Planned reports whether the plan holds any blob.
func (p *Pusher) Planned() bool { return len(p.blobs) > 0 }

// Close ends the pusher: the transfer in flight stops before its next chunk and later offers are
// refused as cancelled.
func (p *Pusher) Close() {
	p.mu.Lock()
	p.closed = true
	stop := p.stop
	p.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// Refuse reports a refused request to the Restorer (a frame the driver fenced out). The same
// reason for the same digest is reported once, so a node that floods requests costs one event.
func (p *Pusher) Refuse(ctx context.Context, sha, reason string) {
	if !sha256Hex.MatchString(sha) {
		sha = "" // a node-made string is never a map key
	}
	key := reason + "/" + sha
	p.mu.Lock()
	seen := p.refused[key]
	if !seen && p.byReason[reason] < maxRefused {
		p.refused[key] = true
		p.byReason[reason]++
	} else if !seen {
		seen = true // this reason has said enough
	}
	p.mu.Unlock()
	if !seen {
		p.src.RestoreEvent(ctx, transport.RestoreEvent{Kind: transport.RestoreRefused, SHA256: sha, Reason: reason})
	}
}

// Offer takes one offset frame of the node for the lease (already fenced to its lease and epoch).
// An offset for a blob whose transfer runs is that transfer's answer; any other is a request.
func (p *Pusher) Offer(ctx context.Context, o protocol.Offset) {
	if o.Stream != Stream {
		return
	}
	sha := strings.ToLower(o.SHA256)
	o.SHA256 = sha
	b, ok := p.blobs[sha]
	switch {
	case !ok:
		p.Refuse(ctx, sha, ReasonNotPlanned)
		return
	case o.Offset < 0 || o.Offset > b.Size:
		p.Refuse(ctx, sha, ReasonBadOffset)
		return
	}
	if reason := p.route(sha, b.Size, o); reason != "" {
		p.Refuse(ctx, sha, reason)
	}
}

// route sends the offset to the running transfer of its blob or queues it as a request. It returns
// the reason to refuse it with, or "" when it was taken or is harmless to drop.
func (p *Pusher) route(sha string, size int64, o protocol.Offset) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ReasonCancelled
	}
	if ch := p.active[sha]; ch != nil {
		for {
			select {
			case ch <- o:
				return ""
			default:
			}
			select { // full: drop the oldest answer
			case <-ch:
			default:
			}
		}
	}
	switch {
	case p.queued[sha] || o.Offset == size: // asked already, or a late confirmation: nothing to send
		return ""
	}
	if p.stop != nil { // the node fetches one blob at a time: a request for another digest means it gave up on the running one
		p.stop()
	}
	switch {
	case p.asked[sha] >= p.maxAsks():
		return ReasonRepeated
	}
	select {
	case p.queue <- sha:
		p.queued[sha] = true
		p.asked[sha]++
	default: // the node asks again after its idle wait
	}
	return ""
}

// Run serves the requests, one blob at a time, until ctx ends.
func (p *Pusher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case sha := <-p.queue:
			tctx, cancel := context.WithCancel(ctx)
			p.mu.Lock()
			delete(p.queued, sha)
			ch := make(chan protocol.Offset, answerLen)
			p.active[sha] = ch
			p.stop = cancel
			closed := p.closed
			p.mu.Unlock()
			if !closed {
				p.push(tctx, p.blobs[sha], ch)
			}
			p.mu.Lock()
			delete(p.active, sha)
			p.stop = nil
			p.mu.Unlock()
			cancel()
		}
	}
}

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
			return again
		}
	}
}
