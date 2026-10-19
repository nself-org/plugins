//go:build integration

package contention

// Scenario 11 (laptop contention): one subtest per acceptance line of P7-NODE-13. The CPU measurement
// reads the cgroup's own cpu.stat; where the host does not delegate a cgroup the same subtest asserts
// the honest report (priority applied, cpu cap "unknown" with a reason) instead of a measurement.

import (
	"context"
	"encoding/json"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

type passIsolator struct{}

func (passIsolator) Prepare(context.Context, *exec.JobSpec) error { return nil }

func TestScenario11(t *testing.T) {
	t.Run("reserved_cpu_holds_while_the_operator_is_active", scenarioReservedCPU)
	t.Run("on_battery_node_takes_no_new_lease_and_foreground_still_runs", scenarioBattery)
	t.Run("personal_store_and_team_coordinator_never_exceed_the_reservation", scenarioTwoCoordinators)
	t.Run("laptop_defaults_to_intermittent_and_never_double_leases_across_sleep", scenarioSleep)
}

// buildCLI builds the real binary so `nodes set` is exercised end to end.
func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "ci")
	cmd := osexec.Command("go", "build", "-mod=vendor", "-o", bin, "../../../cmd")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func seedNode(t *testing.T, home string) store.NodeRecord {
	t.Helper()
	raw, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = "laptop-1", "laptop-1"
	raw, _ = json.Marshal(c)
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	n := store.NodeRecord{ID: "laptop-1", Name: "laptop-1", State: "offline", Capability: raw, Lifecycle: []byte(`{"discovered":true,"authorized_projects":["project"],"eligible":true,"revoked_at":null}`), Trust: []byte(`{}`), DeployHost: []byte(`{}`)}
	if err = st.NodePut(context.Background(), n, "operator"); err != nil {
		t.Fatal(err)
	}
	return n
}

