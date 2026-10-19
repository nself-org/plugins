//go:build linux

package exec

import (
	"context"
	"os"
	osexec "os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// D-1008: the end-of-command snapshot of a short command reads no process table.
// Each full read was a stat of every process on the host, run twice per funnel
// call and about twenty-five times per V15 run.
func TestShortCommandReadsNoProcessTable(t *testing.T) {
	if !childrenFiles() {
		t.Skip("the kernel has no /proc/<pid>/task/<tid>/children")
	}
	before := tableReads.Load()
	res := (&Executor{Grace: 100 * time.Millisecond}).Run(context.Background(), JobSpec{
		AttemptID: "no-walk", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"true"},
	})
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run: %+v", res)
	}
	if n := tableReads.Load() - before; n != 0 {
		t.Fatalf("a short command read the full process table %d times", n)
	}
}

// What a command forks and leaves behind (here a background sleep whose parent
// shell exits at once) is a child of the coordinator, tagged with the attempt;
// the end-of-command snapshot must find it without a table walk.
func TestSnapshotAfterExitFindsAdoptedOrphan(t *testing.T) {
	if !childrenFiles() {
		t.Skip("the kernel has no /proc/<pid>/task/<tid>/children")
	}
	if err := ensureSubreaper(); err != nil {
		t.Skip(err)
	}
	const attempt = "orphan-attempt"
	cmd := osexec.Command("sh", "-c", "sleep 30 >/dev/null 2>&1 & echo $! ; exit 0")
	cmd.Env = append(os.Environ(), "NSELF_CI_EXEC_ATTEMPT="+attempt)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := strconv.Atoi(string(out[:len(out)-1]))
	if err != nil {
		t.Fatalf("pid %q: %v", out, err)
	}
	defer func() {
		_ = syscall.Kill(orphan, syscall.SIGKILL)
		var st syscall.WaitStatus
		_, _ = syscall.Wait4(orphan, &st, 0, nil)
	}()
	tr := trackDescendants(cmd.Process.Pid, attempt)
	defer tr.stop()
	before := tableReads.Load()
	tr.snapshotAfterExit()
	found := false
	for _, pid := range tr.pids() {
		found = found || pid == orphan
	}
	if !found {
		t.Fatalf("the orphan %d was not found; seen %v", orphan, tr.pids())
	}
	if n := tableReads.Load() - before; n != 0 {
		t.Fatalf("finding the orphan read the full process table %d times", n)
	}
}

// A grandchild below a pid seen earlier is found through the children lists too.
func TestQuickTableFollowsSeenSubtree(t *testing.T) {
	if !childrenFiles() {
		t.Skip("the kernel has no /proc/<pid>/task/<tid>/children")
	}
	cmd := osexec.Command("sh", "-c", "sh -c 'sleep 30 & wait' & wait")
	if err := prepareProcess(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = killGroup(cmd.Process.Pid); _ = cmd.Wait() }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		parents, ok := quickTable(os.Getpid(), cmd.Process.Pid, nil, "")
		if !ok {
			t.Fatal("quickTable refused on a kernel with children files")
		}
		depth := 0
		for pid := range parents {
			for p, n := pid, 0; n < 8; n++ {
				if p = parents[p]; p == cmd.Process.Pid {
					depth = max(depth, 2)
				}
			}
		}
		if len(parents) >= 3 && depth == 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("quickTable did not reach the grandchildren of the root")
}
