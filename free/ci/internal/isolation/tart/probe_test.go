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
	r.p.slots.release()
	if !r.p.slots.reserve(0, 0) {
		t.Fatal("a released slot is free again")
	}
	r.p.slots.release()
	r.p.slots.release()
	one, two := r.p.slots.reserve(1, 0), r.p.slots.reserve(1, 0)
	if !one || two {
		t.Fatal("one foreign VM running leaves one slot")
	}
	r.p.slots.release()
	// a clone that is booting and not listed yet is still busy; one that is listed is not counted twice
	if r.p.slots.Free(0, 1) != 1 || !r.p.slots.reserve(0, 1) || r.p.slots.Free(0, 1) != 1 || !r.p.slots.reserve(0, 1) || r.p.slots.reserve(0, 1) {
		t.Fatal("own listed clone and own reservation are the same slot")
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
