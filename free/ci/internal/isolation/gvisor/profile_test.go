package gvisor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func wrapped(t *testing.T, network string) exec.JobSpec {
	t.Helper()
	p := newProfile(okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), okStarter(), nil)
	spec, err := p.Wrap(exec.JobSpec{AttemptID: "a1", CoordinatorID: "c1", Network: modelScope(network)})
	if err != nil || spec.Plan == nil {
		t.Fatalf("wrap: %v", err)
	}
	return spec
}

var run1 = exec.ContainerRun{Name: "nself-ci-a1", Image: ProbeImage, Workspace: "/work/a1", Workdir: "/repo", Command: []string{"sh", "-c", "echo ok"},
	AttemptID: "a1", CoordinatorID: "c1", CPU: 1, MemoryMB: 128}

func TestPlanAddsRunscToTheHardenedCommand(t *testing.T) {
	cmd, err := wrapped(t, "").Plan.Plan(run1)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Binary != "docker" || cmd.Args[0] != "run" || cmd.Args[1] != "--runtime=runsc" {
		t.Fatalf("runtime not first after run: %v", cmd.Args)
	}
	joined := " " + strings.Join(cmd.Args, " ") + " "
	for _, want := range []string{" --cap-drop=ALL ", " --network=none ", " --read-only ", " --user 65532:65532 ", " --pids-limit 512 ", " " + ProbeImage + " sh -c echo ok "} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	if strings.Count(joined, "--runtime") != 1 {
		t.Errorf("runtime named more than once: %s", joined)
	}
}

func TestLanScopeKeepsTheContainerNetworkPlan(t *testing.T) {
	cmd, err := wrapped(t, "lan").Plan.Plan(run1)
	if err != nil || !strings.Contains(strings.Join(cmd.Args, " "), "--network=bridge") {
		t.Fatalf("%v %v", cmd.Args, err)
	}
}

func TestCleanupHelperRunsUnderRunscToo(t *testing.T) {
	cmd, err := wrapped(t, "").Plan.Cleanup(run1)
	if err != nil || cmd == nil || cmd.Args[1] != "--runtime=runsc" || !strings.Contains(strings.Join(cmd.Args, " "), "--name nself-ci-a1-clean") {
		t.Fatalf("%v %v", cmd, err)
	}
}

func TestUnenforceableScopeIsRefusedNotWidened(t *testing.T) {
	p := newProfile(okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), okStarter(), nil)
	_, err := p.Wrap(exec.JobSpec{AttemptID: "a1", Network: "internet"})
	var ie *isolation.Error
	if !errors.As(err, &ie) || ie.Code != "E679" {
		t.Fatalf("want E679, got %v", err)
	}
}

// The container profile re-probes on its own clock and may fall through to podman, whose runsc was never checked.
func TestWithRuntimeRefusesAnotherBinary(t *testing.T) {
	_, err := withRuntime(exec.ContainerCommand{Binary: "podman", Args: []string{"run", "--rm", "img"}})
	var ie *isolation.Error
	if !errors.As(err, &ie) || ie.Code != "E678" {
		t.Fatalf("want E678, got %v", err)
	}
}

func TestWithRuntimeRefusesANonRunCommand(t *testing.T) {
	if _, err := withRuntime(exec.ContainerCommand{Binary: "docker", Args: []string{"exec", "x"}}); err == nil {
		t.Fatal("a non-run command must be refused")
	}
}

func TestTeardownGoesThroughTheContainerProfile(t *testing.T) {
	p := newProfile(&fakeRunner{info: map[string]string{}}, okStarter(), nil)
	err := p.Teardown(context.Background(), isolation.Attempt{ID: "a1"})
	var ie *isolation.Error
	if !errors.As(err, &ie) || ie.Code != "E678" {
		t.Fatalf("teardown without a runtime must say so, got %v", err)
	}
}

func modelScope(s string) model.NetworkScope { return model.NetworkScope(s) }

// A teardown with an already-cancelled context must not leave a cached failure behind for the next probe.
func TestCancelledTeardownDoesNotPoisonTheNextProbe(t *testing.T) {
	r := okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`)
	r.ctxAware = true
	p := newProfile(r, okStarter(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = p.Teardown(ctx, isolation.Attempt{ID: "a1"})
	if av := p.Probe(context.Background()); !av.Available() {
		t.Fatalf("a cancelled teardown poisoned the probe: %+v", av)
	}
}
