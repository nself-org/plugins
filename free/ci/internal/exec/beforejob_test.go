package exec

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestBeforeJobRunsInTheWorkspaceBeforeTheCommand(t *testing.T) {
	var wsPath string
	spec := JobSpec{AttemptID: "bj", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", "cat prepared.txt"},
		BeforeJob: func(_ context.Context, ws *os.Root) error {
			wsPath = ws.Name()
			return ws.WriteFile("prepared.txt", []byte("ready"), 0o644)
		}}
	var out strings.Builder
	spec.Output = func(b []byte) { out.Write(b) }
	r := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), spec)
	if r.Err != nil || r.Attempt.State != "passed" || strings.TrimSpace(out.String()) != "ready" {
		t.Fatalf("result %+v output %q", r, out.String())
	}
	if _, err := os.Stat(wsPath); !os.IsNotExist(err) {
		t.Fatalf("the workspace must still be removed: %v", err)
	}
}

func TestBeforeJobErrorFailsTheAttemptInfraWithoutRunningTheCommand(t *testing.T) {
	home := t.TempDir()
	boom := errors.New("E712 prepare: no space")
	spec := JobSpec{AttemptID: "bj2", CoordinatorID: "coord", Home: home, Command: []string{"sh", "-c", "echo ran > " + home + "/ran"},
		BeforeJob: func(context.Context, *os.Root) error { return boom }}
	r := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), spec)
	if !errors.Is(r.Err, boom) || r.Attempt.State == "passed" || r.Attempt.FailureClass != "infra" {
		t.Fatalf("result %+v", r)
	}
	if _, err := os.Stat(home + "/ran"); !os.IsNotExist(err) {
		t.Fatal("the command ran although BeforeJob failed")
	}
}
