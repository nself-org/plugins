package contention

import (
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func settings(mut func(*reservation.Settings)) func() reservation.Settings {
	return func() reservation.Settings {
		s := reservation.Defaults("n1", true, true)
		if mut != nil {
			mut(&s)
		}
		return s
	}
}

func laptop(cpus, battery int, plugged bool) func() exec.Capacity {
	return func() exec.Capacity {
		return exec.Capacity{CPUs: cpus, MemoryMB: 16384, FreeMemoryMB: 8192, BatteryPercent: battery, PluggedIn: plugged}
	}
}

func newGate(t *testing.T, dir string, total int, probe func() exec.Capacity, set func() reservation.Settings) *Gate {
	t.Helper()
	tab, err := slots.Open(dir, total)
	if err != nil {
		t.Fatal(err)
	}
	return &Gate{Dir: dir, Table: tab, Probe: probe, Settings: set}
}

func TestReadSignals(t *testing.T) {
	cases := []struct {
		name string
		c    exec.Capacity
		inj  *Signals
		want Signals
	}{
		{"desktop, no battery", exec.Capacity{BatteryPercent: -1}, nil, Signals{Interactive: Unknown}},
		{"laptop on mains", exec.Capacity{BatteryPercent: 80, PluggedIn: true}, nil, Signals{BatteryKnown: true, Interactive: Unknown}},
		{"laptop on battery", exec.Capacity{BatteryPercent: 80}, nil, Signals{BatteryKnown: true, OnBattery: true, Interactive: Unknown}},
		{"terminal attached", exec.Capacity{BatteryPercent: -1, Interactive: true}, nil, Signals{Interactive: Active}},
		{"injected idle wins", exec.Capacity{BatteryPercent: -1, Interactive: true}, &Signals{Interactive: Idle}, Signals{Interactive: Idle}},
		{"injected battery", exec.Capacity{BatteryPercent: -1}, &Signals{BatteryKnown: true, OnBattery: true}, Signals{BatteryKnown: true, OnBattery: true, Interactive: Unknown}},
	}
	for _, c := range cases {
		if got := ReadSignals(c.c, c.inj); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestBatteryPolicies(t *testing.T) {
	for _, tc := range []struct {
		policy string
		plug   bool
		allow  bool
		drain  bool
	}{
		{store.OnBatteryPause, false, false, false},
		{store.OnBatteryDrain, false, false, true},
		{store.OnBatteryAllow, false, true, false},
		{store.OnBatteryPause, true, true, false},
	} {
		g := newGate(t, t.TempDir(), 8, laptop(8, 60, tc.plug), settings(func(s *reservation.Settings) { s.OnBattery = tc.policy }))
		d := g.Admit(exec.Remote, Need{}, false)
		if d.Allowed != tc.allow || d.Drain != tc.drain || (!d.Allowed && d.Reason != model.NodeOnBattery) {
			t.Errorf("%s plugged=%v: %+v", tc.policy, tc.plug, d)
		}
	}
}

func TestForegroundIsNeverGated(t *testing.T) {
	// On battery, everything reserved, interactive: the operator's own run still goes (X17).
	g := newGate(t, t.TempDir(), 1, laptop(2, 10, false), settings(func(s *reservation.Settings) { s.ReservedCPU = 2 }))
	g.Signal = func() *Signals { return &Signals{Interactive: Active} }
	d := g.Admit(exec.Foreground, Need{}, false)
	if !d.Allowed || !d.Floor {
		t.Fatalf("foreground refused: %+v", d)
	}
	if l, _, err := g.Acquire("a1", slots.Self("c", "l1"), exec.Foreground, Need{}, false); l != nil || err != nil {
		t.Fatalf("foreground took a slot: %v %v", l, err)
	}
	if g.Admit(exec.Remote, Need{}, false).Allowed {
		t.Fatal("a remote lease passed the same conditions")
	}
}

func TestInteractiveProtect(t *testing.T) {
	active := func() *Signals { return &Signals{Interactive: Active} }
	cases := []struct {
		name       string
		policy     string
		sig        func() *Signals
		mode       exec.Mode
		background bool
		allow      bool
	}{
		{"protect, active, background", store.InteractiveProtect, active, exec.Remote, true, false},
		{"protect, active, background mode", store.InteractiveProtect, active, exec.Background, false, false},
		{"protect, active, normal remote job", store.InteractiveProtect, active, exec.Remote, false, true},
		{"protect, idle", store.InteractiveProtect, func() *Signals { return &Signals{Interactive: Idle} }, exec.Remote, true, true},
		{"protect, unknown", store.InteractiveProtect, nil, exec.Remote, true, true},
		{"ignore, active", store.InteractiveIgnore, active, exec.Remote, true, true},
	}
	for _, c := range cases {
		g := newGate(t, t.TempDir(), 8, laptop(8, -1, false), settings(func(s *reservation.Settings) { s.Interactive = c.policy }))
		g.Signal = c.sig
		d := g.Admit(c.mode, Need{}, c.background)
		if d.Allowed != c.allow || (!d.Allowed && d.Reason != model.NodeInteractive) {
			t.Errorf("%s: %+v", c.name, d)
		}
	}
}

// Two coordinators (a PERSONAL store and a TEAM one) each open the node's one slot table: together
// they never exceed the reservation, however the requests interleave.
func TestTwoCoordinatorsShareOneReservation(t *testing.T) {
	dir := t.TempDir()
	set := settings(func(s *reservation.Settings) { s.ReservedCPU = 4 })
	personal := newGate(t, dir, 8, laptop(8, -1, false), set)
	team := newGate(t, dir, 8, laptop(8, -1, false), set)
	held := 0
	for i := 0; i < 8; i++ {
		g, name := personal, "personal"
		if i%2 == 1 {
			g, name = team, "team"
		}
		l, d, err := g.Acquire(name+string(rune('a'+i)), slots.Self(name, "l"+string(rune('a'+i))), exec.Remote, Need{}, false)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if l != nil {
			held++
		} else if d.Reason != model.NodeReservation {
			t.Fatalf("request %d refused for %q", i, d.Reason)
		}
	}
	if held != 4 {
		t.Fatalf("held %d leases, the reservation leaves 4", held)
	}
	if used, _ := personal.Table.Used(); used != 4 {
		t.Fatalf("table holds %d", used)
	}
}

func TestSlotCapStillApplies(t *testing.T) {
	g := newGate(t, t.TempDir(), 2, laptop(8, -1, false), settings(nil))
	for i := 0; i < 2; i++ {
		if l, _, err := g.Acquire("a"+string(rune('0'+i)), slots.Self("c", "l"), exec.Remote, Need{}, false); l == nil || err != nil {
			t.Fatalf("lease %d: %v", i, err)
		}
	}
	_, _, err := g.Acquire("a9", slots.Self("c", "l"), exec.Remote, Need{}, false)
	var se *slots.Error
	if !errors.As(err, &se) {
		t.Fatalf("third lease: %v, want the slots E654", err)
	}
}

func TestOneSlotPerAttemptAndWake(t *testing.T) {
	set := settings(nil) // laptop defaults: intermittent
	g := newGate(t, t.TempDir(), 4, laptop(4, 90, true), set)
	acquire := func(attempt, lease string) *Lease {
		t.Helper()
		l, _, err := g.Acquire(attempt, slots.Self("c", lease), exec.Remote, Need{}, false)
		if l == nil || err != nil {
			t.Fatalf("acquire %s: %v", attempt, err)
		}
		return l
	}
	acquire("att-1", "lease-1")
	acquire("att-1", "lease-2") // the coordinator re-leased the same attempt
	if used, _ := g.Table.Used(); used != 1 || g.Held() != 1 {
		t.Fatalf("one attempt holds %d slots", used)
	}
	acquire("att-2", "lease-3")
	got := g.OnWake()
	if len(got) != 2 {
		t.Fatalf("OnWake fenced %v", got)
	}
	if used, _ := g.Table.Used(); used != 0 {
		t.Fatalf("slots after wake: %d", used)
	}
	acquire("att-1", "lease-4")
	if used, _ := g.Table.Used(); used != 1 {
		t.Fatalf("slots after re-lease: %d", used)
	}
	persistent := newGate(t, t.TempDir(), 4, laptop(4, 90, true), settings(func(s *reservation.Settings) { s.Persistence = store.PersistencePersist }))
	if l, _, _ := persistent.Acquire("p1", slots.Self("c", "l"), exec.Remote, Need{}, false); l == nil || len(persistent.OnWake()) != 0 {
		t.Fatal("a persistent node dropped a lease on wake")
	}
}

func TestSleepWatcher(t *testing.T) {
	wall := time.Unix(1000, 0)
	var mono time.Duration
	w := &SleepWatcher{Wall: func() time.Time { return wall }, Mono: func() time.Duration { return mono }}
	if w.Tick() {
		t.Fatal("first tick reported a sleep")
	}
	wall, mono = wall.Add(5*time.Second), mono+5*time.Second
	if w.Tick() {
		t.Fatal("ordinary time reported as a sleep")
	}
	wall, mono = wall.Add(2*time.Hour), mono+3*time.Second // the clock ran on while we did not
	if !w.Tick() {
		t.Fatal("a two hour wall gap was not seen as a sleep")
	}
	if w.Tick() {
		t.Fatal("one sleep reported twice")
	}
}
