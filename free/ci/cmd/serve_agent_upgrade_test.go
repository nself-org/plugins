package main

// Purpose: D-1383, coordinator side: the agent listener `ci serve` starts registers itself as the process's update.AgentControl,
//          and an upgrade sent through it (update.AgentNode, the pull carrier of `nself ci nodes upgrade`) reaches a connected
//          agent as the protocol `upgrade` frame; the agent's upgrade-result (done, or refused with its E666 reason) is
//          recorded and reaches the operator; stop unregisters the control.
// Inputs:  a temp store and data dir (useAgentStore); a fake agent: the real pull carrier and session with an Upgrade hook that
//          swaps nothing. Outputs: pass or fail.
// Constraints: synthetic ids only; the agent and the listener are stopped by the test.

import (
	"context"
	"crypto/tls"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/agent/update"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

func TestNodesUpgradeReachesAConnectedAgentThroughTheRegisteredControl(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	prev := agentStoreSource
	t.Cleanup(func() { agentStoreSource = prev })
	useAgentStore(st, dir, "")
	ctx := context.Background()
	ca, err := pki.New(ctx, pki.Options{Store: st, DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	addAgentTestNode(t, st, "n-1")
	cert := agentTestNodeCert(t, ca, "n-1")

	stop, err := startAgentListener(ctx, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = stop(ctx)
		}
	})
	srv := activeAgent.Load()
	waitFor(t, "the listener to bind", func() bool { return srv.Addr() != nil })
	ctl, err := update.CurrentAgentControl()
	if err != nil || ctl != update.AgentControl(srv) {
		t.Fatalf("registered control %v (%v), want the listener's server", ctl, err)
	}

	// the fake agent: refuses 1.4.0 as a downgrade, accepts 1.6.0
	var mu sync.Mutex
	var asked []protocol.Upgrade
	hook := func(_ context.Context, u protocol.Upgrade) (string, error) {
		mu.Lock()
		asked = append(asked, u)
		mu.Unlock()
		if u.Version == "1.4.0" {
			return update.ReasonDowngrade, errors.New("E666: downgrade")
		}
		return "", nil
	}
	agentCtx, agentStop := context.WithCancel(ctx)
	ran := runFakeAgent(t, agentCtx, ca, &cert, "https://"+srv.Addr().String(), hook)
	t.Cleanup(func() { agentStop(); <-ran })
	waitFor(t, "the agent's hello", func() bool { v, verr := ctl.AgentVersion(ctx, "n-1"); return verr == nil && v == "1.5.0" })

	url := "https://example.invalid/v1.6.0/nself-ci-agent-linux-amd64.manifest.json"
	node := &update.AgentNode{NodeID: "n-1", Ctl: ctl, ResultWait: 10 * time.Second}
	err = installWhenLive(t, node, update.InstallInput{Version: "1.4.0", ManifestURL: url})
	if update.ReasonOf(err) != update.ReasonDowngrade || !strings.Contains(err.Error(), "E666") {
		t.Fatalf("refused upgrade: %v, want E666 with reason downgrade", err)
	}
	if err = installWhenLive(t, node, update.InstallInput{Version: "1.6.0", ManifestURL: url, AllowDowngrade: true}); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	r, err := ctl.WaitUpgradeResult(ctx, "n-1", "1.6.0", time.Second)
	if err != nil || r.Status != protocol.UpgradeDone {
		t.Fatalf("recorded result %+v (%v), want done for 1.6.0", r, err)
	}
	mu.Lock()
	got := append([]protocol.Upgrade{}, asked...)
	mu.Unlock()
	if len(got) != 2 || got[1] != (protocol.Upgrade{Version: "1.6.0", ManifestURL: url, AllowDowngrade: true}) {
		t.Fatalf("the agent received %+v", got)
	}

	agentStop()
	<-ran
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err = stop(sctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	stopped = true
	if _, err = update.CurrentAgentControl(); !errors.Is(err, update.ErrNoAgentControl) {
		t.Fatalf("after stop: %v, want ErrNoAgentControl", err)
	}
}

// installWhenLive retries Install until the agent's control stream is open (the hello comes a moment before it).
func installWhenLive(t *testing.T, n *update.AgentNode, in update.InstallInput) error {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		err := n.Install(context.Background(), in)
		if !errors.Is(err, agentapi.ErrNoControlStream) || time.Now().After(end) {
			return err
		}
	}
}

// runFakeAgent runs one pull session of node n-1 against base until ctx ends; the returned channel closes when it has.
func runFakeAgent(t *testing.T, ctx context.Context, ca *pki.CA, cert *tls.Certificate, base string, hook func(context.Context, protocol.Upgrade) (string, error)) <-chan struct{} {
	t.Helper()
	state := t.TempDir()
	key, err := nodekey.LoadOrCreate(state)
	if err != nil {
		t.Fatal(err)
	}
	table, err := slots.Open(state, 1)
	if err != nil {
		t.Fatal(err)
	}
	tlsc, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: ca.RootDER(), Cert: cert, ServerName: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	car := agent.NewCarrier(agent.CarrierConfig{Client: agent.Options{BaseURL: base, TLS: tlsc}, Logf: t.Logf})
	s, err := session.New(session.Config{
		Carrier: car, Key: key, Slots: table, StateDir: state,
		Facts: session.Facts{AgentVersion: "1.5.0", MachineID: "m-1", CapabilityDigest: "sha256:00"},
		Runner: session.RunnerFunc(func(context.Context, session.JobRun) (session.JobResult, error) {
			return session.JobResult{}, errors.New("no jobs here")
		}),
		Coordinator: session.Coordinator{ID: "local", RootFingerprint: pki.CertFingerprint(ca.RootDER())},
		Upgrade:     hook,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); _ = s.Run(ctx) }()
	return done
}
