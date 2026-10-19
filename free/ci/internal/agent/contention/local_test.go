package contention

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

func TestPolicyFile(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := LoadPolicy(dir); ok || err != nil {
		t.Fatalf("empty dir: %v %v", ok, err)
	}
	want := reservation.Defaults("n1", true, true)
	want.ReservedCPU = 2
	if err := SavePolicy(dir, want); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := LoadPolicy(dir); !ok || err != nil || got != want {
		t.Fatalf("round trip: %+v %v %v", got, ok, err)
	}
	if st, _ := os.Stat(filepath.Join(dir, policyFile)); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	if err := SavePolicy(dir, reservation.Settings{NodeID: "n1"}); err == nil {
		t.Fatal("an invalid policy was saved")
	}
	if err := os.WriteFile(filepath.Join(dir, policyFile), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadPolicy(dir); err == nil {
		t.Fatal("a corrupt policy loaded")
	}
	if got := SettingsFrom(dir, "n1", true, true)(); got.ReservedCPU < 1e6 {
		t.Fatalf("a corrupt file opened the node: %+v", got)
	}
	if got := SettingsFrom(t.TempDir(), "n1", true, true)(); got.Interactive != "protect" || got.ReservedCPU != 0 {
		t.Fatalf("defaults: %+v", got)
	}
}

func TestApplyFactsStaysValid(t *testing.T) {
	raw, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = c.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	for _, tc := range []struct {
		name    string
		cap     exec.Capacity
		inj     *Signals
		pct     bool
		plugged *bool
		inter   *string
	}{
		{"battery, unplugged, active", exec.Capacity{BatteryPercent: 40}, &Signals{Interactive: Active}, true, ptr(false), ptr("active")},
		{"daemon with /dev/null stdin is not active", exec.Capacity{BatteryPercent: 40, Interactive: true}, nil, true, ptr(false), nil},
		{"desktop, nothing known", exec.Capacity{BatteryPercent: -1}, nil, false, nil, nil},
		{"mains, injected idle", exec.Capacity{BatteryPercent: 99, PluggedIn: true}, &Signals{Interactive: Idle}, true, ptr(true), ptr("idle")},
	} {
		sig := ReadSignals(tc.cap, tc.inj)
		ApplyFacts(&c, tc.cap, sig, 3, 1, now)
		a := c.Availability
		if (a.BatteryPct.Value != nil) != tc.pct || !eq(a.PluggedIn.Value, tc.plugged) || !eq(a.Interactive.Value, tc.inter) || *a.Slots.Value != 3 || *a.Running.Value != 1 {
			t.Errorf("%s: %+v", tc.name, a)
		}
		for _, ok := range []bool{a.BatteryPct.Valid(), a.PluggedIn.Valid(), a.Interactive.Valid(), a.Slots.Valid(), a.Running.Valid()} {
			if !ok {
				t.Errorf("%s: an invalid fact: %+v", tc.name, a)
			}
		}
		if err := model.ValidateCapability(c); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func eq[T comparable](a, b *T) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
