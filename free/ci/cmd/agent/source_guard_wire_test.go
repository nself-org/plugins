package main

// Purpose: P7-WIRE-05. A pull or stdio agent refuses a lease that names a revision when no source tree
//          was staged (unless the job declares needs_source false), and the coordinator never settles an
//          unbound pass as a pass. Every case goes through a real entry: the agent's `serve --stdio` and
//          `pull` argv, and the coordinator's run.Coordinator over the real ssh driver, whose "ssh" is a
//          pipe into the agent protocol session with a scripted node that reports an unbound pass.
// Inputs:  a git repository, a scripted coordinator on pipes, the pull rig of pull_e2e_test.go.
// Outputs: pass or fail.
// Constraints: unix only (shell jobs); ssh.Default is set as cmd/transport_ssh.go does and restored.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	_ "github.com/nself-org/plugins/free/ci/internal/schedtel" // the binary registers the telemetry sink the run records outcomes through
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

func noSource() *bool { no := false; return &no }

// bareLease names a revision but carries no checkout: what a carrier without a checkout route sends.
func bareLease(id string, job model.Job) protocol.Lease {
	return protocol.Lease{LeaseID: id, AttemptID: "attempt-" + id, Epoch: 1, Token: "opaque-token-7f3a", CompatMode: protocol.Compat15,
		Job: job, Checkout: protocol.Checkout{Revision: rev40}, Secrets: []protocol.SecretRef{}, ExpiresAt: "2030-01-01T00:00:00Z"}
}

func passed(raw json.RawMessage) bool {
	var ev struct {
		Result string `json:"result"`
	}
	return json.Unmarshal(raw, &ev) == nil && ev.Result == "pass"
}

func TestWire05LeaseWithoutSourceRefused(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	p := startServe(t, "--coordinator", "peer-1")
	handshake(p)
	l := bareLease("ns1", model.Job{Kind: "static", Run: []string{"sh", "-c", "echo STEP-RAN"}})
	p.send("lease", l, l.Epoch, l.LeaseID)
	res, logs := resultOf(t, p)
	if strings.Contains(logs, "STEP-RAN") || passed(res.Evidence) {
		t.Fatalf("a lease naming a revision ran on an empty tree:\n%s\n%s", res.Evidence, logs)
	}
}

func TestWire05NoSourceJobAllowed(t *testing.T) {
	skipUnixOnly(t)
	t.Run("stdio", func(t *testing.T) {
		useState(t)
		p := startServe(t, "--coordinator", "peer-1")
		handshake(p)
		l := bareLease("ns2", model.Job{Kind: "static", Run: []string{"sh", "-c", "echo STEP-RAN"}, NeedsSource: noSource()})
		p.send("lease", l, l.Epoch, l.LeaseID)
		res, logs := resultOf(t, p)
		if !strings.Contains(logs, "STEP-RAN") || !passed(res.Evidence) {
			t.Fatalf("a needs_source false job did not pass:\n%s\n%s", res.Evidence, logs)
		}
	})
	// carrier B (pull) has no checkout route: only a job that declares needs_source false runs there
	for name, c := range map[string]struct {
		needs *bool
		pass  bool
	}{"declared": {needs: noSource(), pass: true}, "undeclared": {needs: nil, pass: false}} {
		t.Run("pull "+name, func(t *testing.T) {
			state := useState(t)
			r := newPullRig(t)
			r.enroll(state, "n-1", r.base)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			startPull(t, ctx)
			pullWait(t, 15*time.Second, "the agent to connect", func() bool { return len(r.srv.NodesConnected()) == 1 })
			leaseID, att, ep := r.seed("1", "n-1")
			l := protocol.Lease{LeaseID: leaseID, AttemptID: att, Epoch: ep, CompatMode: protocol.Compat15,
				Job:      model.Job{Kind: "static", Run: []string{"sh", "-c", "echo PULL-STEP-RAN"}, NeedsSource: c.needs},
				Checkout: protocol.Checkout{Revision: rev40}, Secrets: []protocol.SecretRef{}, ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}
			var node model.Capability
			node.Identity.ID = "n-1"
			runErr := make(chan error, 1)
			go func() { runErr <- r.drv.Run(context.Background(), node, l, r.sink) }()
			select {
			case <-runErr:
			case <-time.After(30 * time.Second):
				t.Fatalf("the lease never ended:\n%s", r.sink.text())
			}
			r.sink.mu.Lock()
			ok := len(r.sink.results) == 1 && passed(r.sink.results[0].Evidence)
			r.sink.mu.Unlock()
			ran := strings.Contains(r.sink.text(), "PULL-STEP-RAN")
			if ok != c.pass || ran != c.pass {
				t.Fatalf("pass %v ran %v, want %v:\n%s", ok, ran, c.pass, r.sink.text())
			}
		})
	}
}

