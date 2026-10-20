package agent

// Purpose: agent-to-coordinator blobs over carrier B (artifact and cache streams, CACHE delta 5): `digest`
//          announces a blob, the coordinator answers with an `offset` frame naming the bytes the sink already
//          holds, `chunk` frames continue from there, and the blob is verified by digest before it is
//          committed. It is agentapi.BlobHandler.
// Inputs:  digest and chunk frames of a lease (envelope lease_id and epoch), routed to the run's Sink.Blob.
// Outputs: offset frames; bytes in the sink's BlobWriter; an error that cancels the lease and fails Run with
//          E656 transport.agent_blob when a blob is corrupt.
// Constraints: a blob that starts at offset 0 is hashed here; a resumed blob must be verified by the sink on
//          Close (it implements Verifier), because the bytes before the offset were never seen by this
//          process; a corrupt, oversized or unverifiable blob fails closed: the writer is aborted, never
//          committed; a chunk out of order is answered with the expected offset (at most maxResync times); a
//          lease may open maxLeaseBlobs blobs of maxLeaseBytes in total; a digest is 64 lower-case hex.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"hash"
	"regexp"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// Blob limits.
const (
	maxResync     = 3
	maxBlob       = 4 << 30
	maxLeaseBlobs = 1024
	maxLeaseBytes = 16 << 30
)

// ReasonBlob is the E656 reason of a refused blob.
const ReasonBlob = "transport.agent_blob"

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Aborter is implemented by a BlobWriter that can discard what it holds.
type Aborter interface{ Abort() error }

// Verifier is implemented by a BlobWriter that can check the sha256 of everything it holds (hex, lower
// case) before the blob is committed; Close commits.
type Verifier interface{ Verify(wantSHA256 string) error }

type blobState struct {
	head    transport.BlobHeader
	w       transport.BlobWriter
	next    int64
	h       hash.Hash // nil for a resumed blob
	resyncs int
}

// blobSet is the blob state of one lease.
type blobSet struct {
	mu     sync.Mutex
	active map[string]*blobState // by stream
	count  int
	bytes  int64
}

// blobError is a refused blob: it cancels the lease on the agent and fails the run.
type blobError struct{ e *Error }

func (b blobError) Error() string { return b.e.Error() }
func (b blobError) Unwrap() error { return b.e }
func (blobError) Refusal() bool   { return true }

func badBlob(format string, a ...any) error { return blobError{fail(ReasonBlob, format, a...)} }

// Blobs returns the agentapi.BlobHandler of the driver.
func (d *Driver) Blobs() agentapi.BlobHandler { return blobHandler{d} }

type blobHandler struct{ d *Driver }

// blobSetFor returns the blob set of a route, creating it on first use; nil once the route was released.
func (d *Driver) blobSetFor(rt *route) *blobSet {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.routes[rt.leaseID] != rt {
		return nil
	}
	if rt.blobs == nil {
		rt.blobs = &blobSet{active: map[string]*blobState{}}
	}
	return rt.blobs
}

// Frame handles one digest or chunk frame of a lease.
func (h blobHandler) Frame(ctx context.Context, node string, m protocol.Message) ([]protocol.Message, error) {
	rt, late, err := (*hubSink)(h.d).route(m.LeaseID)
	if err != nil {
		return nil, err
	}
	if late || rt.node != node {
		return nil, nil
	}
	bs := h.d.blobSetFor(rt)
	if bs == nil { // the run ended since the route was found: nothing may open a writer in the sink now
		return nil, nil
	}
	bs.mu.Lock()
	defer bs.mu.Unlock()
	var out []protocol.Message
	switch b := m.Body.(type) {
	case *protocol.Digest:
		out, err = bs.onDigest(ctx, rt, *b, m)
	case *protocol.Chunk:
		err = bs.onChunk(*b)
	}
	if err != nil {
		bs.discardLocked()
		rt.fail(err)
	}
	return out, err
}

func offsetFrame(m protocol.Message, st *blobState) protocol.Message {
	ep := *m.Epoch
	return protocol.Message{Type: "offset", LeaseID: m.LeaseID, Epoch: &ep, Body: protocol.Offset{Stream: st.head.Stream, SHA256: st.head.SHA256, Offset: st.next}}
}

