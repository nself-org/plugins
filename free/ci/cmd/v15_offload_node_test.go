package main

// Purpose: a registered ssh node behind an in-process agent session, for the
// cmd-level offload test (P7-AUTO-13). It is the test/sim foreground fixture
// (P7-AUTO-28) reduced to what `nself ci --where <node>` needs: the node is in
// the store under NSELF_CI_HOME, ssh.Default starts the session over pipes, and
// the node stages the checkout bundle the real driver sends, as `serve --stdio`
// does. No Docker, no network, no real ssh.
// Inputs:  the run store directory and a body for the node's job.
// Outputs: what the node's job found in the tree it was given.
// Constraints: ssh.Default is restored after the test; tests do not run in parallel.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

type oflKey struct{}

func (oflKey) KeyID() string        { return "k1" }
func (oflKey) PublicString() string { return "pub" }

// oflSeen is what the node's job found in its checkout.
type oflSeen struct {
	err   error
	head  string
	files map[string]string // name -> content for the probed names; a missing name is absent
}

// oflNode is one registered node and the agent behind it.
type oflNode struct {
	id, host, state string
	probe           []string
	seen            chan oflSeen
}

// oflStage is the node side of the checkout: the lease announcement and the
// bundle's chunk frames go to the receiver and never reach the session.
type oflStage struct {
	session.Carrier
	r *checkout.Receiver
}

func (c *oflStage) Recv() (protocol.Message, error) {
	for {
		m, err := c.Carrier.Recv()
		if err != nil {
			return m, err
		}
		switch b := m.Body.(type) {
		case *protocol.Lease:
			_ = c.r.Expect(*b)
		case *protocol.Chunk:
			if b.Stream != checkout.Stream {
				return m, nil
			}
			epoch := uint64(0)
			if m.Epoch != nil {
				epoch = *m.Epoch
			}
			_ = c.r.Feed(m.LeaseID, epoch, *b)
			continue
		}
		return m, nil
	}
}

// serve runs one agent session; the job stages the bundle, records the tree and passes.
func (n *oflNode) serve(ctx context.Context, table *slots.Table, in io.Reader, out io.Writer) error {
	car, err := session.NewStreamCarrier(in, out, protocol.CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}, true)
	if err != nil {
		return err
	}
	recv := checkout.NewReceiver(filepath.Join(n.state, "checkout"), nil)
	job := session.RunnerFunc(func(ctx context.Context, j session.JobRun) (session.JobResult, error) {
		s := oflSeen{files: map[string]string{}}
		src, err := recv.Stage(ctx, j.Lease, j.Root.Dir)
		if err != nil {
			s.err = err
			n.seen <- s
			return session.JobResult{}, err
		}
		if b, e := exec.Command("git", "-C", src, "rev-parse", "HEAD").Output(); e == nil {
			s.head = strings.TrimSpace(string(b))
		}
		for _, name := range n.probe {
			if b, e := os.ReadFile(filepath.Join(src, name)); e == nil {
				s.files[name] = string(b)
			}
		}
		n.seen <- s
		j.Output([]byte("tree checked\n"))
		ev, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: j.Lease.Checkout.Revision,
			Attempt: &model.AttemptRef{ID: j.Lease.AttemptID, N: 1}, Result: "pass"})
		return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: ev}, nil
	})
	s, err := session.New(session.Config{Carrier: &oflStage{Carrier: car, r: recv}, Key: oflKey{}, Slots: table, Runner: job, StateDir: n.state,
		Hooks: []session.JobHook{}, Coordinator: session.Coordinator{ID: "coord", RootFingerprint: "fp"},
		Facts: session.Facts{AgentVersion: minAgentVersion, MachineID: n.id}})
	if err != nil {
		return err
	}
	return s.Run(ctx)
}

// start is the ssh Starter: the "connection" is a pair of pipes to n's session.
func (n *oflNode) start(_ context.Context, tgt remote.Target, _ string) (*ssh.Conn, error) {
	if !strings.HasSuffix(tgt.Dest, "@"+n.host) {
		return nil, fmt.Errorf("no node at %s", tgt.Dest)
	}
	toAgentR, toAgentW := io.Pipe()
	fromAgentR, fromAgentW := io.Pipe()
	errR, errW := io.Pipe()
	table, err := slots.Open(n.state, 2)
	if err != nil {
		return nil, err
	}
	actx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		e := n.serve(actx, table, toAgentR, fromAgentW)
		_ = fromAgentW.Close()
		_ = errW.Close()
		done <- e
	}()
	kill := func() error { _ = fromAgentW.Close(); _ = toAgentR.Close(); cancel(); return nil }
	return &ssh.Conn{Stdin: toAgentW, Stdout: fromAgentR, Stderr: errR, Wait: func() error { <-done; return nil }, Kill: kill}, nil
}

// oflRegisterNode registers node id in the store under home and points ssh.Default at it.
func oflRegisterNode(t *testing.T, home, repo, id string, probe ...string) *oflNode {
	t.Helper()
	n := &oflNode{id: id, host: "10.0.0.5", state: t.TempDir(), probe: probe, seen: make(chan oflSeen, 4)}
	ctx := context.Background()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	pin := remote.PinnedHostKeys{Path: filepath.Join(home, "known_hosts")}
	if err = pin.Add("nself-ci-"+id, n.host+" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDummyKeyForTestsOnlyDummyKeyForTestsOnly000"); err != nil {
		t.Fatal(err)
	}
	c, err := planview.LocalCapability(planview.LocalFacts{Project: repo, Platform: runtime.GOOS + "/" + runtime.GOARCH, CPUs: 8, MemMB: 8192, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	lan, owned, uid := "lan", "owned", true
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = id, id, "ssh", "ssh"
	c.Identity.SSH = &model.CapabilitySSH{Hostname: n.host, Port: 22, User: "ci"}
	c.SecretsEligible = true
	c.Location.Kind.Value, c.Economics.Class.Value = &lan, &owned
	c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.NodePut(ctx, store.NodeRecord{ID: id, Name: id, State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"),
		DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.NewAdminAuthority("test")
	if err != nil {
		t.Fatal(err)
	}
	trust := model.CapabilityTrust{
		Accepts:       oflAssigned([]model.TrustClass{"owner", "internal"}),
		Isolation:     oflAssigned(model.Isolation("container")),
		Network:       oflAssigned([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}),
		SecretClasses: oflAssigned([]model.SecretClass{"none"}),
		PrivacyZone:   oflAssigned(model.PrivacyZone("private-infrastructure")),
	}
	reg := registry.New(st)
	if _, err = reg.SetTrust(ctx, id, trust, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = reg.SetState(ctx, id, "online", "", "test"); err != nil {
		t.Fatal(err)
	}
	d := ssh.Default
	home0, start0, ver0, hb0 := d.Home, d.Start, d.SSHVersion, d.Heartbeat
	d.Home, d.Start, d.Heartbeat = home, n.start, time.Second
	d.SSHVersion = func(context.Context) (remote.Version, error) { return remote.Version{Major: 9, Minor: 6}, nil }
	t.Cleanup(func() { d.Home, d.Start, d.SSHVersion, d.Heartbeat = home0, start0, ver0, hb0 })
	return n
}

func oflAssigned[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"}
}

// wait returns what the node's job saw, or fails when no job reached the node.
func (n *oflNode) wait(t *testing.T) oflSeen {
	t.Helper()
	select {
	case s := <-n.seen:
		return s
	case <-time.After(60 * time.Second):
		t.Fatal("no job reached the node")
	}
	panic("unreachable")
}
