package restore

// Purpose: the coordinator pusher against a simulated node: request, announce, chunks, confirmation,
// a node that already holds the blob, a re-hash failure, and every refusal.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

type src struct {
	blobs  []transport.RestoreBlob
	events chan transport.RestoreEvent
}

func (s *src) RestoreBlobs(context.Context) ([]transport.RestoreBlob, error) { return s.blobs, nil }
func (s *src) RestoreEvent(_ context.Context, e transport.RestoreEvent)      { s.events <- e }

// simNode answers the way a node does: the position after an announce, the confirmation after a
// whole blob. badFinals counts the finals it rejects (a re-hash failure) before it accepts one.
type simNode struct {
	p         *Pusher
	size      int64
	held      bool
	silent    bool
	noConfirm bool
	badFinals int
	onChunk   func() // called once, after the first chunk

	mu     sync.Mutex
	got    bytes.Buffer
	chunks int
}

func (n *simNode) Digest(d protocol.Digest) error {
	if n.silent {
		return nil
	}
	off := int64(0)
	if n.held {
		off = n.size
	}
	n.p.Offer(context.Background(), protocol.Offset{Stream: Stream, SHA256: d.SHA256, Offset: off})
	return nil
}

func (n *simNode) Chunk(c protocol.Chunk) error {
	n.mu.Lock()
	n.chunks++
	if f := n.onChunk; f != nil && n.chunks == 1 {
		n.onChunk = nil
		n.mu.Unlock()
		f()
		n.mu.Lock()
	}
	defer n.mu.Unlock()
	data, _ := base64.StdEncoding.DecodeString(c.Data)
	if c.Offset == 0 {
		n.got.Reset()
	}
	n.got.Write(data)
	if !c.Final || n.noConfirm {
		return nil
	}
	pos := n.size
	if n.badFinals > 0 {
		n.badFinals--
		pos = 0
	}
	n.p.Offer(context.Background(), protocol.Offset{Stream: Stream, SHA256: c.Digest, Offset: pos})
	return nil
}

var _ transfer.Uploader = (*simNode)(nil)

func blobOf(data []byte) (transport.RestoreBlob, string) {
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	return transport.RestoreBlob{SHA256: sha, Size: int64(len(data)), Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }}, sha
}

type rig struct {
	p    *Pusher
	n    *simNode
	s    *src
	sha  string
	data []byte
}

