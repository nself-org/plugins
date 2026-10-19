package specs

// Purpose: restore one spec blob (tar + gzip) into its directory (Epic CACHE
// D5, R-SEC-29). Every header goes through authz.Extractor before anything is
// written, and every write goes through an os.Root pinned on the directory.
// Inputs:  the blob reader, the destination directory, the sanitiser limits.
// Outputs: UnpackStats; *authz.PathError (E683) for a refused header.
// Constraints: traversal, absolute names, hard links, links out of the tree,
// devices, FIFOs, set-id bits, case collisions, writing through an earlier
// symlink and entry, size and ratio bombs are all refused by the Extractor; an
// error leaves a partial directory that the caller removes (RemoveTree). The
// decompressed stream is bounded, whatever the headers claim. Directory modes
// are applied last (owner access always kept) so a read-only directory can
// still be filled; file times and modes are restored, set-id bits never.
// The compression ratio is judged while the bytes are written (bombGuard):
// the sanitiser only sees a header, before its data was read, so it is given
// the entry's own size as its packed size and never trips on the ratio.

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
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

// countReader counts the compressed bytes consumed so far.
type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// ratioSlack is the compressed read-ahead the guard forgives (flate buffers 4 KiB).
const ratioSlack = 64 << 10

// bombGuard stops an archive whose output outgrows MaxRatio times the
// compressed bytes consumed so far, per entry and over the whole archive, once
// the output passes RatioFloor.
type bombGuard struct {
	cr    *countReader
	lim   authz.Limits
	total int64
}

type guardedEntry struct {
	g     *bombGuard
	r     io.Reader
	name  string
	start int64
	n     int64
}

func (g *bombGuard) entry(r io.Reader, name string) *guardedEntry {
	return &guardedEntry{g: g, r: r, name: name, start: g.cr.n}
}

func (e *guardedEntry) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	e.n += int64(n)
	e.g.total += int64(n)
	g := e.g
	if (e.n > g.lim.RatioFloor && e.n > g.lim.MaxRatio*(g.cr.n-e.start+ratioSlack)) || (g.total > g.lim.RatioFloor && g.total > g.lim.MaxRatio*(g.cr.n+ratioSlack)) {
		return n, &authz.PathError{Reason: authz.SizeRatio, Name: e.name}
	}
	return n, err
}

// effective fills the zero fields of lim with the defaults.
func effective(lim authz.Limits) authz.Limits {
	d := authz.DefaultLimits()
	if lim.MaxFileBytes > 0 {
		d.MaxFileBytes = lim.MaxFileBytes
	}
	if lim.MaxTotalBytes > 0 {
		d.MaxTotalBytes = lim.MaxTotalBytes
	}
	if lim.MaxEntries > 0 {
		d.MaxEntries = lim.MaxEntries
	}
	if lim.MaxRatio > 0 {
		d.MaxRatio = lim.MaxRatio
	}
	if lim.RatioFloor > 0 {
		d.RatioFloor = lim.RatioFloor
	}
	d.AllowDotGit = lim.AllowDotGit
	return d
}

type dirMode struct {
	name string
	mode os.FileMode
	at   time.Time
}

// Unpack extracts the tar+gzip stream r into dir (created 0700 if missing).
func Unpack(r io.Reader, dir string, lim authz.Limits) (UnpackStats, error) {
	var st UnpackStats
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return st, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return st, err
	}
	defer func() { _ = root.Close() }()
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
			if err := root.MkdirAll(name, 0o700); err != nil {
				return st, err
			}
			dirs = append(dirs, dirMode{name, os.FileMode(h.Mode&0o777) | 0o700, h.ModTime})
			st.Dirs++
		case tar.TypeSymlink:
			if err := parents(root, name); err != nil {
				return st, err
			}
			if err := root.Symlink(h.Linkname, name); err != nil {
				return st, err
			}
			st.Links++
		default:
			if err := writeFile(root, name, guard.entry(tr, h.Name), h); err != nil {
				return st, err
			}
			st.Files++
			st.Bytes += h.Size
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i].name) > len(dirs[j].name) })
	for _, d := range dirs {
		if err := root.Chmod(d.name, d.mode); err != nil {
			return st, err
		}
		_ = root.Chtimes(d.name, d.at, d.at)
	}
	return st, nil
}

func parents(root *os.Root, name string) error {
	if dir := path.Dir(name); dir != "." {
		return root.MkdirAll(dir, 0o700)
	}
	return nil
}

func writeFile(root *os.Root, name string, r io.Reader, h *tar.Header) error {
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
	if err := root.Chmod(name, os.FileMode(h.Mode&0o777)|0o400); err != nil {
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
