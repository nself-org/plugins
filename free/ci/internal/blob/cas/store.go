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
	home         string
	mu           sync.Mutex
	beforeRename func() error
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

// secureDir creates each missing component and rejects symlinks in the tree.
func secureDir(path string) error {
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	if parent != path {
		if err := secureDir(parent); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.Mkdir(path, 0700); errors.Is(err, os.ErrExist) {
			info, err = os.Lstat(path)
		} else if err == nil {
			return nil
		}
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrInvalid
	}
	return nil
}

func checkDir(path string) error {
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		info, err := lstat(p)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrInvalid
		}
		if filepath.Dir(p) == p {
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

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	err = d.Sync()
	closeErr := d.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func randomName() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *Store) temp() (*os.File, error) {
	dir := filepath.Join(s.blobs(), "tmp")
	if err := secureDir(dir); err != nil {
		return nil, err
	}
	for i := 0; i < 3; i++ {
		name, err := randomName()
		if err != nil {
			return nil, err
		}
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, errors.New("cas: temporary name collision")
}

func (s *Store) publish(tmp string, r Realm, digest string) error {
	dst, err := s.blobPath(r, digest)
	if err != nil {
		return err
	}
	if err = secureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	if s.beforeRename != nil {
		if err = s.beforeRename(); err != nil {
			return err
		}
	}
	if err = os.Rename(tmp, dst); err != nil {
		return err
	}
	return syncDir(filepath.Dir(dst))
}

func validKind(kind string) bool             { return kind == "cache" || kind == "artifacts" }
func matchingKind(r Realm, kind string) bool { return strings.HasPrefix(string(r), kind+"-") }
