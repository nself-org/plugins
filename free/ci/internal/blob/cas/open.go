package cas

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Stat reports a blob without reading it; Open is required before trusting bytes.
func (s *Store) Stat(realm Realm, digest string) (os.FileInfo, error) {
	path, err := s.blobPath(realm, digest)
	if err != nil {
		return nil, err
	}
	if err = checkDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	return info, nil
}

// Open verifies the complete file before exposing its first byte.
func (s *Store) Open(realm Realm, digest string) (io.ReadCloser, error) {
	path, err := s.blobPath(realm, digest)
	if err != nil {
		return nil, err
	}
	if err = checkDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if n != info.Size() {
		_ = f.Close()
		if err := s.quarantine(path); err != nil {
			return nil, err
		}
		return nil, ErrSizeMismatch
	}
	if hex.EncodeToString(h.Sum(nil)) != digest {
		_ = f.Close()
		if err := s.quarantine(path); err != nil {
			return nil, err
		}
		return nil, ErrDigestMismatch
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func (s *Store) quarantine(path string) error {
	dir := filepath.Join(s.blobs(), "corrupt")
	if err := secureDir(dir); err != nil {
		return err
	}
	name, err := randomName()
	if err != nil {
		return err
	}
	if err = os.Rename(path, filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return syncDir(dir)
}
