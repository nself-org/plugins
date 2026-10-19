package cas

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// Stat reports a blob without reading it; Open is required before trusting bytes.
func (s *Store) Stat(realm Realm, digest string) (os.FileInfo, error) {
	path, err := s.blobPath(realm, digest)
	if err != nil {
		return nil, err
	}
	if err = s.checkDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	root, err := s.storeRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	rel, err := filepath.Rel(s.blobs(), path)
	if err != nil {
		return nil, err
	}
	info, err := root.Lstat(rel)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	return info, nil
}

// openBlob opens the named blob for reading after the store's path checks: the
// directory chain is ours, the entry is a regular file, and the open descriptor
// is the entry the checks saw. The caller closes the file and decides how far to
// trust its bytes.
func (s *Store) openBlob(realm Realm, digest string) (*os.File, os.FileInfo, string, error) {
	path, err := s.blobPath(realm, digest)
	if err != nil {
		return nil, nil, "", err
	}
	if err = s.checkDir(filepath.Dir(path)); err != nil {
		return nil, nil, "", err
	}
	dirBefore, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return nil, nil, "", err
	}
	root, err := s.storeRoot()
	if err != nil {
		return nil, nil, "", err
	}
	defer func() { _ = root.Close() }()
	rel, err := filepath.Rel(s.blobs(), path)
	if err != nil {
		return nil, nil, "", err
	}
	entry, err := root.Lstat(rel)
	if err != nil {
		return nil, nil, "", err
	}
	if !entry.Mode().IsRegular() {
		return nil, nil, "", ErrInvalid
	}
	f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, "", err
	}
	info, err := f.Stat()
	if err == nil && (!info.Mode().IsRegular() || !os.SameFile(entry, info)) {
		err = ErrInvalid
	}
	if err == nil {
		var dirAfter os.FileInfo
		if dirAfter, err = os.Lstat(filepath.Dir(path)); err == nil && !os.SameFile(dirBefore, dirAfter) {
			err = ErrInvalid
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, nil, "", err
	}
	return f, info, path, nil
}

// Open copies and hashes into an unlinked snapshot before serving any bytes.
// Later mutations of the named blob cannot change this reader's content.
func (s *Store) Open(realm Realm, digest string) (io.ReadCloser, error) {
	f, info, path, err := s.openBlob(realm, digest)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	snap, err := s.snapshot()
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = snap.Close()
		}
	}()
	h := sha256.New()
	if s.beforeHash != nil {
		s.beforeHash()
	}
	n, err := io.Copy(io.MultiWriter(snap, h), f)
	if err != nil {
		return nil, err
	}
	if n != info.Size() || hex.EncodeToString(h.Sum(nil)) != digest {
		mismatch := ErrDigestMismatch
		if n != info.Size() {
			mismatch = ErrSizeMismatch
		}
		err := s.locked(func() error { return s.quarantine(path, info) })
		if err != nil {
			return nil, err
		}
		return nil, mismatch
	}
	if _, err = snap.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	failed = false
	return snap, nil
}

func (s *Store) snapshot() (*os.File, error) {
	root, err := s.tmpRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	for i := 0; i < 3; i++ {
		name, err := randomName()
		if err != nil {
			return nil, err
		}
		rel := "snapshot-" + name
		f, err := root.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := root.Remove(rel); err != nil {
			_ = f.Close()
			return nil, err
		}
		return f, nil
	}
	return nil, errors.New("cas: snapshot name collision")
}

func verifyPath(root *os.Root, rel, digest string) error {
	f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrInvalid
	}
	current, err := root.Lstat(rel)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(current, info) {
		return ErrInvalid
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	if n != info.Size() {
		return ErrSizeMismatch
	}
	if hex.EncodeToString(h.Sum(nil)) != digest {
		return ErrDigestMismatch
	}
	return nil
}

// quarantine moves only the inode observed by Open. The tree lock excludes
// concurrent Store publications while identity is checked and moved.
func (s *Store) quarantine(path string, observed os.FileInfo) error {
	current, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(current, observed) {
		return nil
	}
	dir := filepath.Join(s.blobs(), "corrupt")
	if err := s.secureDir(dir); err != nil {
		return err
	}
	name, err := randomName()
	if err != nil {
		return err
	}
	root, err := s.storeRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	from, err := filepath.Rel(s.blobs(), path)
	if err != nil {
		return err
	}
	to := filepath.Join("corrupt", name)
	if s.beforeQuarantineRename != nil {
		if err := s.beforeQuarantineRename(); err != nil {
			return err
		}
	}
	if err = root.Rename(from, to); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if s.afterQuarantineRename != nil {
		s.afterQuarantineRename()
	}
	moved, err := root.Lstat(to)
	if err != nil {
		return err
	}
	if !os.SameFile(moved, observed) {
		// A pathname swap won the race; put that inode back if possible.
		if _, e := root.Lstat(from); errors.Is(e, os.ErrNotExist) {
			_ = root.Rename(to, from)
		}
		return ErrInvalid
	}
	return s.syncStoreDir(dir)
}
