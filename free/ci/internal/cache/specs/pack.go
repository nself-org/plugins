package specs

// Purpose: write one spec directory as a tar stream (Epic CACHE D5): regular
// files, directories and symlinks that stay inside the tree, nothing else.
// Inputs:  the directory, a writer, the sanitiser limits.
// Outputs: PackStats; ErrTooLarge when a limit is exceeded.
// Constraints: reads go through an os.Root pinned on the directory, so a path
// swapped by a lingering process cannot redirect them; every entry passes the
// same authz.Extractor a restore applies, so what is packed is restorable (an
// entry the extractor refuses is skipped and counted, never written); headers
// are normalized (no owner names, no set-id bits, second resolution times).

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
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

// Pack writes dir to w as tar.
func Pack(dir string, w io.Writer, lim authz.Limits) (PackStats, error) {
	var st PackStats
	root, err := os.OpenRoot(dir)
	if err != nil {
		return st, err
	}
	defer func() { _ = root.Close() }()
	tw, ext := tar.NewWriter(w), authz.NewExtractor(lim)
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
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
			if err := copyFile(root, p, tw, h.Size); err != nil {
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

// copyFile copies exactly size bytes of p; a file that shrank is an error.
func copyFile(root *os.Root, p string, w io.Writer, size int64) error {
	f, err := root.Open(p)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if n, err := io.CopyN(w, f, size); err != nil {
		return fmt.Errorf("specs: %q changed while it was packed (%d of %d bytes): %w", p, n, size, err)
	}
	return nil
}
