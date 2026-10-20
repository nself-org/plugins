package main

// Purpose: `nself-ci-agent join --advertise` through the real subcommand table (D-1268): the flag is
// declared by join itself, listed by `join -h`, starts the advertisement after the production guard ran
// once, and the advertisement window can never be shorter than the pairing code lifetime.
// Inputs:  a fake discovery.Net and a calm fixture host. Outputs: pass/fail.
// Constraints: no socket is opened; join is pointed at a closed loopback port so it fails after the
//          advertisement started.

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/discovery"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
)

type fakeNet struct {
	mu      sync.Mutex
	started int
	stopped int
}

func (n *fakeNet) Browse(context.Context, string, time.Duration) ([]discovery.Entry, error) {
	return nil, nil
}

func (n *fakeNet) Advertise(discovery.Service) (func(), error) {
	n.mu.Lock()
	n.started++
	n.mu.Unlock()
	return func() { n.mu.Lock(); n.stopped++; n.mu.Unlock() }, nil
}

func (n *fakeNet) LocalAddrs() []netip.Addr { return []netip.Addr{netip.MustParseAddr("192.168.1.20")} }

func (n *fakeNet) counts() (started, stopped int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.started, n.stopped
}

// useAdvertiseGuard swaps in a Guard on the fake net and counts how often the production guard reads the host.
func useAdvertiseGuard(t *testing.T, env map[string]string) (*fakeNet, *int) {
	t.Helper()
	n := &fakeNet{}
	old := advertiseGuard
	advertiseGuard = &discovery.Guard{Home: t.TempDir(), Net: n, Getenv: func(k string) string { return env[k] }}
	t.Cleanup(func() { advertiseGuard = old })
	guardRuns := 0
	oldLocal := prodLocal
	prodLocal = func() deployguard.Local { guardRuns++; return calmLocal() }
	t.Cleanup(func() { prodLocal = oldLocal })
	return n, &guardRuns
}

func TestJoinHelpListsAdvertise(t *testing.T) {
	_, _, stderr := runCmd(t, "join", "-h")
	if !strings.Contains(stderr, "-advertise") {
		t.Fatalf("join -h does not list --advertise:\n%s", stderr)
	}
}

func TestJoinAdvertiseStartsAfterTheGuardRanOnce(t *testing.T) {
	useState(t)
	n, guardRuns := useAdvertiseGuard(t, nil)
	code, _, stderr := runCmd(t, "join", "--advertise", "--expect-fingerprint", "SHA256:x", "https://127.0.0.1:1/enroll")
	if code != 1 {
		t.Fatalf("join against a closed port: code %d, stderr %q", code, stderr)
	}
	if started, _ := n.counts(); started != 1 {
		t.Fatalf("advertisements started %d, want 1; stderr %q", started, stderr)
	}
	if !strings.Contains(stderr, "LAN discovery id:") {
		t.Errorf("the discovery id line is missing: %q", stderr)
	}
	if *guardRuns != 1 {
		t.Errorf("the production guard ran %d times, want 1", *guardRuns)
	}
}

func TestJoinWithoutAdvertiseOpensNothing(t *testing.T) {
	useState(t)
	n, guardRuns := useAdvertiseGuard(t, nil)
	runCmd(t, "join", "--expect-fingerprint", "SHA256:x", "https://127.0.0.1:1/enroll")
	if started, _ := n.counts(); started != 0 {
		t.Fatalf("an advertisement started without the flag")
	}
	if *guardRuns != 1 {
		t.Errorf("the production guard ran %d times, want 1", *guardRuns)
	}
}

func TestJoinAdvertiseRefusedWhenDiscoveryIsOff(t *testing.T) {
	useState(t)
	n, _ := useAdvertiseGuard(t, map[string]string{"NSELF_CI_DISCOVERY": "0"})
	code, _, stderr := runCmd(t, "join", "--advertise", "--expect-fingerprint", "SHA256:x", "https://127.0.0.1:1/enroll")
	if started, _ := n.counts(); code != 1 || started != 0 || !strings.Contains(stderr, "E667") {
		t.Fatalf("code %d started %d stderr %q", code, started, stderr)
	}
}

// If pairing.OfferTTL ever exceeds discovery.MaxWindow every join --advertise would be refused.
func TestAdvertiseWindowCoversThePairingCodeLifetime(t *testing.T) {
	if discovery.MaxWindow < pairing.OfferTTL {
		t.Fatalf("discovery.MaxWindow %s is shorter than pairing.OfferTTL %s", discovery.MaxWindow, pairing.OfferTTL)
	}
}
