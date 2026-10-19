package ssh

// Purpose: turn a node record into the validated ssh Target with the D4 option
// set and the pinned host key.
// Inputs:  model.Capability (identity.id, identity.ssh), the pin file.
// Outputs: remote.Target, the HostKeyAlias.
// Constraints: ParseHostSpec validates the destination; the options are
// CISSHFlags + CIOptions only (plus the port); a node with no pinned key is
// refused with E655 before ssh starts.

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// destination renders the node's ssh destination for ParseHostSpec: the alias
// when only an alias is known (the operator's ssh config may add ProxyJump),
// else [user@]hostname[:port].
func destination(n model.Capability) (string, bool) {
	s := n.Identity.SSH
	if s == nil {
		return "", false
	}
	host := s.Hostname
	if host == "" {
		host = s.Alias
	}
	if host == "" {
		return "", false
	}
	out := host
	if s.User != "" {
		out = s.User + "@" + out
	}
	if s.Port > 0 {
		out += ":" + strconv.Itoa(s.Port)
	}
	return out, true
}

func (d *Driver) target(ctx context.Context, n model.Capability) (remote.Target, error) {
	id := n.Identity.ID
	if err := remote.ValidateNodeID(id); err != nil {
		return remote.Target{}, newErr(CodeTransport, ReasonNoDestination, "node id %q is not valid", id)
	}
	dest, ok := destination(n)
	if !ok {
		return remote.Target{}, newErr(CodeTransport, ReasonNoDestination, "node %s has no ssh destination", id)
	}
	spec, err := remote.ParseHostSpec(dest)
	if err != nil || spec.LegacyPath != "" {
		return remote.Target{}, newErr(CodeTransport, ReasonNoDestination, "node %s has an invalid ssh destination", id)
	}
	pin := remote.PinnedHostKeys{Path: filepath.Join(d.home(), "known_hosts")}
	keys, err := pin.Lookup("nself-ci-" + id)
	if err != nil || len(keys) == 0 {
		return remote.Target{}, newErr(CodeHostKey, ReasonHostKeyUnknown, "node %s has no pinned host key; %s", id, rotateHint)
	}
	ver, err := d.version(ctx)
	if err != nil {
		return remote.Target{}, newErr(CodeTransport, ReasonConnect, "cannot determine the local OpenSSH version: %v", err)
	}
	opts := append(remote.CISSHFlags(), remote.CIOptions(id, pin.Path, ver)...)
	if d.IdentityFile != "" {
		// After the D4 block, which always wins; only this one key is offered to the node.
		opts = append(opts, "-i", d.IdentityFile, "-o", "IdentitiesOnly=yes")
	}
	t, err := spec.Target(opts)
	if err != nil {
		return remote.Target{}, newErr(CodeTransport, ReasonNoDestination, "node %s: %v", id, err)
	}
	return t, nil
}
