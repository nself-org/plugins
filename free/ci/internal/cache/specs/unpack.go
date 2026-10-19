package specs

// Purpose: restore one spec blob (tar + gzip) into its directory (Epic CACHE
// D5, R-SEC-29). Every header goes through authz.Extractor before anything is
// written, and every write goes through an os.Root pinned on the directory.
// Inputs:  the blob reader, the destination directory, the sanitiser limits.
// Outputs: UnpackStats; *authz.PathError (E683) for a refused header.
// Constraints: the destination is a directory of an os.Root pinned before the
// job started. Traversal, absolute names, hard links, links out of the tree,
// devices, FIFOs, set-id bits, case collisions, writing through an earlier
// symlink and entry, size and ratio bombs are all refused by the Extractor; an
// error leaves a partial directory that the caller removes (RemoveTree). The
// decompressed stream is bounded, whatever the headers claim. Directory modes
// are applied last (owner access always kept) so a read-only directory can
// still be filled; file times and modes are restored, set-id bits never.
// The compression ratio is judged by bombGuard (bomb.go) once an entry's bytes
// were written, not at its header: the sanitiser only sees a header, before the
// data was read, so it is given the entry's own size as its packed size and
// never trips on the ratio.

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// UnpackStats counts what Unpack wrote.
type UnpackStats struct {
	Files, Dirs, Links int
	Bytes              int64
}

type dirMode struct {
	name string
	mode os.FileMode
	at   time.Time
}

// UnpackOptions tune Unpack.
type UnpackOptions struct {
	// OtherRead makes the restored tree readable and traversable by other users
	// (dirs 0755, files at least 0444, executable for all when the owner may execute): a container job runs under another UID
	// than the agent that owns the cache root.
	OtherRead bool
}

// Unpack extracts the tar+gzip stream r into the directory rel of base
// (created 0700 if missing).
func Unpack(r io.Reader, base *os.Root, rel string, lim authz.Limits, opt UnpackOptions) (UnpackStats, error) {
	var st UnpackStats
	if err := base.MkdirAll(rel, 0o700); err != nil {
		return st, err
	}
	root, err := base.OpenRoot(rel)
	if err != nil {
		return st, err
	}
	defer func() { _ = root.Close() }()
	return extract(r, root, lim, opt)
}

// Validate reads a cache blob exactly as Unpack would (same sanitiser, same
// size and ratio guards) and writes nothing: Save runs it on every blob it
// packed, so an entry is never committed that a restore would refuse.
func Validate(r io.Reader, lim authz.Limits) error {
	_, err := extract(r, nil, lim, UnpackOptions{})
	return err
}

// extract is Unpack and Validate; a nil root discards the data.
func extract(r io.Reader, root *os.Root, lim authz.Limits, opt UnpackOptions) (UnpackStats, error) {
	var st UnpackStats
	dry := root == nil
	cr := &countReader{r: r}
	gz, err := gzip.NewReader(cr)
	if err != nil {
		return st, fmt.Errorf("specs: not a gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()
	lim = effective(lim)
	tr := tar.NewReader(io.LimitReader(gz, lim.MaxTotalBytes+(1<<30)))
	ext, guard, dirs := authz.NewExtractor(lim), &bombGuard{cr: cr, lim: lim}, []dirMode{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return st, fmt.Errorf("specs: unreadable archive: %w", err)
		}
		name, err := ext.Admit(entryOf(h, h.Size))
		if err != nil {
			return st, err
		}
		if name == "." {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if !dry {
				if err := root.MkdirAll(name, 0o700); err != nil {
					return st, err
				}
				dirs = append(dirs, dirMode{name, dirPerm(h, opt), h.ModTime})
			}
			st.Dirs++
		case tar.TypeSymlink:
			if !dry {
				if err := parents(root, name); err != nil {
					return st, err
				}
				if err := root.Symlink(h.Linkname, name); err != nil {
					return st, err
				}
			}
			st.Links++
		default:
			ge := guard.entry(tr, h.Name)
			if dry {
				if n, err := io.Copy(io.Discard, ge); err != nil || n != h.Size {
					return st, errors.Join(err, io.ErrUnexpectedEOF)
				}
			} else if err := writeFile(root, name, ge, h, opt); err != nil {
				return st, err
			}
			if err := ge.done(); err != nil {
				return st, err
			}
			st.Files++
			st.Bytes += h.Size
		}
	}
	if err := guard.finish(); err != nil {
		return st, err
	}
	if dry {
		return st, nil
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i].name) > len(dirs[j].name) })
	if opt.OtherRead {
		if err := openDirs(root); err != nil {
			return st, err
		}
	}
	for _, d := range dirs {
		if err := root.Chmod(d.name, d.mode); err != nil {
			return st, err
		}
		_ = root.Chtimes(d.name, d.at, d.at)
	}
	return st, nil
}

// openDirs makes every directory of the tree (the root and the implicit
// parents of a crafted archive too) traversable by other users.
func openDirs(root *os.Root) error {
	return fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return root.Chmod(p, info.Mode().Perm()|0o755)
	})
}

func parents(root *os.Root, name string) error {
	if dir := path.Dir(name); dir != "." {
		return root.MkdirAll(dir, 0o700)
	}
	return nil
}

func dirPerm(h *tar.Header, opt UnpackOptions) os.FileMode {
	m := os.FileMode(h.Mode&0o777) | 0o700
	if opt.OtherRead {
		m |= 0o755
	}
	return m
}

func writeFile(root *os.Root, name string, r io.Reader, h *tar.Header, opt UnpackOptions) error {
	if err := parents(root, name); err != nil {
		return err
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, h.Size))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != h.Size {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		return fmt.Errorf("specs: writing %q: %w", name, err)
	}
	perm := os.FileMode(h.Mode&0o777) | 0o400
	if opt.OtherRead {
		perm |= 0o444
		if perm&0o100 != 0 {
			perm |= 0o111
		}
	}
	if err := root.Chmod(name, perm); err != nil {
		return err
	}
	_ = root.Chtimes(name, h.ModTime, h.ModTime)
	return nil
}

// RemoveTree deletes dir and everything in it, first making every directory
// owner-writable (a Go module cache holds read-only directories).
func RemoveTree(dir string) error {
	root, err := os.OpenRoot(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_ = chmodDirs(root, ".")
	_ = root.Close()
	return os.RemoveAll(dir)
}

// RemoveIn deletes the entry rel of base, making directories owner-writable
// first, without ever leaving base: a rel that is a symlink out of base is
// refused (its link is removed, its target never touched).
func RemoveIn(base *os.Root, rel string) error { return removeIn(base, rel) }

// removeIn deletes the directory rel of base (same permission rule as RemoveTree)
// without ever leaving base.
func removeIn(base *os.Root, rel string) error {
	if sub, err := base.OpenRoot(rel); err == nil {
		_ = chmodDirs(sub, ".")
		_ = sub.Close()
	}
	return base.RemoveAll(rel)
}

func chmodDirs(root *os.Root, dir string) error {
	_ = root.Chmod(dir, 0o700)
	f, err := root.Open(dir)
	if err != nil {
		return err
	}
	ents, err := f.ReadDir(-1)
	_ = f.Close()
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.IsDir() {
			_ = chmodDirs(root, path.Join(dir, e.Name()))
		}
	}
	return nil
}
