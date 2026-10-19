package lifecycle

// Purpose: turn a registered node (or a new destination) into the validated ssh Target with the D4 option
// set and the pinned host key.
// Inputs:  the node's identity.ssh and id, the pin file.
// Outputs: remote.Target; an E655 error when the node has no pinned key.
// Constraints: the options are remote.CISSHFlags + remote.CIOptions only, plus the port; the node id is
//          lower case because ssh matches HostKeyAlias case-insensitively (the ssh transport enforces
//          the same); a port is never part of an `add` destination: the host-key scan runs on the
//          destination only, so a port belongs in the operator's ssh_config under a Host alias.

import (
	"context"
	"strconv"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// sshOptions is the D4 option set for the node.
func (d *Deps) sshOptions(ctx context.Context, id string) ([]string, error) {
	ver, err := d.SSHVersion(ctx)
	if err != nil {
		return nil, newErr(CodeNoRoute, "ssh_version", "cannot determine the local OpenSSH version")
	}
	return append(remote.CISSHFlags(), remote.CIOptions(id, d.pinFile(), ver)...), nil
}

// specOf is the validated destination of a registered node.
func specOf(n model.Capability) (remote.HostSpec, error) {
	id := n.Identity.ID
	s := n.Identity.SSH
	if s == nil || (s.Alias == "" && s.Hostname == "") {
		return remote.HostSpec{}, newErr(CodeNoRoute, "no_destination", "node %s has no ssh destination", id)
	}
	host := s.Hostname
	if host == "" {
		host = s.Alias
	}
	dest := host
	if s.User != "" {
		dest = s.User + "@" + host
	}
	if s.Port > 0 {
		dest += ":" + strconv.Itoa(s.Port)
	}
	spec, err := remote.ParseHostSpec(dest)
	if err != nil || spec.LegacyPath != "" {
		return remote.HostSpec{}, newErr(CodeNoRoute, "no_destination", "node %s has an invalid ssh destination", id)
	}
	return spec, nil
}

// targetFor builds the pinned target of a registered node.
func (d *Deps) targetFor(ctx context.Context, n model.Capability) (remote.Target, error) {
	spec, err := specOf(n)
	if err != nil {
		return remote.Target{}, err
	}
	return d.pinnedTarget(ctx, n.Identity.ID, spec)
}

// pinnedTarget refuses a node with no pinned key (E655) and returns its target.
func (d *Deps) pinnedTarget(ctx context.Context, id string, spec remote.HostSpec) (remote.Target, error) {
	keys, err := d.pins().Lookup(pinAlias(id))
	if err != nil || len(keys) == 0 {
		return remote.Target{}, newErr(CodeHostKey, ReasonNoKey, "node %s has no pinned host key; %s", id, rotateHint(id))
	}
	opts, err := d.sshOptions(ctx, id)
	if err != nil {
		return remote.Target{}, err
	}
	t, err := spec.Target(opts)
	if err != nil {
		return remote.Target{}, newErr(CodeNoRoute, "no_destination", "node %s: invalid destination", id)
	}
	return t, nil
}

func rotateHint(id string) string {
	return "run: nself ci nodes rotate " + id + " --host-key SHA256:<fingerprint>"
}
