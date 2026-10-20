package update

// Purpose: the lifecycle seams of `nodes upgrade`: drain, enable, probe and platform of one node, built on lifecycle.Deps.
// Inputs:  a lifecycle.Deps and a node id.
// Outputs: the DrainFn, EnableFn and ProbeFn the carrier nodes take.
// Constraints: drain and enable are the lifecycle commands (one registry audit row each); a node that was not online before the
//          upgrade (draining or in maintenance by the admin's choice) is not enabled again; the probe is a fresh one (the stored
//          snapshot is aged first, as `nodes test` does) and runs through the node's own prober.

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// drainFn drains the node and returns what puts it back as it was: enabled when it was online, its old state and reason when it
// was in maintenance or offline, nothing when the admin had drained it already.
func drainFn(d *lifecycle.Deps, id string) func(context.Context) (func(context.Context) error, error) {
	return func(ctx context.Context) (func(context.Context) error, error) {
		n, err := d.Registry.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		prev, reason := n.Record.State, n.Record.Reason
		if _, err = d.Drain(ctx, id); err != nil {
			return nil, err
		}
		switch prev {
		case "online":
			return func(ctx context.Context) error { _, err := d.Enable(ctx, id); return err }, nil
		case "maintenance", "offline":
			return func(ctx context.Context) error {
				_, err := d.Registry.SetState(ctx, id, prev, reason, d.Actor)
				return err
			}, nil
		}
		return nil, nil
	}
}

func probeFn(d *lifecycle.Deps, id string) func(context.Context) error {
	return func(ctx context.Context) error {
		n, err := d.Registry.Get(ctx, id)
		if err != nil {
			return err
		}
		old := n.Capability
		old.Verified = model.CapabilityVerified{By: "probe", At: time.Now().UTC().Add(-time.Hour)}
		if _, _, err = d.Registry.UpdateCapability(ctx, id, old, "probe"); err != nil {
			return err
		}
		ver, err := d.SSHVersion(ctx)
		if err != nil {
			return fmt.Errorf("E656: cannot determine the local OpenSSH version: %w", err)
		}
		if _, err = d.NewProber(d.Registry, filepath.Join(d.Home, "known_hosts"), ver).Probe(ctx, id); err != nil {
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
