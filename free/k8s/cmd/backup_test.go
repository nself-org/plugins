// Purpose: black-box tests of `nself-k8s backup` and the helpers restore_test.go
// shares: the built binary runs with a stub kubectl on PATH that plays the
// postgres pod (get pods, exec psql / pg_dump / pg_restore).
//
// Inputs: the binary from cli_harness_test.go (cliBin, cliResult, writeFile).
//
// Outputs: test results.
//
// Constraints: no cluster, no docker, no real kubectl. The stub records its argv,
// so a test can prove that a refused command ran no kubectl at all. Fixtures use
// synthetic values only.
package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

// stubKubectl plays kubectl. STUB_MODE is backup or restore; the data files are
// named by STUB_PODS, STUB_DUMP, STUB_FPS and STUB_POST_FPS.
const stubKubectl = `#!/bin/sh
{ printf '%s\n' "$@"; echo '--'; } >> "$STUB_ARGV"
if [ "$1" = "get" ]; then cat "$STUB_PODS"; exit 0; fi
kind=""
for a in "$@"; do
  case "$a" in
    *"pg_dump -U"*) kind=dump ;;
    *"pg_restore -U"*) kind=restore ;;
    *"exec psql"*) kind=psql ;;
    *"pg_restore -f /dev/null"*) kind=verify ;;
  esac
done
case "$kind" in
  dump)
    if [ -n "$STUB_HANG" ]; then echo started >"$STUB_HANG"; exec cat "$STUB_FIFO"; fi
    cat "$STUB_DUMP"; echo "NSELF_RC=0" >&2 ;;
  verify) cat >/dev/null; echo "NSELF_RC=0" >&2 ;;
  restore) cat >/dev/null; if [ -n "$STUB_RESTORE_ERR" ]; then echo "$STUB_RESTORE_ERR" >&2; exit 1; fi; echo "NSELF_RC=0" >&2 ;;
  psql)
    if [ "$STUB_MODE" = "backup" ]; then
      while IFS= read -r line; do
        case "$line" in
          '\echo '*) sentinel="${line#\\echo }"; break ;;
        esac
      done
      echo "00000003-0000001B-1"; echo "160004"; cat "$STUB_FPS"; echo "$sentinel"
      cat >/dev/null
    else
      while IFS= read -r line; do
        case "$line" in
          '\echo '*) sentinel="${line#\\echo }" ;;
        esac
      done
      cat "$STUB_POST_FPS"; echo "$sentinel"
    fi ;;
esac
exit 0
`

const onePod = `{"items":[{"metadata":{"name":"postgres-0","namespace":"pod-ns"},"status":{"phase":"Running","containerStatuses":[{"name":"postgres","ready":true}]}}]}`

const fpLines = `{"schema":"auth","table":"users","rows":2,"sum":"11"}
{"schema":"public","table":"Weird \"name\"","rows":1000,"sum":"-5"}
{"schema":"storage","table":"files","rows":0,"sum":"0"}
`

// k8sRun configures one invocation of the binary against the stub kubectl.
type k8sRun struct {
	Dir        string // working directory (the project)
	Mode       string // backup or restore (what the stub psql does)
	Dump       string
	Fps        string
	PostFps    string
	RestoreErr string
	Pods       string
	NoKubectl  bool
	Stdin      io.Reader
}

type k8sResult struct {
	cliResult
	Calls int // kubectl invocations
}

