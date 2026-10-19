//go:build !windows

package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func spawnWait(t *testing.T, argv, env []string) (string, *Detached) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "detached", "run.ndjson")
	d, err := StartDetached(argv, env, log)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-d.Exited:
	case <-time.After(10 * time.Second):
		t.Fatal("detached child did not end")
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), d
}

func TestSpawnDetachedFilesAndEnv(t *testing.T) {
	log := filepath.Join(t.TempDir(), "detached", "run.ndjson")
	script := `echo "A=$A"; echo "tok=${NSELF_MCP_TOKEN-unset}"; cat; echo "stdin-eof"; echo err >&2;
for f in 3 4 5 6 7 8 9; do if [ -e /dev/fd/$f ]; then echo "open-fd-$f"; fi; done`
	extra, err := os.Open(os.DevNull) // a parent descriptor the child must not get
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = extra.Close() }()
	pid, err := SpawnDetached([]string{"sh", "-c", script}, []string{"A=1", "PATH=" + os.Getenv("PATH")}, log)
	if err != nil || pid <= 0 {
		t.Fatalf("pid %d err %v", pid, err)
	}
	var out []byte
	for i := 0; i < 200; i++ {
		if out, _ = os.ReadFile(log); strings.Contains(string(out), "err") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	got := string(out)
	for _, want := range []string{"A=1\n", "tok=unset\n", "stdin-eof\n", "err\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "open-fd") {
		t.Errorf("the child inherited a descriptor:\n%s", got)
	}
	if st, err := os.Stat(log); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("log mode %v err %v", st.Mode(), err)
	}
	if st, err := os.Stat(filepath.Dir(log)); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("log dir mode %v err %v", st.Mode(), err)
	}
}

func TestSpawnDetachedOwnSession(t *testing.T) {
	log := filepath.Join(t.TempDir(), "s.ndjson")
	d, err := StartDetached([]string{"sleep", "30"}, os.Environ(), log)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(d.PID, syscall.SIGKILL) }()
	// Setsid makes the child the leader of a new session and process group
	if pg, err := syscall.Getpgid(d.PID); err != nil || pg != d.PID || pg == syscall.Getpgrp() {
		t.Fatalf("child process group %d (err %v), want its own (%d), not ours (%d)", pg, err, d.PID, syscall.Getpgrp())
	}
	if runtime.GOOS == "linux" { // /proc/<pid>/stat: pid (comm) state ppid pgrp session ...
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", d.PID))
		if err != nil {
			t.Fatal(err)
		}
		f := strings.Fields(string(b[strings.LastIndexByte(string(b), ')')+1:]))
		if len(f) < 4 || f[3] != fmt.Sprint(d.PID) {
			t.Fatalf("child session %v, want its own pid %d", f, d.PID)
		}
	}
	if err = TerminateDetached(d.PID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-d.Exited:
	case <-time.After(5 * time.Second):
		t.Fatal("TerminateDetached did not stop the child")
	}
}

func TestSpawnDetachedErrors(t *testing.T) {
	dir := t.TempDir()
	cases := map[string][]string{"no command": nil, "missing binary": {filepath.Join(dir, "nope")}}
	for name, argv := range cases {
		_, err := SpawnDetached(argv, nil, filepath.Join(dir, name+".log"))
		var ee *Error
		if err == nil || !asError(err, &ee) || ee.Code != "E648" || !strings.Contains(ee.Message, "spawn") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := TerminateDetached(0); err == nil {
		t.Error("pid 0 accepted")
	}
}

func TestSpawnDetachedReportsExit(t *testing.T) {
	out, d := spawnWait(t, []string{"sh", "-c", "echo hello"}, os.Environ())
	if out != "hello\n" || d.PID <= 0 {
		t.Fatalf("log %q pid %d", out, d.PID)
	}
}
