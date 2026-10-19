package specs

// Purpose: the filesystem and bookkeeping helpers of a Session (session.go):
// pinning the attempt's cache root and its parent, removing them through the
// pinned parent, emptying a spec dir, usage rows and warnings.
// Inputs:  the Session. Outputs: none beyond their effects.
// Constraints: nothing here follows a path the job could have swapped: removal
// goes through the handles opened in pin before the job started.

import (
	"context"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

// pin creates the attempt's cache root and opens handles on it and on its parent.
func (s *Session) pin() (err error) {
	parent := filepath.Dir(s.in.Root)
	if err = os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	if s.parent, err = os.OpenRoot(parent); err != nil {
		return err
	}
	s.name = filepath.Base(s.in.Root)
	if err = s.parent.Mkdir(s.name, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	s.base, err = s.parent.OpenRoot(s.name)
	return err
}

// Cleanup removes the attempt's cache directories.
func (s *Session) Cleanup() {
	if s.base != nil {
		_ = chmodDirs(s.base, ".")
		_ = s.base.Close()
		s.base = nil
	}
	if s.parent != nil { // removal through the pinned parent never leaves it
		_ = s.parent.RemoveAll(s.name)
		_ = s.parent.Close()
		s.parent = nil
	}
}

// reset empties and recreates the dirs of it.
func (s *Session) reset(it *Item) {
	for _, d := range it.Dirs {
		_ = removeIn(s.base, d)
		_ = s.base.MkdirAll(d, 0o700)
	}
}

func (s *Session) warn(msg string) {
	if s.in.Warn != nil {
		s.in.Warn(msg)
	}
}

func (s *Session) use(ctx context.Context, spec, key, result string, bytes, ms int64) {
	if err := s.in.Engine.Entries.RecordUse(ctx, entries.Use{Attempt: s.in.Attempt, Spec: spec, Key: key, Result: result, Bytes: bytes, MS: ms}); err != nil {
		s.warn("cache usage row for spec " + spec + " was not recorded")
	}
}

// host is the host path of a directory relative to the cache root.
func (s *Session) host(rel string) string { return filepath.Join(s.in.Root, filepath.FromSlash(rel)) }
