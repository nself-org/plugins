//go:build !windows

package exec

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type staleChecker map[string]bool

const detachedScript = "import os,time,signal; r,w=os.pipe(); p=os.fork(); (os.close(r),os.setsid(),signal.signal(signal.SIGTERM,signal.SIG_IGN),os.write(w,b'1'),time.sleep(30),os._exit(0)) if p==0 else (os.close(w),os.read(r,1),print(p,flush=True),time.sleep(30))"

func (s staleChecker) Stale(_ context.Context, id string) (bool, error) { return s[id], nil }

func TestCancelGrandchildren(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pidCh := make(chan int, 1)
	e := &Executor{Grace: 100 * time.Millisecond}
	start := time.Now()
	r := e.Run(ctx, JobSpec{AttemptID: "children", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", "trap '' TERM; (trap '' TERM; sleep 30) & echo $$; wait"}, Output: func(p []byte) {
		if n, err := strconv.Atoi(strings.TrimSpace(string(p))); err == nil {
			select {
			case pidCh <- n:
			default:
			}
			cancel()
		}
	}})
	if r.Attempt.FailureClass != "cancelled" {
		t.Fatalf("cancel: %+v", r)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("group did not die within grace + 2s")
	}
	select {
	case pid := <-pidCh:
		deadline := time.Now().Add(2 * time.Second)
		for groupAlive(pid) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if groupAlive(pid) {
			t.Fatalf("process group %d survived", pid)
		}
	default:
		t.Fatal("no process group observed")
	}
}

func TestProcessOrphanSweep(t *testing.T) {
	home := t.TempDir()
	start := func(id string) *osexec.Cmd {
		cmd := osexec.Command("sh", "-c", "sleep 30")
		if err := prepareProcess(cmd); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		if err := writeProcessMarker(home, id, id, cmd.Process.Pid); err != nil {
			_ = killGroup(cmd.Process.Pid)
			t.Fatal(err)
		}
		return cmd
	}
	live := start("live")
	stale := start("stale")
	defer func() {
		_ = killGroup(live.Process.Pid)
		_ = killGroup(stale.Process.Pid)
		_ = live.Wait()
		_ = stale.Wait()
	}()
	e := &Executor{Stale: staleChecker{"live": false, "stale": true}}
	if err := e.sweepProcesses(context.Background(), home, "current"); err != nil {
		t.Fatal(err)
	}
	_ = stale.Wait()
	if !groupAlive(live.Process.Pid) {
		t.Fatal("live coordinator process killed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for groupAlive(stale.Process.Pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if groupAlive(stale.Process.Pid) {
		t.Fatal("stale coordinator process survived")
	}
}

func TestTimeoutGrandchildren(t *testing.T) {
	var pid int
	e := &Executor{Grace: 100 * time.Millisecond}
	result := e.Run(context.Background(), JobSpec{AttemptID: "timeout-children", CoordinatorID: "coord", Home: t.TempDir(), Timeout: 100 * time.Millisecond, Command: []string{"sh", "-c", "trap '' TERM; (trap '' TERM; sleep 30) & echo $$; wait"}, Output: func(p []byte) {
		if n, err := strconv.Atoi(strings.TrimSpace(string(p))); err == nil {
			pid = n
		}
	}})
	if result.Attempt.FailureClass != "timeout" || pid <= 0 {
		t.Fatalf("timeout result=%+v pid=%d", result, pid)
	}
	deadline := time.Now().Add(2 * time.Second)
	for groupAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if groupAlive(pid) {
		t.Fatalf("timed out process group %d survived", pid)
	}
}

func TestVanishedProcessMarkerIsSuccess(t *testing.T) {
	e := &Executor{Stale: staleChecker{"old": true}}
	if err := e.sweepProcessMarker(context.Background(), t.TempDir(), "current", "vanished"); err != nil {
		t.Fatal(err)
	}
}

func TestExitedLeaderKillsLiveGroupMarker(t *testing.T) {
	home := t.TempDir()
	cmd := osexec.Command("sh", "-c", "sleep 0.2; sleep 30 &")
	if err := prepareProcess(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	defer func() { _ = killGroup(pid) }()
	if err := writeProcessMarker(home, "orphan", "old", pid); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if !groupAlive(pid) {
		t.Fatal("shell's child did not retain its process group")
	}
	e := &Executor{Stale: staleChecker{"old": true}}
	if err := e.sweepProcessMarker(context.Background(), home, "current", "orphan"); err != nil {
		t.Fatal(err)
	}
	if groupAlive(pid) {
		t.Fatal("stale process group survived sweep")
	}
	if _, err := os.Stat(filepath.Join(home, "processes", "orphan.json")); !os.IsNotExist(err) {
		t.Fatalf("stale marker retained: %v", err)
	}
}

func TestDetachedGrandchildCancellationBound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child int
	start := time.Now()
	r := (&Executor{Grace: 100 * time.Millisecond}).Run(ctx, JobSpec{AttemptID: "detached", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"python3", "-c", detachedScript}, Output: func(p []byte) {
		if n, err := strconv.Atoi(strings.TrimSpace(string(p))); err == nil {
			child = n
			cancel()
		}
	}})
	if child > 0 {
		defer func() { _ = syscall.Kill(child, syscall.SIGKILL) }()
	}
	if child <= 0 || r.Attempt.FailureClass != "cancelled" || time.Since(start) > 2100*time.Millisecond {
		t.Fatalf("detached run=%+v child=%d elapsed=%s", r, child, time.Since(start))
	}
	if err := syscall.Kill(child, 0); err == nil {
		t.Fatalf("detached grandchild %d survived cancellation", child)
	}
}

func TestDetachedGrandchildTimeoutKillsTree(t *testing.T) {
	var child int
	r := (&Executor{Grace: 100 * time.Millisecond}).Run(context.Background(), JobSpec{AttemptID: "detached-timeout", CoordinatorID: "coord", Home: t.TempDir(), Timeout: 200 * time.Millisecond, Command: []string{"python3", "-c", detachedScript}, Output: func(p []byte) {
		if n, err := strconv.Atoi(strings.TrimSpace(string(p))); err == nil {
			child = n
		}
	}})
	if child > 0 {
		defer func() { _ = syscall.Kill(child, syscall.SIGKILL) }()
	}
	if child <= 0 || r.Attempt.FailureClass != "timeout" {
		t.Fatalf("timeout run=%+v child=%d", r, child)
	}
	if err := syscall.Kill(child, 0); err == nil {
		t.Fatalf("detached grandchild %d survived timeout", child)
	}
}
