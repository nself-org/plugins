//go:build !windows

package exec

import (
	"context"
	"os"
	osexec "os/exec"
	"strings"
	"testing"
	"time"
)

// startGroup runs a sleeping process group and returns its leader.
func startGroup(t *testing.T) *osexec.Cmd {
	t.Helper()
	cmd := osexec.Command("sh", "-c", "sleep 30")
	if err := prepareProcess(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = killGroup(cmd.Process.Pid)
		_ = cmd.Wait()
	})
	return cmd
}

func waitGroupGone(pid int) bool {
	deadline := time.Now().Add(2 * time.Second)
	for groupAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	return !groupAlive(pid)
}

func TestNativeStartIDIsStableAndPerProcess(t *testing.T) {
	self, ok := nativeStartID(os.Getpid())
	if !ok || !strings.HasPrefix(self, "st:") {
		t.Fatalf("self id = %q ok=%v", self, ok)
	}
	if again, _ := nativeStartID(os.Getpid()); again != self {
		t.Fatalf("id changed between reads: %q vs %q", self, again)
	}
	cmd := startGroup(t)
	other, ok := nativeStartID(cmd.Process.Pid)
	if !ok || other == self {
		t.Fatalf("child id %q must differ from self %q", other, self)
	}
	if _, ok := nativeStartID(1 << 30); ok {
		t.Fatal("a pid that cannot exist reported a start id")
	}
}

// A marker written by an older build holds ps lstart text; it must still match
// the live leader, so a legacy marker's group is killed and not leaked.
func TestKillAttemptGroupAcceptsLegacyMarker(t *testing.T) {
	home := t.TempDir()
	cmd := startGroup(t)
	pid := cmd.Process.Pid
	legacy, err := psIdentity(pid)
	if err != nil || legacy == "" {
		t.Fatalf("ps identity: %q %v", legacy, err)
	}
	writeMarkerWith(t, home, "legacy", "old", pid, legacy)
	if err := KillAttemptGroup(home, "legacy"); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if !waitGroupGone(pid) {
		t.Fatal("group with a legacy marker survived KillAttemptGroup")
	}
}

// A marker whose identity differs from the live leader means the PID was
// reused: the group belongs to someone else and must not be killed.
func TestKillAttemptGroupSparesReusedPID(t *testing.T) {
	for name, identity := range map[string]string{"native": "st:1", "legacy": "Thu Jan  1 00:00:00 1970"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			cmd := startGroup(t)
			pid := cmd.Process.Pid
			writeMarkerWith(t, home, "reused", "old", pid, identity)
			if err := KillAttemptGroup(home, "reused"); err != nil {
				t.Fatal(err)
			}
			if !groupAlive(pid) {
				t.Fatal("KillAttemptGroup killed a group whose leader PID was reused")
			}
			if _, err := os.Stat(markerPath(home, "reused")); !os.IsNotExist(err) {
				t.Fatalf("stale marker retained: %v", err)
			}
		})
	}
}

func TestSweepLegacyMarkerKillsAndReusedPIDDropsMarker(t *testing.T) {
	home := t.TempDir()
	legacyCmd := startGroup(t)
	reusedCmd := startGroup(t)
	legacy, err := psIdentity(legacyCmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	writeMarkerWith(t, home, "legacy", "old", legacyCmd.Process.Pid, legacy)
	writeMarkerWith(t, home, "reused", "old", reusedCmd.Process.Pid, "st:1")
	e := &Executor{Stale: staleChecker{"old": true}}
	if err := e.sweepProcessMarker(context.Background(), home, "current", "legacy"); err != nil {
		t.Fatal(err)
	}
	_ = legacyCmd.Wait()
	if !waitGroupGone(legacyCmd.Process.Pid) {
		t.Fatal("legacy-marker group survived sweep")
	}
	if err := e.sweepProcessMarker(context.Background(), home, "current", "reused"); err != nil {
		t.Fatalf("sweep of a reused PID must drop the marker and go on: %v", err)
	}
	if _, err := os.Stat(markerPath(home, "reused")); !os.IsNotExist(err) {
		t.Fatal("marker of a reused PID must be removed")
	}
	if !groupAlive(reusedCmd.Process.Pid) {
		t.Fatal("sweep killed a group whose leader PID was reused")
	}
}

// The tracker must follow nested descendants, not only direct children.
func TestTrackerFollowsNestedDescendants(t *testing.T) {
	cmd := osexec.Command("sh", "-c", "sh -c 'sleep 30 & sleep 30 & wait' & wait")
	if err := prepareProcess(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	tr := trackDescendants(cmd.Process.Pid, "")
	deadline := time.Now().Add(2 * time.Second)
	for len(tr.pids()) < 3 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	got := len(tr.pids())
	tr.signalKill()
	_ = killGroup(cmd.Process.Pid)
	_ = cmd.Wait()
	tr.stop()
	if got < 3 {
		t.Fatalf("tracker saw %d descendants, want at least 3", got)
	}
}

func writeMarkerWith(t *testing.T, home, attempt, coordinator string, pid int, identity string) {
	t.Helper()
	if err := os.MkdirAll(home+"/processes", 0o700); err != nil {
		t.Fatal(err)
	}
	data := `{"pid":` + itoa(pid) + `,"coordinator":"` + coordinator + `","identity":"` + identity + `"}`
	if err := os.WriteFile(markerPath(home, attempt), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
