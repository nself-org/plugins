package update

// Purpose: the lifecycle seams of `nodes upgrade`: drain, enable, probe and platform of one node, built on lifecycle.Deps.
// Inputs:  a lifecycle.Deps and a node id.
// Outputs: the DrainFn, EnableFn and ProbeFn the carrier nodes take.
// Constraints: drain and enable are the lifecycle commands (one registry audit row each); a node that was not online before the
//          upgrade (draining or in maintenance by the admin's choice) is not enabled again; the probe is a fresh one (probe.ForceRefresh,
//          as `nodes test` does) and runs through the node's own prober.

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
)

// DrainReason is the registry reason of a node that `nodes upgrade` drained.
const DrainReason = "upgrade in progress"

// drainFn drains the node and returns what puts it back as it was: enabled when it was online, its old state and reason when it
// was in maintenance, offline or drained by the admin.
func drainFn(d *lifecycle.Deps, id string) func(context.Context) (func(context.Context) error, error) {
	return func(ctx context.Context) (func(context.Context) error, error) {
		n, err := d.Registry.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		prev, reason := n.Record.State, n.Record.Reason
		if prev == "draining" && reason == DrainReason {
			prev = "online" // our own drain from a run that was cut off
		}
		undo := func(ctx context.Context) error {
			if prev == "online" {
				_, err := d.Enable(ctx, id)
				return err
			}
			// maintenance, offline and an admin's own drain go back as they were, with their reason
			_, err := d.Registry.SetState(ctx, id, prev, reason, d.Actor)
			return err
		}
		if _, err = d.Drain(ctx, id); err != nil {
			return nil, err
		}
		// The reason marks the drain as ours: a run that was cut off (Ctrl-C, a lost session) leaves it, and the next run puts the
		// node back online instead of mistaking it for the admin's own drain.
		if _, err = d.Registry.SetState(ctx, id, "draining", DrainReason, d.Actor); err != nil {
			_ = undo(context.WithoutCancel(ctx)) // the first write landed: do not leave the admin's reason on a drained node
			return nil, err
		}
		return undo, nil
	}
}

func probeFn(d *lifecycle.Deps, id string) func(context.Context) error {
	return func(ctx context.Context) error {
		ver, err := d.SSHVersion(ctx)
		if err != nil {
			return fmt.Errorf("E656: cannot determine the local OpenSSH version: %w", err)
		}
		if _, err = d.NewProber(d.Registry, filepath.Join(d.Home, "known_hosts"), ver).Probe(probe.ForceRefresh(ctx), id); err != nil {
			return fmt.Errorf("E656: the capability probe of %s failed: %w", id, err)
		}
		return nil
	}
}

// platformOf is the recorded platform of the node, else `uname -sm` over the pinned target.
func platformOf(ctx context.Context, d *lifecycle.Deps, t remote.Target, c model.Capability) (lifecycle.Platform, error) {
	if p, err := capabilityPlatform(c); err == nil {
		return p, nil
	}
	out, err := d.Run(ctx, t, "uname", "-sm")
	if err != nil {
		return lifecycle.Platform{}, fmt.Errorf("E656: cannot run uname on %s: %w", c.Identity.ID, err)
	}
	return lifecycle.ParsePlatform(lastLine(out))
}

func capabilityPlatform(c model.Capability) (lifecycle.Platform, error) {
	if c.Platform.OS.Value == nil || c.Platform.Arch.Value == nil {
		return lifecycle.Platform{}, fmt.Errorf("E651: the platform of %s is not recorded", c.Identity.ID)
	}
	return lifecycle.Platform{OS: *c.Platform.OS.Value, Arch: *c.Platform.Arch.Value}, nil
}
