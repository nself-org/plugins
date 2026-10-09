package cas

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const blobGrace = time.Hour

func partialTTL() time.Duration { return 24 * time.Hour }
func corruptTTL() time.Duration { return 7 * 24 * time.Hour }

var removeFile = func(s *Store, path string, remove func() error) error {
	if s.beforeRemove != nil {
		if err := s.beforeRemove(path); err != nil {
			return err
		}
	}
	return remove()
}

// Sweep removes unreferenced old blobs in one kind, stale partials, and old corrupt files.
func (s *Store) Sweep(ctx context.Context, kind string, keep func(Realm, string) bool, now time.Time) error {
	if !validKind(kind) || keep == nil {
		return ErrInvalid
	}
	return s.locked(func() error { return s.sweepLocked(ctx, kind, keep, now) })
}

func (s *Store) sweepLocked(ctx context.Context, kind string, keep func(Realm, string) bool, now time.Time) error {
	root := s.blobs()
	if err := s.checkDir(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	entries, err := s.readStoreDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		r := Realm(e.Name())
		if !matchingKind(r, kind) {
			continue
		}
		if err = r.Validate(); err != nil {
			return err
		}
		if !e.IsDir() {
			return ErrInvalid
		}
		base := filepath.Join(root, e.Name(), "sha256")
		walkErr := s.walkFiles(base, func(path string, info os.FileInfo, remove func() error) error {
			digest := filepath.Base(path)
			if err := validateDigest(digest); err != nil {
				return err
			}
			if filepath.Base(filepath.Dir(path)) != digest[:2] {
				return ErrInvalid
			}
			if now.Sub(info.ModTime()) < blobGrace || keep(r, digest) {
				return nil
			}
			return removeFile(s, path, remove)
		})
		if walkErr != nil && !errors.Is(walkErr, os.ErrNotExist) {
			return walkErr
		}
	}
	tempErr := s.sweepTemp(ctx, kind, now)
	return errors.Join(tempErr, s.sweepCorrupt(ctx, now))
}

func (s *Store) sweepTemp(ctx context.Context, kind string, now time.Time) error {
	dir := filepath.Join(s.blobs(), "tmp")
	if err := s.checkDir(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	prune := func(path string, info os.FileInfo, remove func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if now.Sub(info.ModTime()) < partialTTL() {
			return nil
		}
		return removeFile(s, path, remove)
	}
	entries, err := s.readStoreDir(dir)
	if err != nil {
		return err
	}
	if s.afterTempReadDir != nil {
		s.afterTempReadDir()
	}
	var unknown error
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			r := Realm(e.Name())
			if !matchingKind(r, kind) {
				continue
			}
			if err := r.Validate(); err != nil {
				unknown = errors.Join(unknown, errors.New("cas: skipped unknown temp entry: "+e.Name()))
				continue
			}
			if err := s.walkFiles(path, prune); err != nil {
				return err
			}
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return ErrInvalid
		}
		if err := s.pruneTempFile(dir, e.Name(), info, prune); err != nil {
			return err
		}
	}
	return unknown
}

func (s *Store) pruneTempFile(dir, name string, info os.FileInfo, prune func(string, os.FileInfo, func() error) error) error {
	root, err := s.storeRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	before, err := root.Lstat("tmp")
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return ErrInvalid
	}
	tmp, err := root.OpenRoot("tmp")
	if err != nil {
		return err
	}
	defer func() { _ = tmp.Close() }()
	opened, err := tmp.Stat(".")
	if err != nil || !os.SameFile(before, opened) {
		return ErrInvalid
	}
	remove := func() error {
		currentDir, err := os.Lstat(dir)
		if err != nil || !os.SameFile(currentDir, opened) {
			return ErrInvalid
		}
		currentFile, err := tmp.Lstat(name)
		if err != nil {
			return err
		}
		if !os.SameFile(currentFile, info) {
			return ErrInvalid
		}
		return tmp.Remove(name)
	}
	return prune(filepath.Join(dir, name), info, remove)
}

func (s *Store) sweepCorrupt(ctx context.Context, now time.Time) error {
	dir := filepath.Join(s.blobs(), "corrupt")
	if err := s.checkDir(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return s.walkFiles(dir, func(path string, info os.FileInfo, remove func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if now.Sub(info.ModTime()) < corruptTTL() {
			return nil
		}
		return removeFile(s, path, remove)
	})
}

// Usage returns occupied blob bytes for one realm; temporary files are excluded.
func (s *Store) Usage(realm Realm) (int64, error) {
	if err := realm.Validate(); err != nil {
		return 0, err
	}
	root := filepath.Join(s.blobs(), string(realm), "sha256")
	if err := s.checkDir(root); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	var size int64
	err := s.walkFiles(root, func(_ string, info os.FileInfo, _ func() error) error { size += info.Size(); return nil })
	return size, err
}
