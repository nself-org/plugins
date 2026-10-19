package sim

// Purpose: P7-AUTO-28 acceptance. A bare foreground run uses registered ssh nodes: the
// dispatcher places and delivers the lease, the real ssh driver talks to an in-process
// agent session over pipes (the same node-side session the shipped agent runs), and the
// run loop awaits the result. No Docker, no network, no real ssh.
// Inputs:  two registered nodes (n1, n2), a git repository, a scripted node runner.
// Outputs: the run result and the attempt states in the store.
// Constraints: NSELF_V15=1 (the dispatch hook); ssh.Default is configured the way
// cmd/transport_ssh.go does and restored afterwards; tests do not run in parallel.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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
	"github.com/nself-org/plugins/free/ci/internal/run"
	_ "github.com/nself-org/plugins/free/ci/internal/schedtel" // the binary registers the telemetry sink and finalizer; the run records outcomes through them
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

type fakeKey struct{}

func (fakeKey) KeyID() string        { return "k1" }
func (fakeKey) PublicString() string { return "pub" }

// node is one registered ssh node and the in-process agent behind it.
type node struct {
	id, host string
	state    string
	runner   func(ctx context.Context, n *node, j session.JobRun) (session.JobResult, error)
	sessions atomic.Int32
	started  chan string // attempt ids whose job began here
	mu       sync.Mutex
	kill     func() error // drops the live session
	jobs     []string     // lease ids this node ran
}

func (n *node) ran() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.jobs...)
}

func evidence(l protocol.Lease, result string, class model.FailureClass, exit *int) json.RawMessage {
	ev := model.Evidence{Schema: "ci.evidence/v1", Revision: l.Checkout.Revision, Attempt: &model.AttemptRef{ID: l.AttemptID, N: 1}, Result: model.Result(result)}
	if result != "pass" {
		ev.Checks = []model.Check{{ID: "c", JobID: "unit", Result: model.CheckResult(result), FailureClass: class, Exit: exit}}
	}
	raw, _ := json.Marshal(ev)
	return raw
}

// serve runs one agent session on the pipes the driver's "ssh" is connected to.
func (n *node) serve(ctx context.Context, table *slots.Table, in io.Reader, out io.Writer) error {
	car, err := session.NewStreamCarrier(in, out, protocol.CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}, true)
	if err != nil {
		return err
	}
	run := session.RunnerFunc(func(ctx context.Context, j session.JobRun) (session.JobResult, error) {
		n.mu.Lock()
		n.jobs = append(n.jobs, j.Lease.LeaseID)
		n.mu.Unlock()
		select {
		case n.started <- j.Lease.AttemptID:
		default:
		}
		return n.runner(ctx, n, j)
	})
	s, err := session.New(session.Config{Carrier: car, Key: fakeKey{}, Slots: table, Runner: run, StateDir: n.state, Hooks: []session.JobHook{},
		Coordinator: session.Coordinator{ID: "coord", RootFingerprint: "fp"}, Facts: session.Facts{AgentVersion: "1.5.0", MachineID: n.id}})
	if err != nil {
		return err
	}
	return s.Run(ctx)
}

// starter is the ssh Starter: it picks the node by destination host.
func starter(t *testing.T, nodes []*node) ssh.Starter {
	return func(_ context.Context, tgt remote.Target, _ string) (*ssh.Conn, error) {
		var n *node
		for _, c := range nodes {
			if strings.HasSuffix(tgt.Dest, "@"+c.host) {
				n = c
			}
		}
		if n == nil {
			return nil, fmt.Errorf("no node at %s", tgt.Dest)
		}
		n.sessions.Add(1)
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
		kill := func() error {
			cancel()
			_ = toAgentR.Close()
			_ = fromAgentW.Close()
			return nil
		}
		n.mu.Lock()
		n.kill = kill
		n.mu.Unlock()
		return &ssh.Conn{Stdin: toAgentW, Stdout: fromAgentR, Stderr: errR, Wait: func() error { <-done; return nil }, Kill: kill}, nil
	}
}

func (n *node) drop() {
	n.mu.Lock()
	k := n.kill
	n.mu.Unlock()
	if k != nil {
		_ = k()
	}
}

