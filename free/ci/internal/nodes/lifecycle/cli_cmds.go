package lifecycle

// Purpose: one handler per `nodes` subcommand: it builds the options from the flags, calls the lifecycle
// function and returns the machine data with its human renderer.
// Inputs:  Deps, the subcommand, its positional argument and flags.
// Outputs: result; an error with a code.
// Constraints: no logic beyond option building; the behaviour lives in add.go, ops.go, revoke.go and
//          rotate.go.

import (
	"context"
	"errors"
	"flag"
	"io"

	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

func dispatch(ctx context.Context, d *Deps, sub, arg string, f flags, fs *flag.FlagSet) (result, error) {
	switch sub {
	case "add":
		return cmdAdd(ctx, d, arg, f, fs)
	case "list":
		return cmdList(ctx, d)
	case "show":
		return cmdShow(ctx, d, arg)
	case "test":
		return cmdTest(ctx, d, arg, fs)
	case "drain":
		return cmdState(ctx, d, arg, "drained", d.Drain)
	case "enable":
		return cmdState(ctx, d, arg, "enabled", d.Enable)
	case "revoke":
		return cmdRevoke(ctx, d, arg, false)
	case "remove":
		return cmdRevoke(ctx, d, arg, true)
	case "rotate":
		return cmdRotate(ctx, d, arg, f, fs)
	}
	return result{}, usage("unknown nodes subcommand %q", sub)
}

func cmdAdd(ctx context.Context, d *Deps, dest string, f flags, fs *flag.FlagSet) (result, error) {
	o := AddOptions{Dest: dest, Name: f.name, HostKey: f.hostKey, Installer: f.installer, Flags: fs, Trust: DefaultTrust()}
	o.Trust.AcceptsSet = f.accepts != ""
	if f.accepts != "" {
		o.Trust.Accepts = splitList(f.accepts)
	}
	if f.secrets != "" {
		o.Trust.SecretClasses = splitList(f.secrets)
	}
	o.Trust.PrivacyZone, o.Trust.Ownership = f.zone, f.ownership
	if f.agentBinary != "" {
		o.Source = LocalFile{Path: f.agentBinary}
	}
	r, err := d.Add(ctx, o)
	if err != nil {
		return result{}, err
	}
	v := ViewOf(r.Node)
	data := map[string]any{"node": v, "host_key_fingerprints": r.Fingerprints, "agent_version": r.AgentVersion,
		"separation_reason": r.Separation.Reason}
	return result{data, func(w io.Writer) { renderAdd(w, v, r) }}, nil
}

func cmdList(ctx context.Context, d *Deps) (result, error) {
	nodes, err := d.Registry.List(ctx)
	if err != nil {
		return result{}, err
	}
	views := make([]NodeView, 0, len(nodes))
	for _, n := range nodes {
		views = append(views, ViewOf(n))
	}
	return result{map[string]any{"nodes": views}, func(w io.Writer) { renderList(w, views) }}, nil
}

func cmdShow(ctx context.Context, d *Deps, id string) (result, error) {
	n, err := d.Registry.Get(ctx, id)
	if err != nil {
		return result{}, err
	}
	v := ViewOf(n)
	return result{map[string]any{"node": v}, func(w io.Writer) { renderShow(w, v) }}, nil
}

func cmdTest(ctx context.Context, d *Deps, id string, fs *flag.FlagSet) (result, error) {
	r, err := d.Test(ctx, id, fs)
	if err != nil {
		return result{}, err
	}
	return result{map[string]any{"test": r, "node": ViewOf(r.Node)}, func(w io.Writer) { renderTest(w, r) }}, nil
}

func cmdState(ctx context.Context, d *Deps, id, verb string, fn func(context.Context, string) (registry.Node, error)) (result, error) {
	n, err := fn(ctx, id)
	if err != nil {
		return result{}, err
	}
	v := ViewOf(n)
	return result{map[string]any{"node": v}, func(w io.Writer) { renderLine(w, "node "+id+" "+verb+": state "+v.State) }}, nil
}

func cmdRevoke(ctx context.Context, d *Deps, id string, remove bool) (result, error) {
	fn := d.Revoke
	if remove {
		fn = d.Remove
	}
	rep, err := fn(ctx, id)
	if err != nil {
		// Only a partial revoke (committed, cleanup incomplete) has a report worth printing; a rollback
		// or a refusal changed nothing, so it must not claim a revocation.
		var le *Error
		if errors.As(err, &le) && le.Reason == ReasonPartial {
			return result{rep, func(w io.Writer) { renderRevoke(w, rep) }}, err
		}
		return result{}, err
	}
	return result{rep, func(w io.Writer) { renderRevoke(w, rep) }}, nil
}

func cmdRotate(ctx context.Context, d *Deps, id string, f flags, fs *flag.FlagSet) (result, error) {
	if err := d.Rotate(ctx, RotateOptions{NodeID: id, Mode: f.mode, HostKey: f.hostKey, Flags: fs}); err != nil {
		return result{}, err
	}
	data := map[string]any{"node": id, "mode": orDefault(f.mode, HostKeyMode)}
	return result{data, func(w io.Writer) { renderLine(w, "node "+id+" rotated ("+orDefault(f.mode, HostKeyMode)+")") }}, nil
}
