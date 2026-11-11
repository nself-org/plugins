package main

// Agent-level tests of the contention wiring (D-1226): `serve --stdio` admits every lease through the
// node's contention gate. A reserved node refuses with node.reservation in the heartbeat and admits once
// the reservation ends; a laptop on battery refuses with node.on_battery and admits once plugged in.

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/contention"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// fakePower is a capacity probe whose power source the test switches.
type fakePower struct {
	mu      sync.Mutex
	battery int
	plugged bool
}

func (f *fakePower) probe() ciexec.Capacity {
	f.mu.Lock()
	defer f.mu.Unlock()
	return ciexec.Capacity{CPUs: 4, MemoryMB: 8192, FreeMemoryMB: 4096, BatteryPercent: f.battery, PluggedIn: f.plugged}
}

func (f *fakePower) plug(on bool) { f.mu.Lock(); f.plugged = on; f.mu.Unlock() }

func useProbe(t *testing.T, f *fakePower) {
	t.Helper()
	old := contentionProbe
	contentionProbe = f.probe
	t.Cleanup(func() { contentionProbe = old })
}

func contentionLease(id string) protocol.Lease {
	return protocol.Lease{LeaseID: id, AttemptID: "attempt-" + id, Epoch: 1, Token: "opaque-token-" + id, CompatMode: protocol.Compat15,
		Job: model.Job{Kind: "static", Run: []string{"sh", "-c", "echo job-done"}, NeedsSource: noSource()}, Checkout: protocol.Checkout{Revision: rev40},
		Secrets: []protocol.SecretRef{}, ExpiresAt: "2030-01-01T00:00:00Z"}
}

// contentionHandshake checks hello asks for the contention block and answers with one carrying policy.
func contentionHandshake(t *testing.T, p *stdioPeer, policy *protocol.NodePolicy) {
	t.Helper()
	h := p.next("hello").Body.(*protocol.Hello)
	if !slices.Contains(h.CompatModeSupported, protocol.CompatContention) {
		t.Fatalf("hello does not ask for the contention block: %v", h.CompatModeSupported)
	}
	p.send("welcome", protocol.Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s", HeartbeatMS: 60000, AgentTTLMS: 120000,
		Contention: &protocol.Contention{Policy: policy}}, 0, "")
}

// refusal reads frames until the heartbeat that reports id refused and returns its reason and the logs.
func refusal(t *testing.T, p *stdioPeer, id string) (string, string) {
	t.Helper()
	var logs []string
	for {
		m, err := p.dec.Decode()
		if err != nil {
			t.Fatalf("waiting for the refusal of %s: %v (logs %q)", id, err, logs)
		}
		switch b := m.Body.(type) {
		case *protocol.Log:
			logs = append(logs, b.Text)
		case *protocol.Result:
			t.Fatalf("lease %s ran instead of being refused: %s", b.LeaseID, strings.Join(logs, "\n"))
		case *protocol.Heartbeat:
			for _, ls := range b.Leases {
				if ls.LeaseID == id && ls.State == "refused" {
					return ls.Reason, strings.Join(logs, "\n")
				}
			}
		}
	}
}

func passes(t *testing.T, p *stdioPeer) {
	t.Helper()
	res, logs := resultOf(t, p)
	if m := validEvidence(t, res.Evidence); m["result"] != "pass" || !strings.Contains(logs, "job-done") {
		t.Fatalf("the admitted lease did not pass: %v\n%s", m["result"], logs)
	}
}

func drainServe(t *testing.T, p *stdioPeer) {
	t.Helper()
	p.send("drain", protocol.Drain{Deadline: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}, 0, "")
	p.next("ack")
	if code := <-p.done; code != 0 {
		t.Fatalf("serve exit %d", code)
	}
}

