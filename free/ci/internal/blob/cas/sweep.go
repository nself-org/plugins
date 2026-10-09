package cas

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const blobGrace = time.Hour
const partialTTL = 24 * time.Hour
const corruptTTL = 7 * 24 * time.Hour

// Sweep removes unreferenced old blobs in one kind, stale partials, and old corrupt files.
func (s *Store) Sweep(ctx context.Context, kind string, keep func(Realm, string) bool, now time.Time) error {
	if !validKind(kind) || keep == nil {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root := s.blobs()
	if err := checkDir(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
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
		if err = walkFiles(base, func(path string, info os.FileInfo) error {
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
			return os.Remove(path)
		}); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err = s.sweepTemp(ctx, kind, now); err != nil {
		return err
	}
	return s.sweepCorrupt(ctx, now)
}

func walkFiles(root string, fn func(string, os.FileInfo) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return ErrInvalid
		}
		return fn(path, info)
	})
}

func (s *Store) sweepTemp(ctx context.Context, kind string, now time.Time) error {
	dir := filepath.Join(s.blobs(), "tmp")
	if err := checkDir(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	prune := func(path string, info os.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if now.Sub(info.ModTime()) < partialTTL {
			return nil
		}
		return os.Remove(path)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			r := Realm(e.Name())
			if !matchingKind(r, kind) {
				continue
			}
			if err := r.Validate(); err != nil {
				return err
			}
			if err := walkFiles(path, prune); err != nil {
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
		if err := prune(path, info); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) sweepCorrupt(ctx context.Context, now time.Time) error {
	dir := filepath.Join(s.blobs(), "corrupt")
	if err := checkDir(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return walkFiles(dir, func(path string, info os.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if now.Sub(info.ModTime()) < corruptTTL {
			return nil
		}
		return os.Remove(path)
	})
}

// Usage returns occupied blob bytes for one realm; temporary files are excluded.
func (s *Store) Usage(realm Realm) (int64, error) {
	if err := realm.Validate(); err != nil {
		return 0, err
	}
	root := filepath.Join(s.blobs(), string(realm), "sha256")
	if err := checkDir(root); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	var size int64
	err := walkFiles(root, func(_ string, info os.FileInfo) error { size += info.Size(); return nil })
	return size, err
}
