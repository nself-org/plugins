//go:build container

package adhoc

// Purpose: the container behaviour of the ad-hoc job against a real Docker
// daemon: no network by default, a read-only workspace, service reach by name.
// Inputs:  a Docker daemon, the alpine:3.19 image already on the host (no pull);
// TestAdhocServiceReach also needs `nself` with `config services` on PATH.
// Outputs: pass or fail. They call t.Fatal, never t.Skip, when a prerequisite is
// missing (a skipped proof is not a proof).

import (
	"context"
	"os"
	"os/exec"
	"testing"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

type alpineImage struct{}

func (alpineImage) Image(context.Context, cirun.AttemptInfo) (string, error) {
	return "alpine:3.19", nil
}

func needDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatalf("docker is required for the container tests: %v", err)
	}
	if out, err := exec.Command("docker", "image", "inspect", "alpine:3.19", "--format", "{{.Id}}").CombinedOutput(); err != nil {
		t.Fatalf("image alpine:3.19 is required: %v %s", err, out)
	}
}

// containerRig is a handler wired to the real isolation registry and Docker.
func containerRig(t *testing.T, fl Flags) *rigOut {
	t.Helper()
	needDocker(t)
	r := newHandlerRig(t, fl)
	r.h.Select = RegistrySelector
	r.h.Hooks.Image = alpineImage{}
	return r
}

func (r *rigOut) mustPass(t *testing.T, script string) {
	t.Helper()
	argv := []string{"sh", "-c", script}
	r.opts.Adhoc = argv
	exit, err := r.h.Adhoc(context.Background(), r.opts, argv)
	if err != nil || exit != 0 {
		t.Fatalf("exit %d err %v\n%s", exit, err, r.out)
	}
}

// The job passes only when TCP to 1.1.1.1:443 fails for a network reason (not 127, command not found).
func TestAdhocContainerNetwork(t *testing.T) {
	r := containerRig(t, Flags{})
	r.mustPass(t, "nc -z -w 3 1.1.1.1 443; rc=$?; test $rc -ne 0 && test $rc -ne 127")
}

// The job passes only when the workspace and the root filesystem refuse writes and the scratch dir takes one.
func TestAdhocReadOnlyWorkspace(t *testing.T) {
	r := containerRig(t, Flags{})
	r.mustPass(t, "touch /repo/planted 2>/dev/null && exit 11; touch /etc/planted 2>/dev/null && exit 12; touch /tmp/ok || exit 13; exit 0")
	if _, err := os.Stat(r.opts.Root + "/planted"); err == nil {
		t.Fatal("the job wrote into the workspace")
	}
}

// postgres is reachable by service name and port on the service network.
func TestAdhocServiceReach(t *testing.T) {
	if _, err := exec.LookPath("nself"); err != nil {
		t.Fatalf("nself is required on PATH: %v", err)
	}
	t.Setenv("NSELF_V15", "1")
	r := containerRig(t, Flags{})
	r.opts.Services = []string{"postgres"}
	r.mustPass(t, "nc -z -w 10 postgres 5432 || exit 21; nc -z -w 3 1.1.1.1 443; rc=$?; test $rc -ne 0 && test $rc -ne 127")
}
