package specs

// Purpose: scan inside the gzip payloads of a spec's files for registered
// values (npm's _cacache keeps its tarballs gzipped, so the scan of the tar
// stream sees only compressed bytes there).
// Inputs:  the raw bytes of one file that starts with a gzip header.
// Outputs: inflater, ErrSecretMatch, ErrTooLarge.
// Constraints: every member of the payload is inflated and scanned with the
// same windows as the tar stream; the inflated bytes of one save are capped
// (inflateBudget) and a payload that would pass it is refused as too large,
// because it cannot be shown clean. A stream that stops being valid gzip is
// scanned up to the break and then ignored: the tool that reads the file back
// fails on it too, and the raw bytes were scanned by the tar-stream scan. One
// level only: a gzip inside a gzip is scanned as inflated bytes.

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// inflateBudget caps the inflated bytes scanned over one directory (8 GiB).
const inflateBudget = 8 << 30

type inflater struct {
	sc   authz.ValueScanner
	left int64
}

func newInflater(sc authz.ValueScanner) *inflater { return &inflater{sc: sc, left: inflateBudget} }

// isGzip reports whether r starts with the gzip magic, without consuming it.
func isGzip(r *bufio.Reader) bool {
	head, _ := r.Peek(2)
	return len(head) == 2 && head[0] == 0x1f && head[1] == 0x8b
}

// scan reads r to its end. It returns ErrSecretMatch or ErrTooLarge, or nil.
func (in *inflater) scan(r io.Reader) error {
	if err := in.inflate(r); err != nil {
		return err // the caller closes the pipe with it: the file copy stops
	}
	_, _ = io.Copy(io.Discard, r) // never leave the file copy blocked on the pipe
	return nil
}

func (in *inflater) inflate(r io.Reader) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil
	}
	sw := newScanWriter(io.Discard, in.sc)
	n, err := io.Copy(sw, io.LimitReader(gz, in.left+1))
	switch {
	case errors.Is(err, ErrSecretMatch):
		return err
	case n > in.left:
		return fmt.Errorf("%w: the gzip payloads inflate past %d bytes", ErrTooLarge, int64(inflateBudget))
	}
	in.left -= n
	return sw.Close()
}
