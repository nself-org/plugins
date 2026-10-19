package lifecycle

import (
	"context"
	"debug/elf"
	"flag"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestEveryRegistryPointRuns registers one test hook per extension point and checks each runs at its point.
func TestEveryRegistryPointRuns(t *testing.T) {
	t.Cleanup(resetExtensions)
	ctx := context.Background()
	got := map[string]int{}
	RegisterAddHook(PointAdd, func(_ context.Context, e HookEvent) error {
		if e.Point != PointAdd || e.Project != "proj-1" || e.Flags == nil || e.Flags.Lookup("probe-extra") == nil {
			t.Errorf("add event = %+v", e)
		}
		got["add"]++
		return nil
	})
	RegisterAddHook(PointPair, func(context.Context, HookEvent) error { got["pair"]++; return nil })
	RegisterAddHook(PointTest, func(_ context.Context, e HookEvent) error { got["test"]++; return nil })
	RegisterAgentInstaller("fake", func(ctx context.Context, d *Deps, r InstallRequest) error {
		got["installer"]++
		return localFileInstaller(ctx, d, r)
	})
	RegisterFlags("add", func(fs *flag.FlagSet) { got["flags"]++; fs.String("probe-extra", "", "x") })
	RegisterRotateMode("fake", func(context.Context, *Deps, RotateRequest) error { got["mode"]++; return nil })
	RegisterRevoker(func(context.Context, *store.RevokeTx, string) error { got["revoker"]++; return nil })
	RegisterSessionCloser(func(context.Context, string) error { got["closer"]++; return nil })

	k := newKit(t)
	fs := FlagSet("add")
	if fs.Lookup("probe-extra") == nil {
		t.Fatal("registered flag missing from the add flag set")
	}
	if _, err := k.d.Add(ctx, AddOptions{Dest: "ci@hk1", HostKey: fpA, Installer: "fake", Flags: fs,
		Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.d.Test(ctx, "hk1", fs); err != nil {
		t.Fatal(err)
	}
	if err := k.d.Rotate(ctx, RotateOptions{NodeID: "hk1", Mode: "fake", Flags: fs}); err != nil {
		t.Fatal(err)
	}
	// pair has no caller in this Ticket; its hooks are read through the registry accessor.
	for _, h := range ext.hooksAt(PointPair) {
		_ = h(ctx, HookEvent{Point: PointPair})
	}
	if _, err := k.d.Revoke(ctx, "hk1"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"add", "pair", "test", "installer", "flags", "mode", "revoker", "closer"} {
		if got[name] != 1 {
			t.Errorf("%s ran %d times, want 1", name, got[name])
		}
	}
}

func TestFailingAddHookLeavesNoNodeAndNoPin(t *testing.T) {
	t.Cleanup(resetExtensions)
	RegisterAddHook(PointAdd, func(context.Context, HookEvent) error { return errBoom })
	k := newKit(t)
	_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@hk2", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()})
	if err == nil {
		t.Fatal("hook error swallowed")
	}
	if p, _ := k.d.pins().Lookup(pinAlias("hk2")); len(p) != 0 {
		t.Fatal("pin kept")
	}
	if _, gerr := k.d.Registry.Get(context.Background(), "hk2"); CodeOf(gerr) != CodeNotFound || k.auditCount("") != 0 {
		t.Fatalf("node or audit row kept: %v", gerr)
	}
}

func TestRegistryRejectsBadRegistrations(t *testing.T) {
	t.Cleanup(resetExtensions)
	for name, fn := range map[string]func(){
		"nil hook":      func() { RegisterAddHook(PointAdd, nil) },
		"bad point":     func() { RegisterAddHook("later", func(context.Context, HookEvent) error { return nil }) },
		"dup installer": func() { RegisterAgentInstaller(DefaultInstaller, localFileInstaller) },
		"empty mode":    func() { RegisterRotateMode("", func(context.Context, *Deps, RotateRequest) error { return nil }) },
		"nil revoker":   func() { RegisterRevoker(nil) },
		"nil closer":    func() { RegisterSessionCloser(nil) },
		"nil flags":     func() { RegisterFlags("add", nil) },
		"dup rotate":    func() { RegisterRotateMode(HostKeyMode, rotateHostKey) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			fn()
		}()
	}
}

func TestDrainAndEnableClearMaintenance(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("dr1")
	n, err := k.d.Drain(ctx, "dr1")
	if err != nil || n.Record.State != "draining" {
		t.Fatalf("drain: %v %+v", err, n.Record)
	}
	if k.auditCount("") != 2 {
		t.Fatalf("audit rows after add+drain = %d", k.auditCount(""))
	}
	if _, err = k.d.Registry.SetState(ctx, "dr1", "maintenance", "breaker", "cli:test"); err != nil {
		t.Fatal(err)
	}
	n, err = k.d.Enable(ctx, "dr1")
	if err != nil || n.Record.State != "online" {
		t.Fatalf("enable: %v %+v", err, n.Record)
	}
	if k.auditCount("") != 4 {
		t.Fatalf("every mutation must leave one audit row, have %d", k.auditCount(""))
	}
	if _, err = k.d.Drain(ctx, "ghost"); CodeOf(err) != CodeNotFound {
		t.Fatalf("drain unknown: %v", err)
	}
}

func TestTestReportsDriftAndNeedsAgent(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("ts1")
	res, err := k.d.Test(ctx, "ts1", nil)
	if err != nil || !res.Connected || res.AgentVersion != "1.5.0" || len(res.Drift) != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	k.mutate = func(c *model.Capability) {
		c.Resources.CPU = known(1.0, "probe", time.Now().UTC())
	}
	if res, err = k.d.Test(ctx, "ts1", nil); err != nil || len(res.Drift) == 0 {
		t.Fatalf("drift not reported: %+v %v", res, err)
	}
	k.failRun[agentPath] = errBoom
	if _, err = k.d.Test(ctx, "ts1", nil); CodeOf(err) != CodeNoRoute {
		t.Fatalf("dead agent: %v", err)
	}
}

func TestRotateHostKey(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("rt1")
	before, _ := k.d.pins().Lookup(pinAlias("rt1"))
	fs := FlagSet("rotate")
	_ = fs.Set("host-key", fpB)
	// No key with fpB is on the node: refuse, keep the old pin.
	err := k.d.Rotate(ctx, RotateOptions{NodeID: "rt1", Flags: fs})
	errCode(t, err, CodeHostKey)
	if after, _ := k.d.pins().Lookup(pinAlias("rt1")); len(after) != len(before) || after[0] != before[0] {
		t.Fatalf("pin changed on a mismatch: %v -> %v", before, after)
	}
	// The node now presents key B: rotating to B pins it and records the fingerprint.
	k.keys = append(k.keys, keyB)
	if err = k.d.Rotate(ctx, RotateOptions{NodeID: "rt1", Flags: fs}); err != nil {
		t.Fatal(err)
	}
	after, _ := k.d.pins().Lookup(pinAlias("rt1"))
	if len(after) != 1 || after[0] == before[0] {
		t.Fatalf("pin not replaced: %v", after)
	}
	n, _ := k.d.Registry.Get(ctx, "rt1")
	if fps := n.Capability.Identity.HostKeyFingerprints; len(fps) != 1 || fps[0] != fpB {
		t.Fatalf("fingerprints = %v", fps)
	}
	errCode(t, k.d.Rotate(ctx, RotateOptions{NodeID: "rt1", Flags: FlagSet("rotate")}), CodeInvalid)
	errCode(t, k.d.Rotate(ctx, RotateOptions{NodeID: "rt1", Mode: "nope", Flags: fs}), CodeInvalid)
	if _, err = k.d.Revoke(ctx, "rt1"); err != nil {
		t.Fatal(err)
	}
	errCode(t, k.d.Rotate(ctx, RotateOptions{NodeID: "rt1", Flags: fs}), CodeRevoked)
}
