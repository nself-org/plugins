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

// At the deadline the script's children die with it (closure review): a
// background child must not outlive a timed-out gate.
func TestRunRepoScript_DeadlineKillsChildren(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("skip: bash not on PATH")
	}
	old := repoScriptDeadline
	repoScriptDeadline = 300 * time.Millisecond
	t.Cleanup(func() { repoScriptDeadline = old })
	root := t.TempDir()
	pidFile := filepath.Join(root, "child.pid")
	writeScript(t, root, "scripts/ci/vendor-sync-check.sh", `sleep 30 & echo $! > child.pid; wait`)
	if gr := runRepoScript("go:vendor-sync", root, "scripts/ci/vendor-sync-check.sh", false); gr.Passed {
		t.Fatalf("want a failed gate, got %+v", gr)
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
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("child %d outlived the timed-out gate", pid)
}
