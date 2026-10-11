// Purpose: unit tests of Backup against the fake container.
//
// Inputs: fake_test.go.
//
// Outputs: test results.
//
// Constraints: no process, no docker; the integration test is pg_integration_test.go.
package ops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fixedNow = func() time.Time { return time.Date(2026, 10, 11, 4, 5, 6, 0, time.UTC) }

func init() { newSentinel = func() string { return "NSELF_FP_DONE_test" } }

func backupOpts(f *fakeExec, dir string) BackupOptions {
	return BackupOptions{Exec: f, Path: filepath.Join(dir, "backups", "demo_full_20261011_040506.dump"),
		Project: "demo", Release: "nself", Namespace: "ns1", Now: fixedNow}
}

func wantCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	var oe *Error
	if !errors.As(err, &oe) || oe.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
	return oe
}

func TestBackupHappyPath(t *testing.T) {
	f := newFake()
	o := backupOpts(f, t.TempDir())
	res, err := Backup(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(o.Path)
	if string(got) != string(f.dump) || res.Bytes != int64(len(f.dump)) {
		t.Fatalf("dump file = %q (%d bytes), want the streamed bytes", got, res.Bytes)
	}
	if fi, _ := os.Stat(o.Path); fi.Mode().Perm() != 0o600 {
		t.Errorf("dump mode = %v, want 0600", fi.Mode().Perm())
	}
	if fi, _ := os.Stat(filepath.Dir(o.Path)); fi.Mode().Perm() != 0o700 {
		t.Errorf("backup dir mode = %v, want 0700", fi.Mode().Perm())
	}
	mp := MetaPath(o.Path)
	if fi, _ := os.Stat(mp); fi.Mode().Perm() != 0o600 || !strings.HasSuffix(mp, ".meta/demo_full_20261011_040506.dump.json") {
		t.Errorf("sidecar %s mode %v", mp, fi.Mode().Perm())
	}
	if fi, _ := os.Stat(filepath.Dir(mp)); fi.Mode().Perm() != 0o700 {
		t.Errorf(".meta mode = %v, want 0700", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(mp))
	if len(entries) != 1 {
		t.Errorf(".meta holds %d entries, want only the sidecar", len(entries))
	}
	b, _ := os.ReadFile(mp)
	want := `{
  "schema_version": "1",
  "kind": "k8s-backup",
  "project": "demo",
  "file": "demo_full_20261011_040506.dump",
  "bytes": 21,
  "sha256": "` + res.SHA256 + `",
  "created_at": "2026-10-11T04:05:06Z",
  "server_version": "160004",
  "release": "nself",
  "namespace": "ns1",
  "fingerprints": [
    {"schema": "auth", "table": "users", "rows": 2, "sum": "11"},
    {"schema": "public", "table": "t", "rows": 1000, "sum": "-5"},
    {"schema": "storage", "table": "files", "rows": 0, "sum": "0"}
  ]
}`
	var a, w any
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal([]byte(want), &w)
	ab, _ := json.Marshal(a)
	wb, _ := json.Marshal(w)
	if string(ab) != string(wb) {
		t.Errorf("sidecar =\n%s\nwant\n%s", b, want)
	}
	if len(res.SHA256) != 64 || len(res.Tables) != 3 {
		t.Errorf("result = %+v", res)
	}
}

func TestBackupSnapshotHandshakeOrder(t *testing.T) {
	f := newFake()
	if _, err := Backup(context.Background(), backupOpts(f, t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 || f.calls[0].kind != "psql-session" || f.calls[1].kind != "pg_dump" || f.calls[2].kind != "pg_verify" {
		t.Fatalf("calls = %+v, want psql-session, pg_dump, pg_verify", f.calls)
	}
	if f.calls[2].stdin != "<dump>" || strings.Contains(strings.Join(f.calls[2].argv, " "), "-d ") {
		t.Errorf("the verify step must read the dump on stdin and not connect: %+v", f.calls[2])
	}
	last := f.calls[1].argv
	if last[len(last)-1] != f.snapshot || !strings.Contains(last[2], `--snapshot="$1"`) {
		t.Errorf("pg_dump argv = %q: snapshot must be the positional parameter", last)
	}
	script := f.sessions[0].in.text()
	order := []string{"SET statement_timeout = 0;", "SET lock_timeout = 0;", "SET idle_in_transaction_session_timeout = 0;", "SET TimeZone = 'UTC'", "SET bytea_output = 'hex'", "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;",
		"SELECT pg_export_snapshot();", "LOCK TABLE %I.%I IN ACCESS SHARE MODE", "SHOW server_version_num;", `\gexec`, `\echo NSELF_FP_DONE_test`, "COMMIT;", `\q`}
	pos := -1
	for _, want := range order {
		i := strings.Index(script[pos+1:], want)
		if i < 0 {
			t.Fatalf("script lacks %q after offset %d:\n%s", want, pos, script)
		}
		pos += 1 + i
	}
}

func TestBackupNoSecretOnArgv(t *testing.T) {
	const pw, key = "throwaway-fixture-pw-1", "throwaway-fixture-key-2"
	t.Setenv("POSTGRES_PASSWORD", pw)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", key)
	f := newFake()
	if _, err := Backup(context.Background(), backupOpts(f, t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		all := strings.Join(c.argv, "\x00") + c.stdin
		if strings.Contains(all, pw) || strings.Contains(all, key) {
			t.Errorf("a call carries a secret: %q", c.argv)
		}
	}
	if !strings.Contains(strings.Join(psqlArgv, " "), `"$POSTGRES_PASSWORD"`) {
		t.Error("the pod script must read the password from the pod environment")
	}
}

func TestBackupDumpFailureRemovesPartial(t *testing.T) {
	f := newFake()
	f.dumpErr = errors.New("exit status 1")
	o := backupOpts(f, t.TempDir())
	_, err := Backup(context.Background(), o)
	wantCode(t, err, CodeBackupFailed)
	if _, e := os.Stat(o.Path); !os.IsNotExist(e) {
		t.Error("the partial dump was kept")
	}
	if _, e := os.Stat(MetaPath(o.Path)); !os.IsNotExist(e) {
		t.Error("a sidecar was written for a failed backup")
	}
}

func TestBackupRefusesExistingOutput(t *testing.T) {
	f := newFake()
	o := backupOpts(f, t.TempDir())
	if err := os.MkdirAll(filepath.Dir(o.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(o.Path, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Backup(context.Background(), o)
	wantCode(t, err, CodeTarget)
	if b, _ := os.ReadFile(o.Path); string(b) != "precious" {
		t.Error("an existing file was overwritten or removed")
	}
	if len(f.calls) != 0 {
		t.Errorf("calls = %+v, want none", f.calls)
	}
}

func TestBackupSessionDiesBeforeSentinel(t *testing.T) {
	f := newFake()
	f.psqlDies = true
	f.psqlErr = "psql: error: connection to server failed\n"
	o := backupOpts(f, t.TempDir())
	_, err := Backup(context.Background(), o)
	oe := wantCode(t, err, CodeFingerprint)
	if !strings.Contains(oe.Cause, "connection to server failed") {
		t.Errorf("cause = %q", oe.Cause)
	}
	if _, e := os.Stat(o.Path); !os.IsNotExist(e) {
		t.Error("a dump file exists although the fingerprints failed")
	}
	if f.count("pg_dump") != 0 {
		t.Error("pg_dump ran without a snapshot")
	}
}

func TestBackupRejectsBadSnapshotAndVersion(t *testing.T) {
	for name, mut := range map[string]func(*fakeExec){
		"snapshot injection": func(f *fakeExec) { f.snapshot = "00000003-0000001B-1; rm -rf /" },
		"snapshot lowercase": func(f *fakeExec) { f.snapshot = "0000000a-0000001b-1" },
		"version":            func(f *fakeExec) { f.version = "16.4" },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			mut(f)
			_, err := Backup(context.Background(), backupOpts(f, t.TempDir()))
			wantCode(t, err, CodeFingerprint)
			if f.count("pg_dump") != 0 {
				t.Error("pg_dump ran")
			}
		})
	}
}

func TestBackupOddIdentifiers(t *testing.T) {
	f := newFake()
	f.fps = []string{
		`{"schema":"public","table":"Weird \"name\"","rows":3,"sum":"7"}`,
		`{"schema":"we ird","table":"a\nb;DROP","rows":1,"sum":"-1"}`,
	}
	res, err := Backup(context.Background(), backupOpts(f, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tables) != 2 || res.Tables[0].Table != "a\nb;DROP" && res.Tables[1].Table != "a\nb;DROP" {
		t.Errorf("tables = %+v", res.Tables)
	}
}

func TestBackupNeverStartsPgDumpTwice(t *testing.T) {
	f := newFake()
	if _, err := Backup(context.Background(), backupOpts(f, t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if f.count("pg_dump") != 1 || f.count("psql-session") != 1 {
		t.Errorf("calls = %+v", f.calls)
	}
}

func TestBackupVerifyFailureRemovesDumpAndSidecar(t *testing.T) {
	for name, set := range map[string]func(*fakeExec){
		"non-zero exit": func(f *fakeExec) { f.verifyErr = errors.New("exit status 1") },
		"error line":    func(f *fakeExec) { f.verifyStderr = "pg_restore: error: unexpected end of file\n" },
		"both": func(f *fakeExec) {
			f.verifyErr = errors.New("exit status 1")
			f.verifyStderr = "pg_restore: error: could not read from input file: end of file\n"
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			set(f)
			o := backupOpts(f, t.TempDir())
			_, err := Backup(context.Background(), o)
			oe := wantCode(t, err, CodeBackupFailed)
			if !strings.Contains(oe.Cause, "not a complete archive") {
				t.Errorf("cause = %q", oe.Cause)
			}
			if _, e := os.Stat(o.Path); !os.IsNotExist(e) {
				t.Error("the unverified dump was kept")
			}
			if _, e := os.Stat(MetaPath(o.Path)); !os.IsNotExist(e) {
				t.Error("a sidecar was written")
			}
		})
	}
}

func TestBackupSessionEndingDuringDumpIsFatal(t *testing.T) {
	f := newFake()
	f.killSession = true
	o := backupOpts(f, t.TempDir())
	_, err := Backup(context.Background(), o)
	oe := wantCode(t, err, CodeBackupFailed)
	if !strings.Contains(oe.Cause, "before pg_dump finished") {
		t.Errorf("cause = %q", oe.Cause)
	}
	if _, e := os.Stat(o.Path); !os.IsNotExist(e) {
		t.Error("the dump was kept")
	}
	if f.count("pg_verify") != 0 {
		t.Error("verify ran after a fatal session failure")
	}
}

func TestBackupCommitFailureAfterDumpIsAWarning(t *testing.T) {
	f := newFake()
	f.waitErr = errors.New("exit status 2")
	o := backupOpts(f, t.TempDir())
	var warns []string
	o.Warn = func(w string) { warns = append(warns, w) }
	res, err := Backup(context.Background(), o)
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "after pg_dump finished") {
		t.Errorf("warnings = %q", warns)
	}
	if _, e := os.Stat(res.MetaFile); e != nil {
		t.Errorf("no sidecar: %v", e)
	}
}

func TestBackupExistingOutputHasItsOwnRemediation(t *testing.T) {
	exists := errExists()
	other := errTarget("PROJECT_NAME is not set", "")
	if exists.Remediation == other.Remediation || strings.Contains(exists.Remediation, "PROJECT_NAME") {
		t.Errorf("remediations: %q / %q", exists.Remediation, other.Remediation)
	}
}

func TestBoundedBufferConcurrentWriteAndRead(t *testing.T) {
	b := newBounded(128)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ {
			_, _ = b.Write([]byte("error: line\n"))
		}
		close(done)
	}()
	for i := 0; i < 200; i++ {
		_ = b.String()
	}
	<-done
	if len(b.String()) > 128 {
		t.Error("buffer exceeds its bound")
	}
}

func TestBackupRequiresTheExitMarkerOfPgDump(t *testing.T) {
	for name, set := range map[string]func(*fakeExec){
		"no marker":   func(f *fakeExec) { f.noMarker = true },
		"marker rc 1": func(f *fakeExec) { f.rc = "1" },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			set(f)
			o := backupOpts(f, t.TempDir())
			_, err := Backup(context.Background(), o)
			wantCode(t, err, CodeBackupFailed)
			if _, e := os.Stat(o.Path); !os.IsNotExist(e) {
				t.Error("the dump was kept")
			}
			if _, e := os.Stat(MetaPath(o.Path)); !os.IsNotExist(e) {
				t.Error("a sidecar was written")
			}
			if f.count("pg_verify") != 0 {
				t.Error("the check ran on a dump that did not finish")
			}
		})
	}
}

func TestBackupVerifyNeedsMarkerAndWholeFile(t *testing.T) {
	cases := map[string]struct {
		set   func(*fakeExec)
		cause string
	}{
		"no marker":          {func(f *fakeExec) {}, "did not report an exit status"},
		"stdin closed early": {func(f *fakeExec) { f.verifyShort = true }, "did not read the whole file"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			if name == "no marker" {
				// only the check lacks the marker: the dump step still prints it
				f.verifyErr = nil
				f.verifyNoMarker = true
			}
			c.set(f)
			o := backupOpts(f, t.TempDir())
			_, err := Backup(context.Background(), o)
			oe := wantCode(t, err, CodeBackupFailed)
			if !strings.Contains(oe.Cause, c.cause) {
				t.Errorf("cause = %q, want %q", oe.Cause, c.cause)
			}
			if _, e := os.Stat(o.Path); !os.IsNotExist(e) {
				t.Error("the dump was kept")
			}
			if _, e := os.Stat(MetaPath(o.Path)); !os.IsNotExist(e) {
				t.Error("a sidecar was written")
			}
		})
	}
}

func TestBackupRecordsThePodsRealNamespace(t *testing.T) {
	f := newFake()
	f.ns = "pod-ns"
	o := backupOpts(f, t.TempDir())
	o.Namespace = "from-flag"
	if _, err := Backup(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	m, err := readMeta(o.Path)
	if err != nil || m.Namespace != "pod-ns" {
		t.Errorf("namespace = %q, %v", m.Namespace, err)
	}
	f2 := newFake() // an Exec that does not know its namespace falls back to the flag
	o2 := backupOpts(f2, t.TempDir())
	o2.Namespace = "from-flag"
	if _, err := Backup(context.Background(), o2); err != nil {
		t.Fatal(err)
	}
	if m, _ := readMeta(o2.Path); m.Namespace != "from-flag" {
		t.Errorf("fallback namespace = %q", m.Namespace)
	}
}
