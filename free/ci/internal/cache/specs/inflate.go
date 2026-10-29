package specs

// Purpose: scan inside gzip and ZIP payloads of every spec for registered
// values (npm's _cacache keeps its tarballs gzipped, so the scan of the tar
// stream sees only compressed bytes there).
// Inputs:  the raw bytes of one compressed file.
// Outputs: inflater, ErrSecretMatch, ErrTooLarge.
// Constraints: every member of the payload is inflated and scanned with the
// same windows as the tar stream; the inflated bytes of one save are capped
// (inflateBudget) and a payload that would pass it is refused as too large,
// because it cannot be shown clean. A stream that stops being valid gzip is
// scanned up to the break and then ignored: the tool that reads the file back
// fails on it too, and the raw bytes were scanned by the tar-stream scan. One
// level only: a gzip inside a gzip is scanned as inflated bytes.

import (
	"archive/zip"
	"bufio"
	"bytes"
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

// isCompressed detects gzip or ZIP magic without consuming it.
func isCompressed(r *bufio.Reader) bool {
	head, _ := r.Peek(4)
	return len(head) >= 2 && head[0] == 0x1f && head[1] == 0x8b || len(head) == 4 && bytes.Equal(head, []byte{'P', 'K', 3, 4})
}

// scan reads r to its end. It returns ErrSecretMatch or ErrTooLarge, or nil.
func (in *inflater) scan(r io.Reader) error {
	br := bufio.NewReader(r)
	head, _ := br.Peek(4)
	if len(head) == 4 && bytes.Equal(head, []byte{'P', 'K', 3, 4}) {
		return in.zip(br)
	}
	if err := in.inflate(br); err != nil {
		return err // the caller closes the pipe with it: the file copy stops
	}
	_, _ = io.Copy(io.Discard, br) // never leave the file copy blocked on the pipe
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

// ZIP needs random access to its central directory. Bound its compressed size
// as well as the shared inflation budget, refusing payloads we cannot inspect.
const zipScanLimit = 64 << 20

func (in *inflater) zip(r io.Reader) error {
	data, err := io.ReadAll(io.LimitReader(r, zipScanLimit+1))
	if err != nil {
		return err
	}
	if len(data) > zipScanLimit {
		return fmt.Errorf("%w: zip scan limit", ErrTooLarge)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("specs: invalid zip payload: %w", err)
	}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		sw := newScanWriter(io.Discard, in.sc)
		n, err := io.Copy(sw, io.LimitReader(rc, in.left+1))
		_ = rc.Close()
		if err != nil {
			return err
		}
		if n > in.left {
			return fmt.Errorf("%w: zip inflation budget", ErrTooLarge)
		}
		in.left -= n
		if err := sw.Close(); err != nil {
			return err
		}
	}
	return nil
}
