// Purpose: the file side of Backup: where the partial dump lives, publishing it under its
// final name without overwriting, the read-back check of the finished dump, and the small
// counting helpers.
//
// Inputs: the final dump path, the Exec for the check, the size that was streamed.
//
// Outputs: partialPath, publish (hard link, then rename fallback), syncDir, verifyDump
// (E732 on a short read, a missing exit marker or an error line), podNamespace.
//
// Constraints: the partial is <dir>/.meta/<name>.partial, never a top-level file: core
// "nself backup" reads every top-level file of the backup directory. Nothing here prints
// database-provided text; verifyDump keeps the tool's first error line in Error.Detail.
package ops

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// partialPath is where a dump is written before it is published: <dir>/.meta/<name>.partial.
func partialPath(final string) string {
	return filepath.Join(filepath.Dir(final), metaDir, filepath.Base(final)+".partial")
}

// publish gives the finished partial its final name without overwriting an existing file:
// a hard link (which fails if final exists), then the partial is removed. On a file system
// without hard links it falls back to a rename after a second existence check.
func publish(partial, final string) error {
	err := os.Link(partial, final)
	if err == nil {
		_ = os.Remove(partial)
		return nil
	}
	if errors.Is(err, os.ErrExist) {
		return errExists()
	}
	if _, serr := os.Lstat(final); serr == nil {
		return errExists()
	}
	if rerr := os.Rename(partial, final); rerr != nil {
		return errBackup("the finished dump cannot be moved to its final name")
	}
	return nil
}

// syncDir flushes a directory entry to disk (best effort: not every platform allows it).
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// verifyDump streams the stored file into `pg_restore -f /dev/null` in the pod. A
// truncated or corrupt archive fails there (E732); the caller removes the file. The
// check must have read the whole file (size bytes) and the pod must report exit status 0.
func verifyDump(ctx context.Context, ex Exec, path string, size int64) error {
	f, err := os.Open(path)
	if err != nil {
		return errBackup("the finished dump cannot be read back")
	}
	defer func() { _ = f.Close() }()
	in := &countReader{r: f}
	ew := &errLineWriter{}
	runErr := ex.Run(ctx, pgVerifyArgv, in, io.Discard, ew)
	ew.flush()
	if runErr != nil || ew.count > 0 || !ew.finished() {
		e := errBackup("the dump is not a complete archive: " + ew.failure(runErr))
		e.Detail = ew.detail()
		return e
	}
	if in.n != size {
		return errBackup("the dump check did not read the whole file")
	}
	return nil
}

// countReader counts the bytes read from r.
type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// countWriter counts the bytes written to it.
type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) { c.n += int64(len(p)); return len(p), nil }

// podNamespace is the namespace recorded in the sidecar: the pod's real one, else the flag.
func podNamespace(o BackupOptions) string {
	if ns := PodNamespace(o.Exec); ns != "" {
		return ns
	}
	return o.Namespace
}
