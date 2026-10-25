//go:build !windows

package run

// Purpose: the container sweep of Recover removes only containers of coordinators
// this store knows to be stale; another CI home's container is left alone (D-1117).
// Constraints: a fake docker script on a PATH that holds nothing else; no real daemon.

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
)

func TestRecoverSweepSkipsContainersOfUnknownCoordinators(t *testing.T) {
	ciexec.ResetRuntimeInfoCache()
	st := openState(t)
	ctx := context.Background()
	gone := osexec.Command("true")
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	dead, err := st.RegisterCoordinator(ctx, gone.Process.Pid, "t0", bootID())
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	listing := "dead1 " + safeID(dead.ID) + "\nforeign1 another-home-coordinator\n"
	script := "#!/bin/sh\necho \"$*\" >> \"${0%/*}/docker.calls\"\n" +
		"case \"$1\" in\ninfo) echo '{}';;\nps) printf '%s' '" + listing + "';;\nesac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if rep := Recover(ctx, st, "", home); len(rep.Errors) > 0 {
		t.Fatalf("recover errors: %v", rep.Errors)
	}
	calls, _ := os.ReadFile(filepath.Join(bin, "docker.calls"))
	out := string(calls)
	if !strings.Contains(out, "rm -f dead1") {
		t.Errorf("the container of a stale known coordinator was not removed:\n%s", out)
	}
	if strings.Contains(out, "foreign1") {
		t.Errorf("a container of a coordinator unknown to this store was removed:\n%s", out)
	}
}
