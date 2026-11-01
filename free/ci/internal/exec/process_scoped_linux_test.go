//go:build linux

package exec

import (
	"os"
	osexec "os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// startTree starts sh with two sleeping children and returns the shell's pid.
func startTree(t *testing.T) *osexec.Cmd {
	t.Helper()
	cmd := osexec.Command("sh", "-c", "sleep 30 & sleep 30 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Wait() })
	return cmd
}

func waitForChildren(t *testing.T, pid, want int) []int {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if kids, _, ok := childrenOf(pid); !ok {
			t.Skip("this kernel has no /proc/<pid>/task/<tid>/children")
		} else if len(kids) >= want {
			return kids
		}
	}
	t.Fatalf("pid %d never had %d children", pid, want)
	return nil
}

// The scoped view must hold every descendant the full table holds, with the same parents.
func TestScopedTableMatchesFullTable(t *testing.T) {
	cmd := startTree(t)
	kids := waitForChildren(t, cmd.Process.Pid, 2)
	scoped, err := scopedTable([]int{cmd.Process.Pid})
	if err != nil {
		t.Fatal(err)
	}
	full, err := processTable()
	if err != nil {
		t.Fatal(err)
	}
	for _, kid := range kids {
		if scoped[kid] != cmd.Process.Pid || full[kid] != cmd.Process.Pid {
			t.Fatalf("child %d: scoped parent %d, full parent %d, want %d", kid, scoped[kid], full[kid], cmd.Process.Pid)
		}
	}
	if scoped[os.Getpid()] != 0 {
		t.Fatalf("the test process itself is in its own scoped table")
	}
	if len(scoped) >= len(full) && len(full) > 20 {
		t.Fatalf("scoped table has %d entries, the host has %d: it was not scoped", len(scoped), len(full))
	}
}

// An orphan this process adopted (subreaper) shows up as a child of this process in the scoped view.
func TestScopedTableHoldsAdoptedOrphans(t *testing.T) {
	if err := ensureSubreaper(); err != nil {
		t.Skip(err)
	}
	cmd := osexec.Command("sh", "-c", "setsid sleep 30 </dev/null >/dev/null 2>&1 & echo $!")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(orphan, syscall.SIGKILL); _, _ = syscall.Wait4(orphan, nil, 0, nil) })
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		scoped, err := scopedTable([]int{1 << 30})
		if err != nil {
			t.Fatal(err)
		}
		if scoped[orphan] == os.Getpid() {
			return
		}
	}
	t.Fatalf("orphan %d was not seen as a child of this process", orphan)
}

// A root that has exited is skipped, not an error.
func TestScopedTableSkipsGoneRoot(t *testing.T) {
	if _, err := scopedTable([]int{1 << 30}); err != nil {
		t.Fatal(err)
	}
}
