package lifecycle

// Purpose: `nodes test`, `nodes drain` and `nodes enable` (P7-NODE-05, Epic D17).
// Inputs:  a node id.
// Outputs: TestResult (connect, agent hello, probe diff); the node in state draining or online.
// Constraints: a revoked node refuses all three with E657 (an admin recovers it first); `test` connects
//          through the pinned target only, asks the installed agent for `version` (the hello of the ssh
//          carrier) and re-probes; drain lets a running job finish and only blocks new leases (the
//          scheduler reads the state); enable also clears maintenance; each mutating command writes the
//          registry's one audit row.

import (
	"context"
	"flag"
	"reflect"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

// quarantineReason is the maintenance reason registry.CheckAgeRecipient sets when a node presents a
// different identity (E660). The checks below only give the admin a clearer message: the refusal itself
// is inside registry.SetState.
const quarantineReason = registry.QuarantineReason

// TestResult is what `nodes test` reports.
type TestResult struct {
	NodeID       string        `json:"node"`
	Connected    bool          `json:"connected"`
	AgentVersion string        `json:"agent_version"`
	Drift        []string      `json:"drift"`
	Node         registry.Node `json:"-"`
}

// live returns the node, or E650 (missing) or E657 (revoked).
func (d *Deps) live(ctx context.Context, id string) (registry.Node, error) {
	n, err := d.Registry.Get(ctx, id)
	if err != nil {
		return n, err
	}
	if n.Capability.Lifecycle.RevokedAt != nil || n.Record.State == "revoked" {
		return n, newErr(CodeRevoked, ReasonRevoked, "node %s is revoked; an admin must recover it first", id)
	}
	return n, nil
}

// Test connects to the node, asks its agent for the version and probes it again.
func (d *Deps) Test(ctx context.Context, id string, fs *flag.FlagSet) (TestResult, error) {
	res := TestResult{NodeID: id, Drift: []string{}}
	unlock, lerr := d.lockNode(id)
	if lerr != nil {
		return res, newErr(CodeNoRoute, "lock", "node %s is busy: %s", id, oneLine(lerr))
	}
	defer unlock()
	n, err := d.live(ctx, id)
	if err != nil {
		return res, err
	}
	t, err := d.targetFor(ctx, n.Capability)
	if err != nil {
		return res, err
	}
	if res.AgentVersion, err = d.agentVersion(ctx, t); err != nil {
		return res, err
	}
	res.Connected = true
	// A fresh probe, not the prober's 60 s cache: `nodes test` exists to see drift. The node's own prober
	// (NODE-22, with its isolation-only-lowers rule against the stored baseline) runs on the real registry.
	// The node lock keeps rotate, remove and add out meanwhile.
	ver, err := d.SSHVersion(ctx)
	if err != nil {
		return res, newErr(CodeNoRoute, "ssh_version", "cannot determine the local OpenSSH version")
	}
	after, err := d.NewProber(d.Registry, d.pinFile(), ver).Probe(probe.ForceRefresh(ctx), id)
	if err != nil {
		return res, transportErr("probe", "the capability probe failed", err)
	}
	res.Node, res.Drift = after, diffCapability(n.Capability, after.Capability)
	// A test hook may change facts the admin owns (deploy_host, P7-NODE-14): report the stored node.
	hookErr := RunHooks(ctx, PointTest, HookEvent{NodeID: id, Project: d.Project, Flags: fs})
	if fresh, gerr := d.Registry.Get(ctx, id); gerr == nil {
		res.Node = fresh
	}
	if hookErr != nil {
		// The probe is already stored (snapshot and audit row): hand back what was committed with the error.
		code := CodeOf(hookErr)
		if code == "" {
			code = "E607"
		}
		return res, &Error{Code: code, Reason: ReasonCommitted, Msg: "the probe of node " + id + " was recorded, but a test hook failed: " + oneLine(hookErr), cause: hookErr}
	}
	return res, nil
}

func same[T any](a, b model.Fact[T]) bool { return reflect.DeepEqual(a.Value, b.Value) }

// diffCapability lists the JSON pointers of the observed facts that changed between two snapshots.
func diffCapability(old, cur model.Capability) []string {
	out := []string{}
	add := func(path string, equal bool) {
		if !equal {
			out = append(out, path)
		}
	}
	add("/platform/os", same(old.Platform.OS, cur.Platform.OS))
	add("/platform/arch", same(old.Platform.Arch, cur.Platform.Arch))
	add("/platform/kernel", same(old.Platform.Kernel, cur.Platform.Kernel))
	add("/resources/cpu", same(old.Resources.CPU, cur.Resources.CPU))
	add("/resources/mem_mb", same(old.Resources.MemMB, cur.Resources.MemMB))
	add("/tools/docker", same(old.Tools.Docker, cur.Tools.Docker))
	add("/tools/toolchains", reflect.DeepEqual(factValues(old.Tools.Toolchains), factValues(cur.Tools.Toolchains)))
	return out
}

func factValues(m map[string]model.Fact[string]) map[string]string {
	out := make(map[string]string, len(m))
	for k, f := range m {
		if f.Value != nil {
			out[k] = *f.Value
		}
	}
	return out
}

// Drain stops new leases to the node; running jobs finish.
func (d *Deps) Drain(ctx context.Context, id string) (registry.Node, error) {
	n, err := d.live(ctx, id)
	if err != nil {
		return registry.Node{}, err
	}
	if n.Record.Reason == quarantineReason {
		// Draining would overwrite the reason and let a later `enable` clear the quarantine.
		return registry.Node{}, usage("node %s is quarantined (%s): it needs re-verification, not `drain`", id, quarantineReason)
	}
	return d.Registry.SetState(ctx, id, "draining", "drained by admin", d.Actor)
}

// Enable makes the node leasable again and clears drain and maintenance.
func (d *Deps) Enable(ctx context.Context, id string) (registry.Node, error) {
	n, err := d.live(ctx, id)
	if err != nil {
		return registry.Node{}, err
	}
	if n.Record.Reason == quarantineReason {
		return registry.Node{}, usage("node %s is quarantined (%s): it presented a different identity and needs re-verification, not `enable`", id, quarantineReason)
	}
	return d.Registry.SetState(ctx, id, "online", "enabled by admin", d.Actor)
}
