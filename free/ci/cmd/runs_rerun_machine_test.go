package main

// Purpose: pins the D29 machine default for `runs rerun` (P7-CI-62 review): a rerun started by a machine caller
//          (NSELF_INVOKED_BY set) posts no commit status unless --post-status is given; a human rerun posts.
// Constraints: the options come from rerunOptions, the function the handler uses; the poster is a counting stub.

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

type countingReporter struct{ posts atomic.Int32 }

func (c *countingReporter) Post(context.Context, report.Status) error { c.posts.Add(1); return nil }

// rerunPosts runs the options a rerun builds and returns how many statuses the stub poster received.
func rerunPosts(t *testing.T, f rerunFlags, invokedBy string) int32 {
	t.Helper()
	t.Setenv("NSELF_V15", "") // the record gate (pools, protection reads) is not under test: only whether a post is attempted
	root, home := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"NSELF_CI_HOME": home, "NSELF_INVOKED_BY": invokedBy}
	saved := machineNotice
	machineNotice = io.Discard
	defer func() { machineNotice = saved }()
	o := rerunOptions(f, root, func(k string) string { return env[k] })
	yes := true
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{
		"test": {Kind: "test", Run: []string{"sh", "-c", "true"}, Required: &yes}}},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatal(err)
	}
	rep := &countingReporter{}
	c := &cirun.Coordinator{Opts: o, Plan: pl, ConfigDigest: "cfg", Out: &bytes.Buffer{}, Capture: decisionGit, Reporter: rep,
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}, Poll: 10 * time.Millisecond}
	_, err = c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return rep.posts.Load()
}

func TestRerunMachineInvocationPostsNoStatus(t *testing.T) {
	if n := rerunPosts(t, rerunFlags{}, "mcp"); n != 0 {
		t.Fatalf("a rerun invoked by mcp posted %d status(es)", n)
	}
}

func TestRerunPostStatusPostsOnce(t *testing.T) {
	if n := rerunPosts(t, rerunFlags{postStatus: true}, "mcp"); n != 1 {
		t.Fatalf("--post-status posted %d status(es), want 1", n)
	}
}

func TestRerunHumanInvocationPostsOnce(t *testing.T) {
	if n := rerunPosts(t, rerunFlags{}, ""); n != 1 {
		t.Fatalf("a human rerun posted %d status(es), want 1", n)
	}
}

func TestRerunFlagSetHasPostStatus(t *testing.T) {
	var f rerunFlags
	fs := newRerunFlagSet(&f)
	if err := fs.Parse([]string{"--post-status"}); err != nil || !f.postStatus {
		t.Fatalf("--post-status not parsed: %v %+v", err, f)
	}
}
