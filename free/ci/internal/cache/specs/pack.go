package specs

// Purpose: write one spec directory as a tar stream (Epic CACHE D5): regular
// files, directories and symlinks that stay inside the tree, nothing else.
// Inputs:  the directory, a writer, the sanitiser limits.
// Outputs: PackStats; ErrTooLarge when a limit is exceeded.
// Constraints: reads go through an os.Root pinned on the attempt's cache root
// before the job started, so a path swapped by a lingering process cannot
// redirect them; a file is opened non-blocking and must still be a regular file
// once open (a swapped-in FIFO cannot hang the save), and the walk stops when the
// context ends. A file with several hard links is archived like any other: the
// job runs as the operator and could copy the same bytes, and pnpm's store is
// made of such files. Every entry passes the
// same authz.Extractor a restore applies, so what is packed is restorable (an
// entry the extractor refuses is skipped and counted, never written); headers
// are normalized (no owner names, no set-id bits, second resolution times).

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// ErrTooLarge means the directory exceeds an entry-count, file-size or total
// limit; saving a truncated cache would hide the problem, so the save stops.
var ErrTooLarge = errors.New("specs: the cache directory exceeds the archive limits")

// PackStats counts what Pack wrote and skipped.
type PackStats struct {
	Files, Dirs, Links, Skipped int
	Bytes                       int64
}

// Pack writes the directory rel of base to w as tar.
func Pack(ctx context.Context, base *os.Root, rel string, w io.Writer, lim authz.Limits) (PackStats, error) {
	var st PackStats
	root, err := base.OpenRoot(rel)
	if err != nil {
		return st, err
	}
	defer func() { _ = root.Close() }()
	tw, ext := tar.NewWriter(w), authz.NewExtractor(lim)
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, ok, err := header(root, p, info)
		if err != nil {
			return err
		}
		if !ok {
			st.Skipped++
			return skipDir(d)
		}
		e := entryOf(h, h.Size) // packed size is unknown before compression: a ratio of 1
		if _, aerr := ext.Admit(e); aerr != nil {
			if tooLarge(aerr) {
				return fmt.Errorf("%w: %v", ErrTooLarge, aerr)
			}
			st.Skipped++
			return skipDir(d)
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			st.Dirs++
		case tar.TypeSymlink:
			st.Links++
		default:
			if err := copyFile(ctx, root, p, tw, h.Size); err != nil {
				return err
			}
			st.Files++
			st.Bytes += h.Size
		}
		return nil
	})
	if err != nil {
		return st, err
	}
	return st, tw.Close()
}

func skipDir(d fs.DirEntry) error {
	if d.IsDir() {
		return fs.SkipDir
	}
	return nil
}

func tooLarge(err error) bool {
	var pe *authz.PathError
	if !errors.As(err, &pe) {
		return false
	}
	switch pe.Reason {
	case authz.SizeFile, authz.SizeTotal, authz.SizeEntries:
		return true
	}
	return false
}

// header builds the normalized header of p; ok is false for a type the
// archive does not carry.
func header(root *os.Root, p string, info fs.FileInfo) (*tar.Header, bool, error) {
	var link string
	switch t := info.Mode().Type(); t {
	case 0, fs.ModeDir:
	case fs.ModeSymlink:
		var err error
		if link, err = root.Readlink(p); err != nil {
			return nil, false, err
		}
	default:
		return nil, false, nil
	}
	h, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return nil, false, err
	}
	h.Name, h.Uid, h.Gid, h.Uname, h.Gname = p, 0, 0, "", ""
	h.Mode &= 0o777
	h.ModTime, h.AccessTime, h.ChangeTime = info.ModTime().Truncate(time.Second), time.Time{}, time.Time{}
	h.Format = tar.FormatPAX
	if h.Typeflag == tar.TypeDir {
		h.Name = strings.TrimSuffix(p, "/") + "/"
	}
	return h, true, nil
}

// copyFile copies exactly size bytes of p; a file that shrank, or that is no
// longer a regular file once opened, is an error.
func copyFile(ctx context.Context, root *os.Root, p string, w io.Writer, size int64) error {
	f, err := root.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("specs: %q is no longer a regular file", p)
	}
	if n, err := io.CopyN(w, &ctxReader{ctx, f}, size); err != nil {
		return fmt.Errorf("specs: %q changed while it was packed (%d of %d bytes): %w", p, n, size, err)
	}
	return nil
}

// ctxReader stops a copy when the context ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
