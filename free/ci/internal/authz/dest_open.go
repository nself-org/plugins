package authz

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// OpenArtifactGetDest validates and opens an empty destination for extraction.
// A missing leaf is created with mode 0700 through its pinned parent. Callers
// must write through the returned root and close it; the path is for display
// only. Renaming a parent after this call cannot redirect extraction.
func OpenArtifactGetDest(dest, cwd string) (*os.Root, string, error) {
	if dest == "" || !filepath.IsAbs(cwd) {
		return nil, "", reject(DestEmpty, dest)
	}
	requested := dest
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(cwd, requested)
	}
	requested = filepath.Clean(requested)
	// Capture identities before validation, so replacement between validation
	// and opening cannot turn an approved pathname into a different directory.
	parentInfo, err := os.Stat(filepath.Dir(requested))
	if err != nil {
		return nil, "", reject(DestNoParent, dest)
	}
	leafInfo, leafErr := os.Lstat(requested)
	if leafErr != nil && !errors.Is(leafErr, os.ErrNotExist) {
		return nil, "", reject(DestUnreachable, dest)
	}
	resolved, err := ArtifactGetDest(dest, cwd)
	if err != nil {
		return nil, "", err
	}
	parent, err := os.OpenRoot(filepath.Dir(resolved))
	if err != nil {
		return nil, "", reject(DestUnreachable, dest)
	}
	defer parent.Close()
	if opened, err := parent.Stat("."); err != nil || !os.SameFile(parentInfo, opened) {
		return nil, "", reject(DestUnreachable, dest)
	}
	leaf := filepath.Base(resolved)
	if leafErr != nil {
		if err := parent.Mkdir(leaf, 0o700); err != nil {
			return nil, "", reject(DestUnreachable, dest)
		}
		leafInfo, err = parent.Lstat(leaf)
		if err != nil {
			return nil, "", reject(DestUnreachable, dest)
		}
	}
	if !leafInfo.IsDir() {
		return nil, "", reject(DestNotDir, dest)
	}
	root, err := parent.OpenRoot(leaf)
	if err != nil {
		return nil, "", reject(DestUnreachable, dest)
	}
	fail := func(reason PathReason) (*os.Root, string, error) { root.Close(); return nil, "", reject(reason, dest) }
	if opened, err := root.Stat("."); err != nil || !os.SameFile(leafInfo, opened) {
		return fail(DestUnreachable)
	}
	dir, err := root.Open(".")
	if err != nil {
		return fail(DestUnreachable)
	}
	names, err := dir.Readdirnames(1)
	dir.Close()
	if len(names) != 0 || (err != nil && !errors.Is(err, io.EOF)) {
		return fail(DestNotEmpty)
	}
	return root, resolved, nil
}
