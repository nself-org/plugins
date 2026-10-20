package transfer

// Purpose: the sending half of a blob transfer (Epic CACHE D7, protocol v1
// digest/offset/chunk), carrier neutral: it announces a blob, learns from the
// receiver's `offset` answer how many bytes it already holds, and streams the
// rest in 512 KiB chunks. A new offset from the receiver in the middle of the
// stream (a restart, a reconnect) moves the sender to that position, up to
// MaxResumes times.
// Inputs:  an Uploader (digest and chunk frames), a channel of the receiver's
// offset answers, and a blob to read.
// Outputs: the frames above; an E714 error when the receiver stops answering,
// keeps moving the offset, or reports an impossible one.
// Constraints: the receiver verifies the whole-blob digest, never the sender;
// a blob of zero bytes is not announced (the caller sends the artifact-ref
// alone); the sender reads the blob once per pass and never holds it in memory;
// waits are bounded by ctx and by WaitOffset.

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Defaults of the transfer.
const (
	// ChunkSize is the raw bytes per chunk frame (512 KiB: 700 KiB of base64, under the 1 MiB frame limit).
	ChunkSize = 512 << 10
	// MaxResumes is how many times one blob may restart from a reported offset.
	MaxResumes = 3
	// DefaultWaitOffset bounds the wait for an offset answer.
	DefaultWaitOffset = 60 * time.Second
)

// Uploader is the sending side of a carrier (session.Uploader satisfies it).
type Uploader interface {
	Digest(protocol.Digest) error
	Chunk(protocol.Chunk) error
}

// Blob is one blob to send.
type Blob struct {
	Stream string
	SHA256 string // lower-case hex
	Size   int64
	// Open returns the blob's bytes from the start; the sender skips to the offset it needs.
	Open func() (io.ReadCloser, error)
}

// Sender sends blobs through one Uploader.
type Sender struct {
	Up Uploader
	// Offsets carries the receiver's offset answers for the lease.
	Offsets <-chan protocol.Offset
	// ChunkSize, MaxResumes and WaitOffset override the defaults when set.
	ChunkSize  int
	MaxResumes int
	WaitOffset time.Duration
}

func (s *Sender) chunk() int {
	if s.ChunkSize > 0 {
		return s.ChunkSize
	}
	return ChunkSize
}

func (s *Sender) resumes() int {
	if s.MaxResumes > 0 {
		return s.MaxResumes
	}
	return MaxResumes
}

func (s *Sender) wait() time.Duration {
	if s.WaitOffset > 0 {
		return s.WaitOffset
	}
	return DefaultWaitOffset
}

// Send announces b and streams what the receiver does not hold. It returns when the final chunk
// is sent (or the receiver already held every byte); the artifact-ref that follows is the caller's
// confirmation, because the receiver refuses it unless the blob was committed.
func (s *Sender) Send(ctx context.Context, b Blob) error {
	if b.Size < 1 || !sha256Hex.MatchString(b.SHA256) || b.Open == nil {
		return Failf(ReasonProtocol, "a blob to send needs a sha256, a size above zero and a reader")
	}
	if err := s.Up.Digest(protocol.Digest{Stream: b.Stream, SHA256: b.SHA256, Size: b.Size}); err != nil {
		return Failf(ReasonInterrupted, "the announce of blob %s was not sent: %v", short(b.SHA256), err)
	}
	off, err := s.awaitOffset(ctx, b)
	if err != nil {
		return err
	}
	for resumes := 0; ; resumes++ {
		if off == b.Size {
			return nil
		}
		if off < 0 || off > b.Size {
			return Failf(ReasonProtocol, "the receiver reported offset %d for blob %s of %d B", off, short(b.SHA256), b.Size)
		}
		next, err := s.stream(ctx, b, off)
		if err != nil {
			return err
		}
		if next < 0 { // the whole blob went out
			return nil
		}
		if resumes >= s.resumes() {
			return Failf(ReasonInterrupted, "blob %s was restarted %d times; giving up", short(b.SHA256), s.resumes())
		}
		off = next
	}
}

// awaitOffset waits for the receiver's answer to the announce of b.
func (s *Sender) awaitOffset(ctx context.Context, b Blob) (int64, error) {
	timer := time.NewTimer(s.wait())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0, Failf(ReasonInterrupted, "cancelled while waiting for the offset of blob %s", short(b.SHA256))
		case <-timer.C:
			return 0, Failf(ReasonInterrupted, "the receiver gave no offset for blob %s within %s", short(b.SHA256), s.wait())
		case o, ok := <-s.Offsets:
			if !ok {
				return 0, Failf(ReasonInterrupted, "the offset channel closed before blob %s was answered", short(b.SHA256))
			}
			if o.Stream == b.Stream && o.SHA256 == b.SHA256 {
				return o.Offset, nil
			}
		}
	}
}

// stream sends b from off. It returns -1 after the final chunk, or the offset a mid-stream answer
// asked for.
func (s *Sender) stream(ctx context.Context, b Blob, off int64) (int64, error) {
	rc, err := b.Open()
	if err != nil {
		return -1, Failf(ReasonInterrupted, "blob %s cannot be read: %v", short(b.SHA256), err)
	}
	defer func() { _ = rc.Close() }()
	if _, err = io.CopyN(io.Discard, rc, off); err != nil {
		return -1, Failf(ReasonInterrupted, "blob %s is shorter than offset %d: %v", short(b.SHA256), off, err)
	}
	buf := make([]byte, s.chunk())
	for pos := off; pos < b.Size; {
		if err = ctx.Err(); err != nil {
			return -1, Failf(ReasonInterrupted, "cancelled while sending blob %s", short(b.SHA256))
		}
		if moved, o := s.moved(b, pos); moved {
			return o, nil
		}
		want := min(int64(len(buf)), b.Size-pos)
		n, rerr := io.ReadFull(rc, buf[:want])
		if rerr != nil && !(errors.Is(rerr, io.ErrUnexpectedEOF) && int64(n) == want) {
			return -1, Failf(ReasonInterrupted, "blob %s ended at %d of %d B: %v", short(b.SHA256), pos+int64(n), b.Size, rerr)
		}
		final := pos+int64(n) == b.Size
		if err = s.Up.Chunk(protocol.Chunk{Stream: b.Stream, Offset: pos, Data: base64.StdEncoding.EncodeToString(buf[:n]), Digest: b.SHA256, Final: final}); err != nil {
			return -1, Failf(ReasonInterrupted, "a chunk of blob %s was not sent: %v", short(b.SHA256), err)
		}
		pos += int64(n)
	}
	return -1, nil
}

// moved drains the offset answers that arrived while streaming; one for this blob that is not the
// position the sender is at means the receiver restarted and wants the bytes from there.
func (s *Sender) moved(b Blob, pos int64) (bool, int64) {
	for {
		select {
		case o, ok := <-s.Offsets:
			if !ok {
				return false, 0
			}
			if o.Stream == b.Stream && o.SHA256 == b.SHA256 && o.Offset != pos {
				return true, o.Offset
			}
		default:
			return false, 0
		}
	}
}
