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
// Sink contract: a transport.BlobWriter has Abort (a failed blob is discarded,
// never committed by Close); one that starts past offset 0 must also implement
// transport.BlobVerifier, because the transport has not seen the bytes before the
// offset and cannot hash the whole blob itself. A digest must be 64 lower-case hex, the size
// at least 1 byte and at most maxBlob; a lease may open maxLeaseBlobs blobs of
// maxLeaseBytes in total.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"hash"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

const (
	maxResync = 3
	maxBlob   = 4 << 30
	// One lease may commit this many blobs and bytes in total, so a node cannot fill the sink
	// with distinct, individually valid blobs.
	maxLeaseBlobs = 1024
	maxLeaseBytes = 16 << 30
)

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

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
	if !sha256Hex.MatchString(head.SHA256) || head.Size < 1 || head.Size > maxBlob {
		return newErr(CodeTransport, ReasonBlob, "the %s blob announcement is not a sha256 with a size from 1 to %d bytes", d.Stream, int64(maxBlob))
	}
	if s.blobCount+1 > maxLeaseBlobs || s.blobBytes+head.Size > maxLeaseBytes {
		return newErr(CodeTransport, ReasonBlob, "the lease reached its blob budget (%d blobs, %d bytes); the %s blob was refused", maxLeaseBlobs, int64(maxLeaseBytes), d.Stream)
	}
	s.blobCount++
	s.blobBytes += head.Size
	w, err := s.sink.Blob(ctx, head)
	if err != nil {
		return newErr(CodeTransport, ReasonBlob, "the %s blob %s was refused", d.Stream, short(head.SHA256))
	}
	st := &blobState{head: head, w: w, next: w.Offset()}
	if st.next == 0 {
		st.h = sha256.New()
	} else if _, ok := w.(transport.BlobVerifier); !ok {
		s.discard(st)
		return newErr(CodeTransport, ReasonBlob, "the sink holds %d bytes of the %s blob but cannot verify it; refusing to resume", st.next, d.Stream)
	}
	if st.next == head.Size { // the sink already holds every byte: verify it, then say so
		if err := s.commit(st); err != nil {
			return err
		}
		return s.sendOffset(st)
	}
	s.blobs[d.Stream] = st
	return s.sendOffset(st)
}

// commit verifies a blob the sink holds in full, closes the writer and remembers the digest.
func (s *sess) commit(st *blobState) error {
	if v, ok := st.w.(transport.BlobVerifier); ok {
		if err := v.Verify(st.head.SHA256); err != nil {
			return s.failBlob(st, "the blob the sink already holds does not match its digest")
		}
	}
	if err := st.w.Close(); err != nil {
		return newErr(CodeTransport, ReasonBlob, "the %s blob %s failed verification in the sink", st.head.Stream, short(st.head.SHA256))
	}
	s.committed[st.head.SHA256] = true
	return nil
}

// committedBlob reports whether a blob with this sha256 was received whole in this session.
func (s *sess) committedBlob(sha string) bool {
	return s.committed[strings.ToLower(strings.TrimPrefix(sha, "sha256:"))]
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
	if st.next+int64(len(data)) > st.head.Size {
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
	if st.next != st.head.Size {
		return s.failBlob(st, "blob ended short of its announced size")
	}
	if st.h != nil && hex.EncodeToString(st.h.Sum(nil)) != st.head.SHA256 {
		return s.failBlob(st, "blob digest mismatch")
	}
	if st.h == nil {
		if err := st.w.(transport.BlobVerifier).Verify(st.head.SHA256); err != nil {
			return s.failBlob(st, "the resumed blob does not match its digest")
		}
	}
	delete(s.blobs, c.Stream)
	if err := st.w.Close(); err != nil {
		return newErr(CodeTransport, ReasonBlob, "the %s blob %s failed verification in the sink", c.Stream, short(st.head.SHA256))
	}
	s.committed[st.head.SHA256] = true
	return nil
}

func (s *sess) failBlob(st *blobState, why string) error {
	s.discard(st)
	delete(s.blobs, st.head.Stream)
	return newErr(CodeTransport, ReasonBlob, "%s blob %s rejected: %s", st.head.Stream, short(st.head.SHA256), why)
}

// discard aborts a blob writer so Close never commits it.
func (s *sess) discard(st *blobState) {
	_ = st.w.Abort()
}

func short(d string) string {
	if len(d) > 12 {
		return d[:12]
	}
	return d
}
