package lifecycle

// Purpose: `nself ci nodes add <dest>` (P7-NODE-05, Epic D17): scan the host key through ssh, show the
// fingerprint, pin it after an interactive `yes` or an exact --host-key, install the agent, record the
// separation, probe, and register the node authorized for the current project only.
// Inputs:  AddOptions (destination, name, --host-key, --agent-binary, the admin's trust flags).
// Outputs: the registered node; one audit row (node.add); a pinned key in $NSELF_CI_HOME/known_hosts.
// Constraints: the pre-add hooks run before the first contact with the host; nothing is registered until the probe succeeded, so a failed add leaves no node and its
//          pin is rolled back; the node id is lower case; the destination carries no port (see
//          target.go); the probe runs against a throwaway registry so the real store gets exactly one
//          write and one audit row; trust is the admin's flags only (X5).

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// AddOptions are the flags of `nodes add`.
type AddOptions struct {
	Dest      string
	Name      string
	HostKey   string
	Installer string
	Source    AgentSource
	Trust     TrustFlags
	Flags     *flag.FlagSet
}

// AddResult is what `nodes add` reports.
type AddResult struct {
	Node         registry.Node
	Fingerprints []string
	Separation   SeparationFacts
	AgentVersion string
}

// nodeID derives the lower-case node id from the name or the host.
func nodeID(o AddOptions, spec remote.HostSpec) (id, name string, err error) {
	name = o.Name
	if name == "" {
		name = spec.Host
	}
	id = strings.ToLower(name)
	if verr := remote.ValidateNodeID(id); verr != nil {
		return "", "", usage("node name %q cannot be a node id; pass --name with letters, digits, dot, dash or underscore", name)
	}
	return id, name, nil
}

