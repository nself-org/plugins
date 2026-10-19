//go:build linux

package exec

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// A job that declares no memory limit runs go under the 256 MB default demand (D-1009).
func TestGoRunsUnderDefaultDemand(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	var out strings.Builder
	r := (&Executor{}).Run(context.Background(), JobSpec{AttemptID: "default-demand", CoordinatorID: "coord", Home: t.TempDir(), MemoryMB: 256,
		Command: []string{"go", "version"}, Output: func(p []byte) { out.Write(p) }})
	if r.ExitCode != 0 || !strings.Contains(out.String(), "go version") {
		t.Fatalf("exit=%d out=%q run=%+v", r.ExitCode, out.String(), r.Attempt)
	}
}
