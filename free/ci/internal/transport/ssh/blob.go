package ssh

// Purpose: agent to coordinator blobs (artifact and cache streams, CACHE
// delta 5): digest announces a blob, the coordinator answers with the offset
// the Sink already holds, chunks continue from there, and the blob is verified
// by digest before it is committed.
// Inputs:  digest and chunk frames from the agent.
// Outputs: offset frames; bytes written to the Sink's BlobWriter.
// Constraints: a chunk out of order is answered with the expected offset (at
// most maxResync times); a corrupt or oversized blob fails closed: the writer
// is aborted (or closed, when the Sink verifies on Close) and the transfer
// errors; when the blob starts at offset 0 the transport hashes it itself, and
// a resumed blob must be verified by the Sink on Close, whose BlobHeader holds
// the expected sha256.
// ArtifactRef and CacheRef frames name blobs the Sink already committed.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"hash"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

const maxResync = 3

// Aborter is implemented by a BlobWriter that can discard what it holds.
type Aborter interface{ Abort() error }

type blobState struct {
	head    transport.BlobHeader
	w       transport.BlobWriter
	next    int64
	h       hash.Hash // nil for a resumed blob
	resyncs int
}

func (s *sess) onDigest(ctx context.Context, d protocol.Digest) error {
	if d.Stream != "artifact" && d.Stream != "cache" {
		return newErr(CodeTransport, ReasonBlob, "digest frame for stream %q", d.Stream)
	}
	if old := s.blobs[d.Stream]; old != nil {
		s.discard(old)
		delete(s.blobs, d.Stream)
	}
	head := transport.BlobHeader{Stream: d.Stream, SHA256: strings.ToLower(strings.TrimPrefix(d.SHA256, "sha256:")), Size: d.Size}
	w, err := s.sink.Blob(ctx, head)
	if err != nil {
		return newErr(CodeTransport, ReasonBlob, "the %s blob %s was refused", d.Stream, short(head.SHA256))
	}
	st := &blobState{head: head, w: w, next: w.Offset()}
	if st.next == 0 {
		st.h = sha256.New()
	}
	s.blobs[d.Stream] = st
	return s.sendOffset(st)
}

func (s *sess) sendOffset(st *blobState) error {
	return s.send("offset", protocol.Offset{Stream: st.head.Stream, SHA256: st.head.SHA256, Offset: st.next}, true)
}

func (s *sess) onChunk(c protocol.Chunk) error {
	st := s.blobs[c.Stream]
	if st == nil || strings.ToLower(strings.TrimPrefix(c.Digest, "sha256:")) != st.head.SHA256 {
		return newErr(CodeTransport, ReasonBlob, "chunk for a %s blob that was not announced", c.Stream)
	}
	if c.Offset != st.next {
		if st.resyncs++; st.resyncs > maxResync {
			return s.failBlob(st, "the agent kept sending out of order")
		}
		return s.sendOffset(st)
	}
	data, err := base64.StdEncoding.Strict().DecodeString(c.Data)
	if err != nil {
		return s.failBlob(st, "chunk is not base64")
	}
	if st.head.Size > 0 && st.next+int64(len(data)) > st.head.Size {
		return s.failBlob(st, "blob is larger than announced")
	}
	if _, err := st.w.Write(data); err != nil {
		return s.failBlob(st, "the sink could not store the blob")
	}
	st.next += int64(len(data))
	if st.h != nil {
		st.h.Write(data)
	}
	if !c.Final {
		return nil
	}
	if st.head.Size > 0 && st.next != st.head.Size {
		return s.failBlob(st, "blob ended short of its announced size")
	}
	if st.h != nil && hex.EncodeToString(st.h.Sum(nil)) != st.head.SHA256 {
		return s.failBlob(st, "blob digest mismatch")
	}
	delete(s.blobs, c.Stream)
	if err := st.w.Close(); err != nil {
		return newErr(CodeTransport, ReasonBlob, "the %s blob %s failed verification in the sink", c.Stream, short(st.head.SHA256))
	}
	return nil
}

func (s *sess) failBlob(st *blobState, why string) error {
	s.discard(st)
	delete(s.blobs, st.head.Stream)
	return newErr(CodeTransport, ReasonBlob, "%s blob %s rejected: %s", st.head.Stream, short(st.head.SHA256), why)
}

// discard aborts a blob writer, or closes it when it cannot abort (a Sink
// verifies on Close and so rejects the bad content).
func (s *sess) discard(st *blobState) {
	if a, ok := st.w.(Aborter); ok {
		_ = a.Abort()
		return
	}
	_ = st.w.Close()
}

func short(d string) string {
	if len(d) > 12 {
		return d[:12]
	}
	return d
}
