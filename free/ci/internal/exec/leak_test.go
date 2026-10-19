//go:build !windows

package exec

// Purpose: P7-AUTO-16 leak proof. After a pass, a fail, a timeout, a cancel
// and a SIGKILLed child, the job root (Home/work) and the job's temp directory
// hold nothing, and the parent temp directory the operator's TMPDIR names has
// gained no entry. The double-fork-to-launchd escape on macOS (D-0456) is a
// documented limit, not claimed here.
// Constraints: runs on darwin and linux; the job puts its TMPDIR under its
// workspace (a relative path), the way the gate wrappers do.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range list {
		out = append(out, e.Name())
	}
	return out
}

func TestNoLeakedTempAfterKill(t *testing.T) {
	// every leg: make a temp dir and a temp file under the job's TMPDIR, then end in a way
	const work = `mkdir .jobtmp && export TMPDIR=.jobtmp && mktemp -d "$TMPDIR/d.XXXXXX" >/dev/null && mktemp "$TMPDIR/f.XXXXXX" >/dev/null; `
	for name, leg := range map[string]struct {
		script  string
		timeout time.Duration
		cancel  time.Duration
		failed  bool
	}{
		"pass":      {script: "exit 0"},
		"fail":      {script: "exit 3", failed: true},
		"timeout":   {script: "sleep 30", timeout: 500 * time.Millisecond, failed: true},
		"cancel":    {script: "sleep 30", cancel: 300 * time.Millisecond, failed: true},
		"sigkill":   {script: "sleep 30 & kill -KILL $!; wait; exit 9", failed: true},
		"self-kill": {script: "sleep 30 & kill -KILL $$", failed: true},
	} {
		t.Run(name, func(t *testing.T) {
			home, parent, source := t.TempDir(), t.TempDir(), t.TempDir()
			t.Setenv("TMPDIR", parent)
			if err := os.WriteFile(source+"/f.txt", []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if leg.cancel > 0 {
				var stop context.CancelFunc
				ctx, stop = context.WithCancel(ctx)
				time.AfterFunc(leg.cancel, stop)
			}
			e := &Executor{}
			res := e.Run(ctx, JobSpec{AttemptID: "leak-" + name, CoordinatorID: "coord", Job: model.Job{Isolation: "process"}, Home: home,
				SourceDir: source, Timeout: leg.timeout, Command: []string{"sh", "-c", work + leg.script}})
			if (res.Attempt.State == "passed") == leg.failed {
				t.Fatalf("%s: unexpected state %+v err %v", name, res.Attempt, res.Err)
			}
			if left := entries(t, home+"/work"); len(left) != 0 {
				t.Fatalf("job root not empty after %s: %s", name, strings.Join(left, ", "))
			}
			if left := entries(t, parent); len(left) != 0 {
				t.Fatalf("operator TMPDIR gained entries after %s: %s", name, strings.Join(left, ", "))
			}
		})
	}
}
