package deployguard

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func draftCapability(t *testing.T, r *rig, host string) *model.Capability {
	t.Helper()
	return r.capability("draft", ssh(host))
}

func (r *rig) auditCount(action string) int {
	n := 0
	for _, a := range r.audits {
		if a.Action == action {
			n++
		}
	}
	return n
}

func yes(string, string) (bool, error) { return true, nil }

func TestCheckAddMarksAMatchIneligibleWithoutTheFlag(t *testing.T) {
	r := newRig(t)
	_ = r.g.Seen(context.Background(), r.rootA)
	c := draftCapability(t, r, FixtureHostV4)
	res, warn, err := r.g.CheckAdd(context.Background(), AddRequest{NodeID: "box", Dest: "ci@" + FixtureHostV4, Capability: c})
	if err != nil || !res.Matched || len(warn) != 0 {
		t.Fatalf("%+v %v %v", res, warn, err)
	}
	if !c.DeployHost.Matched || c.DeployHost.Override || c.DeployHost.Source == nil || *c.DeployHost.Source != SourceInventory {
		t.Fatalf("draft %+v", c.DeployHost)
	}
	if r.auditCount("node.deploy_host.override") != 0 {
		t.Fatal("audit row without an override")
	}
}

func TestCheckAddLeavesAnUnlistedHostAlone(t *testing.T) {
	r := newRig(t)
	_ = r.g.Seen(context.Background(), r.rootA)
	c := draftCapability(t, r, FixtureOther)
	res, _, err := r.g.CheckAdd(context.Background(), AddRequest{NodeID: "ok", Dest: FixtureOther, Capability: c, Allow: true, Confirm: yes})
	if err != nil || res.Matched || c.DeployHost.Matched || c.DeployHost.Override || r.auditCount("node.deploy_host.override") != 0 {
		t.Fatalf("%+v %v %+v", res, err, c.DeployHost)
	}
}

func TestOverrideNeedsConfirmationAndWritesOneAuditRow(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_ = r.g.Seen(ctx, r.rootA)
	req := func(c *model.Capability, confirm Confirmer) AddRequest {
		return AddRequest{NodeID: "box", Dest: FixtureHostV4, Capability: c, Allow: true, Confirm: confirm, Actor: "cli:ali"}
	}
	// No confirmer, a declined answer and a prompt error all refuse and leave no audit row.
	for name, confirm := range map[string]Confirmer{
		"none":     nil,
		"declined": func(string, string) (bool, error) { return false, nil },
		"error":    func(string, string) (bool, error) { return false, errors.New("closed") },
	} {
		c := draftCapability(t, r, FixtureHostV4)
		_, _, err := r.g.CheckAdd(ctx, req(c, confirm))
		if err == nil || (name != "error" && !strings.Contains(err.Error(), "E663")) || c.DeployHost.Override || c.DeployHost.Matched {
			t.Errorf("%s: err %v draft %+v", name, err, c.DeployHost)
		}
		if r.auditCount("node.deploy_host.override") != 0 {
			t.Fatalf("%s wrote an audit row", name)
		}
	}
	c := draftCapability(t, r, FixtureHostV4)
	var asked, want string
	res, _, err := r.g.CheckAdd(ctx, req(c, func(p, w string) (bool, error) { asked, want = p, w; return true, nil }))
	if err != nil || !res.Matched || !c.DeployHost.Matched || !c.DeployHost.Override {
		t.Fatalf("%+v %v %+v", res, err, c.DeployHost)
	}
	if want != "box" || !strings.Contains(asked, "deploy host") {
		t.Fatalf("prompt %q want %q", asked, want)
	}
	if n := r.auditCount("node.deploy_host.override"); n != 1 {
		t.Fatalf("audit rows = %d", n)
	}
	for _, a := range r.audits {
		if a.Action == "node.deploy_host.override" && (a.Actor != "cli:ali" || a.Target != "box" || !strings.Contains(a.Detail, "source=inventory")) {
			t.Fatalf("audit row %+v", a)
		}
	}
}

func TestTerminalConfirm(t *testing.T) {
	var out bytes.Buffer
	if ok, err := TerminalConfirm(strings.NewReader("box\n"), &out, false)("p", "box"); ok || ReasonOf(err) != ReasonNoTTY {
		t.Fatalf("no tty: %v %v", ok, err)
	}
	for in, want := range map[string]bool{"box\n": true, "box": true, "  box  \r\n": true, "yes\n": false, "BOX\n": false, "": false, "boxx\n": false} {
		out.Reset()
		ok, err := TerminalConfirm(strings.NewReader(in), &out, true)("deploy host!", "box")
		if err != nil || ok != want || !strings.Contains(out.String(), "deploy host!") || !strings.Contains(out.String(), "(box)") {
			t.Errorf("%q: %v %v %q", in, ok, err, out.String())
		}
	}
}

func TestCheckAddRefusesAnUnmatchedNodeWhileASourceWasNeverRead(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_ = r.g.Seen(ctx, r.rootA)
	r.run.HelpSide = "write" // the refresh refuses; nothing was ever cached
	res, warn, err := r.g.CheckAdd(ctx, AddRequest{NodeID: "x", Dest: FixtureHostV4})
	if ReasonOf(err) != ReasonInventory || res.Matched || len(warn) == 0 || !strings.Contains(err.Error(), "never been read") {
		t.Fatalf("%+v %v %v", res, warn, err)
	}
	r.run.HelpSide = "read"
	res, warn, err = r.g.CheckAdd(ctx, AddRequest{NodeID: "x", Dest: FixtureHostV4})
	if err != nil || !res.Matched || len(warn) != 0 {
		t.Fatalf("%+v %v %v", res, warn, err)
	}
	// A source that worked once keeps its cache and no longer blocks an unmatched add.
	r.run.HelpSide = "write"
	res, warn, err = r.g.CheckAdd(ctx, AddRequest{NodeID: "y", Dest: FixtureOther})
	if err != nil || res.Matched || len(warn) == 0 {
		t.Fatalf("after a good read: %+v %v %v", res, warn, err)
	}
	// A matched node is registered ineligible even while the refresh fails.
	if res, _, err = r.g.CheckAdd(ctx, AddRequest{NodeID: "z", Dest: FixtureHostV4}); err != nil || !res.Matched {
		t.Fatalf("matched during a failing refresh: %+v %v", res, err)
	}
}

func TestCheckAddTakesTheDestinationFromTheDraftWhenNoneIsGiven(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_ = r.g.Seen(ctx, r.rootA)
	r.ssh["jump-alias"] = remote.Resolved{Hostname: "unlisted.example.test", ProxyJump: FixtureHostV4}
	c := r.capability("viaalias", func(c *model.Capability) {
		c.Identity.SSH = &model.CapabilitySSH{Alias: "jump-alias", Hostname: "unlisted.example.test", Port: 22, User: "ci"}
	})
	res, _, err := r.g.CheckAdd(ctx, AddRequest{NodeID: "viaalias", Capability: c})
	if err != nil || !res.Matched || !c.DeployHost.Matched {
		t.Fatalf("%+v %v %+v", res, err, c.DeployHost)
	}
}
