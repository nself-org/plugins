package tart

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

func TestProbeFacts(t *testing.T) {
	r := newRig(t, nil)
	av := r.p.Probe(context.Background())
	if !av.Available() || av.Level != "vm" {
		t.Fatalf("%+v", av)
	}
	f := av.Facts
	if f["tart_version"] != "2.99.0-fake" || f["free_slots"] != "2" || f["softnet_block"] != "true" || f["base_image_cached"] != "false" || f["licence_note"] != LicenceNote {
		t.Fatalf("facts %v", f)
	}
	if !strings.Contains(f["licence_note"], "neither bundles nor redistributes") {
		t.Fatal("licence note")
	}
}

func TestProbeUnavailable(t *testing.T) {
	cases := []struct {
		name   string
		mod    func(*Options)
		env    map[string]string
		reason string
	}{
		{"linux", func(o *Options) { o.GOOS = "linux" }, nil, isolation.ReasonRuntimeMissing},
		{"intel mac", func(o *Options) { o.GOARCH = "amd64" }, nil, isolation.ReasonRuntimeMissing},
		{"no tart", func(o *Options) { o.Tart = "/nonexistent/tart" }, nil, isolation.ReasonRuntimeMissing},
		{"no digest", func(o *Options) { o.BaseImage = func() string { return "ghcr.io/x/y:latest" } }, nil, ReasonBaseImage},
		{"no image", func(o *Options) { o.BaseImage = nil }, nil, ReasonBaseImage},
		{"two VMs running", nil, map[string]string{"FAKE_TART_EXTRA_RUNNING": "2"}, ReasonSlots},
		{"list broken", nil, map[string]string{"FAKE_TART_FAIL": "list"}, isolation.ReasonProbeFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, tc.mod)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			av := r.p.Probe(context.Background())
			if av.Available() || av.Reason != tc.reason {
				t.Fatalf("%+v", av)
			}
		})
	}
}

func TestSelectThroughTheRegistry(t *testing.T) {
	r := newRig(t, nil)
	reg := &isolation.Registry{}
	reg.Register(r.p)
	p, av, err := reg.Select(context.Background(), "vm")
	if err != nil || p != r.p || av.Facts["free_slots"] != "2" {
		t.Fatalf("%v %+v", err, av)
	}
	r2 := newRig(t, func(o *Options) { o.Tart = "/nonexistent/tart" })
	reg2 := &isolation.Registry{}
	reg2.Register(r2.p)
	_, _, err = reg2.Select(context.Background(), "vm")
	if e, ok := err.(*isolation.Error); !ok || e.Code != "E678" {
		t.Fatalf("want E678, got %v", err)
	}
}

func TestSlotLedgerNeverOversubscribes(t *testing.T) {
	r := newRig(t, nil)
	first, second, third := r.p.slots.reserve(0, 0), r.p.slots.reserve(0, 0), r.p.slots.reserve(0, 0)
	if !first || !second || third {
		t.Fatal("two reservations fit, a third must not")
	}
	r.p.slots.release(false)
	if !r.p.slots.reserve(0, 0) {
		t.Fatal("a released slot is free again")
	}
	r.p.slots.release(false)
	r.p.slots.release(false)
	one, two := r.p.slots.reserve(1, 0), r.p.slots.reserve(1, 0)
	if !one || two {
		t.Fatal("one foreign VM running leaves one slot")
	}
	r.p.slots.release(false)
}

func TestSlotLedgerCountsOtherProcessesClones(t *testing.T) {
	s := NewSlots()
	// another coordinator process runs one clone (listed: own=1); this process reserves one and is still booting
	if !s.reserve(0, 1) {
		t.Fatal("one slot is free")
	}
	if s.reserve(0, 1) {
		t.Fatal("2 slots: the other process's clone plus our booting reservation are both busy")
	}
	// ours boots and shows in the listing: own=2, booted=1 -> still two busy, none free
	s.markBooted()
	if s.Free(0, 2) != 0 {
		t.Fatal("two VMs are running")
	}
	s.release(true)
	if s.Free(0, 1) != 1 {
		t.Fatal("our VM is gone and the other process's clone remains")
	}
}

