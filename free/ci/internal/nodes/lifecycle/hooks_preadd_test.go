package lifecycle

import (
	"context"
	"debug/elf"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// D-1219: PointPreAdd runs after the destination is parsed and before anything touches the host (host-key
// scan, ssh, install), so a refusal there leaves no contact, no pin and no node.
func TestPreAddRefusalContactsNoHost(t *testing.T) {
	t.Cleanup(resetExtensions)
	var got HookEvent
	RegisterAddHook(PointPreAdd, func(_ context.Context, e HookEvent) error { got = e; return errBoom })
	k := newKit(t)
	scans := 0
	k.d.Scan = func(context.Context, remote.Target, string) ([]remote.HostKey, error) { scans++; return k.keys, nil }
	_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@pre1", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust(), Flags: FlagSet("add")})
	if err != errBoom {
		t.Fatalf("pre-add hook error not returned as it was: %v", err)
	}
	if got.Point != PointPreAdd || got.NodeID != "pre1" || got.Dest != "ci@pre1" || got.Project != "proj-1" || got.Capability != nil || got.Flags == nil {
		t.Fatalf("event %+v", got)
	}
	if scans != 0 || len(k.runs) != 0 || len(k.copies) != 0 {
		t.Fatalf("host contacted: %d scans, %d ssh runs, %d copies", scans, len(k.runs), len(k.copies))
	}
	if p, _ := k.d.pins().Lookup(pinAlias("pre1")); len(p) != 0 {
		t.Fatal("pin kept")
	}
	if _, gerr := k.d.Registry.Get(context.Background(), "pre1"); CodeOf(gerr) != CodeNotFound || k.auditCount("") != 0 {
		t.Fatalf("node or audit row kept: %v", gerr)
	}
}

// A passing pre-add hook runs once and before the add hook; the add then completes as before.
func TestPreAddRunsBeforeAddHook(t *testing.T) {
	t.Cleanup(resetExtensions)
	var order []Point
	RegisterAddHook(PointAdd, func(_ context.Context, e HookEvent) error { order = append(order, e.Point); return nil })
	RegisterAddHook(PointPreAdd, func(_ context.Context, e HookEvent) error { order = append(order, e.Point); return nil })
	k := newKit(t)
	k.addNode("pre2")
	if len(order) != 2 || order[0] != PointPreAdd || order[1] != PointAdd {
		t.Fatalf("hook order %v", order)
	}
}
