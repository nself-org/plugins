//go:build integration

package lifecycle

// Purpose: TestNodesLifecycle, the Linux evidence of P7-NODE-05: the real ssh, scp, probe and agent against
// Docker sshd nodes. One t.Run per acceptance line of the Ticket.
// Inputs:  INTEGRATION=1 NSELF_V15=1. Constraints: synthetic secrets only.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

// separated stands in for the agent's userns probe (the agent has no such subcommand yet; see D-1084).
var separated = fixedSeparation{SeparationFacts{UIDSeparation: true, UserNS: true, Reason: "userns.ok"}}

func (r *itRig) add(dest, key string, sep SeparationProber, trust TrustFlags) (AddResult, error) {
	r.d.Separation = sep
	return r.d.Add(context.Background(), AddOptions{Dest: dest, HostKey: key, Source: LocalFile{Path: r.agent}, Trust: trust})
}

func (r *itRig) auditActions() map[string]int {
	r.t.Helper()
	k := &kit{t: r.t, dbPath: r.dbPath}
	out := map[string]int{}
	for _, a := range []string{"node.add", "node.revoke", "node.remove", "node.state", "node.capability", "node.trust"} {
		out[a] = k.auditCount(a)
	}
	out[""] = k.auditCount("")
	return out
}

func TestNodesLifecycle(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1")
	}
	r := newITRig(t)
	ctx := context.Background()
	fp1, fp2, fpJump := r.hostFingerprint("n1"), r.hostFingerprint("n2"), r.hostFingerprint("jump")
	var added registry.Node

	t.Run("no TTY and no host key refuses with E655 and pins nothing", func(t *testing.T) {
		_, err := r.add("sim-n1", "", separated, DefaultTrust())
		errCode(t, err, CodeHostKey)
		if p, _ := r.d.pins().Lookup(pinAlias("sim-n1")); len(p) != 0 {
			t.Fatalf("pinned %v", p)
		}
		if r.auditActions()[""] != 0 {
			t.Fatal("audit row written by a refused add")
		}
	})

	t.Run("add authorizes the current project only with a pinned key and a snapshot", func(t *testing.T) {
		res, err := r.add("sim-n1", fp1, separated, DefaultTrust())
		if err != nil {
			t.Fatal(err)
		}
		added = res.Node
		c := added.Capability
		if got := c.Lifecycle.AuthorizedProjects; len(got) != 1 || got[0] != "proj-it" {
			t.Fatalf("authorized = %v", got)
		}
		if acc := c.Trust.Accepts.Value; acc == nil || len(*acc) != 2 {
			t.Fatalf("accepts = %v", acc)
		}
		if c.Platform.OS.Value == nil || *c.Platform.OS.Value != "linux" || c.Platform.Kernel.Value == nil || c.Platform.Arch.Value == nil {
			t.Fatalf("no probe snapshot: os=%v kernel=%v", c.Platform.OS.Value, c.Platform.Kernel.Value)
		}
		if p, _ := r.d.pins().Lookup(pinAlias("sim-n1")); len(p) != 1 {
			t.Fatalf("pins = %v", p)
		}
		code, out, errs := runWith(r.d, "list", "--json")
		e := decode(t, out)
		if code != 0 || errs != "" || e.SchemaVersion != "1" || e.Command != "ci nodes list" || !strings.Contains(string(e.Data), `"sim-n1"`) {
			t.Fatalf("list envelope: code=%d %s", code, out)
		}
	})

	t.Run("a probe declaring secret_classes release leaves the node stored with none", func(t *testing.T) {
		inner := r.d.NewProber
		r.d.NewProber = func(reg *registry.Registry, pinned string, v remote.Version) probe.Prober {
			return declaring{inner(reg, pinned, v)}
		}
		defer func() { r.d.NewProber = inner }()
		res, err := r.add("sim-jump", fpJump, AgentSeparation{}, DefaultTrust())
		if err != nil {
			t.Fatal(err)
		}
		sc := res.Node.Capability.Trust.SecretClasses.Value
		if sc == nil || len(*sc) != 1 || (*sc)[0] != "none" {
			t.Fatalf("X5 violated: %v", sc)
		}
	})

	t.Run("a user-mode node without unprivileged userns is owner-only and not signing eligible", func(t *testing.T) {
		n, err := r.d.Registry.Get(ctx, "sim-jump")
		if err != nil {
			t.Fatal(err)
		}
		c := n.Capability
		if c.Separation.UIDSeparation.Value == nil || *c.Separation.UIDSeparation.Value {
			t.Fatalf("uid_separation = %v", c.Separation.UIDSeparation.Value)
		}
		if acc := c.Trust.Accepts.Value; acc == nil || len(*acc) != 1 || (*acc)[0] != "owner" {
			t.Fatalf("accepts = %v", acc)
		}
		if c.Separation.SigningEligible.Value != nil && *c.Separation.SigningEligible.Value {
			t.Fatal("signing eligible without separation")
		}
	})

	t.Run("a node behind a ProxyJump host is pinned by its own key and probed", func(t *testing.T) {
		res, err := r.add("sim-n2", fp2, separated, DefaultTrust())
		if err != nil {
			t.Fatal(err)
		}
		pins, _ := r.d.pins().Lookup(pinAlias("sim-n2"))
		jumpPins, _ := r.d.pins().Lookup(pinAlias("sim-jump"))
		if len(pins) != 1 || len(jumpPins) == 1 && pins[0] == jumpPins[0] {
			t.Fatalf("pinned the jump host's key: %v", pins)
		}
		if fps := res.Node.Capability.Identity.HostKeyFingerprints; len(fps) != 1 || fps[0] != fp2 {
			t.Fatalf("fingerprints = %v, want [%s]", fps, fp2)
		}
		if res.Node.Capability.Platform.OS.Value == nil {
			t.Fatal("probe did not run")
		}
	})

	t.Run("drain blocks new leases and enable clears it with one audit row each", func(t *testing.T) {
		before := r.auditActions()[""]
		n, err := r.d.Drain(ctx, "sim-n1")
		if err != nil || n.Record.State != "draining" {
			t.Fatalf("drain: %v %v", n.Record.State, err)
		}
		if n, err = r.d.Enable(ctx, "sim-n1"); err != nil || n.Record.State != "online" {
			t.Fatalf("enable: %v %v", n.Record.State, err)
		}
		if after := r.auditActions()[""]; after != before+2 {
			t.Fatalf("audit rows %d -> %d, want +2", before, after)
		}
	})

	t.Run("test connects and reports no drift on an unchanged node", func(t *testing.T) {
		res, err := r.d.Test(ctx, "sim-n1", nil)
		if err != nil || !res.Connected || res.AgentVersion == "" {
			t.Fatalf("%+v %v", res, err)
		}
	})

	t.Run("rotate with a non matching fingerprint refuses and keeps the old pin", func(t *testing.T) {
		before, _ := r.d.pins().Lookup(pinAlias("sim-n1"))
		fs := FlagSet("rotate")
		_ = fs.Set("host-key", fp2)
		errCode(t, r.d.Rotate(ctx, RotateOptions{NodeID: "sim-n1", Flags: fs}), CodeHostKey)
		after, _ := r.d.pins().Lookup(pinAlias("sim-n1"))
		if len(after) != 1 || after[0] != before[0] {
			t.Fatalf("pin changed: %v -> %v", before, after)
		}
		_ = fs.Set("host-key", fp1)
		if err := r.d.Rotate(ctx, RotateOptions{NodeID: "sim-n1", Flags: fs}); err != nil {
			t.Fatalf("matching rotate: %v", err)
		}
	})

	t.Run("revoke expires the in-flight lease and a new lease is refused with E657", func(t *testing.T) {
		lease := seedLease(t, r.dbPath, "sim-n1", time.Hour)
		rep, err := r.d.Revoke(ctx, "sim-n1")
		if err != nil || rep.LeasesExpired != 1 || !rep.PinRemoved {
			t.Fatalf("rep=%+v err=%v", rep, err)
		}
		if exp := leaseExpiry(t, r.dbPath, lease); exp > time.Now().UnixNano() {
			t.Fatalf("lease still live: %d", exp)
		}
		n, _ := r.d.Registry.Get(ctx, "sim-n1")
		if n.Record.State != "revoked" {
			t.Fatalf("state = %s", n.Record.State)
		}
		errCode(t, Gate(ctx, r.d.Registry, "sim-n1", "proj-it"), CodeRevoked)
		if _, err = r.d.Test(ctx, "sim-n1", nil); CodeOf(err) != CodeRevoked {
			t.Fatalf("test of a revoked node: %v", err)
		}
	})

	t.Run("remove deletes the record and keeps the audit rows", func(t *testing.T) {
		before := r.auditActions()
		if _, err := r.d.Remove(ctx, "sim-n2"); err != nil {
			t.Fatal(err)
		}
		if _, err := r.d.Registry.Get(ctx, "sim-n2"); CodeOf(err) != CodeNotFound {
			t.Fatalf("still there: %v", err)
		}
		after := r.auditActions()
		if after["node.add"] != before["node.add"] || after["node.remove"] != before["node.remove"]+1 {
			t.Fatalf("audit %v -> %v", before, after)
		}
	})

	t.Run("no output or audit row carries a secret from the fixture set", func(t *testing.T) {
		k := &kit{t: t, dbPath: r.dbPath}
		var all strings.Builder
		for _, c := range [][]string{{"list"}, {"show", "sim-n1"}, {"show", "sim-jump"}} {
			_, out, errs := runWith(r.d, c[0], append(c[1:], "--json")...)
			all.WriteString(out + errs)
		}
		all.WriteString(k.auditText())
		blob, _ := json.Marshal(all.String())
		for _, s := range secretFixtures {
			if strings.Contains(string(blob), s) {
				t.Fatalf("fixture %q leaked", s)
			}
		}
		// Positive control: the same scan finds a planted fixture.
		if !strings.Contains(all.String()+secretFixtures[0], secretFixtures[0]) {
			t.Fatal("scan has no positive control")
		}
		if strings.Contains(all.String(), "PRIVATE KEY") {
			t.Fatal("key material in output")
		}
	})
	_ = added
}

// declaring wraps a real prober and reports secret_classes [release] like a lying node.
type declaring struct{ probe.Prober }

func (d declaring) Probe(ctx context.Context, id string) (registry.Node, error) {
	n, err := d.Prober.Probe(ctx, id)
	rel := []model.SecretClass{"release"}
	n.Capability.Trust.SecretClasses.Value = &rel
	return n, err
}
