package lifecycle

// Purpose: `nodes rotate` (ADR 0025 §10): replace the pinned host key of a node, or run a registered
// rotate mode (P7-NODE-07 registers the node credential modes).
// Inputs:  a node id, --host-key SHA256:<fp> (mode host-key), --mode <name>.
// Outputs: the new pin and the node's recorded fingerprints; one audit row (node.capability).
// Constraints: the key is re-scanned through ssh and pinned only when one captured key has exactly the
//          fingerprint the admin typed; a non-matching fingerprint refuses with E655 and keeps the old
//          pin; a revoked node cannot rotate.

import (
	"context"
	"flag"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// HostKeyMode is the built-in rotate mode.
const HostKeyMode = "host-key"

func init() { RegisterRotateMode(HostKeyMode, rotateHostKey) }

// offHostModes are the rotate modes that act on coordinator state only (host-key recovery, and the PKI
// modes of P7-NODE-07: cert, node-key), so a pull node without an ssh destination or pin can use them.
var offHostModes = map[string]bool{HostKeyMode: true, "cert": true, "node-key": true}

// RotateOptions are the flags of `nodes rotate`.
type RotateOptions struct {
	NodeID  string
	Mode    string
	HostKey string
	Flags   *flag.FlagSet
}

// Rotate runs the selected mode.
func (d *Deps) Rotate(ctx context.Context, o RotateOptions) error {
	mode := orDefault(o.Mode, HostKeyMode)
	fn, ok := ext.mode(mode)
	if !ok {
		return usage("unknown rotate mode %q", mode)
	}
	unlock, err := d.lockNode(o.NodeID)
	if err != nil {
		return newErr(CodeNoRoute, "lock", "cannot lock node %s: %s", o.NodeID, oneLine(err))
	}
	defer unlock()
	n, err := d.live(ctx, o.NodeID)
	if err != nil {
		return err
	}
	spec, err := specOf(n.Capability)
	if err != nil && !offHostModes[mode] {
		return err
	}
	// Modes that talk to the node (P7-NODE-07 credential rotation) get the pinned target, never a bare
	// destination, so a new credential cannot travel over an unverified ssh. Host-key rotation is the
	// recovery path for a lost pin, so it does not need one.
	t, terr := d.targetFor(ctx, n.Capability)
	if terr != nil && !offHostModes[mode] {
		return terr
	}
	return fn(ctx, d, RotateRequest{NodeID: o.NodeID, Target: t, ScanDest: spec.Dest(), HostKey: o.HostKey, Flags: o.Flags})
}

func rotateHostKey(ctx context.Context, d *Deps, r RotateRequest) error {
	want := r.HostKey
	if f := lookupFlag(r.Flags, "host-key"); want == "" && f != nil {
		want = f.Value.String()
	}
	if want == "" {
		return usage("rotate needs --host-key SHA256:<fingerprint>")
	}
	keys, err := d.Scan(ctx, remote.Target{Dest: r.ScanDest}, r.NodeID)
	if err != nil {
		return transportErr("scan", "cannot capture the host key of "+r.ScanDest, err)
	}
	var match *remote.HostKey
	for i := range keys {
		if keys[i].Fingerprint == want {
			match = &keys[i]
		}
	}
	if match == nil {
		return newErr(CodeHostKey, ReasonKeyMismatch, "none of the host keys of %s matches --host-key; the old pin is kept; found: %s", r.ScanDest, fingerprints(keys))
	}
	err = d.withPins(func(p remote.PinnedHostKeys) error {
		// Under the pin lock: a revoke that committed during the scan must win, never get its pin back.
		if _, lerr := d.live(ctx, r.NodeID); lerr != nil {
			return lerr
		}
		return p.Replace(pinAlias(r.NodeID), keyLine(*match))
	})
	if CodeOf(err) == CodeRevoked || CodeOf(err) == CodeNotFound {
		return err
	}
	if err != nil {
		return newErr(CodeHostKey, ReasonKeyMismatch, "cannot write the pinned host key of %s", r.NodeID)
	}
	n, err := d.Registry.Get(ctx, r.NodeID)
	if err != nil {
		return err
	}
	c := n.Capability
	c.Identity.HostKeyFingerprints = []string{match.Fingerprint}
	if _, _, err = d.Registry.UpdateCapability(ctx, r.NodeID, c, d.Actor); err != nil {
		return newErr(CodeHostKey, ReasonKeyMismatch, "the new key is pinned for %s but its record could not be updated: %v", r.NodeID, err)
	}
	return nil
}

func lookupFlag(fs *flag.FlagSet, name string) *flag.Flag {
	if fs == nil {
		return nil
	}
	return fs.Lookup(name)
}
