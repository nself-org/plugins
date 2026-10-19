package reservation

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestDefaults(t *testing.T) {
	cases := []struct {
		operator, battery        bool
		interactive, persistence string
	}{
		{true, true, "protect", "intermittent"},
		{true, false, "protect", "persistent"},
		{false, false, "ignore", "persistent"},
		{false, true, "ignore", "intermittent"},
	}
	for _, c := range cases {
		s := Defaults("n", c.operator, c.battery)
		if s.Interactive != c.interactive || s.Persistence != c.persistence || s.OnBattery != store.OnBatteryPause || s.ReservedCPU != 0 || s.Validate() != nil {
			t.Errorf("Defaults(%v,%v) = %+v", c.operator, c.battery, s)
		}
	}
}

func TestComputeShare(t *testing.T) {
	host := exec.Capacity{CPUs: 8, MemoryMB: 16384, BatteryPercent: -1}
	cases := []struct {
		name  string
		c     exec.Capacity
		s     Settings
		need  Need
		slots int
		cpu   float64
		act   bool
	}{
		{"none reserved", host, Settings{}, Need{}, 8, 8, false},
		{"half reserved", host, Settings{ReservedCPU: 4}, Need{}, 4, 4, true},
		{"cpu hungry jobs", host, Settings{ReservedCPU: 4}, Need{CPU: 3}, 1, 4, true},
		{"memory bound", host, Settings{ReservedMemMB: 12288}, Need{MemMB: 2048}, 2, 8, true},
		{"all reserved", host, Settings{ReservedCPU: 8}, Need{}, 0, 0, true},
		{"over reserved", host, Settings{ReservedCPU: 12}, Need{}, 0, 0, true},
		{"CI switches reservations off", exec.Capacity{CPUs: 8, MemoryMB: 16384, CI: true}, Settings{ReservedCPU: 6}, Need{}, 8, 8, false},
	}
	for _, c := range cases {
		got := Compute(c.c, c.s, c.need)
		if got.Slots != c.slots || got.CPU != c.cpu || got.Active != c.act {
			t.Errorf("%s: %+v, want slots %d cpu %v active %v", c.name, got, c.slots, c.cpu, c.act)
		}
		if (got.Slots == 0) != (got.Reason == model.NodeReservation) {
			t.Errorf("%s: reason %q with %d slots", c.name, got.Reason, got.Slots)
		}
	}
}

func TestParse(t *testing.T) {
	if v, err := ParseCPU("50%", 8); err != nil || v != 4 {
		t.Errorf("50%% of 8 = %v %v", v, err)
	}
	if v, err := ParseCPU("0.5", 0); err != nil || v != 0.5 {
		t.Errorf("0.5 = %v %v", v, err)
	}
	for _, bad := range []string{"", "x", "-1", "101%", "NaN", "Inf"} {
		if _, err := ParseCPU(bad, 8); err == nil {
			t.Errorf("ParseCPU(%q) accepted", bad)
		}
	}
	if _, err := ParseCPU("50%", 0); err == nil {
		t.Error("a percentage of an unknown core count was accepted")
	}
	for in, want := range map[string]int64{"2048": 2048, "2g": 2048, "512MB": 512, "1GB": 1024, "0": 0} {
		if v, err := ParseMemMB(in); err != nil || v != want {
			t.Errorf("ParseMemMB(%q) = %v %v", in, v, err)
		}
	}
	for _, bad := range []string{"", "-1", "abc", "1.5g"} {
		if _, err := ParseMemMB(bad); err == nil {
			t.Errorf("ParseMemMB(%q) accepted", bad)
		}
	}
}

func TestMerge(t *testing.T) {
	cores := 8.0
	cur := Defaults("n1", true, true)
	got, err := Merge(cur, Flags{CPU: "50%", OnBattery: "allow", Given: map[string]bool{"reserve-cpu": true, "on-battery": true}}, &cores)
	if err != nil || got.ReservedCPU != 4 || got.OnBattery != "allow" || got.Interactive != "protect" || got.Persistence != "intermittent" {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err = Merge(got, Flags{Mem: "2g", Given: map[string]bool{"reserve-mem": true}}, nil); err != nil || got.ReservedMemMB != 2048 || got.ReservedCPU != 4 {
		t.Fatalf("a flag not given changed a value: %+v %v", got, err)
	}
	for name, f := range map[string]Flags{
		"percent of unknown cores": {CPU: "50%", Given: map[string]bool{"reserve-cpu": true}},
		"bad interactive":          {Interactive: "maybe", Given: map[string]bool{"interactive": true}},
		"bad battery":              {OnBattery: "", Given: map[string]bool{"on-battery": true}},
		"bad persistence":          {Persistence: "x", Given: map[string]bool{"persistence": true}},
		"bad memory":               {Mem: "lots", Given: map[string]bool{"reserve-mem": true}},
	} {
		if _, err := Merge(cur, f, nil); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
