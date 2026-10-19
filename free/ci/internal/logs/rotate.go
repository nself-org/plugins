package logs

// Purpose: the file operations behind the writer: rotating the active file
// into a gzip segment, making directory entries durable, and keeping the log
// tree private.
// Inputs:  the writer's state (called with its lock held) or plain paths.
// Outputs: segments named by their sequence range and raw size.
// Constraints: the segment is fsynced and renamed into place, and its directory
// synced, before the old active file is removed, so a crash in between leaves
// both and Open drops the duplicate; directories are 0700, files 0600, and a
// symlink or non-regular file where the log should be is refused.

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// rotateTmp is the name of the segment being written.
const rotateTmp = ".rotate.tmp"

// rotate gzips the active file into a segment and starts a new active file. The
// segment is renamed into place before the old file goes, so a crash in
// between leaves both and Open drops the duplicate.
func (w *Writer) rotate() error {
	if w.activeSize == 0 {
		return nil
	}
	active := filepath.Join(w.dir, activeName)
	tmp := filepath.Join(w.dir, rotateTmp)
	if err := gzipFile(active, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	fault("rotate-gzipped")
	if err := os.Rename(tmp, filepath.Join(w.dir, segmentName(w.activeFirst, w.last, w.activeSize))); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := syncDir(w.dir); err != nil { // the segment must be durable before the old file goes
		return err
	}
	fault("rotate-renamed")
	_ = w.f.Close()
	w.f = nil
	if err := os.Remove(active); err != nil {
		w.broken = fail(CodeStore, "cannot replace the rotated log file: %v", err)
		return w.broken
	}
	fault("rotate-removed")
	f, err := os.OpenFile(active, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, fileMode)
	if err != nil {
		w.broken = fail(CodeStore, "cannot start a new log file: %v", err)
		return w.broken
	}
	w.f, w.activeFirst, w.activeSize = f, 0, 0
	return syncDir(w.dir)
}

func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	if _, err = io.Copy(zw, in); err == nil {
		err = zw.Close()
	}
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// syncDir makes a rename or create durable. Windows cannot fsync a directory
// and skips it; everywhere else a failure is returned.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	return err
}

// preparePrivate creates dir and its parent as 0700 directories, or makes the
// existing ones 0700; a symlink or a non-directory in their place is refused,
// so a planted link cannot redirect the log (or its recovery truncation).
func preparePrivate(dir string) error {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fail(CodeStore, "cannot create the log directory: %v", err)
	}
	for _, d := range []string{dir, filepath.Dir(dir)} {
		fi, err := os.Lstat(d)
		if err != nil || !fi.IsDir() {
			return fail(CodeStore, "%s is not a plain directory", filepath.Base(d))
		}
		if fi.Mode().Perm()&0o077 != 0 {
			if err = os.Chmod(d, dirMode); err != nil && runtime.GOOS != "windows" {
				return fail(CodeStore, "cannot make %s private: %v", filepath.Base(d), err)
			}
		}
	}
	return nil
}

// requireRegular refuses an existing path that is a symlink or not a regular file.
func requireRegular(path string) error {
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !fi.Mode().IsRegular() {
		return fail(CodeStore, "%s is not a regular file", filepath.Base(path))
	}
	return nil
}
