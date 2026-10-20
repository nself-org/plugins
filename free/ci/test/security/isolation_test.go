//go:build security

package security

// Purpose: network and host isolation of untrusted work (I07, I08, I17, I18; TM-05, TM-09, TM-11).
//   - the network proxy denies an untrusted job every special, on-link, host and deploy-exclusion
//     address in every spelling, and serves no scope at all below restricted;
//   - an untrusted job never runs under lan or privileged, and only on a runner that proved its scope;
//   - untrusted work never runs beside a secret-bearing attempt on one node (co-tenancy), either way round;
//   - every slot gets its own job UID, never the agent's or the coordinator's, and a node with no
//     separation refuses rather than guesses;
//   - attempt N+1 finds nothing of attempt N: files, containers and volumes are removed and a
//     leftover is reported, not ignored.
// Inputs:  netscope.Facts built from a synthetic LAN; the dispatcher rig; a temporary host root.
// Outputs: pass or fail.
// Constraints: no root, no docker: the container runtime is a fake behind hygiene.Runtime.

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/hygiene"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// syntheticLAN is a home LAN with a deploy host the operator excluded from CI reach.
func syntheticLAN() netscope.Facts {
	return netscope.Facts{
		OnLink:         []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24"), netip.MustParsePrefix("10.8.0.0/16"), netip.MustParsePrefix("2001:db8:10::/64")},
		HostAddrs:      []netip.Addr{netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("2001:db8:10::5")},
		Exclusion:      []netip.Prefix{netip.MustParsePrefix("93.184.216.50/32"), netip.MustParsePrefix("8.8.8.0/28")},
		ExclusionHosts: []string{"deploy.internal.example"},
	}
}

// spellings returns the forms of an IPv4 address an attacker can use to hide it from a naive filter.
func spellings(a netip.Addr) []netip.Addr {
	b := a.As4()
	return []netip.Addr{
		a,
		netip.AddrFrom16([16]byte{10: 0xff, 11: 0xff, 12: b[0], 13: b[1], 14: b[2], 15: b[3]}),     // ::ffff:a.b.c.d
		netip.AddrFrom16([16]byte{0x00, 0x64, 0xff, 0x9b, 12: b[0], 13: b[1], 14: b[2], 15: b[3]}), // 64:ff9b::a.b.c.d
		netip.AddrFrom16([16]byte{0x20, 0x02, b[0], b[1], b[2], b[3], 15: 1}),                      // 6to4
		a.WithZone("eth0").Unmap(),
	}
}

// TestUntrustedCannotReachExclusionOrOnLink is I18: for the two scopes that reach the proxy, no
// address of the LAN, the host, the deploy exclusion set or a special range is allowed, in any spelling.
func TestUntrustedCannotReachExclusionOrOnLink(t *testing.T) {
	f := syntheticLAN()
	forbidden := []string{
		"192.168.1.77", "192.168.1.5", "10.8.4.4", "10.0.0.1", "172.16.9.9", "192.168.200.1", // RFC1918, on-link and not
		"93.184.216.50", "8.8.8.7", // deploy exclusion (public addresses: only the exclusion set stops them)
		"127.0.0.1", "169.254.169.254", "169.254.1.1", "100.64.0.1", "0.0.0.0", "224.0.0.1", // loopback, metadata, CGNAT, unspecified, multicast
	}
	for _, scope := range []model.NetworkScope{"restricted", "internet"} {
		for _, s := range forbidden {
			a := netip.MustParseAddr(s)
			for _, form := range spellings(a) {
				if v := netscope.Allowed(scope, form, f); v.Allowed {
					t.Errorf("scope %s allowed %s (a spelling of %s)", scope, form, s)
				}
			}
		}
		for _, s := range []string{"::1", "fe80::1", "fc00::1", "fd12:3456::1", "2001:db8:10::99", "2001:db8:10::5", "ff02::1"} {
			if v := netscope.Allowed(scope, netip.MustParseAddr(s), f); v.Allowed {
				t.Errorf("scope %s allowed %s", scope, s)
			}
		}
		// control: a public address is reachable, so the denials above are the filter and not a closed door
		if v := netscope.Allowed(scope, netip.MustParseAddr("93.184.216.34"), f); !v.Allowed {
			t.Errorf("control: scope %s refused a public address: %+v", scope, v)
		}
	}
	for _, scope := range []model.NetworkScope{"none", "lan", "privileged"} {
		if v := netscope.Allowed(scope, netip.MustParseAddr("93.184.216.34"), f); v.Allowed {
			t.Errorf("scope %s is served by the proxy", scope)
		}
	}
}

