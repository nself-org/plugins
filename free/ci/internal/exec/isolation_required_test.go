package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingIsolationRefusesBeforeHostExecution(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, "executed")
	ex := &Executor{}
	result := ex.Run(context.Background(), JobSpec{AttemptID: "missing-isolation", CoordinatorID: "coord", Home: home, Command: []string{"sh", "-c", "touch " + marker}})
	if result.Err == nil || !result.Refused || result.Attempt.FailureClass != "infra" {
		t.Fatalf("missing policy: %+v", result)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("host command executed: %v", err)
	}
	if ex.Isolation != nil {
		t.Fatal("executor installed an implicit policy")
	}
}
