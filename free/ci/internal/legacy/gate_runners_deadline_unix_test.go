//go:build unix

package legacy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStepDeadline(t *testing.T) {
	for _, c := range []struct {
		timeout int
		want    time.Duration
	}{
		{0, 630 * time.Second},
		{120, 630 * time.Second},
		{900, 930 * time.Second},
	} {
		if got := stepDeadline(c.timeout); got != c.want {
			t.Errorf("stepDeadline(%d) = %s, want %s", c.timeout, got, c.want)
		}
	}
}

// D-0922: a gate command that blocks for good must end as a failed gate at the
// deadline, and its children must die with it.
func TestRunStepWithin_DeadlineFailsTheGateAndKillsChildren(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("skip: sh not on PATH")
	}
	root := t.TempDir()
	pidFile := filepath.Join(root, "child.pid")
	start := time.Now()
	gr := runStepWithin("go:test", root, 2*time.Second, false, "sh", "-c", "sleep 60 & echo $! > child.pid; wait")
	if gr.Passed || gr.Skipped {
		t.Fatalf("want a failed gate, got %+v", gr)
	}
	if !strings.Contains(gr.Output, "timed out after 2s") {
		t.Errorf("output should say it timed out, got %q", gr.Output)
	}
	if took := time.Since(start); took > 20*time.Second {
		t.Errorf("gate ran %s past a 2s deadline", took)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("child %d outlived the timed-out gate", pid)
}

func TestRunStepWithin_FastCommandStillPasses(t *testing.T) {
	if gr := runStepWithin("x", t.TempDir(), time.Minute, false, "true"); !gr.Passed {
		t.Fatalf("want pass, got %+v", gr)
	}
}
