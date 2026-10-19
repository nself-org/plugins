package adhoc

// Purpose: the Isolator of an ad-hoc job. It wraps the registered isolation
// (P7-TRUST-04's registry: container, sandboxed-container) and adds what only
// this job needs: the minimal env policy, a read-only workspace mount and the
// network scope. The wrapped profile still builds and the exec backstop still
// checks the container command; this layer only narrows it.
// Inputs:  the JobSpec the executor prepared (attempt id, job env incl.
// NSELF_CI_SERVICE_NETWORK, the profile's planner after inner.Prepare).
// Outputs: the JobSpec with Env, Plan and Network set.
// Constraints: fail closed: if the profile did not install a planner, or the
// planned command lacks the network or mount flag this layer rewrites, the job
// does not start (a rewrite that silently missed would leave the workspace
// writable or the network wider than recorded). The profile only knows the
// none scope, so it is asked for none and this layer swaps the flag.

import (
	"context"
	"strings"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
)

// isolator implements exec.Isolator.
type isolator struct {
	inner   ciexec.Isolator
	mode    Mode
	scratch string
	net     *serviceNet
}

func (i *isolator) Prepare(ctx context.Context, spec *ciexec.JobSpec) error {
	process := i.mode.Level == "process"
	policy := envPolicy{container: !process, scratch: i.scratch}
	spec.Env = policy
	spec.Network = "none" // the profiles know none; restricted is swapped in by the planner below
	if process {
		return nil // the executor baseline: no profile, host networking
	}
	if i.inner == nil {
		return noIsolation("no isolation hook is registered; refusing to run a container job without one")
	}
	if i.mode.Network == "internet" {
		// The profile's own network planner decides: the default one refuses internet
		// (P7-TRUST-08 installs the scope planner), and it is never widened to bridge here.
		spec.Network = "internet"
	}
	if err := i.inner.Prepare(ctx, spec); err != nil {
		return err
	}
	spec.Env = policy // a profile that rebuilt the JobSpec must not drop the minimal env
	if spec.Plan == nil {
		return noIsolation("the %s profile installed no container planner; nothing was run", i.mode.Level)
	}
	network := ""
	if i.mode.Network == "restricted" {
		name, err := i.net.Ensure(ctx, spec.AttemptID, spec.Job.Env["NSELF_CI_SERVICE_NETWORK"])
		if err != nil {
			return err
		}
		network = name
	}
	spec.Plan = &planner{inner: spec.Plan, network: network}
	return nil
}

// planner narrows the profile's container command: read-only workspace and the
// recorded network.
type planner struct {
	inner   ciexec.ContainerPlanner
	network string
}

func (p *planner) Plan(r ciexec.ContainerRun) (ciexec.ContainerCommand, error) {
	cmd, err := p.inner.Plan(r)
	if err != nil {
		return cmd, err
	}
	mount := "type=bind,src=" + r.Workspace + ",dst=/repo"
	args := append([]string(nil), cmd.Args...)
	var gotNet, gotMount bool
	for k, a := range args {
		if a == r.Image { // the flag region ends at the image: the argv after it is the user's, never rewritten
			break
		}
		switch {
		case a == "--network=none" && p.network != "":
			args[k], gotNet = "--network="+p.network, true
		case a == mount:
			args[k], gotMount = mount+",readonly=true", true
		}
	}
	if p.network == "" {
		gotNet = true // none and internet are the profile's own flags: only the mount is rewritten
	}
	if !gotNet || !gotMount {
		return cmd, noIsolation("the container profile built a command without the %s; refusing to start it", missing(gotNet, gotMount))
	}
	cmd.Args = args
	return cmd, nil
}

func missing(net, mount bool) string {
	var m []string
	if !net {
		m = append(m, "network flag")
	}
	if !mount {
		m = append(m, "workspace mount")
	}
	return strings.Join(m, " and ")
}

// Cleanup is the profile's own: it runs with the network none and removes what
// the job UID created (nothing, with a read-only workspace).
func (p *planner) Cleanup(r ciexec.ContainerRun) (*ciexec.ContainerCommand, error) {
	return p.inner.Cleanup(r)
}