// TestUntrustedNetworkScopeFloor is I07 at the scope layer: the default is restricted, lan and
// privileged are refused, and a runner that has not proven its scope is not LAN-free.
func TestUntrustedNetworkScopeFloor(t *testing.T) {
	if got, err := netscope.Effective("untrusted", ""); err != nil || got != "restricted" {
		t.Fatalf("untrusted with no declared scope: %q %v", got, err)
	}
	for _, scope := range []model.NetworkScope{"lan", "privileged"} {
		if got, err := netscope.Effective("untrusted", scope); err == nil {
			t.Fatalf("untrusted declaring %s got %q", scope, got)
		}
	}
	d := gvisorRunner("gv", 1).Decl
	d.NoLANRoute = false
	job := trust.JobFacts{Trust: "untrusted", Network: "restricted"}
	if len(trust.Admissible(job, d, repoPolicy(t, "public", "untrusted"))) == 0 {
		t.Fatal("an unproven runner admitted an untrusted job")
	}
	netscope.Apply(&d, "restricted", []model.NetworkScope{"restricted"})
	if reasons := trust.Admissible(job, d, repoPolicy(t, "public", "untrusted")); len(reasons) != 0 {
		t.Fatalf("a runner that proved restricted was refused: %v", reasons)
	}
}

// TestCotenancyUntrustedNeverBesideSecrets (TM-11): a node running a secret-bearing attempt takes
// no untrusted job, and a node running untrusted work takes no secret-bearing job.
func TestCotenancyUntrustedNeverBesideSecrets(t *testing.T) {
	beside := gvisorRunner("gv-busy", 4)
	beside.Decl.Running = trust.Running{SecretBearing: true}
	r := newRig(t, repoPolicy(t, "public", "untrusted"), beside)
	id, a, err := r.submit(newJob("fork-beside", "untrusted", "restricted"), "untrusted", settle)
	waitsBecause(t, r, id, a, err, "isolation.cotenancy")

	untrustedHere := lanRunner("lan-busy", 4)
	untrustedHere.Decl.Isolation = "vm"
	untrustedHere.Decl.Running = trust.Running{Untrusted: true}
	r = newRig(t, repoPolicy(t, "private", "owner"), untrustedHere)
	id, a, err = r.submit(newJob("secret-beside", "owner", "lan", "project"), "owner", settle)
	waitsBecause(t, r, id, a, err, "isolation.cotenancy")

	// control: with nothing running the same two jobs are placed
	free := gvisorRunner("gv-free", 4)
	r = newRig(t, repoPolicy(t, "public", "untrusted"), free)
	if _, a, err = r.submit(newJob("fork-alone", "untrusted", "restricted"), "untrusted", 2*settle); runnerOf(t, a, err) != "gv-free" {
		t.Fatal("control: the fork job must be placed on an idle sandbox node")
	}
}

// TestPerSlotUIDSeparation is I17: slots get distinct job UIDs that are neither the agent's nor the
// coordinator's, and a node that cannot separate refuses instead of reusing the agent's UID.
func TestPerSlotUIDSeparation(t *testing.T) {
	const agent, coord = 1001, 1002
	f := hygiene.Facts{Isolation: "sandboxed-container", Trust: "untrusted", Coordinator: "team", Ownership: "operator", Slots: 3, AgentUID: agent, CoordinatorUID: coord}
	seen := map[int]int{}
	for slot := 0; slot < 3; slot++ {
		p, err := hygiene.Separation(f, slot)
		if err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
		if p.UID == agent || p.UID == coord || p.UID <= 0 {
			t.Fatalf("slot %d got uid %d, the agent's or coordinator's", slot, p.UID)
		}
		if other, dup := seen[p.UID]; dup {
			t.Fatalf("slots %d and %d share uid %d", other, slot, p.UID)
		}
		seen[p.UID] = slot
	}
	// job-shim: two slots may never be handed the same user, and a duplicate user table is refused
	shim := hygiene.Facts{Isolation: "process", Trust: "internal", Coordinator: "team", Ownership: "operator", Slots: 2, AgentUID: agent, CoordinatorUID: coord, JobShim: true, JobUIDs: []int{2000, 2000}}
	if _, err := hygiene.Separation(shim, 0); err == nil {
		t.Fatal("a job-user table with a duplicate uid was accepted")
	}
	shim.JobUIDs = []int{agent, 2001}
	if _, err := hygiene.Separation(shim, 0); err == nil {
		t.Fatal("a job user equal to the agent's uid was accepted")
	}
	// no separation and no exception: refused (E682), never the agent's uid
	bare := hygiene.Facts{Isolation: "process", Trust: "internal", Coordinator: "team", Ownership: "operator", Slots: 1, AgentUID: agent, CoordinatorUID: coord}
	if p, err := hygiene.Separation(bare, 0); err == nil {
		t.Fatalf("a node with no separation was given uid %d (%s)", p.UID, p.Mode)
	}
	// the scheduler side: a node without per-slot separation is not eligible for a team job
	weak := gvisorRunner("weak", 2)
	weak.Decl.UIDSeparation = false
	r := newRig(t, repoPolicy(t, "public", "untrusted"), weak)
	id, a, err := r.submit(newJob("fork-uid", "untrusted", "restricted"), "untrusted", settle)
	waitsBecause(t, r, id, a, err, "isolation.uid_separation")
}

// fakeRuntime is the container runtime behind hygiene: labelled containers and volumes.
type fakeRuntime struct {
	mu       sync.Mutex
	items    map[string]string // kind/id -> attempt label
	stubborn map[string]bool   // ids that refuse removal
}