// stubEnv lays out the stub tools and data files and returns the stub dir and the environment.
func stubEnv(t *testing.T, o k8sRun) (string, []string) {
	t.Helper()
	stubs := t.TempDir()
	for _, tool := range []string{"sh", "cat"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(p, filepath.Join(stubs, tool)); err != nil {
			t.Fatal(err)
		}
	}
	if !o.NoKubectl {
		writeFile(t, filepath.Join(stubs, "kubectl"), stubKubectl, 0o755)
	}
	pods := o.Pods
	if pods == "" {
		pods = onePod
	}
	data := map[string]string{"pods": pods, "dump": o.Dump, "fps": o.Fps, "post": o.PostFps}
	for k, v := range data {
		writeFile(t, filepath.Join(stubs, k), v, 0o644)
	}
	return stubs, []string{"PATH=" + stubs, "HOME=" + stubs, "STUB_ARGV=" + filepath.Join(stubs, "kubectl.argv"), "STUB_MODE=" + o.Mode,
		"STUB_PODS=" + filepath.Join(stubs, "pods"), "STUB_DUMP=" + filepath.Join(stubs, "dump"),
		"STUB_FPS=" + filepath.Join(stubs, "fps"), "STUB_POST_FPS=" + filepath.Join(stubs, "post"),
		"STUB_RESTORE_ERR=" + o.RestoreErr}
}

// runK8s execs the binary with the stub kubectl as the only tool on PATH.
func runK8s(t *testing.T, o k8sRun, args ...string) k8sResult {
	t.Helper()
	stubs, env := stubEnv(t, o)
	argv := filepath.Join(stubs, "kubectl.argv")
	cmd := exec.Command(cliBin, args...)
	cmd.Env = env
	cmd.Dir = o.Dir
	cmd.Stdin = o.Stdin
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	res := k8sResult{cliResult: cliResult{Stdout: so.String(), Stderr: se.String(), ArgvFile: argv}}
	if ee, ok := err.(*exec.ExitError); ok {
		res.Exit = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v", cliBin, err)
	}
	res.Calls = len(argvCalls(t, argv))
	return res
}

// project makes a project dir whose .env names the project.
func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=demo\n", 0o644)
	return dir
}

func TestBackupJSONEnvelopeNamesFileAndTableCounts(t *testing.T) {
	dir := project(t)
	r := runK8s(t, k8sRun{Dir: dir, Mode: "backup", Dump: "PGDMP-synthetic", Fps: fpLines}, "backup", "--json", "--namespace", "ns1", "--tag", "t1")
	d := wantSuccess(t, r.cliResult, "k8s backup")
	file, _ := d.Data["file"].(string)
	if !strings.HasPrefix(filepath.Base(file), "demo_full_") || !strings.HasSuffix(file, "_t1.dump") {
		t.Fatalf("file = %q", file)
	}
	if got, _ := os.ReadFile(file); string(got) != "PGDMP-synthetic" {
		t.Errorf("dump = %q", got)
	}
	if d.Data["bytes"].(float64) != 15 || len(d.Data["sha256"].(string)) != 64 {
		t.Errorf("data = %v", d.Data)
	}
	tables := d.Data["tables"].([]any)
	if len(tables) != 3 {
		t.Fatalf("tables = %v", tables)
	}
	seen := map[string]float64{}
	for _, x := range tables {
		m := x.(map[string]any)
		seen[m["schema"].(string)+"."+m["table"].(string)] = m["rows"].(float64)
	}
	if seen["auth.users"] != 2 || seen[`public.Weird "name"`] != 1000 || seen["storage.files"] != 0 {
		t.Errorf("tables = %v", seen)
	}
	if meta, _ := d.Data["metadata_file"].(string); !strings.HasSuffix(meta, ".meta/"+filepath.Base(file)+".json") {
		t.Errorf("metadata_file = %q", meta)
	}
	// kubectl never saw a value: argv holds the fixed pod scripts only.
	b, _ := os.ReadFile(r.ArgvFile)
	if !strings.Contains(string(b), `$POSTGRES_PASSWORD`) || !strings.Contains(string(b), "ns1") {
		t.Errorf("kubectl argv:\n%s", b)
	}
}

