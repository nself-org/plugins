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
// <dir>/.meta/<name>.partial (O_EXCL, 0600; .meta is 0700 and ignored by core "nself
// backup", which reads every top-level file of <dir>). Only after the dump is checked and
// the sidecar is written is the partial linked to <dir>/<name> without overwriting, so an
// interrupted backup never leaves a truncated file under a core backup name. On any
// failure the partial and the sidecar are removed. A killed process (SIGKILL) can leave a
// .partial file in .meta; it is safe to delete.
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
	Warn      func(string) // a problem that does not invalidate the backup; may be nil
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
		return res, errExists()
	}
	dir := filepath.Dir(o.Path)
	partial := partialPath(o.Path)
	if err := os.MkdirAll(filepath.Dir(partial), 0o700); err != nil {
		return res, errBackup("the backup directory cannot be created")
	}
	psqlErr := newBounded(stderrKeep)
	sess, err := o.Exec.Start(ctx, psqlArgv, psqlErr)
	if err != nil {
		return res, errBackup("the psql session could not be started")
	}
	ended, created, metaWritten := false, false, false
	var eof chan struct{} // closed when the session's stdout ends
	defer func() {
		if !ended {
			_ = sess.Stdin().Close()
			sess.Kill()
			_ = sess.Wait()
		}
		if err != nil {
			if created {
				_ = os.Remove(partial)
			}
			if metaWritten {
				_ = os.Remove(MetaPath(o.Path))
			}
		}
	}()

	snap, version, fps, err := openSnapshot(sess, psqlErr)
	if err != nil {
		return res, err
	}
	argv, _ := pgDumpArgv(snap) // snap was validated by openSnapshot
	eof = make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, sess.Stdout())
		close(eof)
	}()
	f, ferr := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if ferr != nil {
		if errors.Is(ferr, os.ErrExist) {
			err = errTargetWith("a partial backup of this name exists in .meta: another backup is running or one was killed",
				"wait for the other backup, or delete "+filepath.Base(partial)+" from the .meta directory")
		} else {
			err = errBackup("the backup file cannot be created")
		}
		return res, err
	}
	created = true
	h := sha256.New()
	cw := &countWriter{}
	dumpErr := &errLineWriter{}
	runErr := o.Exec.Run(ctx, argv, nil, io.MultiWriter(f, h, cw), dumpErr)
	dumpErr.flush()
	if serr := f.Sync(); runErr == nil && serr != nil {
		runErr = serr
	}
	if cerr := f.Close(); runErr == nil && cerr != nil {
		runErr = cerr
	}
	if runErr != nil || dumpErr.count > 0 || !dumpErr.finished() {
		e := errBackup(dumpErr.failure(runErr))
		e.Detail = dumpErr.detail()
		err = e
		return res, err
	}
	if cw.n == 0 {
		err = errBackup("pg_dump wrote no data")
		return res, err
	}
	select {
	case <-eof: // the session ended while pg_dump was still running
		err = errBackup("the psql session holding the snapshot ended before pg_dump finished")
		return res, err
	default:
	}
	// The dump is complete: read it back through pg_restore (no database) to prove it is a
	// whole, readable archive before it gets a checksum and a sidecar.
	if err = verifyDump(ctx, o.Exec, partial, cw.n); err != nil {
		return res, err
	}
	// pg_dump exited 0, so the snapshot was imported and the fingerprints were read before
	// it: a session that fails now (COMMIT, exit status) does not change the backup.
	if werr := finishSession(sess, eof, &ended); werr != nil && o.Warn != nil {
		o.Warn("the psql session that held the snapshot ended with an error after pg_dump finished; the dump and its fingerprints were taken before it and are not affected")
	}
	meta := Meta{SchemaVersion: metaVersion, Kind: metaKind, Project: o.Project, File: filepath.Base(o.Path),
		Bytes: cw.n, SHA256: hex.EncodeToString(h.Sum(nil)), CreatedAt: o.Now().UTC().Format(time.RFC3339),
		ServerVersion: version, Release: o.Release, Namespace: podNamespace(o), Fingerprints: fps}
	// A file with the final name may have appeared while the dump ran (two backups in the
	// same second, a user-made --output): do not write a sidecar next to it.
	if _, serr := os.Lstat(o.Path); serr == nil {
		err = errExists()
		return res, err
	}
	if werr := writeMeta(o.Path, meta); werr != nil {
		var oe *Error
		if errors.As(werr, &oe) {
			err = oe // an existing sidecar is left alone; metaWritten stays false
		} else {
			err = errBackup("the metadata file could not be written")
		}
		return res, err
	}
	metaWritten = true // our own link succeeded: the cleanup may remove this sidecar
	if err = publish(partial, o.Path); err != nil {
		return res, err
	}
	created = false // the partial is now the backup
	syncDir(dir)
	syncDir(filepath.Dir(partial))
	return BackupResult{File: o.Path, MetaFile: MetaPath(o.Path), Bytes: meta.Bytes, SHA256: meta.SHA256, Tables: fps}, nil
}

// openSnapshot sends the opening script and reads the snapshot id, the server
// version and the fingerprints, up to the sentinel. The session stays open.
func openSnapshot(sess Session, stderr *boundedBuffer) (snap, version string, fps []Fingerprint, err error) {
	sentinel := newSentinel()
	script := sessionPrelude +
		"BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n" +
		"SELECT pg_export_snapshot();\n" +
		lockSQL +
		"SHOW server_version_num;\n" +
		fingerprintSQL +
		"\\echo " + sentinel + "\n"
	_, _ = io.WriteString(sess.Stdin(), script) // a dead psql shows as EOF below
	lines, ok := readUntil(sess.Stdout(), sentinel)
	if !ok {
		return "", "", nil, errFingerprint("the psql session ended before it returned the snapshot and the fingerprints")
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
func finishSession(sess Session, eof <-chan struct{}, ended *bool) error {
	_, werr := io.WriteString(sess.Stdin(), "COMMIT;\n\\q\n")
	_ = sess.Stdin().Close()
	<-eof
	*ended = true
	if err := sess.Wait(); err != nil {
		return err
	}
	return werr
}
