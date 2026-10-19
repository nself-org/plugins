package lifecycle

import (
	"context"
	"debug/elf"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
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
		return k.st.NodeSetDeployHost(ctx, e.NodeID, 0, model.CapabilityDeployHost{Matched: true, Source: &src}, "cli:test", "matched host")
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

// D-1219: the pre-add hook runs on the destination alone and a refusal there happens before any contact:
// no host-key scan, no ssh command, no copy, no pin, no node.
func TestPreAddHookRunsBeforeAnyContact(t *testing.T) {
	for _, tc := range []struct {
		name string
		hook error
	}{{"refused", errBoom}, {"passes", nil}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(resetExtensions)
			k := newKit(t)
			scans := 0
			scan := k.d.Scan
			k.d.Scan = func(ctx context.Context, tg remote.Target, id string) ([]remote.HostKey, error) {
				scans++
				return scan(ctx, tg, id)
			}
			var ev HookEvent
			RegisterAddHook(PointPreAdd, func(_ context.Context, e HookEvent) error {
				ev = e
				if scans != 0 || len(k.runs) != 0 {
					t.Errorf("pre-add ran after contact: %d scans, %d runs", scans, len(k.runs))
				}
				return tc.hook
			})
			_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@pre1", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()})
			if ev.Point != PointPreAdd || ev.Dest != "ci@pre1" || ev.NodeID == "" || ev.Capability != nil {
				t.Fatalf("event %+v", ev)
			}
			if tc.hook == nil {
				if err != nil || scans == 0 {
					t.Fatalf("err %v scans %d", err, scans)
				}
				return
			}
			if err == nil || scans != 0 || len(k.runs) != 0 || len(k.copies) != 0 {
				t.Fatalf("a refused pre-add still made contact: err %v scans %d runs %v copies %v", err, scans, k.runs, k.copies)
			}
			if p, _ := k.d.pins().Lookup(pinAlias("pre1")); len(p) != 0 {
				t.Fatal("a refused pre-add pinned a key")
			}
		})
	}
}

// An unknown installer fails before the pre-add hook: the operator is not asked to confirm an override for an
// add that cannot proceed.
func TestPreAddHookDoesNotRunForAnUnknownInstaller(t *testing.T) {
	t.Cleanup(resetExtensions)
	k := newKit(t)
	ran := false
	RegisterAddHook(PointPreAdd, func(context.Context, HookEvent) error { ran = true; return nil })
	_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@pre2", HostKey: fpA, Installer: "typo", Trust: DefaultTrust()})
	if err == nil || ran {
		t.Fatalf("err %v, pre-add ran %v", err, ran)
	}
}
