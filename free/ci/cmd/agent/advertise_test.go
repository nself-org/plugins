package main

// Purpose: the glue of `join --advertise` (P7-NODE-12): the flag is declared on join, the advertisement
// starts once after the PreStart hooks, stops with the join, and the window never outlives the pairing code.
// Inputs: a fake discovery.Net and a scripted environment. Outputs: pass/fail.
// Constraints: no socket is opened; the package-level guard is replaced and restored per test.

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/discovery"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
)

type fakeNet struct{ adverts, stops int }

func (n *fakeNet) Browse(context.Context, string, time.Duration) ([]discovery.Entry, error) {
	return nil, nil
}

func (n *fakeNet) Advertise(discovery.Service) (func(), error) {
	n.adverts++
	return func() { n.stops++ }, nil
}

func (n *fakeNet) LocalAddrs() []netip.Addr { return []netip.Addr{netip.MustParseAddr("192.168.1.9")} }

// useAdvertiseGuard swaps advertiseGuard and the request flag for one test.
func useAdvertiseGuard(t *testing.T, g *discovery.Guard, requested bool) {
	t.Helper()
	oldG, oldR, oldL := advertiseGuard, advertiser.requested, advertiser.live
	advertiseGuard, advertiser.requested, advertiser.live = g, requested, nil
	t.Cleanup(func() { advertiseGuard, advertiser.requested, advertiser.live = oldG, oldR, oldL })
}

// D-1268: MaxWindow is a copy of pairing.OfferTTL; if the code outlives the cap every join --advertise fails.
func TestAdvertiseWindowCoversThePairingCode(t *testing.T) {
	if discovery.MaxWindow < pairing.OfferTTL {
		t.Fatalf("discovery.MaxWindow %s is shorter than pairing.OfferTTL %s: every join --advertise would fail", discovery.MaxWindow, pairing.OfferTTL)
	}
}

func TestJoinHelpListsAdvertise(t *testing.T) {
	code, _, stderr, _ := joinRun(t, nil, "", "-h")
	if code != 0 || !strings.Contains(stderr, "-advertise") {
		t.Fatalf("join -h = %d, want the flag listed:\n%s", code, stderr)
	}
}

func TestAdvertiseIsNotAPreStartHook(t *testing.T) {
	regMu.RLock()
	defer regMu.RUnlock()
	for _, p := range prestart {
		if p.name == advertiseName {
			t.Fatalf("advertise must start after the guard, not as a PreStart hook: %+v", prestart)
		}
	}
}

func TestAdvertiseStartNeedsTheFlag(t *testing.T) {
	n := &fakeNet{}
	useAdvertiseGuard(t, &discovery.Guard{Net: n, Home: t.TempDir()}, false)
	if err := advertiseStart(context.Background(), "join"); err != nil || n.adverts != 0 || advertiser.live != nil {
		t.Fatalf("without the flag: err %v, %d adverts", err, n.adverts)
	}
	advertiser.requested = true
	if err := advertiseStart(context.Background(), "pull"); err != nil || n.adverts != 0 {
		t.Fatalf("another command must not advertise: err %v, %d adverts", err, n.adverts)
	}
}

func TestAdvertiseStartsOnceAndStopsWithTheJoin(t *testing.T) {
	n := &fakeNet{}
	useAdvertiseGuard(t, &discovery.Guard{Net: n, Home: t.TempDir(), Getenv: func(string) string { return "" }}, true)
	ctx := context.Background()
	if err := advertiseStart(ctx, "join"); err != nil {
		t.Fatal(err)
	}
	if n.adverts != 1 || advertiser.live == nil {
		t.Fatalf("%d adverts, live %v", n.adverts, advertiser.live)
	}
	if err := advertiseStop(ctx, enroll.JoinResult{}, ""); err != nil || n.stops != 1 {
		t.Fatalf("stop: err %v, %d stops", err, n.stops)
	}
}

// With discovery off, join --advertise is refused (E667) before it reads a state directory or a code.
func TestJoinAdvertiseRefusedWhenDiscoveryIsOff(t *testing.T) {
	n := &fakeNet{}
	useAdvertiseGuard(t, &discovery.Guard{Net: n, Home: t.TempDir(), Getenv: func(k string) string {
		if k == "NSELF_CI_DISCOVERY" {
			return "0"
		}
		return ""
	}}, false)
	code, _, stderr, rest := joinRun(t, nil, "code\n", "--advertise", "https://c.test:3866")
	if code != 1 || n.adverts != 0 || !strings.Contains(stderr, "off") || rest != len("code\n") {
		t.Fatalf("code %d, %d adverts, rest %d:\n%s", code, n.adverts, rest, stderr)
	}
	if advertiser.requested != true {
		t.Fatalf("the flag value did not reach the hook")
	}
}
