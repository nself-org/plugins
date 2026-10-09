package cas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// Put streams bytes into an exclusive temporary file, then publishes by digest.
func (s *Store) Put(ctx context.Context, realm Realm, src io.Reader) (string, int64, error) {
	if err := realm.Validate(); err != nil {
		return "", 0, err
	}
	if src == nil {
		return "", 0, ErrInvalid
	}
	f, tempPath, err := s.temp()
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = s.removeStore(tempPath) }()
	defer func() { _ = f.Close() }()
	h := sha256.New()
	buf := make([]byte, 32*1024)
	var size int64
	emptyReads := 0
	for {
		if err = ctx.Err(); err != nil {
			return "", 0, err
		}
		n, e := src.Read(buf)
		if n == 0 && e == nil {
			emptyReads++
			if emptyReads >= 100 {
				return "", 0, io.ErrNoProgress
			}
		} else {
			emptyReads = 0
		}
		if n != 0 {
			k, werr := f.Write(buf[:n])
			if werr != nil {
				return "", 0, werr
			}
			if k != n {
				return "", 0, io.ErrShortWrite
			}
			h.Write(buf[:n])
			size += int64(n)
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", 0, e
		}
	}
	if err = f.Sync(); err != nil {
		return "", 0, err
	}
	if err = f.Close(); err != nil {
		return "", 0, err
	}
	digest := hex.EncodeToString(h.Sum(nil))
	err = s.locked(func() error { return s.publish(tempPath, realm, digest) })
	if err != nil {
		return "", 0, err
	}
	return digest, size, nil
}
