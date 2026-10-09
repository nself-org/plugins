package cas

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store owns one on-disk content-addressed tree under NSELF_CI_HOME.
type Store struct {
	home                   string
	mu                     sync.Mutex
	beforeRename           func() error
	beforeHash             func()
	afterLockStat          func()
	afterPartialStat       func()
	beforeOffsetRename     func() error
	beforeRemove           func(string) error
	beforeQuarantineRename func() error
	afterQuarantineRename  func()
	afterTempReadDir       func()
}

var lstat = os.Lstat

func New(home string) (*Store, error) {
	if home == "" {
		return nil, ErrInvalid
	}
	root, err := filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	return &Store{home: root}, nil
}

func (s *Store) blobs() string { return filepath.Join(s.home, "blobs") }

// secureDir creates missing components below blobs and rejects links there.
func (s *Store) secureDir(path string) error {
	path = filepath.Clean(path)
	if path == s.blobs() {
		if err := os.MkdirAll(s.home, 0700); err != nil {
			return err
		}
		if err := mkdirBlobDir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		return s.checkDir(path)
	}
	if !strings.HasPrefix(path, s.blobs()+string(os.PathSeparator)) {
		return ErrInvalid
	}
	parent := filepath.Dir(path)
	if parent != path {
		if err := s.secureDir(parent); err != nil {
			return err
		}
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
	info, err := root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		if err = root.Mkdir(rel, 0700); errors.Is(err, os.ErrExist) {
			info, err = root.Lstat(rel)
		} else if err == nil {
			return s.checkDir(path)
		}
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrInvalid
	}
	return s.checkDir(path)
}

func (s *Store) checkDir(path string) error {
	path = filepath.Clean(path)
	if path != s.blobs() && !strings.HasPrefix(path, s.blobs()+string(os.PathSeparator)) {
		return ErrInvalid
	}
	for p := path; ; p = filepath.Dir(p) {
		info, err := lstat(p)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrInvalid
		}
		if p == s.blobs() {
			return nil
		}
	}
}

func (s *Store) blobPath(r Realm, digest string) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	if err := validateDigest(digest); err != nil {
		return "", err
	}
	return filepath.Join(s.blobs(), string(r), "sha256", digest[:2], digest), nil
}

var randomName = func() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

var mkdirBlobDir = os.Mkdir

func (s *Store) tmpRoot() (*os.Root, error) {
	dir := filepath.Join(s.blobs(), "tmp")
	if err := s.secureDir(dir); err != nil {
		return nil, err
	}
	before, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	root, err := s.storeRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	tmp, err := root.OpenRoot("tmp")
	if err != nil {
		return nil, err
	}
	after, err := os.Lstat(dir)
	if err != nil || !os.SameFile(before, after) || after.Mode()&os.ModeSymlink != 0 {
		_ = tmp.Close()
		return nil, ErrInvalid
	}
	actual, err := tmp.Stat(".")
	if err != nil || !os.SameFile(before, actual) {
		_ = tmp.Close()
		return nil, ErrInvalid
	}
	return tmp, nil
}

func (s *Store) temp() (*os.File, string, error) {
	dir := filepath.Join(s.blobs(), "tmp")
	root, err := s.tmpRoot()
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = root.Close() }()
	for i := 0; i < 3; i++ {
		name, err := randomName()
		if err != nil {
			return nil, "", err
		}
		f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return f, filepath.Join(dir, name), err
	}
	return nil, "", errors.New("cas: temporary name collision")
}

func (s *Store) publish(tmp string, r Realm, digest string) error {
	dst, err := s.blobPath(r, digest)
	if err != nil {
		return err
	}
	if err = s.secureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	// Keep the root handle across the hook and rename. Root.Rename cannot
	// traverse a swapped symlink outside blobs, even if the pathname changes.
	root, err := s.storeRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	dirBefore, err := os.Lstat(filepath.Dir(dst))
	if err != nil {
		return err
	}
	tmpRel, err := filepath.Rel(s.blobs(), tmp)
	if err != nil {
		return err
	}
	dstRel, err := filepath.Rel(s.blobs(), dst)
	if err != nil {
		return err
	}
	if s.beforeRename != nil {
		if err = s.beforeRename(); err != nil {
			return err
		}
	}
	if err = root.Rename(tmpRel, dstRel); err != nil {
		return err
	}
	dirAfter, err := os.Lstat(filepath.Dir(dst))
	if err != nil || !os.SameFile(dirBefore, dirAfter) {
		// A directory swap cannot make a successful publication visible.
		_ = root.Remove(dstRel)
		return ErrInvalid
	}
	// Verify the published pathname, not the old temporary descriptor.
	if err := verifyPath(root, dstRel, digest); err != nil {
		_ = root.Remove(dstRel)
		return err
	}
	dirAfter, err = os.Lstat(filepath.Dir(dst))
	if err != nil || !os.SameFile(dirBefore, dirAfter) {
		_ = root.Remove(dstRel)
		return ErrInvalid
	}
	return s.syncStoreDir(filepath.Dir(dst))
}

func validKind(kind string) bool             { return kind == "cache" || kind == "artifacts" }
func matchingKind(r Realm, kind string) bool { return strings.HasPrefix(string(r), kind+"-") }
