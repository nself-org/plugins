//go:build !windows

package exec

import (
	"context"
	osexec "os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

type staleChecker map[string]bool

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