// Add registers a node. See the file comment for the order of the steps.
func (d *Deps) Add(ctx context.Context, o AddOptions) (res AddResult, err error) {
	spec, err := remote.ParseHostSpec(o.Dest)
	if err != nil || spec.LegacyPath != "" {
		return res, usage("destination %q is not [user@]host", o.Dest)
	}
	if spec.Port > 0 {
		return res, usage("put the port in your ssh config under a Host alias, then add the alias (the host-key scan runs on the destination only)")
	}
	if d.Project == "" {
		return res, usage("no current project: run inside a repository or pass --project")
	}
	if err = validateTrust(o.Trust); err != nil {
		return res, err
	}
	id, name, err := nodeID(o, spec)
	if err != nil {
		return res, err
	}
	unlock, err := d.lockNode(id)
	if err != nil {
		return res, newErr(CodeNoRoute, "lock", "cannot lock node %s: %s", id, oneLine(err))
	}
	defer unlock()
	if _, gerr := d.Registry.Get(ctx, id); gerr == nil {
		return res, usage("node %s is already registered; use `nodes test` or `nodes rotate`", id)
	} else if CodeOf(gerr) != CodeNotFound {
		return res, gerr
	}
	installer, ok := ext.installer(orDefault(o.Installer, DefaultInstaller))
	if !ok {
		return res, usage("unknown agent installer %q", o.Installer)
	}
	// Nothing has touched the host yet: a pre-add hook (the deploy-host check) refuses here, with no scan,
	// no ssh and no agent install on the host.
	if err = RunHooks(ctx, PointPreAdd, HookEvent{NodeID: id, Project: d.Project, Flags: o.Flags, Dest: o.Dest}); err != nil {
		return res, err
	}
	keys, err := d.Scan(ctx, remote.Target{Dest: spec.Dest()}, id)
	if err != nil {
		return res, transportErr("scan", "cannot capture the host key of "+spec.Dest(), err)
	}
	chosen, err := d.confirmKeys(id, spec.Dest(), keys, o.HostKey)
	if err != nil {
		return res, err
	}
	if err = d.pin(id, chosen); err != nil {
		return res, err
	}
	defer func() {
		if err != nil {
			_ = d.withPins(func(p remote.PinnedHostKeys) error { return p.Remove(pinAlias(id)) })
		}
	}()
	return d.addPinned(ctx, o, spec, id, name, chosen, installer)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// addPinned runs the steps after the key is pinned.
func (d *Deps) addPinned(ctx context.Context, o AddOptions, spec remote.HostSpec, id, name string, keys []remote.HostKey, installer AgentInstaller) (res AddResult, err error) {
	t, err := d.pinnedTarget(ctx, id, spec)
	if err != nil {
		return res, err
	}
	plat, err := d.platform(ctx, t)
	if err != nil {
		return res, err
	}
	src := o.Source
	if src == nil {
		// No --agent-binary: an agent already on the node is used as it is; otherwise the registered default
		// (the signed Release source) installs one.
		if _, terr := d.Run(ctx, t, "test", "-x", agentPath); terr != nil {
			if src = ext.defaultSource(); src == nil {
				return res, usage("the agent is not installed on %s; pass --agent-binary <path>", id)
			}
		}
	}
	if src != nil {
		req := InstallRequest{NodeID: id, Target: t, Platform: plat, Dir: agentDir, Source: src, Flags: o.Flags}
		if err = installer(ctx, d, req); err != nil {
			return res, err
		}
	}
	if res.AgentVersion, err = d.agentVersion(ctx, t); err != nil {
		return res, err
	}
	run := func(ctx context.Context, argv ...string) (string, error) { return d.Run(ctx, t, argv...) }
	res.Separation = d.Separation.Detect(ctx, run, plat)
	for _, k := range keys {
		res.Fingerprints = append(res.Fingerprints, k.Fingerprint)
	}
	now := time.Now().UTC()
	draft := newCapability(id, name, spec, plat, res.Fingerprints, res.Separation, res.AgentVersion, now)
	probed, err := d.probeDraft(ctx, draft)
	if err != nil {
		return res, err
	}
	if err = applyTrust(&probed, o.Trust, now); err != nil {
		return res, err
	}
	if err = RunHooks(ctx, PointAdd, HookEvent{NodeID: id, Project: d.Project, Flags: o.Flags, Dest: o.Dest, Capability: &probed}); err != nil {
		return res, err
	}
	online := "online"
	probed.Availability.State = model.Fact[string]{Value: &online, Source: "assigned", ObservedAt: now, Confidence: "known"}
	probed.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{d.Project}}
	doc, err := marshalCapability(probed)
	if err != nil {
		return res, err
	}
	admin, err := store.NewAdminAuthority(d.Actor)
	if err != nil {
		return res, usage("no admin identity for the audit row")
	}
	if err = d.Store.NodeAdd(ctx, store.NodeRecord{ID: id, Name: name, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		return res, err
	}
	// The node is committed: a failed read-back must not roll the pin back (the node would be left
	// registered with no pin), so it is not an error of the add.
	if n, gerr := d.Registry.Get(ctx, id); gerr == nil {
		res.Node = n
	}
	return res, nil
}

// probeDraft probes the draft in a throwaway registry and returns the probed capability.
func (d *Deps) probeDraft(ctx context.Context, draft model.Capability) (model.Capability, error) {
	ver, err := d.SSHVersion(ctx)
	if err != nil {
		return model.Capability{}, newErr(CodeNoRoute, "ssh_version", "cannot determine the local OpenSSH version")
	}
	if err = os.MkdirAll(d.Home, 0o700); err != nil {
		return model.Capability{}, err
	}
	dir, err := os.MkdirTemp(d.Home, ".add-probe-")
	if err != nil {
		return model.Capability{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	tmp, err := store.Open(filepath.Join(dir, "probe.db"), store.Options{})
	if err != nil {
		return model.Capability{}, err
	}
	defer func() { _ = tmp.Close() }()
	reg := registry.New(tmp)
	if _, err = reg.Register(ctx, draft, "probe"); err != nil {
		return model.Capability{}, err
	}
	n, err := d.NewProber(reg, d.pinFile(), ver).Probe(ctx, draft.Identity.ID)
	if err != nil {
		return model.Capability{}, transportErr("probe", "the capability probe failed", err)
	}
	c := n.Capability
	// Register resets what a node must not choose; keep the observed separation of the draft.
	c.Separation = draft.Separation
	c.SecretsEligible = draft.SecretsEligible
	return c, nil
}
