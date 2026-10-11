// Purpose: Restore streams a k8s backup into the postgres container with
// pg_restore --clean --if-exists and succeeds only when the post-restore table
// fingerprints equal the ones recorded at backup time.
//
// Inputs: an Exec into the postgres container and the dump path (its sidecar is
// <dir>/.meta/<file>.json).
//
// Outputs: the verified table fingerprints, or a coded error (E733 to E737).
//
// Constraints: nothing runs in the container until the sidecar is valid (E734) and
// the file's size and sha256 equal the recorded ones (E735). A pg_restore error
// line or a non-zero exit is E736 (core v1.5 strictness). The fingerprints are then
// read in one REPEATABLE READ READ ONLY transaction and compared; any difference is
// E737 and Restore never returns success before that comparison. A writer active
// during the restore makes the fingerprints differ (documented, E737).
package ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
)

// RestoreOptions is the input of Restore.
type RestoreOptions struct {
	Exec Exec
	File string
}

// RestoreResult is what a verified restore returns.
type RestoreResult struct {
	File   string
	Tables []Fingerprint
}

// Restore restores o.File and verifies it. See the file comment.
func Restore(ctx context.Context, o RestoreOptions) (RestoreResult, error) {
	var res RestoreResult
	fi, err := os.Stat(o.File)
	if err != nil || !fi.Mode().IsRegular() {
		return res, errInvalid("the backup file is missing or not a regular file", "")
	}
	meta, err := readMeta(o.File)
	if err != nil {
		return res, err
	}
	f, err := os.Open(o.File)
	if err != nil {
		return res, errInvalid("the backup file cannot be read", "")
	}
	defer func() { _ = f.Close() }()
	if err := verifyChecksum(f, meta); err != nil {
		return res, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return res, errInvalid("the backup file cannot be read", "")
	}
	// Hash again while streaming: a file changed after the check above is caught.
	h := sha256.New()
	ew := &errLineWriter{}
	runErr := o.Exec.Run(ctx, pgRestoreArgv, io.TeeReader(f, h), io.Discard, ew)
	ew.flush()
	if runErr != nil || ew.count > 0 {
		cause := ew.first
		if cause == "" {
			cause = exitCause(runErr)
		}
		return res, errRestore(cause)
	}
	if hex.EncodeToString(h.Sum(nil)) != meta.SHA256 {
		return res, errChecksum("the file changed while it was restored")
	}
	got, err := ReadFingerprints(ctx, o.Exec)
	if err != nil {
		return res, err
	}
	if diffs := Compare(meta.Fingerprints, got); len(diffs) > 0 {
		return res, errMismatch(diffs)
	}
	return RestoreResult{File: o.File, Tables: got}, nil
}

// verifyChecksum compares the size and sha256 of f with the sidecar.
func verifyChecksum(f *os.File, m Meta) error {
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return errInvalid("the backup file cannot be read", "")
	}
	if n != m.Bytes || hex.EncodeToString(h.Sum(nil)) != m.SHA256 {
		return errChecksum("")
	}
	return nil
}

// ReadFingerprints reads the fingerprints of the current database in one
// REPEATABLE READ READ ONLY transaction.
func ReadFingerprints(ctx context.Context, ex Exec) ([]Fingerprint, error) {
	script := sessionPrelude + "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n" + fingerprintSQL + "COMMIT;\n"
	var out strings.Builder
	serr := newBounded(stderrKeep)
	if err := ex.Run(ctx, psqlArgv, strings.NewReader(script), &out, serr); err != nil {
		return nil, errFingerprint(exitCause(err) + ": " + errorLine(serr.String()))
	}
	return parseFingerprints(strings.Split(out.String(), "\n"))
}

// errLineWriter reads pg_restore's stderr line by line, counting error lines and
// keeping the first one. It holds at most one partial line.
type errLineWriter struct {
	partial []byte
	count   int
	first   string
}

const maxLine = 4096

func (w *errLineWriter) Write(p []byte) (int, error) {
	for _, c := range p {
		if c == '\n' {
			w.line()
			continue
		}
		if len(w.partial) < maxLine {
			w.partial = append(w.partial, c)
		}
	}
	return len(p), nil
}

func (w *errLineWriter) flush() {
	if len(w.partial) > 0 {
		w.line()
	}
}

func (w *errLineWriter) line() {
	l := strings.TrimSpace(string(w.partial))
	w.partial = w.partial[:0]
	low := strings.ToLower(l)
	if strings.HasPrefix(low, "pg_restore: error:") || strings.HasPrefix(low, "error:") {
		w.count++
		if w.first == "" {
			w.first = l
		}
	}
}