func (f *fakeRuntime) add(kind exec.ResourceKind, id, attempt string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[string(kind)+"/"+id] = attempt
}

func (f *fakeRuntime) List(_ context.Context, kind exec.ResourceKind, attempt string) ([]exec.Labelled, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []exec.Labelled
	for k, a := range f.items {
		if id, ok := strings.CutPrefix(k, string(kind)+"/"); ok && a == attempt {
			out = append(out, exec.Labelled{ID: id, Attempt: a})
		}
	}
	return out, nil
}

func (f *fakeRuntime) Remove(_ context.Context, kind exec.ResourceKind, id, attempt string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stubborn[id] {
		return errors.New("resource is busy")
	}
	delete(f.items, string(kind)+"/"+id)
	return nil
}

// TestWorkspaceHygieneBetweenAttempts is I08: nothing of attempt N survives for attempt N+1, and a
// leftover that could not be removed is reported as a leak (the node drains with E681).
func TestWorkspaceHygieneBetweenAttempts(t *testing.T) {
	base := t.TempDir()
	rt := &fakeRuntime{items: map[string]string{}, stubborn: map[string]bool{}}
	h := &hygiene.Host{Base: base, Runtime: rt}
	ctx := context.Background()

	root, err := hygiene.JobRoot(base, "att.a1")
	if err != nil {
		t.Fatal(err)
	}
	secretFile := filepath.Join(root, "workspace", "token.txt")
	if err = os.MkdirAll(filepath.Dir(secretFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(secretFile, []byte("synthetic-not-a-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("/etc", filepath.Join(root, "workspace", "escape")); err != nil {
		t.Fatal(err)
	}
	rt.add(exec.KindContainer, "c1", "att.a1")
	rt.add(exec.KindVolume, "v1", "att.a1")

	if err = h.Cleanup(ctx, "att.a1"); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	rep, err := h.LeakCheck(ctx, "att.a1", 0)
	if err != nil || !rep.Clean() {
		t.Fatalf("leak check after cleanup: %+v %v", rep, err)
	}
	if _, serr := os.Lstat(root); !os.IsNotExist(serr) {
		t.Fatalf("the job root of attempt 1 survived: %v", serr)
	}
	if _, serr := os.Stat("/etc/hostname"); serr != nil && !os.IsNotExist(serr) {
		t.Fatalf("control: /etc became unreadable: %v", serr)
	}
	next, err := hygiene.JobRoot(base, "att.a2")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(next)
	if err != nil || len(entries) != 0 {
		t.Fatalf("attempt 2 starts with %d entries (%v)", len(entries), err)
	}
	if info, serr := os.Stat(next); serr != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("job root mode %v (%v), want 0700", info.Mode().Perm(), serr)
	}

	// A container that will not die is a leak, not a silent success.
	rt.add(exec.KindContainer, "stuck", "att.a2")
	rt.stubborn["stuck"] = true
	if err = h.Cleanup(ctx, "att.a2"); err == nil {
		t.Fatal("cleanup reported success while a container survived")
	}
	rep, _ = h.LeakCheck(ctx, "att.a2", 0)
	if rep.Clean() {
		t.Fatal("leak check was clean with a stuck container")
	}
	found := false
	for _, l := range rep.Leaks {
		found = found || l.Kind == hygiene.LeakContainer
	}
	if !found {
		t.Fatalf("leaks %+v do not name the container", rep.Leaks)
	}
}

// TestRetryNeverLowersIsolation is I10 (TM-07, TM-08): a retry, or a re-placement after a probe lowered a
// node's level, keeps the floor of the attempt before it. Over every pair of levels the retry floor is
// at least both, and the scheduler refuses a node below the previous attempt's level.
func TestRetryNeverLowersIsolation(t *testing.T) {
	levels := model.EnumValues("Isolation")
	for _, prev := range levels {
		for _, required := range levels {
			got := trust.RetryIsolation(model.Isolation(prev), model.Isolation(required))
			if trust.RankIsolation(got) < trust.RankIsolation(model.Isolation(prev)) || trust.RankIsolation(got) < trust.RankIsolation(model.Isolation(required)) {
				t.Errorf("retry after %s with floor %s allowed %s", prev, required, got)
			}
		}
	}
	weak := lanRunner("container-node", 2)
	weak.Decl.Isolation = "container"
	world := func(previous map[string]model.Isolation) sched.World {
		return sched.World{Runners: []sched.Runner{weak}, Policy: sched.Policy{Preset: "balanced", Trust: repoPolicy(t, "private", "owner")}, Costs: map[string]sched.Cost{},
			Previous: map[string]string{}, PreviousIsolation: previous}
	}
	job := newJob("retry", "owner", "lan")
	if res := sched.Place(world(nil), []sched.Job{job}); len(res.Placements) != 1 {
		t.Fatalf("control: a first attempt on a container node was not placed: %+v", res.Explanations)
	}
	res := sched.Place(world(map[string]model.Isolation{job.Key: "vm"}), []sched.Job{job})
	if len(res.Placements) != 0 {
		t.Fatalf("a retry after a vm attempt was placed on a container node: %+v", res.Placements)
	}
}
