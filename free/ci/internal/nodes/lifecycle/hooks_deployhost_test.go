package lifecycle

import (
	"context"
	"debug/elf"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// P7-NODE-14: an add hook sees the destination and the draft capability and may set deploy_host on it;
// the node is registered carrying that fact; a test hook's change to the stored node is what Test reports.
func TestAddHookSeesDestAndMarksDeployHostOnTheRegisteredNode(t *testing.T) {
	t.Cleanup(resetExtensions)
	var dest string
	RegisterAddHook(PointAdd, func(_ context.Context, e HookEvent) error {
		dest = e.Dest
		if e.Capability == nil || e.Capability.Identity.ID != e.NodeID {
			t.Errorf("draft capability = %+v", e.Capability)
			return nil
		}
		src := "inventory"
		e.Capability.DeployHost = model.CapabilityDeployHost{Matched: true, Source: &src}
		return nil
	})
	k := newKit(t)
	r, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@hk3", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()})
	if err != nil {
		t.Fatal(err)
	}
	dh := r.Node.Capability.DeployHost
	if dest != "ci@hk3" || !dh.Matched || dh.Source == nil || *dh.Source != "inventory" || !strings.Contains(string(r.Node.Record.DeployHost), `"matched":true`) {
		t.Fatalf("dest %q deploy_host %+v column %s", dest, dh, r.Node.Record.DeployHost)
	}
}

func TestTestReportsTheNodeAsAHookLeftIt(t *testing.T) {
	t.Cleanup(resetExtensions)
	k := newKit(t)
	k.addNode("hk4")
	src := "never-list"
	RegisterAddHook(PointTest, func(ctx context.Context, e HookEvent) error {
		return k.st.NodeSetDeployHost(ctx, e.NodeID, model.CapabilityDeployHost{Matched: true, Source: &src}, "cli:test", "matched host")
	})
	res, err := k.d.Test(context.Background(), "hk4", FlagSet("test"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Node.Capability.DeployHost.Matched {
		t.Fatalf("Test reported a stale node: %+v", res.Node.Capability.DeployHost)
	}
	RegisterAddHook(PointTest, func(context.Context, HookEvent) error { return errBoom })
	if _, err = k.d.Test(context.Background(), "hk4", FlagSet("test")); err == nil {
		t.Fatal("test hook error swallowed")
	}
}
