//go:build linux

package hygiene

import (
	"bufio"
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// daemonise starts the test binary as a session leader that listens on a TCP port and carries the
// attempt tag, like a job child that called setsid and left its process group.
func daemonise(t *testing.T, attempt string) (pid, port int, cmd *osexec.Cmd) {
	t.Helper()
	cmd = osexec.Command(os.Args[0], "-test.run=NoSuchTest")
	cmd.Env = append(os.Environ(), "HYGIENE_HELPER=listen", "NSELF_CI_EXEC_ATTEMPT="+attempt)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("helper did not report its port: %v", err)
	}
	port, err = strconv.Atoi(line[:len(line)-1])
	if err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid, port, cmd
}

func alive(cmd *osexec.Cmd) bool {
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
		return false
	case <-time.After(300 * time.Millisecond):
		return true
	}
}

// TestCleanupKillsDaemonisedChildAndKeepsHome is the Epic acceptance for process-level jobs: a job
// that daemonises a child, leaves a listening socket, writes files in its scratch and a symlink to
// $HOME is fully cleaned, and $HOME is untouched.
func TestCleanupKillsDaemonisedChildAndKeepsHome(t *testing.T) {
	h, _ := testHost(t)
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "profile"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := seedRoot(t, h, "daemon1")
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tmp", "scratch"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home, filepath.Join(root, "home")); err != nil {
		t.Fatal(err)
	}
	_, port, child := daemonise(t, "daemon1")
	_, _, bystander := daemonise(t, "someone-else")

	rep, err := h.LeakCheck(context.Background(), "daemon1", 0)
	if codeOf(err) != CodeLeak || !hasKind(rep, LeakProcess) || !hasKind(rep, LeakSocket) || !hasKind(rep, LeakRoot) {
		t.Fatalf("before cleanup the leak must show process, socket and root: %v %+v (port %d)", err, rep, port)
	}
	if err := h.Cleanup(context.Background(), "daemon1"); err != nil {
		t.Fatal(err)
	}
	if alive(child) {
		t.Fatal("the daemonised child survived cleanup")
	}
	if rep, err := h.LeakCheck(context.Background(), "daemon1", 0); err != nil || !rep.Clean() {
		t.Fatalf("after cleanup: %v %+v", err, rep)
	}
	if b, err := os.ReadFile(filepath.Join(home, "profile")); err != nil || string(b) != "mine" {
		t.Fatalf("$HOME content was touched: %v %q", err, b)
	}
	if syscall.Kill(bystander.Process.Pid, 0) != nil {
		t.Fatal("cleanup killed a process of another attempt")
	}
}

func TestLeakCheckByJobUIDFindsOnlyThatUID(t *testing.T) {
	h, _ := testHost(t)
	_, _, _ = daemonise(t, "uid-attempt")
	// the helper runs under this test's own UID: a scan for another UID must not match it
	rep, err := h.LeakCheck(context.Background(), "uid-clean", 4242424)
	if err != nil || !rep.Clean() {
		t.Fatalf("a different job uid matched: %v %+v", err, rep)
	}
	rep, err = h.LeakCheck(context.Background(), "uid-clean", os.Geteuid())
	if codeOf(err) != CodeLeak || !hasKind(rep, LeakProcess) {
		t.Fatalf("own uid scan should see the helper: %v %+v", err, rep)
	}
}
