// Purpose: unit tests of Restore against the fake container: the order of the
// checks, the refusals that run nothing, and that success needs equal fingerprints.
//
// Inputs: fake_test.go and a backup made by Backup.
//
// Outputs: test results.
//
// Constraints: no process, no docker.
package ops

import (
	"context"
	"os"
	"strings"
	"testing"
)

// madeBackup runs Backup on a fresh fake and returns the dump path; the returned
// fake is ready for a restore (post-restore fingerprints equal the recorded ones).
func madeBackup(t *testing.T) (*fakeExec, string) {
	t.Helper()
	f := newFake()
	o := backupOpts(f, t.TempDir())
	if _, err := Backup(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	f.calls, f.sessions = nil, nil
	f.postFps = f.fps
	return f, o.Path
}

func TestRestoreVerifiesFingerprints(t *testing.T) {
	f, path := madeBackup(t)
	res, err := Restore(context.Background(), RestoreOptions{Exec: f, File: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tables) != 3 {
		t.Errorf("tables = %+v", res.Tables)
	}
	if len(f.calls) != 2 || f.calls[0].kind != "pg_restore" || f.calls[1].kind != "psql" {
		t.Fatalf("calls = %+v, want pg_restore then the fingerprint psql", f.calls)
	}
	if got := strings.Join(f.calls[0].argv, " "); !strings.Contains(got, "--clean --if-exists") {
		t.Errorf("pg_restore argv = %q", got)
	}
	if string(f.restored) != string(f.dump) {
		t.Error("pg_restore did not receive the dump bytes on stdin")
	}
	if s := f.calls[1].stdin; !strings.Contains(s, "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;") || !strings.Contains(s, "SET extra_float_digits = 3;") {
		t.Errorf("fingerprint script = %q", s)
	}
}

func TestRestoreTamperedDumpRunsNothing(t *testing.T) {
	f, path := madeBackup(t)
	b, _ := os.ReadFile(path)
	b[len(b)/2] ^= 0x01
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Restore(context.Background(), RestoreOptions{Exec: f, File: path})
	oe := wantCode(t, err, CodeChecksum)
	if oe.Class != ClassInfra {
		t.Errorf("class = %s", oe.Class)
	}
	if len(f.calls) != 0 {
		t.Errorf("calls = %+v, want none", f.calls)
	}
}

func TestRestoreTruncatedDumpRunsNothing(t *testing.T) {
	f, path := madeBackup(t)
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(path, b[:len(b)-1], 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Restore(context.Background(), RestoreOptions{Exec: f, File: path})
	wantCode(t, err, CodeChecksum)
	if len(f.calls) != 0 {
		t.Errorf("calls = %+v", f.calls)
	}
}

func TestRestoreMissingOrBadSidecarRunsNothing(t *testing.T) {
	cases := map[string]func(path string){
		"missing":  func(p string) { _ = os.Remove(MetaPath(p)) },
		"not json": func(p string) { _ = os.WriteFile(MetaPath(p), []byte("{nope"), 0o600) },
		"wrong kind": func(p string) {
			_ = os.WriteFile(MetaPath(p), []byte(`{"schema_version":"1","kind":"x","sha256":"`+strings.Repeat("a", 64)+`","fingerprints":[]}`), 0o600)
		},
		"bad sum": func(p string) {
			_ = os.WriteFile(MetaPath(p), []byte(`{"schema_version":"1","kind":"k8s-backup","sha256":"zz","fingerprints":[]}`), 0o600)
		},
		"no fps": func(p string) {
			_ = os.WriteFile(MetaPath(p), []byte(`{"schema_version":"1","kind":"k8s-backup","sha256":"`+strings.Repeat("a", 64)+`"}`), 0o600)
		},
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			f, path := madeBackup(t)
			mut(path)
			_, err := Restore(context.Background(), RestoreOptions{Exec: f, File: path})
			wantCode(t, err, CodeBackupInvalid)
			if len(f.calls) != 0 {
				t.Errorf("calls = %+v", f.calls)
			}
		})
	}
}

func TestRestoreMissingFileAndDirectory(t *testing.T) {
	f, path := madeBackup(t)
	for _, p := range []string{path + ".nope", strings.TrimSuffix(path, "/demo_full_20261011_040506.dump")} {
		_, err := Restore(context.Background(), RestoreOptions{Exec: f, File: p})
		wantCode(t, err, CodeBackupInvalid)
	}
	if len(f.calls) != 0 {
		t.Errorf("calls = %+v", f.calls)
	}
}

func TestRestoreFingerprintMismatchIsNeverSuccess(t *testing.T) {
	f, path := madeBackup(t)
	f.postFps = []string{
		`{"schema":"auth","table":"users","rows":2,"sum":"12"}`,
		`{"schema":"public","table":"t","rows":999,"sum":"-5"}`,
		`{"schema":"public","table":"extra","rows":1,"sum":"1"}`,
	}
	res, err := Restore(context.Background(), RestoreOptions{Exec: f, File: path})
	oe := wantCode(t, err, CodeMismatch)
	if len(res.Tables) != 0 || res.File != "" {
		t.Errorf("a failed restore returned a result: %+v", res)
	}
	for _, want := range []string{"auth.users content differs", "public.t rows 1000 != 999", "public.extra extra", "storage.files missing"} {
		if !strings.Contains(oe.Cause, want) {
			t.Errorf("cause %q lacks %q", oe.Cause, want)
		}
	}
	if oe.Class != ClassInfra {
		t.Errorf("class = %s", oe.Class)
	}
}

func TestRestoreMismatchListsAtMostTwentyDiffs(t *testing.T) {
	var want []Fingerprint
	for i := 0; i < 30; i++ {
		want = append(want, Fingerprint{Schema: "s", Table: "t" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Rows: 1, Sum: "1"})
	}
	oe := errMismatch(Compare(want, nil))
	if n := strings.Count(oe.Cause, "missing"); n != maxDiffs || !strings.Contains(oe.Cause, "and 10 more") {
		t.Errorf("cause lists %d missing, want %d plus 'and 10 more': %s", n, maxDiffs, oe.Cause)
	}
}

func TestRestoreErrorLineFailsEvenAtExitZero(t *testing.T) {
	f, path := madeBackup(t)
	f.restoreErr = "pg_restore: warning: harmless\npg_restore: error: could not execute query: ERROR:  relation \"x\" does not exist\npg_restore: error: second\n"
	_, err := Restore(context.Background(), RestoreOptions{Exec: f, File: path})
	oe := wantCode(t, err, CodeRestoreFailed)
	if !strings.Contains(oe.Cause, "could not execute query") || strings.Contains(oe.Cause, "second") {
		t.Errorf("cause = %q, want the first error line only", oe.Cause)
	}
	if f.count("psql") != 0 {
		t.Error("fingerprints were read after a failed pg_restore")
	}
}

func TestRestoreNonZeroExitFails(t *testing.T) {
	f, path := madeBackup(t)
	f.restoreRC = os.ErrDeadlineExceeded
	_, err := Restore(context.Background(), RestoreOptions{Exec: f, File: path})
	wantCode(t, err, CodeRestoreFailed)
}

func TestRestoreFingerprintQueryFailure(t *testing.T) {
	f, path := madeBackup(t)
	f.postFps = []string{"this is not json"}
	_, err := Restore(context.Background(), RestoreOptions{Exec: f, File: path})
	wantCode(t, err, CodeFingerprint)
}

func TestErrLineWriterSplitsAcrossWrites(t *testing.T) {
	w := &errLineWriter{}
	for _, part := range []string{"pg_rest", "ore: error: bo", "om\nnext line without newline"} {
		_, _ = w.Write([]byte(part))
	}
	w.flush()
	if w.count != 1 || w.first != "pg_restore: error: boom" {
		t.Errorf("count=%d first=%q", w.count, w.first)
	}
}

func TestRestoreFileChangedDuringStreamHasItsOwnMessage(t *testing.T) {
	f, path := madeBackup(t)
	f.onRestore = func() {
		fh, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = fh.Close() }()
		_, _ = fh.WriteAt([]byte("X"), 2)
	}
	_, err := Restore(context.Background(), RestoreOptions{Exec: f, File: path})
	oe := wantCode(t, err, CodeChecksum)
	if !strings.Contains(oe.Message, "pg_restore ran") || !strings.Contains(oe.Message, "not verified") || strings.Contains(oe.Message, "nothing was restored") {
		t.Errorf("message = %q", oe.Message)
	}
	if f.count("psql") != 0 {
		t.Error("fingerprints were read after the file changed")
	}
}

func TestPrepareRestoreChecksWithoutACluster(t *testing.T) {
	_, path := madeBackup(t)
	p, err := PrepareRestore(path)
	if err != nil {
		t.Fatal(err)
	}
	if w := p.TargetWarning("nself", "ns1"); w != "" {
		t.Errorf("same target warned: %s", w)
	}
	if w := p.TargetWarning("nself", ""); w != "" {
		t.Errorf("unknown namespace warned: %s", w)
	}
	w := p.TargetWarning("other", "ns2")
	for _, want := range []string{`"nself"`, `"ns1"`, `"other"`, `"ns2"`} {
		if !strings.Contains(w, want) {
			t.Errorf("warning %q lacks %s", w, want)
		}
	}
	b, _ := os.ReadFile(path)
	b[0] ^= 1
	_ = os.WriteFile(path, b, 0o600)
	_, err = PrepareRestore(path)
	wantCode(t, err, CodeChecksum)
	_ = os.Remove(MetaPath(path))
	_, err = PrepareRestore(path)
	wantCode(t, err, CodeBackupInvalid)
}
