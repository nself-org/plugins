package records

import (
	"errors"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"io"
	"os"
)

// Keep exactly the compressed bytes streamed to the blob; scan before its EOF commits it.
func (r *scanReader) scanCompressed(chunk []byte) error {
	if r.compressed == nil && r.read == int64(len(chunk)) && authz.Compressed(chunk) {
		f, err := os.CreateTemp("", "nself-artifact-scan-*")
		if err != nil {
			return err
		}
		r.compressed = f
	}
	if r.compressed == nil {
		return nil
	}
	if _, err := r.compressed.Write(chunk); err != nil {
		return err
	}
	if r.err != io.EOF {
		return nil
	}
	budget := int64(8 << 30)
	err := authz.ScanArchive(r.compressed, r.read, r.sc, &budget)
	if errors.Is(err, authz.ErrSecretContent) {
		r.matched = true
		return failf(ReasonSecretMatch, "%s holds a registered secret value; it is not stored", r.name)
	}
	if err != nil {
		return failf(ReasonUnreadable, "%s could not be scanned", r.name)
	}
	return nil
}

func (r *scanReader) Close() {
	if r.compressed != nil {
		_ = r.compressed.Close()
		_ = os.Remove(r.compressed.Name())
	}
}
