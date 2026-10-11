// Purpose: Backup streams pg_dump -Fc out of the postgres container into a file in
// the core backup layout, with a sidecar of per-table content fingerprints read
// in the same snapshot as the dump.
//
// Inputs: an Exec into the postgres container, the target path (target.go), the
// project, release and namespace to record.
//
// Outputs: the dump file (0600, O_EXCL), its sidecar (sidecar.go) and a result
// with the size, the sha256 and the table fingerprints.
//
// Constraints: one psql session runs BEGIN ISOLATION LEVEL REPEATABLE READ READ
// ONLY, exports a snapshot, reads the fingerprints and stays open while
// `pg_dump --snapshot` streams, so the dump and the fingerprints describe the same
// moment even with writers active. The stream goes through a sha256 hasher into
// the file; nothing is copied inside the pod. On any failure the partial dump is
// removed and no sidecar is written.
package ops

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// newSentinel returns the marker line psql echoes after the fingerprints. Tests
// replace it.
var newSentinel = func() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "NSELF_FP_DONE_" + hex.EncodeToString(b)
}

var versionRe = regexp.MustCompile(`^[0-9]{5,7}$`)

// stderrKeep bounds the stderr kept from a pod command.
const stderrKeep = 64 << 10

// BackupOptions is the input of Backup.
type BackupOptions struct {
	Exec      Exec
	Path      string // dump file to create (must not exist)
	Project   string
	Release   string
	Namespace string
	Now       func() time.Time
}

// BackupResult describes the backup that was made.
type BackupResult struct {
	File     string
	MetaFile string
	Bytes    int64
	SHA256   string
	Tables   []Fingerprint
}

// Backup makes one backup. See the file comment.
func Backup(ctx context.Context, o BackupOptions) (BackupResult, error) {
	var res BackupResult
	if _, err := os.Lstat(o.Path); err == nil {
		return res, errTarget("the backup file already exists: nothing was written", "")
	}
	if err := os.MkdirAll(filepath.Dir(o.Path), 0o700); err != nil {
		return res, errBackup("the backup directory cannot be created")
	}
	psqlErr := newBounded(stderrKeep)
	sess, err := o.Exec.Start(ctx, psqlArgv, psqlErr)
	if err != nil {
		return res, errBackup("the psql session could not be started")
	}
	ended, created := false, false
	defer func() {
		if !ended {
			_ = sess.Stdin().Close()
			sess.Kill()
			_ = sess.Wait()
		}
		if err != nil && created {
			_ = os.Remove(o.Path)
		}
	}()

	snap, version, fps, err := openSnapshot(sess, psqlErr)
	if err != nil {
		return res, err
	}
	argv, _ := pgDumpArgv(snap) // snap was validated by openSnapshot
	f, ferr := os.OpenFile(o.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if ferr != nil {
		if errors.Is(ferr, os.ErrExist) {
			err = errTarget("the backup file already exists: nothing was written", "")
		} else {
			err = errBackup("the backup file cannot be created")
		}
		return res, err
	}
	created = true
	h := sha256.New()
	cw := &countWriter{}
	dumpErr := newBounded(stderrKeep)
	runErr := o.Exec.Run(ctx, argv, nil, io.MultiWriter(f, h, cw), dumpErr)
	if serr := f.Sync(); runErr == nil && serr != nil {
		runErr = serr
	}
	if cerr := f.Close(); runErr == nil && cerr != nil {
		runErr = cerr
	}
	if runErr != nil {
		err = errBackup(exitCause(runErr) + ": " + errorLine(dumpErr.String()))
		return res, err
	}
	if cw.n == 0 {
		err = errBackup("pg_dump wrote no data")
		return res, err
	}
	if err = finishSession(sess, &ended); err != nil {
		return res, err
	}
	meta := Meta{SchemaVersion: metaVersion, Kind: metaKind, Project: o.Project, File: filepath.Base(o.Path),
		Bytes: cw.n, SHA256: hex.EncodeToString(h.Sum(nil)), CreatedAt: o.Now().UTC().Format(time.RFC3339),
		ServerVersion: version, Release: o.Release, Namespace: o.Namespace, Fingerprints: fps}
	if werr := writeMeta(o.Path, meta); werr != nil {
		err = errBackup("the metadata file could not be written")
		return res, err
	}
	return BackupResult{File: o.Path, MetaFile: MetaPath(o.Path), Bytes: meta.Bytes, SHA256: meta.SHA256, Tables: fps}, nil
}

// openSnapshot sends the opening script and reads the snapshot id, the server
// version and the fingerprints, up to the sentinel. The session stays open.
func openSnapshot(sess Session, stderr *boundedBuffer) (snap, version string, fps []Fingerprint, err error) {
	sentinel := newSentinel()
	script := sessionPrelude +
		"BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n" +
		"SELECT pg_export_snapshot();\n" +
		"SHOW server_version_num;\n" +
		fingerprintSQL +
		"\\echo " + sentinel + "\n"
	_, _ = io.WriteString(sess.Stdin(), script) // a dead psql shows as EOF below
	lines, ok := readUntil(sess.Stdout(), sentinel)
	if !ok {
		return "", "", nil, errFingerprint(errorLine(stderr.String()))
	}
	if len(lines) < 2 {
		return "", "", nil, errFingerprint("psql did not return a snapshot")
	}
	snap, version = strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	if _, valid := pgDumpArgv(snap); !valid {
		return "", "", nil, errFingerprint("psql returned something that is not a snapshot id")
	}
	if !versionRe.MatchString(version) {
		return "", "", nil, errFingerprint("psql returned something that is not a server version")
	}
	fps, err = parseFingerprints(lines[2:])
	return snap, version, fps, err
}

// readUntil returns the lines before the line equal to sentinel; false when the
// stream ends first.
func readUntil(r io.Reader, sentinel string) ([]string, bool) {
	br := bufio.NewReader(r)
	var lines []string
	for {
		l, err := br.ReadString('\n')
		l = strings.TrimRight(l, "\r\n")
		if l == sentinel {
			return lines, true
		}
		if l != "" {
			lines = append(lines, l)
		}
		if err != nil {
			return lines, false
		}
	}
}

// finishSession commits the snapshot transaction, quits psql and waits for it.
func finishSession(sess Session, ended *bool) error {
	_, _ = io.WriteString(sess.Stdin(), "COMMIT;\n\\q\n")
	_ = sess.Stdin().Close()
	_, _ = io.Copy(io.Discard, sess.Stdout())
	*ended = true
	if err := sess.Wait(); err != nil {
		return errBackup("the psql session ended with " + exitCause(err))
	}
	return nil
}

// countWriter counts the bytes written to it.
type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) { c.n += int64(len(p)); return len(p), nil }
