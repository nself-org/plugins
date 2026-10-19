//go:build !windows

package exec

// Purpose: D-1057 regression tests. A SIGKILLed agent leaves the job's process group behind, and a
// descendant that called setsid or double-forked is outside that group; the process marker must
// name such descendants so the next agent start can kill them.

import (
	"context"
	"encoding/json"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// startEscapee runs a sleeper in its own session: outside the leader's process group, as a job's
// setsid child is. The returned channel closes once the process has been reaped.
func startEscapee(t *testing.T) (*osexec.Cmd, <-chan struct{}) {
	t.Helper()
	cmd := osexec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
	return cmd, done
}

func writeMarkerWithDescendants(t *testing.T, home, attempt string, leader int, descendants []map[string]any) {
	t.Helper()
	if err := writeProcessMarker(home, attempt, "old", leader); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(markerPath(home, attempt))
	if err != nil {
		t.Fatal(err)
	}
	var marker map[string]any
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatal(err)
	}
	marker["descendants"] = descendants
	data, _ = json.Marshal(marker)
	if err := os.WriteFile(markerPath(home, attempt), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSweepKillsRecordedDescendantOutsideTheGroup(t *testing.T) {
	home := t.TempDir()
	leader := startGroup(t)
	escapee, done := startEscapee(t)
	id, ok := nativeStartID(escapee.Process.Pid)
	if !ok {
		t.Fatal("no start id for the escapee")
	}
	writeMarkerWithDescendants(t, home, "a1", leader.Process.Pid, []map[string]any{{"pid": escapee.Process.Pid, "identity": id}})
	e := &Executor{Stale: staleChecker{"old": true}}
	if err := e.sweepProcessMarker(context.Background(), home, "current", "a1"); err != nil {
		t.Fatal(err)
	}
	_ = leader.Wait()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a recorded descendant outside the process group survived the sweep")
	}
}

func TestSweepKillsRecordedDescendantWhenTheLeaderGroupIsGone(t *testing.T) {
	home := t.TempDir()
	leader := startGroup(t)
	escapee, done := startEscapee(t)
	id, _ := nativeStartID(escapee.Process.Pid)
	writeMarkerWithDescendants(t, home, "a2", leader.Process.Pid, []map[string]any{{"pid": escapee.Process.Pid, "identity": id}})
	_ = killGroup(leader.Process.Pid)
	_ = leader.Wait()
	e := &Executor{Stale: staleChecker{"old": true}}
	if err := e.sweepProcessMarker(context.Background(), home, "current", "a2"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a recorded descendant survived once its leader's group was gone")
	}
}

func TestSweepSparesDescendantWhosePIDWasReused(t *testing.T) {
	home := t.TempDir()
	leader := startGroup(t)
	escapee, done := startEscapee(t)
	writeMarkerWithDescendants(t, home, "a3", leader.Process.Pid, []map[string]any{{"pid": escapee.Process.Pid, "identity": "st:1"}})
	e := &Executor{Stale: staleChecker{"old": true}}
	if err := e.sweepProcessMarker(context.Background(), home, "current", "a3"); err != nil {
		t.Fatal(err)
	}
	_ = leader.Wait()
	select {
	case <-done:
		t.Fatal("the sweep killed a process whose start time differs from the marker")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestKillAttemptGroupKillsRecordedDescendant(t *testing.T) {
	home := t.TempDir()
	leader := startGroup(t)
	escapee, done := startEscapee(t)
	id, _ := nativeStartID(escapee.Process.Pid)
	writeMarkerWithDescendants(t, home, "a4", leader.Process.Pid, []map[string]any{{"pid": escapee.Process.Pid, "identity": id}})
	if err := KillAttemptGroup(home, "a4"); err != nil {
		t.Fatal(err)
	}
	_ = leader.Wait()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("KillAttemptGroup left a recorded descendant running")
	}
}

// While a job runs, its marker must list the descendants the tracker has seen, so an agent that is
// SIGKILLed mid-job leaves enough on disk for the next start to find them.
func TestRunPersistsDescendantsInTheMarker(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child int
	listed := false
	e := &Executor{Grace: 100 * time.Millisecond}
	e.Run(ctx, JobSpec{AttemptID: "persist", CoordinatorID: "coord", Home: home, Command: []string{"sh", "-c", "sleep 30 & echo $!; wait"}, Output: func(p []byte) {
		n, err := strconv.Atoi(strings.TrimSpace(string(p)))
		if err != nil {
			return
		}
		child = n
		deadline := time.Now().Add(3 * time.Second)
		for !listed && time.Now().Before(deadline) {
			var marker struct {
				Descendants []struct {
					PID      int    `json:"pid"`
					Identity string `json:"identity"`
				} `json:"descendants"`
			}
			if data, err := os.ReadFile(markerPath(home, "persist")); err == nil && json.Unmarshal(data, &marker) == nil {
				for _, d := range marker.Descendants {
					listed = listed || d.PID == child && d.Identity != ""
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
	}})
	if child <= 0 {
		t.Fatal("no descendant observed")
	}
	if !listed {
		t.Fatalf("marker never listed descendant %d", child)
	}
}
