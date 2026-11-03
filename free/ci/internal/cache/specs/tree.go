package specs

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

var errCloneUnsupported = errors.New("specs: tree cloning is unsupported")

// restoreBlob clones a verified tree when enabled. Any tree failure returns to
// the ordinary unpack path, which retains the restore's refusal behaviour.
func (e *Engine) restoreBlob(ctx context.Context, base *os.Root, realm cas.Realm, digest, rel string, opt UnpackOptions) error {
	if cloneSupported && e.Trees != "" && !opt.OtherRead {
		if err := e.cloneBlob(ctx, base, realm, digest, rel, opt); err == nil {
			return nil
		}
	}
	return e.unpackBlob(ctx, base, realm, digest, rel, opt)
}

// cloneBlob publishes only fully verified trees and clones into an absent
// attempt directory. Warm hits still verify the stored blob before serving it.
func (e *Engine) cloneBlob(ctx context.Context, base *os.Root, realm cas.Realm, digest, rel string, opt UnpackOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsLocal(rel) || filepath.Clean(rel) == "." {
		return errors.New("specs: invalid tree destination")
	}
	sum := sha256.Sum256([]byte(string(realm) + "\x00" + digest))
	tree := filepath.Join(e.Trees, fmt.Sprintf("%x", sum)[:32])
	info, err := os.Lstat(tree)
	if errors.Is(err, os.ErrNotExist) {
		if err := e.buildTree(ctx, realm, digest, tree, opt); err != nil {
			return err
		}
	} else {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("specs: cached tree is not a directory")
		}
		if err := e.Blobs.ReadVerified(realm, digest, func(r io.Reader) error {
			_, err := io.Copy(io.Discard, &ctxReader{ctx, r})
			return err
		}); err != nil {
			return err
		}
	}
	if err := base.MkdirAll(filepath.Dir(rel), 0o700); err != nil {
		return err
	}
	if err := base.Remove(rel); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := cloneTree(tree, filepath.Join(base.Name(), rel)); err != nil {
		_ = removeIn(base, rel)
		return err
	}
	return nil
}

// buildTree unpacks into a private root and publishes it atomically. Concurrent
// publishers may lose the rename; their temporary trees are always removed.
func (e *Engine) buildTree(ctx context.Context, realm cas.Realm, digest, tree string, opt UnpackOptions) error {
	if err := os.MkdirAll(e.Trees, 0o700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(e.Trees, ".tmp-")
	if err != nil {
		return err
	}
	defer func() { _ = RemoveTree(tmp) }()
	root, err := os.OpenRoot(tmp)
	if err != nil {
		return err
	}
	err = e.unpackBlob(ctx, root, realm, digest, ".", opt)
	_ = root.Close()
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, tree); err != nil {
		if info, statErr := os.Lstat(tree); statErr != nil || !info.IsDir() {
			return err
		}
	}
	return nil
}