func newRig(t *testing.T, tune func(*simNode, *Pusher)) *rig {
	t.Helper()
	data := bytes.Repeat([]byte("0123456789abcdef"), 100000) // 1.6 MB: four chunks
	b, sha := blobOf(data)
	s := &src{blobs: []transport.RestoreBlob{b}, events: make(chan transport.RestoreEvent, 256)}
	n := &simNode{size: b.Size}
	p, err := New(context.Background(), s, n)
	if err != nil {
		t.Fatal(err)
	}
	p.Wait, p.Confirm = 200*time.Millisecond, 200*time.Millisecond
	n.p = p
	if tune != nil {
		tune(n, p)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return &rig{p: p, n: n, s: s, sha: sha, data: data}
}

func (r *rig) request(sha string) {
	r.p.Offer(context.Background(), protocol.Offset{Stream: Stream, SHA256: sha, Offset: 0})
}

func (r *rig) event(t *testing.T) transport.RestoreEvent {
	t.Helper()
	select {
	case e := <-r.s.events:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("no restore event")
		return transport.RestoreEvent{}
	}
}

func TestPushSendsAndConfirms(t *testing.T) {
	r := newRig(t, nil)
	r.request(r.sha)
	e := r.event(t)
	if e.Kind != transport.RestoreCopied || e.SHA256 != r.sha || e.Bytes != int64(len(r.data)) {
		t.Fatalf("event %+v", e)
	}
	if !bytes.Equal(r.n.got.Bytes(), r.data) || r.n.chunks != 4 {
		t.Fatalf("node got %d B in %d chunks", r.n.got.Len(), r.n.chunks)
	}
}

func TestPushHeldCopySendsNoChunk(t *testing.T) {
	r := newRig(t, func(n *simNode, _ *Pusher) { n.held = true })
	r.request(r.sha)
	if e := r.event(t); e.Kind != transport.RestoreCopied {
		t.Fatalf("event %+v", e)
	}
	if r.n.chunks != 0 {
		t.Fatalf("a held blob cost %d chunks", r.n.chunks)
	}
}

func TestPushSendsAgainAfterRehashFailure(t *testing.T) {
	r := newRig(t, func(n *simNode, _ *Pusher) { n.badFinals = 1 })
	r.request(r.sha)
	if e := r.event(t); e.Kind != transport.RestoreCopied {
		t.Fatalf("event %+v", e)
	}
	if r.n.chunks != 8 {
		t.Fatalf("expected the blob twice (8 chunks), got %d", r.n.chunks)
	}
	if !bytes.Equal(r.n.got.Bytes(), r.data) {
		t.Fatal("second pass did not restart at zero")
	}
}

func TestPushGivesUpWhenTheNodeKeepsRejecting(t *testing.T) {
	r := newRig(t, func(n *simNode, p *Pusher) { n.badFinals = 100; p.Resumes = 1 })
	r.request(r.sha)
	if e := r.event(t); e.Kind != transport.RestoreFailed {
		t.Fatalf("event %+v", e)
	}
	if r.n.chunks != 8 {
		t.Fatalf("expected 2 passes, got %d chunks", r.n.chunks)
	}
}

func TestPushFailsWhenTheNodeIsSilent(t *testing.T) {
	r := newRig(t, func(n *simNode, p *Pusher) { n.silent = true; p.Wait = 30 * time.Millisecond })
	r.request(r.sha)
	if e := r.event(t); e.Kind != transport.RestoreFailed {
		t.Fatalf("event %+v", e)
	}
}

func TestPushFailsWithoutConfirmation(t *testing.T) {
	r := newRig(t, func(n *simNode, p *Pusher) { n.noConfirm = true; p.Confirm = 30 * time.Millisecond })
	r.request(r.sha)
	e := r.event(t)
	if e.Kind != transport.RestoreFailed || !strings.Contains(e.Reason, "confirm") {
		t.Fatalf("event %+v", e)
	}
}

func TestOfferRefusals(t *testing.T) {
	r := newRig(t, nil)
	other := strings.Repeat("ab", 32)
	cases := []struct {
		name string
		o    protocol.Offset
		want string
	}{
		{"digest outside the plan", protocol.Offset{Stream: Stream, SHA256: other}, ReasonNotPlanned},
		{"position past the size", protocol.Offset{Stream: Stream, SHA256: r.sha, Offset: int64(len(r.data)) + 1}, ReasonBadOffset},
	}
	for _, c := range cases {
		r.p.Offer(context.Background(), c.o)
		e := r.event(t)
		if e.Kind != transport.RestoreRefused || e.Reason != c.want {
			t.Errorf("%s: event %+v, want %s", c.name, e, c.want)
		}
	}
	for i := 0; i < 50; i++ { // a flood of the same refusal is reported once
		r.p.Offer(context.Background(), protocol.Offset{Stream: Stream, SHA256: other})
		r.p.Offer(context.Background(), protocol.Offset{Stream: Stream, SHA256: r.sha, Offset: -1})
	}
	r.p.Offer(context.Background(), protocol.Offset{Stream: "artifact", SHA256: other}) // another stream is not ours
	if r.n.chunks != 0 || len(r.s.events) != 0 {
		t.Fatalf("a refusal sent %d chunks, %d extra events", r.n.chunks, len(r.s.events))
	}
	if !r.p.Planned() {
		t.Fatal("refusals emptied the plan")
	}
}

func TestOfferAfterCloseIsRefusedAsCancelled(t *testing.T) {
	r := newRig(t, nil)
	r.p.Close()
	r.request(r.sha)
	if e := r.event(t); e.Kind != transport.RestoreRefused || e.Reason != ReasonCancelled {
		t.Fatalf("event %+v", e)
	}
	if r.n.chunks != 0 {
		t.Fatal("a cancelled lease was served")
	}
}

func TestOpenErrorIsAFailureNotAPanic(t *testing.T) {
	r := newRig(t, nil)
	r.p.blobs[r.sha] = transport.RestoreBlob{SHA256: r.sha, Size: int64(len(r.data)), Open: func() (io.ReadCloser, error) { return nil, io.ErrClosedPipe }}
	r.request(r.sha)
	if e := r.event(t); e.Kind != transport.RestoreFailed {
		t.Fatalf("event %+v", e)
	}
}

func TestCloseStopsTheTransferInFlight(t *testing.T) {
	r := newRig(t, func(n *simNode, p *Pusher) { n.noConfirm = true })
	started := make(chan struct{})
	r.n.onChunk = func() { close(started); r.p.Close() }
	r.request(r.sha)
	<-started
	if e := r.event(t); e.Kind != transport.RestoreFailed {
		t.Fatalf("event %+v", e)
	}
	r.n.mu.Lock()
	defer r.n.mu.Unlock()
	if r.n.chunks > 2 {
		t.Fatalf("%d chunks went out after the cancel", r.n.chunks)
	}
}

func TestLateConfirmationAndUpperCaseRequests(t *testing.T) {
	r := newRig(t, nil)
	r.p.Offer(context.Background(), protocol.Offset{Stream: Stream, SHA256: r.sha, Offset: int64(len(r.data))}) // nothing runs: a late confirmation, ignored
	if len(r.p.queue) != 0 || r.n.chunks != 0 {
		t.Fatal("a late confirmation started a transfer")
	}
	r.request(strings.ToUpper(r.sha))
	if e := r.event(t); e.Kind != transport.RestoreCopied || e.SHA256 != r.sha {
		t.Fatalf("an upper-case request was not served: %+v", e)
	}
}

func TestRepeatedRequestsForOneBlobAreRefused(t *testing.T) {
	r := newRig(t, func(_ *simNode, p *Pusher) { p.Resumes = 1 })
	for i := 0; i < 2*(1+1); i++ { // every allowed request is served
		r.request(r.sha)
		if e := r.event(t); e.Kind != transport.RestoreCopied {
			t.Fatalf("request %d: %+v", i, e)
		}
	}
	r.request(r.sha)
	if e := r.event(t); e.Kind != transport.RestoreRefused || e.Reason != ReasonRepeated {
		t.Fatalf("a fifth request was not refused: %+v", e)
	}
}

func TestRefusalTableStaysBoundedAndNeverKeysOnNodeStrings(t *testing.T) {
	r := newRig(t, nil)
	for i := 0; i < 5000; i++ {
		r.p.Refuse(context.Background(), strings.Repeat("x", 4096)+string(rune('a'+i%26))+strings.Repeat("y", i), ReasonNotPlanned)
		r.p.Refuse(context.Background(), hex.EncodeToString([]byte{byte(i), byte(i >> 8), 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30}), ReasonNotPlanned)
	}
	r.p.mu.Lock()
	n := len(r.p.refused)
	r.p.mu.Unlock()
	if n > maxRefused {
		t.Fatalf("%d refusals remembered, the bound is %d", n, maxRefused)
	}
	if got := len(r.s.events); got > maxRefused+1 {
		t.Fatalf("%d events for a flood", got)
	}
}

func TestRequestForAnotherBlobEndsTheRunningTransfer(t *testing.T) {
	first, second := bytes.Repeat([]byte("first blob "), 200000), []byte("second blob")
	b1, sha1 := blobOf(first)
	b2, sha2 := blobOf(second)
	s := &src{blobs: []transport.RestoreBlob{b1, b2}, events: make(chan transport.RestoreEvent, 16)}
	n := &simNode{size: b1.Size, noConfirm: true}
	p, err := New(context.Background(), s, n)
	if err != nil {
		t.Fatal(err)
	}
	p.Wait, p.Confirm = 5*time.Second, 5*time.Second
	n.p = p
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	p.Offer(ctx, protocol.Offset{Stream: Stream, SHA256: sha1})
	deadline := time.After(5 * time.Second)
	for { // the first transfer is out and waiting for its confirmation
		n.mu.Lock()
		c := n.chunks
		n.mu.Unlock()
		if c > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the first transfer never started")
		case <-time.After(5 * time.Millisecond):
		}
	}
	start := time.Now()
	p.Offer(ctx, protocol.Offset{Stream: Stream, SHA256: sha2}) // the node gave up on the first blob and asks for another
	select {
	case e := <-s.events:
		if e.SHA256 != sha1 || e.Kind != transport.RestoreFailed {
			t.Fatalf("event %+v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("the abandoned transfer held the pusher for %s", time.Since(start))
	}
}
