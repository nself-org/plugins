package s3store

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Errors a caller maps to HTTP statuses.
var (
	// ErrBusy means no upload slot freed up in time (503, retry later).
	ErrBusy = errors.New("s3store: upload capacity reached")
	// ErrBodyTruncated means the request body ended before the sender finished (nothing was committed).
	ErrBodyTruncated = errors.New("s3store: request body ended early")
	// ErrLengthMismatch means the bytes read differ from the declared length (nothing was committed).
	ErrLengthMismatch = errors.New("s3store: body length differs from the declared length")
)

// strictReader turns a source io.ErrUnexpectedEOF into ErrBodyTruncated. io.ReadFull produces ErrUnexpectedEOF
// itself for a short final read; if a source's own ErrUnexpectedEOF passed through, Put could not tell a truncated
// request from the end of the stream and would commit a short object.
type strictReader struct{ r io.Reader }

func (s *strictReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return n, fmt.Errorf("%w (%v)", ErrBodyTruncated, err)
	}
	return n, err
}

// checkCommit is the last gate before PutObject or CompleteMultipartUpload: the request must still be alive and
// the bytes read must equal the declared length (when one was declared).
func checkCommit(ctx context.Context, read, declared int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if declared >= 0 && read != declared {
		return fmt.Errorf("%w: read %d, declared %d", ErrLengthMismatch, read, declared)
	}
	return nil
}