func TestBackupHumanModePrintsSuccess(t *testing.T) {
	dir := project(t)
	r := runK8s(t, k8sRun{Dir: dir, Mode: "backup", Dump: "x", Fps: fpLines}, "backup", "--output", filepath.Join(dir, "out", "b.dump"))
	if r.Exit != 0 || !strings.Contains(r.Stdout+r.Stderr, "Backup written") {
		t.Fatalf("exit %d\n%s\n%s", r.Exit, r.Stdout, r.Stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", ".meta", "b.dump.json")); err != nil {
		t.Error(err)
	}
}

func TestBackupErrorsAreCodedEnvelopes(t *testing.T) {
	dir := project(t)
	t.Run("no kubectl E730", func(t *testing.T) {
		r := runK8s(t, k8sRun{Dir: dir, NoKubectl: true}, "backup", "--json")
		wantFailure(t, r.cliResult, "k8s backup", "E730", output.ClassInfra)
	})
	t.Run("no pod E731", func(t *testing.T) {
		r := runK8s(t, k8sRun{Dir: dir, Pods: `{"items":[]}`}, "backup", "--json")
		d := wantFailure(t, r.cliResult, "k8s backup", "E731", output.ClassUser)
		if !strings.Contains(d.Error.Message, "found 0") {
			t.Errorf("message = %q", d.Error.Message)
		}
	})
	t.Run("no project name E738", func(t *testing.T) {
		r := runK8s(t, k8sRun{Dir: t.TempDir()}, "backup", "--json")
		wantFailure(t, r.cliResult, "k8s backup", "E738", output.ClassUser)
		if r.Calls != 0 {
			t.Error("kubectl ran before the target was resolved")
		}
	})
	t.Run("existing output E738", func(t *testing.T) {
		out := filepath.Join(dir, "exists.dump")
		writeFile(t, out, "keep", 0o600)
		r := runK8s(t, k8sRun{Dir: dir, Mode: "backup", Dump: "x", Fps: fpLines}, "backup", "--json", "--output", out)
		wantFailure(t, r.cliResult, "k8s backup", "E738", output.ClassUser)
		if b, _ := os.ReadFile(out); string(b) != "keep" {
			t.Error("existing file touched")
		}
	})
	t.Run("human mode exit 1", func(t *testing.T) {
		r := runK8s(t, k8sRun{Dir: dir, Pods: `{"items":[]}`}, "backup")
		if r.Exit != 1 || !strings.Contains(r.Stderr, "E731") || strings.Contains(r.Stdout, "Backup written") {
			t.Errorf("exit %d stdout %q stderr %q", r.Exit, r.Stdout, r.Stderr)
		}
	})
	t.Run("bad release E401", func(t *testing.T) {
		r := runK8s(t, k8sRun{Dir: dir}, "backup", "--json", "--release", "--x")
		wantFailure(t, r.cliResult, "k8s backup", "E401", output.ClassUser)
	})
}

func TestBackupKilledByTermSignalLeavesNoCoreNamedFile(t *testing.T) {
	dir := project(t)
	stubs, env := stubEnv(t, k8sRun{Dir: dir, Mode: "backup", Dump: "x", Fps: fpLines})
	fifo := filepath.Join(stubs, "hang.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("no fifo:", err)
	}
	started := filepath.Join(stubs, "started")
	cmd := exec.Command(cliBin, "backup", "--json")
	cmd.Env = append(env, "STUB_HANG="+started, "STUB_FIFO="+fifo)
	cmd.Dir = dir
	var so bytes.Buffer
	cmd.Stdout = &so
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("the dump never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if p, _ := filepath.Glob(filepath.Join(dir, "backups", ".meta", "*.partial")); len(p) != 1 {
		t.Errorf("partials mid-dump = %v", p)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("the backup did not stop on SIGTERM")
	}
	if d, _ := filepath.Glob(filepath.Join(dir, "backups", "*_full_*.dump")); len(d) != 0 {
		t.Errorf("a dump with a core name stayed: %v", d)
	}
	if p, _ := filepath.Glob(filepath.Join(dir, "backups", ".meta", "*")); len(p) != 0 {
		t.Errorf(".meta is not empty after the interrupt: %v", p)
	}
}
