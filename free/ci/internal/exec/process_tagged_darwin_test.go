//go:build darwin

package exec

import (
	"context"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHelperSleeper is the daemon daemonize starts: it only sleeps, and only when asked to.
func TestHelperSleeper(t *testing.T) {
	if os.Getenv("NSELF_TEST_SLEEPER") == "1" {
		time.Sleep(5 * time.Minute)
	}
}

// daemonize starts a process carrying the attempt tag whose parent exits at once, so launchd adopts it.
func daemonize(t *testing.T, attempt string) int {
	t.Helper()
	dir := t.TempDir()
	pidFile := dir + "/pid"
	// macOS hides the environment of Apple platform binaries (/bin/sleep), so the daemon is this test binary.
	cmd := osexec.Command("sh", "-c", "(\"$1\" -test.run=TestHelperSleeper & echo $! > \"$0\") ; exit 0", pidFile, os.Args[0])
	cmd.Env = append(os.Environ(), "NSELF_CI_EXEC_ATTEMPT="+attempt, "NSELF_TEST_SLEEPER=1")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	var pid int
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		data, _ := os.ReadFile(pidFile)
		if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			pid = n
			break
		}
	}
	if pid == 0 {
		t.Fatal("daemon pid not written")
	}
	t.Cleanup(func() {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	})
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if parents, err := processTable(); err == nil && parents[pid] == launchdPID {
			return pid
		}
	}
	t.Fatal("daemon was not adopted by launchd")
	return 0
}

// D-0456: a double-forked job process is found through the attempt tag in its environment.
func TestDarwinFindsDaemonizedDescendantByTag(t *testing.T) {
	pid := daemonize(t, "tag-find")
	found := func(attempt string, since time.Time) bool {
		for _, p := range adoptedDescendants(nil, attempt, since) {
			if p == pid {
				return true
			}
		}
		return false
	}
	if !found("tag-find", time.Now().Add(-time.Minute)) {
		t.Fatal("daemonized process not found by its attempt tag")
	}
	if found("tag-other", time.Time{}) {
		t.Fatal("process found under another attempt's tag")
	}
	if found("tag-find", time.Now().Add(time.Minute)) {
		t.Fatal("process older than the tracker was claimed")
	}
	if len(adoptedDescendants(nil, "", time.Time{})) != 0 {
		t.Fatal("an empty attempt must match nothing")
	}
}

// D-1057: the sweep of a dead agent's attempt kills the daemonized process the group kill cannot reach.
func TestDarwinSweepKillsDaemonizedDescendant(t *testing.T) {
	pid := daemonize(t, "sweep-daemon")
	home := t.TempDir()
	leader := startGroup(t)
	identity, err := processIdentity(leader.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	writeMarkerWith(t, home, "sweep-daemon", "old", leader.Process.Pid, identity)
	e := &Executor{Stale: staleChecker{"old": true}}
	if err := e.sweepProcessMarker(context.Background(), home, "current", "sweep-daemon"); err != nil {
		t.Fatal(err)
	}
	_ = leader.Wait()
	if processAlive(pid) {
		t.Fatal("daemonized process survived the sweep")
	}
}

// environHas must read the environment, not the arguments.
func TestEnvironHasSkipsArguments(t *testing.T) {
	buf := func(argc byte, parts ...string) []byte {
		out := []byte{argc, 0, 0, 0}
		for _, p := range parts {
			out = append(out, p...)
			out = append(out, 0)
		}
		return out
	}
	raw := buf(2, "/bin/x", "", "x", "NSELF_CI_EXEC_ATTEMPT=a", "NSELF_CI_EXEC_ATTEMPT=b", "PATH=/bin")
	if environHas(raw, "NSELF_CI_EXEC_ATTEMPT=a") {
		t.Fatal("an argument matched as an environment entry")
	}
	if !environHas(raw, "NSELF_CI_EXEC_ATTEMPT=b") || environHas(raw, "NSELF_CI_EXEC_ATTEMPT=") {
		t.Fatal("environment entry not matched exactly")
	}
	if environHas(nil, "A=b") || environHas([]byte{9, 0, 0, 0, 'x'}, "A=b") {
		t.Fatal("a short buffer matched")
	}
}
