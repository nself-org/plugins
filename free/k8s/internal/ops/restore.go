// Purpose: Restore streams a k8s backup into the postgres container with
// pg_restore --clean --if-exists and succeeds only when the post-restore table
// fingerprints equal the ones recorded at backup time.
//
// Inputs: the dump path (PrepareRestore) and an Exec into the postgres container (Run) (its sidecar is
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
	"fmt"
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

// Prepared is a backup file whose sidecar is valid and whose size and sha256 equal
// the recorded ones. Nothing has touched the cluster yet.
type Prepared struct {
	file string
	meta Meta
}

// PrepareRestore runs the checks that need no cluster: the file, its sidecar (E734)
// and its checksum (E735). The caller looks up the pod only after this succeeds.
func PrepareRestore(file string) (*Prepared, error) {
	fi, err := os.Stat(file)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, errInvalid("the backup file is missing or not a regular file", "")
	}
	meta, err := readMeta(file)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, errInvalid("the backup file cannot be read", "")
	}
	defer func() { _ = f.Close() }()
	if err := verifyChecksum(f, meta); err != nil {
		return nil, err
	}
	return &Prepared{file: file, meta: meta}, nil
}

// TargetWarning names the release and namespace of the backup when they differ from
// the restore target ("" when they match or the target does not say).
func (p *Prepared) TargetWarning(release, namespace string) string {
	relDiff := p.meta.Release != "" && release != "" && p.meta.Release != release
	nsDiff := p.meta.Namespace != "" && namespace != "" && p.meta.Namespace != namespace
	if !relDiff && !nsDiff {
		return ""
	}
	return fmt.Sprintf("the backup was taken from release %q in namespace %q; restoring into release %q in namespace %q",
		p.meta.Release, p.meta.Namespace, release, namespace)
}

// Restore prepares and restores o.File and verifies it. See the file comment.
func Restore(ctx context.Context, o RestoreOptions) (RestoreResult, error) {
	p, err := PrepareRestore(o.File)
	if err != nil {
		return RestoreResult{}, err
	}
	return p.Run(ctx, o.Exec)
}

// Run restores the prepared file through ex and verifies the fingerprints.
func (p *Prepared) Run(ctx context.Context, ex Exec) (RestoreResult, error) {
	var res RestoreResult
	f, err := os.Open(p.file)
	if err != nil {
		return res, errInvalid("the backup file cannot be read", "")
	}
	defer func() { _ = f.Close() }()
	// Hash again while streaming: a file changed since the check is caught.
	h := sha256.New()
	in := &countReader{r: io.TeeReader(f, h)}
	ew := &errLineWriter{}
	runErr := ex.Run(ctx, pgRestoreArgv, in, io.Discard, ew)
	ew.flush()
	if runErr != nil || ew.count > 0 || !ew.finished() {
		return res, errRestore(ew.failure(runErr), ew.detail())
	}
	if in.n != p.meta.Bytes {
		// pg_restore ended cleanly without taking every byte (a short read), or the file
		// changed in size: that is not the same as a changed file of full length.
		return res, errRestoreShort()
	}
	if hex.EncodeToString(h.Sum(nil)) != p.meta.SHA256 {
		return res, errChangedDuringRestore()
	}
	got, err := ReadFingerprints(ctx, ex)
	if err != nil {
		return res, err
	}
	if diffs := Compare(p.meta.Fingerprints, got); len(diffs) > 0 {
		return res, errMismatch(diffs)
	}
	return RestoreResult{File: p.file, Tables: got}, nil
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
// REPEATABLE READ READ ONLY transaction. The script ends with an \echo of a random
// sentinel: kubectl can exit 0 on a cut-off stream, so the output counts only when the
// sentinel is its last line (E733 otherwise, also for empty output).
func ReadFingerprints(ctx context.Context, ex Exec) ([]Fingerprint, error) {
	sentinel := newSentinel()
	script := sessionPrelude + "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n" + fingerprintSQL + "COMMIT;\n\\echo " + sentinel + "\n"
	var out strings.Builder
	serr := newBounded(stderrKeep)
	if err := ex.Run(ctx, psqlArgv, strings.NewReader(script), &out, serr); err != nil {
		return nil, errFingerprint(exitCause(err))
	}
	lines, ok := readUntil(strings.NewReader(out.String()), sentinel)
	if !ok || !strings.HasSuffix(strings.TrimRight(out.String(), "\r\n"), sentinel) {
		return nil, errFingerprint("the fingerprint query did not finish: its output ended without the completion marker")
	}
	return parseFingerprints(lines)
}

// errLineWriter reads a pod command's stderr line by line. It counts error lines, keeps the
// first one, and tracks the exit marker (rcMarker): how many marker lines there were, the
// last status and whether the marker was the last non-empty line. It holds at most one
// partial line, so a marker split across writes is parsed once and memory stays bounded.
type errLineWriter struct {
	partial []byte
	count   int
	first   string
	rcCount int
	rc      string
	rcLast  bool
}

const maxLine = 4096

const rcPrefix = "NSELF_RC="

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
	if l == "" {
		return
	}
	if v, ok := strings.CutPrefix(l, rcPrefix); ok && isDigits(v) {
		w.rcCount++
		w.rc = v
		w.rcLast = true
		return
	}
	w.rcLast = false
	low := strings.ToLower(l)
	if strings.HasPrefix(low, "pg_restore: error:") || strings.HasPrefix(low, "pg_dump: error:") || strings.HasPrefix(low, "error:") {
		w.count++
		if w.first == "" {
			w.first = l
		}
	}
}

func isDigits(s string) bool {
	if s == "" || len(s) > 3 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// finished reports whether the command in the pod proved that it ran to the end with
// status 0: exactly one marker line, NSELF_RC=0, and it is the last line on stderr.
func (w *errLineWriter) finished() bool { return w.rcCount == 1 && w.rc == "0" && w.rcLast }

// failure describes why a pod command is not a success, in fixed words: the local exit
// status, the marker state and the number of error lines. It never contains text the
// database or the tool printed (that is detail()).
func (w *errLineWriter) failure(runErr error) string {
	var parts []string
	if runErr != nil {
		parts = append(parts, exitCause(runErr))
	}
	switch {
	case w.rcCount == 0:
		parts = append(parts, "the command in the pod did not report an exit status: the stream was cut off")
	case w.rcCount > 1:
		parts = append(parts, "the command in the pod reported its exit status more than once")
	case w.rc != "0":
		parts = append(parts, "the command in the pod exited with status "+w.rc)
	case !w.rcLast:
		parts = append(parts, "the exit status was not the last line from the pod")
	}
	if w.count > 0 {
		parts = append(parts, fmt.Sprintf("%d error line(s)", w.count))
	}
	if len(parts) == 0 {
		return "the command in the pod did not end cleanly"
	}
	return strings.Join(parts, "; ")
}

// detail is the first error line the command printed (database-provided text).
func (w *errLineWriter) detail() string { return w.first }