func scenarioReservedCPU(t *testing.T) {
	home := t.TempDir()
	seedNode(t, home)
	cli := buildCLI(t)
	cmd := osexec.Command(cli, "nodes", "set", "laptop-1", "--reserve-cpu", "50%", "--interactive", "protect", "--json")
	cmd.Env = append(os.Environ(), "NSELF_CI_HOME="+home, "NSELF_V15=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("nodes set: %v\n%s", err, out)
	}
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	node, err := registry.New(st).Get(context.Background(), "laptop-1")
	if err != nil {
		t.Fatal(err)
	}
	cpu := node.Capability.Resources.Reserved.CPU
	if cpu.Value == nil || *cpu.Value != 4 || cpu.Source != "assigned" {
		t.Fatalf("`nodes set --reserve-cpu 50%%` left reserved cpu %+v on an 8 core node", cpu)
	}

	// The same policy on this host: half of its cores, the operator active (injected signal).
	cpus := runtime.NumCPU()
	stateDir := t.TempDir()
	set := reservation.Defaults("laptop-1", true, true)
	set.ReservedCPU = float64(cpus) / 2
	if err = SavePolicy(stateDir, set); err != nil {
		t.Fatal(err)
	}
	table, err := slots.Open(stateDir, cpus)
	if err != nil {
		t.Fatal(err)
	}
	probe := func() exec.Capacity {
		return exec.Capacity{CPUs: cpus, MemoryMB: 8192, FreeMemoryMB: 4096, BatteryPercent: -1}
	}
	gate := &Gate{Dir: stateDir, Table: table, Probe: probe, Settings: SettingsFrom(stateDir, "laptop-1", true, true), Signal: func() *Signals { return &Signals{Interactive: Active} }}
	if d := gate.Admit(exec.Remote, Need{}, true); d.Allowed || d.Reason != model.NodeInteractive {
		t.Fatalf("a background job was admitted while the operator is active: %+v", d)
	}
	d := gate.Admit(exec.Remote, Need{}, false)
	if want := int(set.ReservedCPU); (d.Share.Slots == 0) == d.Allowed || d.Share.CPU != float64(cpus)-set.ReservedCPU || (cpus >= 2 && d.Share.Slots != cpus-want) {
		t.Fatalf("reservation share: %+v", d)
	}

	// The scheduler reads the facts this layer reports: the background job waits with node.interactive.
	c := node.Capability
	ApplyFacts(&c, probe(), ReadSignals(probe(), &Signals{Interactive: Active}), d.Share.Slots, 0, time.Now().UTC())
	w := sched.World{Runners: []sched.Runner{{ID: "laptop-1", Capability: c, Location: "lan", Class: "owned", Provider: "operator", Avail: sched.Avail{Slots: 2, CPU: 4, MemMB: 4096},
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", NoLANRoute: true}}},
		Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	job := sched.Job{Key: "bg", ID: "bg", Project: "project", Pipeline: sched.Pipeline{ID: "p", Support: []string{"linux/amd64"}, Band: "background"},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true}
	res := sched.Place(w, []sched.Job{job})
	if len(res.Placements) != 0 || len(res.Explanations) != 1 || len(res.Explanations[0].Alternatives) == 0 {
		t.Fatalf("background job placed on a node in use: %+v", res)
	}
	seen := false
	for _, r := range res.Explanations[0].Alternatives[0].Reasons {
		seen = seen || r.Code == string(model.NodeInteractive)
	}
	if !seen {
		t.Fatalf("ci explain lacks node.interactive: %+v", res.Explanations[0].Alternatives[0].Reasons)
	}

	// A CPU hog runs under the isolator; its measured CPU must stay inside the remaining share.
	pl := HostPlatform()
	pl.Sweep()
	var prio Priority
	iso := &Isolator{Inner: passIsolator{}, Share: func() reservation.Share { return d.Share }, Platform: &pl, Report: func(_ string, p Priority) { prio = p }}
	before, hadBefore := pl.CgroupUsageUsec()
	script := `i=0; while [ $i -lt ` + itoa(cpus) + ` ]; do (while :; do :; done) & i=$((i+1)); done; sleep 3; kill $(jobs -p) 2>/dev/null; wait`
	r := (&exec.Executor{Isolation: iso}).Run(context.Background(), exec.JobSpec{AttemptID: "hog-1", CoordinatorID: "c1", Home: t.TempDir(), Command: []string{"sh", "-c", script}})
	if r.Err != nil {
		t.Fatalf("hog: %v", r.Err)
	}
	defer pl.ReleaseGroup("hog-1")
	if prio.Nice != Applied && runtime.GOOS != "windows" {
		t.Fatalf("background priority not applied: %+v", prio)
	}
	if prio.CPU != Applied {
		if prio.CPU != NoInfo || prio.Reason == "" {
			t.Fatalf("cpu cap neither applied nor honestly unknown: %+v", prio)
		}
		t.Logf("cgroup not delegated here, reported unknown: %s", prio.Reason)
		return
	}
	after, ok := pl.CgroupUsageUsec()
	if !ok || !hadBefore && before != 0 {
		t.Fatal("no cpu.stat for the job group")
	}
	used := time.Duration(after-before) * time.Microsecond
	allowed := time.Duration(float64(r.Finished.Sub(r.Started)) * d.Share.CPU)
	if used > allowed+allowed/5+200*time.Millisecond {
		t.Fatalf("hog used %v of CPU, the remaining share allows %v", used, allowed)
	}
	t.Logf("hog used %v of CPU in %v, remaining share %.1f cores", used, r.Finished.Sub(r.Started), d.Share.CPU)
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func scenarioBattery(t *testing.T) {
	var mu sync.Mutex
	plugged := true
	probe := func() exec.Capacity {
		mu.Lock()
		defer mu.Unlock()
		return exec.Capacity{CPUs: 4, MemoryMB: 8192, FreeMemoryMB: 4096, BatteryPercent: 70, PluggedIn: plugged}
	}
	dir := t.TempDir()
	table, err := slots.Open(dir, 4)
	if err != nil {
		t.Fatal(err)
	}
	gate := &Gate{Dir: dir, Table: table, Probe: probe, Settings: SettingsFrom(dir, "laptop-1", true, true)}
	lease, d, err := gate.Acquire("running-1", slots.Self("c1", "l1"), exec.Remote, Need{}, false)
	if lease == nil || err != nil {
		t.Fatalf("plugged in: %+v %v", d, err)
	}
	done := make(chan exec.AttemptResult, 1)
	go func() {
		done <- (&exec.Executor{Isolation: passIsolator{}}).Run(context.Background(), exec.JobSpec{AttemptID: "running-1", CoordinatorID: "c1", Home: t.TempDir(), Command: []string{"sh", "-c", "sleep 1"}})
	}()
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	plugged = false
	mu.Unlock()
	for _, mode := range []exec.Mode{exec.Remote, exec.Background} {
		if l, d, _ := gate.Acquire("new-"+string(mode), slots.Self("c1", "l2"), mode, Need{}, false); l != nil || d.Reason != model.NodeOnBattery {
			t.Fatalf("%s lease on battery: %+v", mode, d)
		}
	}
	if used, _ := table.Used(); used != 1 {
		t.Fatalf("the running lease was disturbed: %d slots", used)
	}
	r := <-done
	if r.Err != nil || r.ExitCode != 0 {
		t.Fatalf("the running job did not finish: %+v", r)
	}
	// The operator's own foreground run still goes, one job, with nothing reserved or plugged in.
	if d := gate.Admit(exec.Foreground, Need{}, false); !d.Allowed || !d.Floor {
		t.Fatalf("foreground gated on battery: %+v", d)
	}
	fg := (&exec.Executor{}).Run(context.Background(), exec.JobSpec{AttemptID: "fg-1", CoordinatorID: "c1", Home: t.TempDir(), Command: []string{"true"}})
	if fg.Err != nil || fg.ExitCode != 0 {
		t.Fatalf("foreground run: %+v", fg)
	}
	_ = lease.Release()
	mu.Lock()
	plugged = true
	mu.Unlock()
	if l, d, err := gate.Acquire("new-after", slots.Self("c1", "l3"), exec.Remote, Need{}, false); l == nil || err != nil {
		t.Fatalf("plugging in did not resume leases: %+v %v", d, err)
	}
}

func scenarioTwoCoordinators(t *testing.T) {
	dir := t.TempDir()
	set := reservation.Defaults("laptop-1", true, false)
	set.ReservedCPU = 6
	if err := SavePolicy(dir, set); err != nil {
		t.Fatal(err)
	}
	probe := func() exec.Capacity {
		return exec.Capacity{CPUs: 8, MemoryMB: 8192, FreeMemoryMB: 4096, BatteryPercent: -1}
	}
	mk := func() *Gate {
		tab, err := slots.Open(dir, 8)
		if err != nil {
			t.Fatal(err)
		}
		return &Gate{Dir: dir, Table: tab, Probe: probe, Settings: SettingsFrom(dir, "laptop-1", true, false)}
	}
	personal, team := mk(), mk()
	var wg sync.WaitGroup
	var mu sync.Mutex
	held := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g, who := personal, "personal"
			if i%2 == 1 {
				g, who = team, "team"
			}
			l, _, _ := g.Acquire(who+itoa(i), slots.Self(who, "l"+itoa(i)), exec.Remote, Need{}, false)
			if l != nil {
				mu.Lock()
				held++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	used, _ := personal.Table.Used()
	if used > 2 || held > 2 {
		t.Fatalf("two coordinators hold %d leases (table %d) on a 2-core share", held, used)
	}
	if held == 0 {
		t.Fatal("the share was never used")
	}
}

func scenarioSleep(t *testing.T) {
	probe := func() exec.Capacity {
		return exec.Capacity{CPUs: 4, MemoryMB: 8192, FreeMemoryMB: 4096, BatteryPercent: 80, PluggedIn: true}
	}
	dir := t.TempDir()
	defaults := reservation.Defaults("laptop-1", true, reservation.HasBattery(probe()))
	if defaults.Persistence != store.PersistenceInterm {
		t.Fatalf("a laptop defaults to %q, not intermittent", defaults.Persistence)
	}
	tab, err := slots.Open(dir, 4)
	if err != nil {
		t.Fatal(err)
	}
	gate := &Gate{Dir: dir, Table: tab, Probe: probe, Settings: SettingsFrom(dir, "laptop-1", true, true)}
	wall, mono := time.Unix(1_000_000, 0), time.Duration(0)
	w := &SleepWatcher{Wall: func() time.Time { return wall }, Mono: func() time.Duration { return mono }}
	w.Tick()
	if l, _, err := gate.Acquire("attempt-7", slots.Self("c1", "lease-1"), exec.Remote, Need{}, false); l == nil || err != nil {
		t.Fatalf("first lease: %v", err)
	}
	wall, mono = wall.Add(3*time.Hour), mono+time.Second // the lid closed
	if !w.Tick() {
		t.Fatal("the sleep was not noticed")
	}
	if fenced, err := gate.OnWake(); err != nil || len(fenced) != 1 || fenced[0] != "attempt-7" {
		t.Fatalf("OnWake fenced %v", fenced)
	}
	if l, _, err := gate.Acquire("attempt-7", slots.Self("c1", "lease-2"), exec.Remote, Need{}, false); l == nil || err != nil {
		t.Fatalf("re-lease after sleep: %v", err)
	}
	if used, _ := tab.Used(); used != 1 || gate.Held() != 1 {
		t.Fatalf("attempt-7 holds %d slots after the re-lease", used)
	}
}