// wireRun is a coordinator run of one remote-only job over the real ssh driver to this package's serve.
type wireRun struct {
	repo, home, sha, state string
}

func newWireRun(t *testing.T) *wireRun {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	w := &wireRun{repo: t.TempDir(), home: t.TempDir(), state: t.TempDir()}
	git(t, w.repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(w.repo, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, w.repo, "add", "f.txt")
	git(t, w.repo, "commit", "-q", "-m", "init")
	w.sha = git(t, w.repo, "rev-parse", "HEAD")
	ctx := context.Background()
	st, err := store.Open(filepath.Join(w.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	pin := remote.PinnedHostKeys{Path: filepath.Join(w.home, "known_hosts")}
	if err = pin.Add("nself-ci-n1", "10.0.0.5 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDummyKeyForTestsOnlyDummyKeyForTestsOnly000"); err != nil {
		t.Fatal(err)
	}
	c, err := planview.LocalCapability(planview.LocalFacts{Project: w.repo, Platform: runtime.GOOS + "/" + runtime.GOARCH, CPUs: 8, MemMB: 8192, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	lan, owned, uid := "lan", "owned", true
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = "n1", "n1", "ssh", "ssh"
	c.Identity.SSH = &model.CapabilitySSH{Hostname: "10.0.0.5", Port: 22, User: "ci"}
	c.Location.Kind.Value, c.Economics.Class.Value = &lan, &owned
	c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.NodePut(ctx, store.NodeRecord{ID: "n1", Name: "n1", State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.NewAdminAuthority("test")
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New(st)
	trust := model.CapabilityTrust{
		Accepts:       wireFact([]model.TrustClass{"owner", "internal"}),
		Isolation:     wireFact(model.Isolation("container")),
		Network:       wireFact([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}),
		SecretClasses: wireFact([]model.SecretClass{"none"}),
		PrivacyZone:   wireFact(model.PrivacyZone("private-infrastructure")),
	}
	if _, err = reg.SetTrust(ctx, "n1", trust, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = reg.SetState(ctx, "n1", "online", "", "test"); err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	d := ssh.Default
	home, repo, minV, start, ver, renew, personal, hb, ttl, grace := d.Home, d.RepoDir, d.MinAgentVersion, d.Start, d.SSHVersion, d.Renew, d.Personal, d.Heartbeat, d.AgentTTL, d.CancelGrace
	d.Home, d.RepoDir, d.MinAgentVersion, d.Start = w.home, w.repo, Version, wireStarter(t, w.state)
	d.SSHVersion = func(context.Context) (remote.Version, error) { return remote.Version{Major: 9, Minor: 6}, nil }
	d.Renew, d.Personal, d.Heartbeat, d.AgentTTL, d.CancelGrace = cirun.RenewLease, true, time.Second, 25*time.Second, 3*time.Second
	t.Cleanup(func() {
		d.Home, d.RepoDir, d.MinAgentVersion, d.Start, d.SSHVersion, d.Renew, d.Personal = home, repo, minV, start, ver, renew, personal
		d.Heartbeat, d.AgentTTL, d.CancelGrace = hb, ttl, grace
	})
	return w
}

func wireFact[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"}
}

type wireKey struct{}

func (wireKey) KeyID() string        { return "k1" }
func (wireKey) PublicString() string { return "pub" }

// unboundPassNode is the node's runner: it reports a pass whose evidence is not bound to the revision,
// what an agent sends when its job ran on a tree that is not the leased commit (no tree, or a dirty one).
func unboundPassNode(_ context.Context, j session.JobRun) (session.JobResult, error) {
	reason := model.BindingReason("dirty")
	ev := model.Evidence{Schema: "ci.evidence/v1", Revision: j.Lease.Checkout.Revision, Attempt: &model.AttemptRef{ID: j.Lease.AttemptID, N: 1},
		Result: "pass", Binding: model.Binding{State: "unbound", Reason: &reason}}
	raw, err := json.Marshal(ev)
	return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: raw}, err
}

// wireStarter is the ssh Starter: the "remote" end is an agent protocol session (the one `serve --stdio`
// runs) on pipes, whose job runner is the scripted node.
func wireStarter(t *testing.T, state string) ssh.Starter {
	return func(_ context.Context, _ remote.Target, _ string) (*ssh.Conn, error) {
		toAgentR, toAgentW := io.Pipe()
		fromAgentR, fromAgentW := io.Pipe()
		errR, errW := io.Pipe()
		table, err := slots.Open(state, 2)
		if err != nil {
			return nil, err
		}
		actx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { // the carrier writes its preamble at once: the driver reads it after Start returns
			defer close(done)
			defer func() { _ = fromAgentW.Close(); _ = errW.Close() }()
			car, err := session.NewStreamCarrier(toAgentR, fromAgentW, protocol.CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}, true)
			if err != nil {
				t.Errorf("carrier: %v", err)
				return
			}
			s, err := session.New(session.Config{Carrier: car, Key: wireKey{}, Slots: table, Runner: session.RunnerFunc(unboundPassNode), StateDir: state, Hooks: []session.JobHook{},
				Coordinator: session.Coordinator{ID: "coord", RootFingerprint: "fp"}, Facts: session.Facts{AgentVersion: Version, MachineID: "n1"}})
			if err != nil {
				t.Errorf("session: %v", err)
				return
			}
			_ = s.Run(actx)
		}()
		kill := func() error { _ = fromAgentW.Close(); _ = toAgentR.Close(); cancel(); return nil }
		t.Cleanup(func() { _ = kill(); <-done })
		return &ssh.Conn{Stdin: toAgentW, Stdout: fromAgentR, Stderr: errR, Wait: func() error { <-done; return nil }, Kill: kill}, nil
	}
}

func (w *wireRun) capture(_ context.Context, argv ...string) (string, int, error) {
	j := strings.Join(argv, " ")
	switch {
	case strings.Contains(j, "rev-parse HEAD"):
		return w.sha + "\n", 0, nil
	case strings.Contains(j, "remote get-url"):
		return "", 1, nil
	}
	return "", 0, nil
}

type wireNoReport struct{}

func (wireNoReport) Post(context.Context, report.Status) error { return nil }

func (w *wireRun) run(t *testing.T, job model.Job) (cirun.Result, string) {
	t.Helper()
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"unit": job}},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	c := &cirun.Coordinator{Opts: cirun.Options{Root: w.repo, Home: w.home, NoStatus: true}, Plan: pl, ConfigDigest: "cfg", Out: out, Capture: w.capture, Reporter: wireNoReport{},
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}, Poll: 10 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	return res, out.String()
}

// The node's evidence says pass but unbound (probe): the run must settle the attempt failed with the
// named reason and not retry it, and pass it only when the job declares needs_source false.
func TestWire05UnboundPassRejected(t *testing.T) {
	yes := true
	job := model.Job{Kind: "test", Run: []string{"true"}, Required: &yes, RemoteOnly: true, Retry: &model.RetryConfig{InfraMax: 1}}

	w := newWireRun(t)
	res, out := w.run(t, job)
	if len(res.Summary.Jobs) != 1 {
		t.Fatalf("jobs %+v\n%s", res.Summary.Jobs, out)
	}
	jo := res.Summary.Jobs[0]
	if res.Exit == 0 || jo.State != "failed" || !strings.Contains(jo.Detail, "unbound_pass") || jo.N != 1 {
		t.Fatalf("an unbound pass settled exit %d state %s n %d detail %q tail %q\n%s", res.Exit, jo.State, jo.N, jo.Detail, jo.Tail, out)
	}

	job.NeedsSource = noSource()
	w2 := newWireRun(t)
	res, out = w2.run(t, job)
	if jo := res.Summary.Jobs[0]; res.Exit != 0 || jo.State != "passed" {
		t.Fatalf("a needs_source false job: exit %d state %s detail %q\n%s", res.Exit, jo.State, jo.Detail, out)
	}
}