func TestVMAndEphemeralProfilesShareOneLedger(t *testing.T) {
	slots := NewSlots()
	a := New("vm", Options{Slots: slots})
	b := New("ephemeral-vm", Options{Slots: slots})
	if !a.slots.reserve(0, 0) || !b.slots.reserve(0, 0) || a.slots.reserve(0, 0) || b.slots.reserve(0, 0) {
		t.Fatal("the two profiles must draw on two slots in total")
	}
}

func TestSoftnetBlockOptionProbe(t *testing.T) {
	if !softnetBlockSupported([]byte("--net-softnet-allow x\n--net-softnet-block y")) || softnetBlockSupported([]byte("--net-softnet")) || softnetBlockSupported([]byte("--net-softnet-block y")) {
		t.Fatal("probe text")
	}
}

func TestWithoutBlockOptionScopesBelowLanAreUnenforceable(t *testing.T) {
	r := newRig(t, func(o *Options) {
		o.Net = NetSource{Facts: func(context.Context) (netFacts, error) { return netFacts{}, nil }}
	})
	t.Setenv("FAKE_TART_NOBLOCK", "1")
	for _, scope := range []string{"none", "restricted", "internet"} {
		s := r.spec("u-"+scope, "true")
		s.Network = modelScope(scope)
		res, _ := r.run(context.Background(), s)
		if res.Err == nil || !strings.Contains(res.Err.Error(), "E679") || !strings.Contains(res.Err.Error(), "network.scope_unenforceable") {
			t.Fatalf("%s: %+v", scope, res)
		}
	}
	s := r.spec("u-lan", "true") // lan needs no softnet
	if res, _ := r.run(context.Background(), s); res.Err != nil {
		t.Fatalf("lan: %v", res.Err)
	}
	r.noClones()
}

// D-1381: a burst grant holds its slot in the shared ledger, so a regular vm job booting in the same process
// is refused at its own Wrap and the granted job's Wrap succeeds instead of failing for lack of a slot.
func TestHeldSlotIsKeptForItsOwnWrap(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, nil)
	a, b := isolation.Attempt{ID: "burst-A", CoordinatorID: "coord-test"}, isolation.Attempt{ID: "burst-B", CoordinatorID: "coord-test"}
	if err := r.p.Hold(ctx, a); err != nil {
		t.Fatalf("hold A: %v", err)
	}
	if err := r.p.Hold(ctx, a); err != nil {
		t.Fatalf("a second hold of the same attempt is idempotent: %v", err)
	}
	if err := r.p.Hold(ctx, b); err != nil {
		t.Fatalf("hold B: %v", err)
	}
	if err := r.p.Hold(ctx, isolation.Attempt{ID: "burst-C"}); err == nil || err.(*isolation.Error).Reason != ReasonSlots {
		t.Fatalf("a third hold must be refused with %s, got %v", ReasonSlots, err)
	}
	if _, err := r.p.Wrap(r.spec("regular-1", "true")); err == nil {
		t.Fatal("a regular job must not boot while both slots are held")
	}
	sa, err := r.p.Wrap(r.spec("burst-A", "true"))
	if err != nil {
		t.Fatalf("the held attempt must find its slot: %v", err)
	}
	sa.Release(ctx)
	if err := r.p.Hold(ctx, isolation.Attempt{ID: "burst-C"}); err != nil {
		t.Fatalf("the slot of a released attempt is free again: %v", err)
	}
}

func TestTeardownGivesBackAnUnusedHold(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, nil)
	for _, id := range []string{"burst-A", "burst-B"} {
		if err := r.p.Hold(ctx, isolation.Attempt{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.p.Teardown(ctx, isolation.Attempt{ID: "burst-A", CoordinatorID: "coord-test"}); err != nil {
		t.Fatal(err)
	}
	if err := r.p.Hold(ctx, isolation.Attempt{ID: "burst-C"}); err != nil {
		t.Fatalf("a torn-down grant must free its hold: %v", err)
	}
}
