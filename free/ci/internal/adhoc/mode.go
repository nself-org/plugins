package adhoc

// Purpose: decide how one ad-hoc job runs: the isolation level, the network
// scope and the trust class, from the flags and what this host can deliver.
// Inputs:  run.Options (network, trust, services), the ad-hoc flags, a
// Selector that says whether an isolation level is available.
// Outputs: a Mode, or E646 / E647 / E627 before anything is started.
// Constraints: the level is never lowered (an untrusted source that cannot get
// sandboxed-container fails with E647, it does not fall back to container);
// the container is the default and `process` needs the flag; a network scope
// the level cannot enforce is refused, not ignored.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// Flags are the ad-hoc flags that run.Options has no field for; cmd/v15_adhoc.go
// parses them.
type Flags struct {
	Isolation string // --isolation: "", container, process, sandboxed-container
	Image     string // --image ref@digest (also exported as NSELF_CI_IMAGE for --container)
	Trust     string // --trust, moved out of run.Options so Validate does not demand a classifier
}

// Mode is how one ad-hoc job runs.
type Mode struct {
	Level   model.Isolation    // process, container or sandboxed-container: the level asked for
	Served  model.Isolation    // the level the registry served (>= Level); the evidence records this
	Network model.NetworkScope // none, restricted, internet (container) or lan (process)
	Trust   model.TrustClass   // the requested trust class ("" = the operator's own command)
}

// Selector reports the level this host serves for the one asked (nil error, level
// at least the one asked); production uses the isolation registry.
type Selector func(ctx context.Context, level model.Isolation) (model.Isolation, error)

// RegistrySelector asks the default isolation registry and returns the effective
// level of the profile it picked.
func RegistrySelector(ctx context.Context, level model.Isolation) (model.Isolation, error) {
	_, av, err := isolation.Select(ctx, level)
	return av.Level, err
}

// Resolve picks the Mode. trust is the class the operator asked for with
// --trust, raised to untrusted when the registered classifier says so.
func Resolve(ctx context.Context, o cirun.Options, fl Flags, trust model.TrustClass, sel Selector) (Mode, error) {
	m := Mode{Trust: trust}
	switch fl.Isolation {
	case "", "container", "process", "sandboxed-container":
	default:
		return m, usage("unknown --isolation %q (container, process, sandboxed-container)", fl.Isolation)
	}
	level, served, err := chooseLevel(ctx, fl.Isolation, trust, sel)
	if err != nil {
		return m, err
	}
	m.Level, m.Served = level, served
	if m.Network, err = chooseNetwork(o, level, trust); err != nil {
		return m, err
	}
	return m, nil
}

// chooseLevel returns the level asked for and the level the registry served for it
// (process serves itself).
func chooseLevel(ctx context.Context, asked string, trust model.TrustClass, sel Selector) (level, served model.Isolation, err error) {
	switch {
	case trust == "untrusted":
		if asked != "" && asked != "sandboxed-container" {
			return "", "", trustIsolation("--trust untrusted needs the sandboxed-container level, not --isolation %s; nothing was run", asked)
		}
		if served, err = sel(ctx, "sandboxed-container"); err != nil {
			return "", "", trustIsolation("--trust untrusted needs the sandboxed-container level and this host cannot provide it (%v); nothing was run", err)
		}
		return "sandboxed-container", served, nil
	case trust == "collaborator" && asked == "process":
		return "", "", trustIsolation("--trust collaborator needs at least container isolation, not --isolation process; nothing was run")
	}
	switch asked {
	case "process":
		return "process", "process", nil
	case "sandboxed-container":
		if served, err = sel(ctx, "sandboxed-container"); err != nil {
			return "", "", trustIsolation("--isolation sandboxed-container is unavailable on this host (%v); nothing was run", err)
		}
		return "sandboxed-container", served, nil
	}
	if served, err = sel(ctx, "container"); err != nil {
		return "", "", noIsolation("no container runtime is available (%v); start Docker or pass --isolation process", err)
	}
	return "container", served, nil
}

func chooseNetwork(o cirun.Options, level model.Isolation, trust model.TrustClass) (model.NetworkScope, error) {
	services := len(o.Services) > 0
	if level == "process" {
		switch o.Network {
		case "", "lan":
			return "lan", nil // host networking: the process shares the host's network (ADR 0025)
		case "none", "restricted":
			return "", usage("--network %s cannot be enforced for a process; use a container (the default) or --network lan", o.Network)
		}
		return "", usage("--network %s needs a container", o.Network)
	}
	if services && trust == "untrusted" {
		// the service containers run under the plain runtime on a network with a gateway, not in the sandbox
		return "", trustIsolation("--trust untrusted never gets --service: the service containers are not sandboxed; nothing was run")
	}
	switch o.Network {
	case "":
		if services {
			return "restricted", nil // D9: the service network only
		}
		return "none", nil
	case "none":
		if services {
			return "", usage("--service needs the restricted network scope (the service network), not --network none")
		}
		return "none", nil
	case "restricted":
		if !services {
			return "", usage("--network restricted needs a --service to reach")
		}
		return "restricted", nil
	case "internet":
		if trust == "untrusted" {
			return "", trustIsolation("--trust untrusted never gets --network internet; nothing was run")
		}
		return "internet", nil
	}
	return "", usage("--network %s needs --isolation process", o.Network)
}
