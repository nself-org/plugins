//go:build !windows

package exec

import (
	"context"
	"strings"
	"testing"
)

// The folded wrapper must give the job exactly the environment env would have.
func TestEnvWrapperAppliedWithoutEnvProcess(t *testing.T) {
	t.Setenv("GIT_DIR", "/leak")
	var out strings.Builder
	res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), JobSpec{
		AttemptID: "envfold", CoordinatorID: "coord", Home: t.TempDir(),
		Command: []string{"env", "-u", "GIT_DIR", "FOLDED=yes", "sh", "-c", `echo "dir=[${GIT_DIR-unset}] folded=$FOLDED"`},
		Output:  func(b []byte) { out.Write(b) },
	})
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run: %+v", res)
	}
	if got := strings.TrimSpace(out.String()); got != "dir=[unset] folded=yes" {
		t.Fatalf("job saw %q", got)
	}
}