func (bs *blobSet) onDigest(ctx context.Context, rt *route, d protocol.Digest, m protocol.Message) ([]protocol.Message, error) {
	if d.Stream != "artifact" && d.Stream != "cache" {
		return nil, badBlob("digest frame for stream %q", d.Stream)
	}
	head := transport.BlobHeader{Stream: d.Stream, SHA256: strings.ToLower(strings.TrimPrefix(d.SHA256, "sha256:")), Size: d.Size}
	if !sha256Hex.MatchString(head.SHA256) || head.Size < 1 || head.Size > maxBlob {
		return nil, badBlob("the %s blob announcement is not a sha256 with a size from 1 to %d bytes", d.Stream, int64(maxBlob))
	}
	if old := bs.active[d.Stream]; old != nil {
		if old.head.SHA256 == head.SHA256 && old.head.Size == head.Size { // the same blob announced again after a reconnect
			return []protocol.Message{offsetFrame(m, old)}, nil
		}
		old.discard()
		delete(bs.active, d.Stream)
	}
	if bs.count+1 > maxLeaseBlobs || bs.bytes+head.Size > maxLeaseBytes {
		return nil, badBlob("the lease reached its blob budget; the %s blob was refused", d.Stream)
	}
	bs.count++
	bs.bytes += head.Size
	w, err := rt.sink.Blob(ctx, head)
	if err != nil {
		return nil, badBlob("the %s blob %s was refused", d.Stream, short(head.SHA256))
	}
	st := &blobState{head: head, w: w, next: w.Offset()}
	if _, ok := w.(Aborter); !ok {
		return nil, badBlob("the sink cannot discard a %s blob; refusing to stream into it", d.Stream)
	}
	if st.next == 0 {
		st.h = sha256.New()
	} else if _, ok := w.(Verifier); !ok {
		st.discard()
		return nil, badBlob("the sink holds %d bytes of the %s blob but cannot verify it; refusing to resume", st.next, d.Stream)
	}
	if st.next == head.Size { // the sink already holds every byte: verify and commit
		if err = st.commit(); err != nil {
			return nil, err
		}
		return []protocol.Message{offsetFrame(m, st)}, nil
	}
	bs.active[d.Stream] = st
	return []protocol.Message{offsetFrame(m, st)}, nil
}

func (st *blobState) commit() error {
	if v, ok := st.w.(Verifier); ok {
		if err := v.Verify(st.head.SHA256); err != nil {
			st.discard()
			return badBlob("%s blob %s rejected: the stored bytes do not match the digest", st.head.Stream, short(st.head.SHA256))
		}
	}
	if err := st.w.Close(); err != nil {
		return badBlob("the %s blob %s failed verification in the sink", st.head.Stream, short(st.head.SHA256))
	}
	return nil
}

func (bs *blobSet) onChunk(c protocol.Chunk) error {
	st := bs.active[c.Stream]
	if st == nil || strings.ToLower(strings.TrimPrefix(c.Digest, "sha256:")) != st.head.SHA256 {
		return badBlob("chunk for a %s blob that was not announced", c.Stream)
	}
	if c.Offset != st.next {
		if st.resyncs++; st.resyncs > maxResync {
			return bs.failBlob(st, "the agent kept sending out of order")
		}
		return nil // the agent re-announces (digest) to learn the offset
	}
	data, err := base64.StdEncoding.Strict().DecodeString(c.Data)
	if err != nil {
		return bs.failBlob(st, "chunk is not base64")
	}
	if st.next+int64(len(data)) > st.head.Size {
		return bs.failBlob(st, "blob is larger than announced")
	}
	if _, err = st.w.Write(data); err != nil {
		return bs.failBlob(st, "the sink could not store the blob")
	}
	st.next += int64(len(data))
	if st.h != nil {
		st.h.Write(data)
	}
	if !c.Final {
		return nil
	}
	if st.next != st.head.Size {
		return bs.failBlob(st, "blob ended short of its announced size")
	}
	if st.h != nil && hex.EncodeToString(st.h.Sum(nil)) != st.head.SHA256 {
		return bs.failBlob(st, "blob digest mismatch")
	}
	if st.h == nil {
		if err = st.w.(Verifier).Verify(st.head.SHA256); err != nil {
			return bs.failBlob(st, "the resumed blob does not match its digest")
		}
	}
	delete(bs.active, c.Stream)
	if err = st.w.Close(); err != nil {
		return badBlob("the %s blob %s failed verification in the sink", c.Stream, short(st.head.SHA256))
	}
	return nil
}

func (bs *blobSet) failBlob(st *blobState, why string) error {
	st.discard()
	delete(bs.active, st.head.Stream)
	return badBlob("%s blob %s rejected: %s", st.head.Stream, short(st.head.SHA256), why)
}

// discardAll aborts every blob in flight.
func (bs *blobSet) discardAll() {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.discardLocked()
}

func (bs *blobSet) discardLocked() {
	for k, st := range bs.active {
		st.discard()
		delete(bs.active, k)
	}
}

func (st *blobState) discard() {
	if a, ok := st.w.(Aborter); ok {
		_ = a.Abort()
	}
}

func short(d string) string {
	return d[:min(len(d), 12)]
}
