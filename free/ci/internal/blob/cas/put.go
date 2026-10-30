package cas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// Put streams bytes into an exclusive temporary file, then publishes by digest.
func (s *Store) Put(ctx context.Context, realm Realm, src io.Reader) (string, int64, error) {
	p, err := s.Prepare(ctx, realm, src)
	if err != nil {
		return "", 0, err
	}
	return p.Publish()
}

// Prepared is a blob that is fully written, hashed and synced in a temporary
// file but not visible in the store yet. Publish or Discard it exactly once.
type Prepared struct {
	s        *Store
	realm    Realm
	tempPath string
	digest   string
	size     int64
}

// Prepare is Put without the publish: the work that does not touch the store's
// namespace (read, hash, write, fsync) runs here, so several blobs can prepare at
// once and publish only when all of them are good (D-1515).
func (s *Store) Prepare(ctx context.Context, realm Realm, src io.Reader) (*Prepared, error) {
	if err := realm.Validate(); err != nil {
		return nil, err
	}
	if src == nil {
		return nil, ErrInvalid
	}
	f, tempPath, err := s.temp()
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = s.removeStore(tempPath)
		}
	}()
	defer func() { _ = f.Close() }()
	h := sha256.New()
	buf := make([]byte, 32*1024)
	var size int64
	emptyReads := 0
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		n, e := src.Read(buf)
		if n == 0 && e == nil {
			emptyReads++
			if emptyReads >= 100 {
				return nil, io.ErrNoProgress
			}
		} else {
			emptyReads = 0
		}
		if n != 0 {
			k, werr := f.Write(buf[:n])
			if werr != nil {
				return nil, werr
			}
			if k != n {
				return nil, io.ErrShortWrite
			}
			h.Write(buf[:n])
			size += int64(n)
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
	}
	if err = f.Sync(); err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	keep = true
	return &Prepared{s: s, realm: realm, tempPath: tempPath, digest: hex.EncodeToString(h.Sum(nil)), size: size}, nil
}

// Publish moves the blob into the store under its digest.
func (p *Prepared) Publish() (string, int64, error) {
	defer func() { _ = p.s.removeStore(p.tempPath) }()
	if err := p.s.locked(func() error { return p.s.publish(p.tempPath, p.realm, p.digest) }); err != nil {
		return "", 0, err
	}
	return p.digest, p.size, nil
}

// Size is the number of bytes written.
func (p *Prepared) Size() int64 { return p.size }

// Discard drops the temporary file without publishing.
func (p *Prepared) Discard() { _ = p.s.removeStore(p.tempPath) }
