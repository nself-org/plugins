package adhoc

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/services"
)

// fakePlanner stands in for the container profile's planner.
type fakePlanner struct{ args []string }

func (f fakePlanner) Plan(r ciexec.ContainerRun) (ciexec.ContainerCommand, error) {
	args := f.args
	if args == nil {
		args = []string{"run", "--network=none", "--mount", "type=bind,src=" + r.Workspace + ",dst=/repo", "img", "true"}
	}
	return ciexec.ContainerCommand{Binary: "docker", Args: args}, nil
}
func (fakePlanner) Cleanup(ciexec.ContainerRun) (*ciexec.ContainerCommand, error) { return nil, nil }

// fakeProfile mimics the registry isolator: it installs a planner.
type fakeProfile struct {
	plan     ciexec.ContainerPlanner
	err      error
	seen     model.NetworkScope
	clearEnv bool // behave like a profile whose Wrap rebuilt the JobSpec
}

func (f *fakeProfile) Prepare(_ context.Context, s *ciexec.JobSpec) error {
	f.seen = s.Network
	if f.err != nil {
		return f.err
	}
	s.Plan = f.plan
	if f.clearEnv {
		s.Env = nil
	}
	return nil
}

func TestPlannerNarrowsWorkspaceAndNetwork(t *testing.T) {
	run := ciexec.ContainerRun{Workspace: "/w/ws"}
	cmd, err := (&planner{inner: fakePlanner{}, network: "nself-ci-a1"}).Plan(run)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(cmd.Args, "--network=nself-ci-a1") || !slices.Contains(cmd.Args, "type=bind,src=/w/ws,dst=/repo,readonly=true") {
		t.Fatalf("not narrowed: %v", cmd.Args)
	}
	if slices.Contains(cmd.Args, "--network=none") {
		t.Fatalf("the none flag stayed: %v", cmd.Args)
	}
}

