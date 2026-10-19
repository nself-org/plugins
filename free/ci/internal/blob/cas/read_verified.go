package cas

// Purpose: stream a blob to a consumer in one read, verifying its digest as the
// consumer reads (D-1008 round 3). Open copies the whole blob into an unlinked
// snapshot and hashes it before serving a byte: for a 100 MB build cache that is
// a second 100 MB write and about 200 ms on the start path of a job.
// Inputs:  a realm, a digest and a consumer of the bytes.
// Outputs: nil when the consumer succeeded and the bytes it read are the blob.
// Constraints: the consumer reads the one descriptor the store checked, so what
// is hashed is exactly what it consumed (no second read, no window to swap the
// file); it must treat the bytes as untrusted until ReadVerified returns nil and
// throw away what it built on any error. A digest or size mismatch quarantines
// the blob, like Open, and is reported even when the consumer failed first (a
// corrupt blob is named as corrupt, not as an unreadable archive).

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// ReadVerified calls fn with the bytes of the named blob and verifies them.
func (s *Store) ReadVerified(realm Realm, digest string, fn func(io.Reader) error) error {
	f, info, path, err := s.openBlob(realm, digest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h, count := sha256.New(), &countingReader{r: f}
	tee := io.TeeReader(count, h)
	if s.beforeHash != nil {
		s.beforeHash()
	}
	fnErr := fn(tee)
	if _, err := io.Copy(io.Discard, tee); err != nil { // the rest of the blob, so the digest covers all of it
		return err
	}
	if count.n != info.Size() || hex.EncodeToString(h.Sum(nil)) != digest {
		mismatch := ErrDigestMismatch
		if count.n != info.Size() {
			mismatch = ErrSizeMismatch
		}
		if err := s.locked(func() error { return s.quarantine(path, info) }); err != nil {
			return err
		}
		return mismatch
	}
	return fnErr
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