type fixture struct {
	t     *testing.T
	repo  string
	home  string
	sha   string
	nodes []*node
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newFixture builds the repository, registers the nodes and configures ssh.Default.
func newFixture(t *testing.T, runners map[string]func(ctx context.Context, n *node, j session.JobRun) (session.JobResult, error)) *fixture {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	f := &fixture{t: t, repo: t.TempDir(), home: t.TempDir()}
	gitIn(t, f.repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(f.repo, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, f.repo, "add", "f.txt")
	gitIn(t, f.repo, "commit", "-q", "-m", "init")
	f.sha = gitIn(t, f.repo, "rev-parse", "HEAD")
	ctx := context.Background()
	st, err := store.Open(filepath.Join(f.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	pin := remote.PinnedHostKeys{Path: filepath.Join(f.home, "known_hosts")}
	for i, id := range []string{"n1", "n2"} {
		n := &node{id: id, host: fmt.Sprintf("10.0.0.%d", 5+i), state: t.TempDir(), started: make(chan string, 8), runner: runners[id]}
		f.nodes = append(f.nodes, n)
		if err := pin.Add("nself-ci-"+id, n.host+" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDummyKeyForTestsOnlyDummyKeyForTestsOnly000"); err != nil {
			t.Fatal(err)
		}
		c, err := planview.LocalCapability(planview.LocalFacts{Project: f.repo, Platform: runtime.GOOS + "/" + runtime.GOARCH, CPUs: 8, MemMB: 8192, Now: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		lan, owned, uid := "lan", "owned", true
		c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = id, id, "ssh", "ssh"
		c.Identity.SSH = &model.CapabilitySSH{Hostname: n.host, Port: 22, User: "ci"}
		c.SecretsEligible = true
		c.Location.Kind.Value, c.Economics.Class.Value = &lan, &owned
		c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
		reg := registry.New(st)
		doc, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if err = st.NodePut(ctx, store.NodeRecord{ID: id, Name: id, State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
			t.Fatal(err)
		}
		admin, err := store.NewAdminAuthority("test")
		if err != nil {
			t.Fatal(err)
		}
		trust := model.CapabilityTrust{
			Accepts:       assigned([]model.TrustClass{"owner", "internal"}),
			Isolation:     assigned(model.Isolation("container")),
			Network:       assigned([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}),
			SecretClasses: assigned([]model.SecretClass{"none"}),
			PrivacyZone:   assigned(model.PrivacyZone("private-infrastructure")),
		}
		if _, err = reg.SetTrust(ctx, id, trust, admin); err != nil {
			t.Fatal(err)
		}
		if _, err = reg.SetState(ctx, id, "online", "", "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	d := ssh.Default
	was := struct {
		home, repo, min string
		start           ssh.Starter
		ver             func(context.Context) (remote.Version, error)
		renew           func(context.Context, string, uint64) error
		personal        bool
		hb, ttl, grace  time.Duration
	}{d.Home, d.RepoDir, d.MinAgentVersion, d.Start, d.SSHVersion, d.Renew, d.Personal, d.Heartbeat, d.AgentTTL, d.CancelGrace}
	d.Home, d.RepoDir, d.MinAgentVersion, d.Start = f.home, f.repo, "1.5.0", starter(t, f.nodes)
	d.SSHVersion = func(context.Context) (remote.Version, error) { return remote.Version{Major: 9, Minor: 6}, nil }
	d.Renew, d.Personal, d.Heartbeat, d.AgentTTL, d.CancelGrace = run.RenewLease, true, time.Second, 25*time.Second, 3*time.Second
	t.Cleanup(func() {
		d.Home, d.RepoDir, d.MinAgentVersion, d.Start, d.SSHVersion, d.Renew, d.Personal = was.home, was.repo, was.min, was.start, was.ver, was.renew, was.personal
		d.Heartbeat, d.AgentTTL, d.CancelGrace = was.hb, was.ttl, was.grace
	})
	return f
}

func assigned[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"}
}

func (f *fixture) git(_ context.Context, argv ...string) (string, int, error) {
	j := strings.Join(argv, " ")
	switch {
	case strings.Contains(j, "rev-parse HEAD"):
		return f.sha + "\n", 0, nil
	case strings.Contains(j, "remote get-url"):
		return "", 1, nil
	}
	return "", 0, nil
}

type noReport struct{}

func (noReport) Post(context.Context, report.Status) error { return nil }

// start begins a run of one remote-only job named unit and returns its result channel.
func (f *fixture) start(ctx context.Context, job model.Job) (<-chan run.Result, *bytes.Buffer) {
	f.t.Helper()
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"unit": job}},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		f.t.Fatal(err)
	}
	out := &bytes.Buffer{}
	c := &run.Coordinator{Opts: run.Options{Root: f.repo, Home: f.home, NoStatus: true}, Plan: pl, ConfigDigest: "cfg", Out: out, Capture: f.git, Reporter: noReport{},
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}, Poll: 10 * time.Millisecond}
	ch := make(chan run.Result, 1)
	go func() {
		res, err := c.Run(ctx)
		if err != nil {
			f.t.Errorf("Run: %v\n%s", err, out)
		}
		ch <- res
	}()
	return ch, out
}

func remoteJob(infraMax int) model.Job {
	yes := true
	return model.Job{Kind: "test", Run: []string{"true"}, Required: &yes, RemoteOnly: true, Retry: &model.RetryConfig{InfraMax: infraMax}}
}

func (f *fixture) states(t *testing.T) map[string]string {
	t.Helper()
	st, err := store.Open(filepath.Join(f.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	out := map[string]string{}
	runs, err := st.PipelinesByRevision(context.Background(), f.sha)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs of %s: %v %v", f.sha, runs, err)
	}
	atts, err := st.PipelineAttempts(context.Background(), runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range atts {
		out[a.ID] = string(a.State)
	}
	return out
}

func wait[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(60 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
	panic("unreachable")
}

func passRunner(lines ...string) func(context.Context, *node, session.JobRun) (session.JobResult, error) {
	return func(_ context.Context, _ *node, j session.JobRun) (session.JobResult, error) {
		for _, l := range lines {
			j.Output([]byte(l + "\n"))
		}
		return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: evidence(j.Lease, "pass", "", nil)}, nil
	}
}

func TestForegroundRemoteHappy(t *testing.T) {
	r := passRunner("hello from the node", "second line")
	f := newFixture(t, map[string]func(context.Context, *node, session.JobRun) (session.JobResult, error){"n1": r, "n2": r})
	ch, out := f.start(context.Background(), remoteJob(1))
	res := wait(t, ch, "the run")
	if res.Exit != 0 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s\n%s", res.Exit, res.Summary.Result, out)
	}
	jo := res.Summary.Jobs[0]
	if jo.State != "passed" || jo.N != 1 {
		t.Fatalf("job outcome %+v", jo)
	}
	ran := append(f.nodes[0].ran(), f.nodes[1].ran()...)
	if len(ran) != 1 {
		t.Fatalf("the job ran on %d nodes: %v", len(ran), ran)
	}
	if !strings.Contains(jo.Tail, "hello from the node") {
		t.Fatalf("the node's output did not reach the run: %q", jo.Tail)
	}
	for _, n := range f.nodes { // the node's job root is gone once the result is acknowledged
		if entries, _ := os.ReadDir(filepath.Join(n.state, "jobs")); len(entries) != 0 {
			t.Errorf("node %s kept %d job roots", n.id, len(entries))
		}
	}
	for id, s := range f.states(t) {
		if s != "passed" {
			t.Errorf("attempt %s is %s", id, s)
		}
	}
	// the scheduler's telemetry records the outcome of a remote attempt as it does a local one
	db, err := sql.Open("sqlite", filepath.Join(f.home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err = db.QueryRow("SELECT count(*) FROM schedtel_outcome").Scan(&n); err != nil || n != 1 {
		t.Errorf("schedtel outcomes recorded = %d (%v), want 1", n, err)
	}
}

func TestForegroundRemoteExitCode(t *testing.T) {
	one := 3
	r := func(_ context.Context, _ *node, j session.JobRun) (session.JobResult, error) {
		j.Output([]byte("boom\n"))
		return session.JobResult{Outcome: session.Outcome{State: "failed", ExitCode: 3}, Evidence: evidence(j.Lease, "fail", "code", &one)}, nil
	}
	f := newFixture(t, map[string]func(context.Context, *node, session.JobRun) (session.JobResult, error){"n1": r, "n2": r})
	ch, out := f.start(context.Background(), remoteJob(1))
	res := wait(t, ch, "the run")
	jo := res.Summary.Jobs[0]
	if res.Exit == 0 || jo.State != "failed" || jo.FailureClass != "code" || jo.ExitCode != 3 || jo.N != 1 {
		t.Fatalf("a failing remote job: exit %d outcome %+v\n%s", res.Exit, jo, out)
	}
}

func TestForegroundRemoteLostSession(t *testing.T) {
	var first atomic.Bool
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	r := func(ctx context.Context, n *node, j session.JobRun) (session.JobResult, error) {
		if first.CompareAndSwap(false, true) { // the first attempt: the session dies under the running job
			j.Output([]byte("started\n"))
			select {
			case <-hang:
			case <-ctx.Done():
			}
			return session.JobResult{Outcome: session.Outcome{State: "cancelled"}, Evidence: evidence(j.Lease, "cancelled", "", nil)}, nil
		}
		return passRunner("second try")(ctx, n, j)
	}
	f := newFixture(t, map[string]func(context.Context, *node, session.JobRun) (session.JobResult, error){"n1": r, "n2": r})
	ch, out := f.start(context.Background(), remoteJob(1))
	var first1 *node
	select {
	case <-f.nodes[0].started:
		first1 = f.nodes[0]
	case <-f.nodes[1].started:
		first1 = f.nodes[1]
	case <-time.After(30 * time.Second):
		t.Fatal("the job never started on a node")
	}
	first1.drop()
	res := wait(t, ch, "the run")
	if res.Exit != 0 {
		t.Fatalf("the retry did not pass: exit %d\n%s", res.Exit, out)
	}
	jo := res.Summary.Jobs[0]
	if jo.State != "passed" || jo.N != 2 || len(jo.Retries) != 1 {
		t.Fatalf("outcome %+v", jo)
	}
	other := f.nodes[0]
	if first1 == f.nodes[0] {
		other = f.nodes[1]
	}
	if len(other.ran()) != 1 || len(first1.ran()) != 1 {
		t.Fatalf("the retry must run once elsewhere: first node %v, other node %v", first1.ran(), other.ran())
	}
	st := f.states(t)
	lost, passed := 0, 0
	for _, s := range st {
		switch s {
		case "cancelled", "lost", "retryable":
			lost++
		case "passed":
			passed++
		}
	}
	if passed != 1 || lost != 1 {
		t.Fatalf("attempt states %v", st)
	}
	if !strings.Contains(strings.ToLower(jo.Retries[0].Diagnosis), "lost") {
		t.Fatalf("retry reason %+v", jo.Retries[0])
	}
}

func TestForegroundRemoteCancel(t *testing.T) {
	killed := make(chan struct{})
	r := func(ctx context.Context, _ *node, j session.JobRun) (session.JobResult, error) {
		j.Output([]byte("working\n"))
		<-ctx.Done() // the agent kills the job's process group when the lease is cancelled
		close(killed)
		return session.JobResult{Outcome: session.Outcome{State: "cancelled"}, Evidence: evidence(j.Lease, "cancelled", "", nil)}, nil
	}
	f := newFixture(t, map[string]func(context.Context, *node, session.JobRun) (session.JobResult, error){"n1": r, "n2": r})
	ssh.Default.Heartbeat = 20 * time.Second             // the agent beats rarely, so a fenced heartbeat cannot be what stops the job: the cancel must
	t.Cleanup(run.SetRemoteCancelWait(10 * time.Minute)) // and the run's own wait cannot end it either
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, out := f.start(ctx, remoteJob(1))
	select {
	case <-f.nodes[0].started:
	case <-f.nodes[1].started:
	case <-time.After(30 * time.Second):
		t.Fatal("the job never started on a node")
	}
	cancel()                                // Ctrl-C
	res := wait(t, ch, "the cancelled run") // without the carrier cancel nothing ends it before the test's own deadline
	select {
	case <-killed:
	case <-time.After(15 * time.Second):
		t.Fatalf("the remote job was not stopped\n%s", out)
	}
	if res.Summary.Jobs[0].State != "cancelled" || res.Summary.Result != "cancelled" {
		t.Fatalf("cancelled run: %+v %s", res.Summary.Jobs[0], res.Summary.Result)
	}
	for id, s := range f.states(t) {
		if s != "cancelled" {
			t.Errorf("attempt %s is %s, want cancelled", id, s)
		}
	}
}

// A job that declares a secret class no registered node offers cannot be placed remotely:
// the node never sees the job, and no secret value is sent (TestSecretsNeverToForeignNode
// covers the lease itself).
func TestForegroundRemoteSecretsStayOffNodes(t *testing.T) {
	r := passRunner("must not run")
	f := newFixture(t, map[string]func(context.Context, *node, session.JobRun) (session.JobResult, error){"n1": r, "n2": r})
	job := remoteJob(0)
	job.Secrets = []string{"project/KEY"}
	ch, out := f.start(context.Background(), job)
	res := wait(t, ch, "the run")
	if res.Exit == 0 || len(f.nodes[0].ran())+len(f.nodes[1].ran()) != 0 {
		t.Fatalf("a secret-bearing job reached a node that does not offer the class: exit %d\n%s", res.Exit, out)
	}
	if !strings.Contains(out.String(), "E618") {
		t.Fatalf("the job must be refused as unplaceable: %s", out)
	}
}
