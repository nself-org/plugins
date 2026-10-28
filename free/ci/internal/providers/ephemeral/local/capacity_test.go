package local

// Purpose: burst capacity follows the live load of the node (D-1381): regular lane jobs and the host's free
//          memory shrink what burst may grant, and a Tart slot a regular vm job took after the probe is a quota
//          refusal at Provision, not a failure at Wrap.
// Inputs:  a fake backend, scripted host capacity and lane count, a scripted vm profile.
// Outputs: pass or fail.
// Constraints: no process, no network.

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

func grantUntilQuota(t *testing.T, p *Provider, mut func(*burst.Intent)) int {
	t.Helper()
	got := 0
	for i := 0; i < 12; i++ {
		in := intentFor(fmt.Sprintf("LOAD%d", i))
		if mut != nil {
			mut(&in)
		}
		_, err := p.Provision(context.Background(), in)
		if errors.Is(err, burst.ErrQuota) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got++
	}
	return got
}

// TestBusyNodeSubtractsRegularLanes: an 8-core node whose lanes run 5 jobs offers burst 3 slots, and a lane
// job that starts while burst runs takes a slot from the next grant.
func TestBusyNodeSubtractsRegularLanes(t *testing.T) {
	var lanes atomic.Int64
	host := func() (exec.Capacity, reservation.Settings) {
		return exec.Capacity{CPUs: 8, MemoryMB: 64 << 10, FreeMemoryMB: 64 << 10, BatteryPercent: -1}, reservation.Settings{}
	}
	t.Run("lanes running before burst", func(t *testing.T) {
		lanes.Store(5)
		p := newProvider(t, newFake(), func(o *Options) { o.Host, o.Lanes = host, func() int { return int(lanes.Load()) } })
		if got := grantUntilQuota(t, p, nil); got != 3 {
			t.Fatalf("granted %d slots on a node whose lanes hold 5 of 8, want 3", got)
		}
	})
	t.Run("without a lane source the whole share is offered", func(t *testing.T) {
		p := newProvider(t, newFake(), func(o *Options) { o.Host = host })
		if got := grantUntilQuota(t, p, nil); got != 8 {
			t.Fatalf("granted %d, want 8", got)
		}
	})
	t.Run("a lane that starts later takes a slot from the next grant", func(t *testing.T) {
		lanes.Store(6)
		p := newProvider(t, newFake(), func(o *Options) { o.Host, o.Lanes = host, func() int { return int(lanes.Load()) } })
		if _, err := p.Provision(context.Background(), intentFor("LATE1")); err != nil {
			t.Fatal(err)
		}
		lanes.Store(7) // 8 - 7 = 1 slot for burst, and burst already holds it
		if _, err := p.Provision(context.Background(), intentFor("LATE2")); !errors.Is(err, burst.ErrQuota) {
			t.Fatalf("second grant with 7 of 8 lanes busy and one burst job: %v, want ErrQuota", err)
		}
	})
	t.Run("lanes beyond the share offer nothing, never a negative limit", func(t *testing.T) {
		lanes.Store(20)
		p := newProvider(t, newFake(), func(o *Options) { o.Host, o.Lanes = host, func() int { return int(lanes.Load()) } })
		if got := grantUntilQuota(t, p, nil); got != 0 {
			t.Fatalf("granted %d", got)
		}
	})
}

// TestFreeMemoryLimitsBurst: the node has room for three more 2 GB jobs in memory right now, however many cores it has.
func TestFreeMemoryLimitsBurst(t *testing.T) {
	free := 3 * 2048
	host := func() (exec.Capacity, reservation.Settings) {
		return exec.Capacity{CPUs: 64, MemoryMB: 64 << 10, FreeMemoryMB: free, BatteryPercent: -1}, reservation.Settings{}
	}
	big := func(in *burst.Intent) { in.Spec.MemMB = 2048 }
	p := newProvider(t, newFake(), func(o *Options) { o.Host = host })
	if got := grantUntilQuota(t, p, big); got != 3 {
		t.Fatalf("granted %d jobs of 2 GB with 6 GB free, want 3", got)
	}
	// the three jobs start: their memory is inside the host's free figure from now on
	for i := 0; i < 3; i++ {
		p.Observe("burst-"+fmt.Sprintf("LOAD%d", i), 1)
	}
	free = 10 * 2048 // memory came back: the next grant sees it
	if _, err := p.Provision(context.Background(), func() burst.Intent { in := intentFor("MEMBACK"); big(&in); return in }()); err != nil {
		t.Fatalf("free memory grew but the grant was refused: %v", err)
	}
}

// holdingProfile is a vm profile that can take a slot ahead of Wrap, with the answer scripted.
type holdingProfile struct {
	fakeProfile
	holdErr error
	held    []string
}

func (p *holdingProfile) Hold(_ context.Context, a isolation.Attempt) error {
	if p.holdErr == nil {
		p.held = append(p.held, a.ID)
	}
	return p.holdErr
}

// TestTartSlotIsHeldAtProvision: a granted vm slot is reserved in the profile's ledger at Provision, and a slot
// that a non-burst vm job took between the probe and the hold is a quota refusal that reserves nothing.
func TestTartSlotIsHeldAtProvision(t *testing.T) {
	vm := &holdingProfile{fakeProfile: fakeProfile{level: "vm", av: isolation.Availability{Level: "vm", Facts: map[string]string{"slots_total": "2", "foreign_running": "0", "own_running": "0"}}}}
	b := NewTartBackend(vm).(*tartBackend)
	b.platform = "darwin/arm64"
	p := newProvider(t, b, nil)
	in := intentFor("HOLD1")
	in.Spec.Platform = "darwin/arm64"
	res, err := p.Provision(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(vm.held) != 1 || vm.held[0] != res.ID {
		t.Fatalf("held %v, want the granted attempt %q", vm.held, res.ID)
	}
	vm.holdErr = isolation.Unavailable("isolation.tart_no_free_slot", "both macOS VM slots are in use")
	in2 := intentFor("HOLD2")
	in2.Spec.Platform = "darwin/arm64"
	if _, err = p.Provision(context.Background(), in2); !errors.Is(err, burst.ErrQuota) {
		t.Fatalf("a slot lost to a regular vm job must be ErrQuota, got %v", err)
	}
	if p.count() != 1 {
		t.Fatalf("%d reservations; the refused grant must reserve nothing", p.count())
	}
	vm.holdErr = errors.New("tart exploded")
	if _, err = p.Provision(context.Background(), in2); errors.Is(err, burst.ErrQuota) || CodeOf(err) != CodeUnavailable {
		t.Fatalf("another hold failure is an E698 outage, not a quota refusal: %v", err)
	}
}