// TestServeRefusesALeaseOnAReservedNodeUntilTheReservationEnds: the policy arrives in welcome (the pull
// route), holds every core, and the lease is refused with node.reservation; the reservation is then ended
// through `policy --set` (the ssh route of `nodes set`) and the next lease runs, without a restart.
func TestServeRefusesALeaseOnAReservedNodeUntilTheReservationEnds(t *testing.T) {
	skipUnixOnly(t)
	state := useState(t)
	useProbe(t, &fakePower{battery: -1})
	p := startServe(t, "--coordinator", "peer-1")
	reserved := protocol.NodePolicy{Node: "laptop-1", ReservedCPU: 4, Interactive: store.InteractiveIgnore, OnBattery: store.OnBatteryPause, Persistence: store.PersistencePersist}
	contentionHandshake(t, p, &reserved)
	l := contentionLease("r1")
	p.send("lease", l, l.Epoch, l.LeaseID)
	reason, logs := refusal(t, p, "r1")
	if reason != string(model.NodeReservation) || !strings.Contains(logs, "node.reservation") {
		t.Fatalf("refusal names %q, logs %q", reason, logs)
	}
	if s, ok, err := contention.LoadPolicy(state); err != nil || !ok || s.ReservedCPU != 4 {
		t.Fatalf("the welcome policy was not stored: %+v %v %v", s, ok, err)
	}

	open := reserved
	open.ReservedCPU, open.Version = 0, 2
	raw, _ := json.Marshal(store.NodeSettings{NodeID: open.Node, ReservedCPU: open.ReservedCPU, Interactive: open.Interactive, OnBattery: open.OnBattery, Persistence: open.Persistence, Version: open.Version})
	if code, out, errs := runCmd(t, "policy", "--set", base64.RawURLEncoding.EncodeToString(raw)); code != 0 || !strings.Contains(out, `"reserved_cpu":0`) {
		t.Fatalf("policy --set: code %d out %q err %q", code, out, errs)
	}
	l = contentionLease("r2")
	p.send("lease", l, l.Epoch, l.LeaseID)
	passes(t, p)
	drainServe(t, p)
}

// TestServeTakesNoLeaseOnBatteryAndResumesWhenPluggedIn: a node never configured, with a battery, on
// battery power refuses with node.on_battery (the D10 default pause); plugged in, the next lease runs.
func TestServeTakesNoLeaseOnBatteryAndResumesWhenPluggedIn(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	power := &fakePower{battery: 40}
	useProbe(t, power)
	p := startServe(t, "--coordinator", "peer-1")
	contentionHandshake(t, p, nil)
	l := contentionLease("b1")
	p.send("lease", l, l.Epoch, l.LeaseID)
	if reason, logs := refusal(t, p, "b1"); reason != string(model.NodeOnBattery) || !strings.Contains(logs, "node.on_battery") {
		t.Fatalf("refusal names %q, logs %q", reason, logs)
	}
	power.plug(true)
	l = contentionLease("b2")
	p.send("lease", l, l.Epoch, l.LeaseID)
	passes(t, p)
	drainServe(t, p)
}

// TestPolicyRefusesAnInvalidValueAndKeepsTheOldOne: `policy --set` validates before it replaces the file.
func TestPolicyRefusesAnInvalidValueAndKeepsTheOldOne(t *testing.T) {
	useState(t)
	if code, out, _ := runCmd(t, "policy"); code != 0 || strings.TrimSpace(out) != "none" {
		t.Fatalf("no policy: code %d out %q", code, out)
	}
	good, _ := json.Marshal(store.NodeSettings{NodeID: "n1", ReservedCPU: 2, Interactive: store.InteractiveIgnore, OnBattery: store.OnBatteryAllow, Persistence: store.PersistencePersist})
	if code, _, errs := runCmd(t, "policy", "--set", base64.RawURLEncoding.EncodeToString(good)); code != 0 {
		t.Fatalf("valid policy refused: %s", errs)
	}
	bad, _ := json.Marshal(map[string]any{"node": "n1", "reserved_cpu": -1, "interactive": "ignore", "on_battery": "allow", "persistence": "persistent"})
	for _, arg := range []string{base64.RawURLEncoding.EncodeToString(bad), "%%%", base64.RawURLEncoding.EncodeToString([]byte(`{"node":"n1","extra":1}`))} {
		if code, _, _ := runCmd(t, "policy", "--set", arg); code != 1 {
			t.Fatalf("invalid policy %q: code %d", arg, code)
		}
	}
	if code, out, _ := runCmd(t, "policy"); code != 0 || !strings.Contains(out, `"reserved_cpu":2`) {
		t.Fatalf("the old policy did not survive: %q", out)
	}
	if code, _, _ := runCmd(t, "policy", "extra"); code != 2 {
		t.Fatalf("usage: code %d", code)
	}
}
