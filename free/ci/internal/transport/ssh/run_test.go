package ssh

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"os"
	"os/exec"
	"path/filepath"
)

func TestSSHTransportStreamsLogsAndDeliversResult(t *testing.T) {
	node, home := testNode(t)
	a := newAgentRig(t, okRunner("one", "two", "three"))
	var got startArgs
	d := newDriver(t, home, a, &got)
	sink := &recSink{}
	if err := d.Run(context.Background(), node, testLease(), sink); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Join(sink.lines(), ","); lines != "one,two,three" {
		t.Fatalf("lines = %q", lines)
	}
	if len(sink.results) != 1 || sink.results[0].Epoch != 4 || sink.results[0].LeaseID != "lease-1" {
		t.Fatalf("results = %+v", sink.results)
	}
	if got.command != agentCommand {
		t.Fatalf("command = %q", got.command)
	}
	if entries, _ := os.ReadDir(filepath.Join(a.state, "jobs")); len(entries) != 0 {
		t.Fatalf("job root left after finalize: %v", entries)
	}
}

func TestSSHTransportForcesOptionSetAndPin(t *testing.T) {
	node, home := testNode(t)
	a := newAgentRig(t, okRunner())
	var got startArgs
	d := newDriver(t, home, a, &got)
	if err := d.Run(context.Background(), node, testLease(), &recSink{}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got.target.Options, " ")
	for _, want := range []string{"-T", "-a", "-x", "BatchMode=yes", "ForwardAgent=no", "ForwardX11=no", "ClearAllForwardings=yes", "PermitLocalCommand=no",
		"ControlMaster=no", "ControlPath=none", "RemoteCommand=none", "StrictHostKeyChecking=yes", "HostKeyAlias=nself-ci-node1",
		"UserKnownHostsFile=" + filepath.Join(home, "known_hosts"), "GlobalKnownHostsFile=/dev/null", "VerifyHostKeyDNS=no", "KnownHostsCommand=none", "ConnectTimeout=10", "-p 2222"} {
		if !strings.Contains(joined, want) {
			t.Errorf("options lack %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "accept-new") || strings.Contains(joined, "StrictHostKeyChecking=no") {
		t.Fatalf("host key checking relaxed: %s", joined)
	}
	if got.target.Dest != "ci@10.0.0.5" {
		t.Fatalf("dest = %q", got.target.Dest)
	}
}

func TestSSHTransportRefusesBadNodes(t *testing.T) {
	node, home := testNode(t)
	a := newAgentRig(t, okRunner())
	started := false
	d := newDriver(t, home, a, nil)
	d.Start = func(context.Context, remote.Target, string) (*Conn, error) {
		started = true
		return nil, errors.New("must not start")
	}
	for name, mutate := range map[string]func(*model.Capability){
		"option as host": func(n *model.Capability) {
			n.Identity.SSH = &model.CapabilitySSH{Hostname: "-oProxyCommand=touch /tmp/x"}
		},
		"space in host":   func(n *model.Capability) { n.Identity.SSH = &model.CapabilitySSH{Hostname: "a b"} },
		"shell in user":   func(n *model.Capability) { n.Identity.SSH = &model.CapabilitySSH{Hostname: "h", User: "x;id"} },
		"no destination":  func(n *model.Capability) { n.Identity.SSH = nil },
		"bad node id":     func(n *model.Capability) { n.Identity.ID = "../evil" },
		"unpinned node":   func(n *model.Capability) { n.Identity.ID = "node2" },
		"legacy path dst": func(n *model.Capability) { n.Identity.SSH = &model.CapabilitySSH{Hostname: "h:/etc"} },
	} {
		n := node
		mutate(&n)
		err := d.Run(context.Background(), n, testLease(), &recSink{})
		var te *Error
		if !errors.As(err, &te) {
			t.Errorf("%s: err = %v", name, err)
		}
		if name == "unpinned node" && te != nil && te.Code != CodeHostKey {
			t.Errorf("unpinned node: code %s, want %s", te.Code, CodeHostKey)
		}
	}
	if started {
		t.Fatal("ssh started for a node that failed validation")
	}
}

func TestSSHTransportSessionDropIsLost(t *testing.T) {
	node, home := testNode(t)
	running := make(chan struct{})
	var stopped atomic.Bool
	a := newAgentRig(t, session.RunnerFunc(func(ctx context.Context, j session.JobRun) (session.JobResult, error) {
		close(running)
		<-ctx.Done()
		stopped.Store(true)
		return session.JobResult{Outcome: session.Outcome{State: "cancelled"}, Evidence: evidence()}, nil
	}))
	d := newDriver(t, home, a, nil)
	var conn *Conn
	orig := d.Start
	d.Start = func(ctx context.Context, tg remote.Target, c string) (*Conn, error) {
		cn, err := orig(ctx, tg, c)
		conn = cn
		return cn, err
	}
	go func() {
		<-running
		_ = conn.Kill() // the ssh process dies under the session
	}()
	err := d.Run(context.Background(), node, testLease(), &recSink{})
	if !IsLost(err) || Reason(err) != "transport.ssh_session_lost" {
		t.Fatalf("err = %v, want session lost", err)
	}
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeTransport {
		t.Fatalf("err = %v, want code %s", err, CodeTransport)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !stopped.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !stopped.Load() {
		t.Fatal("the node did not stop the job after the session was lost")
	}
}

func TestSSHTransportCancelStopsJobWithinFiveSeconds(t *testing.T) {
	node, home := testNode(t)
	running := make(chan struct{})
	a := newAgentRig(t, session.RunnerFunc(func(ctx context.Context, j session.JobRun) (session.JobResult, error) {
		close(running)
		<-ctx.Done()
		return session.JobResult{Outcome: session.Outcome{State: "cancelled", FailureClass: "cancelled"}, Evidence: evidence()}, nil
	}))
	d := newDriver(t, home, a, nil)
	ctx, cancel := context.WithCancel(context.Background())
	sink := &recSink{}
	errc := make(chan error, 1)
	go func() { errc <- d.Run(ctx, node, testLease(), sink) }()
	<-running
	start := time.Now()
	cancel()
	err := <-errc
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("cancel took %v", took)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.results) != 1 {
		t.Fatalf("the cancelled result was not delivered: %d", len(sink.results))
	}
}

func TestSSHTransportPreambleNoiseIsSkipped(t *testing.T) {
	node, home := testNode(t)
	a := newAgentRig(t, okRunner("x"))
	d := newDriver(t, home, a, nil)
	orig := d.Start
	d.Start = func(ctx context.Context, tg remote.Target, c string) (*Conn, error) {
		cn, err := orig(ctx, tg, c)
		if err != nil {
			return nil, err
		}
		cn.Stdout = io.MultiReader(strings.NewReader("Welcome to node1\nLast login: never\n"), cn.Stdout)
		return cn, nil
	}
	sink := &recSink{}
	if err := d.Run(context.Background(), node, testLease(), sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.results) != 1 {
		t.Fatal("no result after a login banner")
	}
}

// TestSSHTransportRefusesACorruptedCheckoutBeforeAnyStep proves the agent verifies the
// bundle digest and no job step runs when the bytes do not match.
func TestSSHTransportRefusesACorruptedCheckoutBeforeAnyStep(t *testing.T) {
	node, home := testNode(t)
	ran := make(chan struct{}, 1)
	a := newAgentRig(t, session.RunnerFunc(func(ctx context.Context, j session.JobRun) (session.JobResult, error) {
		ran <- struct{}{}
		return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: evidence()}, nil
	}))
	recv := checkout.NewReceiver(t.TempDir(), nil)
	a.hooks = []session.JobHook{{Name: "co", Before: func(ctx context.Context, l protocol.Lease, root session.JobRoot) (session.JobSetup, error) {
		src, err := recv.Stage(ctx, l, root.Dir)
		return session.JobSetup{SourceDir: src}, err
	}}}
	a.tap = func(c session.Carrier) session.Carrier { return &feedCarrier{Carrier: c, r: recv} }
	repo, rev := makeRepo(t)
	d := newDriver(t, home, a, nil)
	d.RepoDir = repo
	d.afterBundle = func(p string) {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		b[len(b)/2] ^= 0xff
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	l := testLease()
	l.Checkout.Revision = rev
	sink := &recSink{}
	start := time.Now()
	err := d.Run(context.Background(), node, l, sink)
	t.Logf("run took %v: %v; results=%d", time.Since(start), err, len(sink.results))
	select {
	case <-ran:
		t.Fatal("a job step ran on a checkout whose digest did not match")
	default:
	}
}

// feedCarrier routes the lease and checkout chunks to a receiver, as serve --stdio does.
type feedCarrier struct {
	session.Carrier
	r *checkout.Receiver
}

func (c *feedCarrier) Recv() (protocol.Message, error) {
	for {
		m, err := c.Carrier.Recv()
		if err != nil {
			return m, err
		}
		switch b := m.Body.(type) {
		case *protocol.Lease:
			_ = c.r.Expect(*b)
		case *protocol.Chunk:
			var e uint64
			if m.Epoch != nil {
				e = *m.Epoch
			}
			_ = c.r.Feed(m.LeaseID, e, *b)
			continue
		}
		return m, nil
	}
}

func makeRepo(t *testing.T) (dir, rev string) {
	t.Helper()
	dir = t.TempDir()
	g := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	g("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g("add", "f.txt")
	g("commit", "-q", "-m", "one")
	return dir, g("rev-parse", "HEAD")
}
