package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func afterJobSpec(t *testing.T, home, script string, after func(context.Context, string) error) JobSpec {
	t.Helper()
	return JobSpec{AttemptID: "aj", CoordinatorID: "coord", Home: home, Command: []string{"sh", "-c", script}, AfterJob: after}
}

func TestAfterJobSeesTheWorkspaceBeforeItIsRemoved(t *testing.T) {
	home := t.TempDir()
	var seen string
	var wsPath string
	r := (&Executor{}).Run(context.Background(), afterJobSpec(t, home, "echo out > produced.txt", func(_ context.Context, ws string) error {
		wsPath = ws
		b, err := os.ReadFile(filepath.Join(ws, "produced.txt"))
		seen = strings.TrimSpace(string(b))
		return err
	}))
	if r.Err != nil || r.Attempt.State != "passed" || seen != "out" {
		t.Fatalf("result %+v seen %q", r, seen)
	}
	if wsPath != filepath.Join(home, "work", "aj") {
		t.Fatalf("workspace %q", wsPath)
	}
	if _, err := os.Stat(wsPath); !os.IsNotExist(err) {
		t.Fatalf("the workspace must still be removed after the hook: %v", err)
	}
}

func TestAfterJobErrorFailsTheAttemptInfra(t *testing.T) {
	boom := errors.New("E712 missing: no output")
	r := (&Executor{}).Run(context.Background(), afterJobSpec(t, t.TempDir(), "true", func(context.Context, string) error { return boom }))
	if !errors.Is(r.Err, boom) || r.Attempt.State != "failed" || r.Attempt.FailureClass != "infra" {
		t.Fatalf("result %+v", r)
	}
}

func TestAfterJobSkippedWhenTheJobFailsOrIsCancelled(t *testing.T) {
	calls := 0
	hook := func(context.Context, string) error { calls++; return nil }
	if r := (&Executor{}).Run(context.Background(), afterJobSpec(t, t.TempDir(), "exit 4", hook)); r.ExitCode != 4 || r.Attempt.State == "passed" {
		t.Fatalf("failing job: %+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	(&Executor{}).Run(ctx, afterJobSpec(t, t.TempDir(), "true", hook))
	if calls != 0 {
		t.Fatalf("hook ran %d times for a failed or cancelled job", calls)
	}
}
