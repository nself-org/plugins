package cas

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Refresh restarts the age of an existing blob without re-streaming its bytes.
//
// Under the tree lock it re-verifies the stored bytes against digest, then sets
// the modification time to now, so Sweep's blobGrace and the cache commit's
// freshness check treat the blob as just written. A blob that fails
// verification is quarantined exactly as Open would and the mismatch is
// returned; a missing blob returns an error matching os.ErrNotExist (the caller
// must Put it again).
func (s *Store) Refresh(realm Realm, digest string) error {
	path, err := s.blobPath(realm, digest)
	if err != nil {
		return err
	}
	return s.locked(func() error {
		if err := s.checkDir(filepath.Dir(path)); err != nil {
			return err
		}
		root, err := s.storeRoot()
		if err != nil {
			return err
		}
		defer func() { _ = root.Close() }()
		rel, err := s.storeRel(path)
		if err != nil {
			return err
		}
		observed, err := root.Lstat(rel)
		if err != nil {
			return err
		}
		if !observed.Mode().IsRegular() {
			return ErrInvalid
		}
		if err = verifyPath(root, rel, digest); err != nil {
			if errors.Is(err, ErrDigestMismatch) || errors.Is(err, ErrSizeMismatch) {
				if qerr := s.quarantine(path, observed); qerr != nil {
					return qerr
				}
			}
			return err
		}
		now := time.Now()
		if err = root.Chtimes(rel, now, now); err != nil {
			return err
		}
		current, err := root.Lstat(rel)
		if err != nil || !os.SameFile(current, observed) {
			return ErrInvalid
		}
		return nil
	})
}