func TestPlannerFailsClosedWhenAFlagIsMissing(t *testing.T) {
	for name, args := range map[string][]string{
		"no network flag": {"run", "--mount", "type=bind,src=/w/ws,dst=/repo", "img", "true"},
		"no mount":        {"run", "--network=none", "img", "true"},
		"other mount":     {"run", "--network=none", "--mount", "type=bind,src=/other,dst=/repo", "img"},
	} {
		_, err := (&planner{inner: fakePlanner{args: args}, network: "none"}).Plan(ciexec.ContainerRun{Workspace: "/w/ws"})
		if err == nil || codeOf(err) != CodeNoIsolation {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestIsolatorLevels(t *testing.T) {
	ctx := context.Background()
	t.Run("process", func(t *testing.T) {
		spec := &ciexec.JobSpec{Network: "internet"}
		if err := (&isolator{mode: Mode{Level: "process", Network: "lan"}, scratch: "/s"}).Prepare(ctx, spec); err != nil {
			t.Fatal(err)
		}
		if spec.Plan != nil || spec.Env == nil {
			t.Fatalf("process: plan %v env %v", spec.Plan, spec.Env)
		}
	})
	t.Run("none stays the profile's own flag", func(t *testing.T) {
		prof := &fakeProfile{plan: fakePlanner{}}
		spec := &ciexec.JobSpec{AttemptID: "a1"}
		if err := (&isolator{inner: prof, mode: Mode{Level: "container", Network: "none"}}).Prepare(ctx, spec); err != nil {
			t.Fatal(err)
		}
		cmd, err := spec.Plan.Plan(ciexec.ContainerRun{Workspace: "/w"})
		if err != nil || prof.seen != "none" || !slices.Contains(cmd.Args, "--network=none") || !slices.Contains(cmd.Args, "type=bind,src=/w,dst=/repo,readonly=true") {
			t.Fatalf("seen %q args %v err %v", prof.seen, cmd.Args, err)
		}
	})
	t.Run("internet goes to the profile, which decides; it is never widened to bridge here", func(t *testing.T) {
		prof := &fakeProfile{plan: fakePlanner{}}
		spec := &ciexec.JobSpec{AttemptID: "a1"}
		if err := (&isolator{inner: prof, mode: Mode{Level: "container", Network: "internet"}}).Prepare(ctx, spec); err != nil {
			t.Fatal(err)
		}
		cmd, err := spec.Plan.Plan(ciexec.ContainerRun{Workspace: "/w"})
		if err != nil || prof.seen != "internet" || slices.Contains(cmd.Args, "--network=bridge") {
			t.Fatalf("seen %q args %v err %v", prof.seen, cmd.Args, err)
		}
		refuse := errors.New("E679: network scope internet cannot be enforced")
		prof = &fakeProfile{err: refuse}
		if err := (&isolator{inner: prof, mode: Mode{Level: "container", Network: "internet"}}).Prepare(ctx, &ciexec.JobSpec{}); !errors.Is(err, refuse) {
			t.Fatalf("the profile's refusal was swallowed: %v", err)
		}
	})
	t.Run("a profile that drops the env cannot make the job inherit the default one", func(t *testing.T) {
		prof := &fakeProfile{plan: fakePlanner{}, clearEnv: true}
		spec := &ciexec.JobSpec{AttemptID: "a1"}
		if err := (&isolator{inner: prof, mode: Mode{Level: "container", Network: "none"}}).Prepare(ctx, spec); err != nil {
			t.Fatal(err)
		}
		if spec.Env == nil {
			t.Fatal("the minimal env policy was lost")
		}
	})
	t.Run("a profile error passes through, no profile fails closed", func(t *testing.T) {
		boom := errors.New("E678: gone")
		if err := (&isolator{inner: &fakeProfile{err: boom}, mode: Mode{Level: "container"}}).Prepare(ctx, &ciexec.JobSpec{}); !errors.Is(err, boom) {
			t.Fatalf("%v", err)
		}
		for name, iso := range map[string]*isolator{
			"nil inner":   {mode: Mode{Level: "container"}},
			"no planner":  {inner: &fakeProfile{}, mode: Mode{Level: "container"}},
			"sandbox too": {inner: &fakeProfile{}, mode: Mode{Level: "sandboxed-container"}},
		} {
			if err := iso.Prepare(ctx, &ciexec.JobSpec{}); codeOf(err) != CodeNoIsolation {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
}

// dockerLog records the docker calls of serviceNet.
type dockerLog struct {
	calls   [][]string
	outputs map[string]string
	failOn  string
}

func (d *dockerLog) Run(_ context.Context, c services.Call) (services.Result, error) {
	d.calls = append(d.calls, c.Argv)
	j := strings.Join(c.Argv, " ")
	if d.failOn != "" && strings.Contains(j, d.failOn) {
		return services.Result{ExitCode: 1, Output: "boom"}, nil
	}
	best := ""
	for k := range d.outputs { // the longest matching key wins, so the answer never depends on map order
		if strings.Contains(j, k) && len(k) > len(best) {
			best = k
		}
	}
	return services.Result{Output: d.outputs[best]}, nil
}

func TestServiceNetIsInternalAndAliased(t *testing.T) {
	d := &dockerLog{outputs: map[string]string{"network inspect": "proj-postgres-1\nproj-redis-1\n", "docker inspect --format": "svc"}}
	n := &serviceNet{docker: d}
	name, err := n.Ensure(context.Background(), "att1", "proj_default")
	if err != nil || name != "nself-ci-att1" {
		t.Fatalf("%q %v", name, err)
	}
	var create []string
	connects := 0
	for _, c := range d.calls {
		if strings.Join(c[:3], " ") == "docker network create" {
			create = c
		}
		if strings.Join(c[:3], " ") == "docker network connect" {
			connects++
			if c[3] != "--alias" || c[4] != "svc" || c[5] != "nself-ci-att1" {
				t.Errorf("connect %v", c)
			}
		}
	}
	if !slices.Contains(create, "--internal") || connects != 2 {
		t.Fatalf("create %v connects %d", create, connects)
	}
	n.Remove(context.Background())
	if last := d.calls[len(d.calls)-1]; strings.Join(last, " ") != "docker network rm nself-ci-att1" {
		t.Fatalf("not removed: %v", last)
	}
}

func TestServiceNetFailsClosed(t *testing.T) {
	if _, err := (&serviceNet{docker: &dockerLog{}}).Ensure(context.Background(), "a", ""); codeOf(err) != CodeNoIsolation {
		t.Errorf("no compose network: %v", err)
	}
	if _, err := (&serviceNet{docker: &dockerLog{}}).Ensure(context.Background(), "a", "proj_default"); codeOf(err) != CodeNoIsolation {
		t.Errorf("no service container: %v", err)
	}
	if _, err := (&serviceNet{docker: &dockerLog{failOn: "network create"}}).Ensure(context.Background(), "a", "proj_default"); codeOf(err) != CodeNoIsolation {
		t.Errorf("create failed: %v", err)
	}
}

// The user's own argv after the image is never rewritten, even when it looks like a runtime flag.
func TestPlannerLeavesTheUserArgvAlone(t *testing.T) {
	in := fakePlanner{args: []string{"run", "--network=none", "--mount", "type=bind,src=/w,dst=/repo", "img", "printf", "--network=none", "type=bind,src=/w,dst=/repo"}}
	p := &planner{inner: in, network: "bridge"}
	got, err := p.Plan(ciexec.ContainerRun{Workspace: "/w", Image: "img"})
	if err != nil {
		t.Fatal(err)
	}
	tail := strings.Join(got.Args[4:], " ")
	if tail != "img printf --network=none type=bind,src=/w,dst=/repo" {
		t.Fatalf("user argv rewritten: %q", tail)
	}
}
