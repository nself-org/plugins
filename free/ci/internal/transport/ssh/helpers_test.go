package ssh

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

type fakeKey struct{}

func (fakeKey) KeyID() string        { return "k1" }
func (fakeKey) PublicString() string { return "pub" }

// recSink records everything a driver delivers.
type recSink struct {
	mu        sync.Mutex
	logs      []protocol.Log
	results   []protocol.Result
	artifacts []protocol.ArtifactRef
	refs      []protocol.CacheRef
	blobs     map[string]*memBlob
	startAt   int64  // offset a Blob writer already holds
	prefix    []byte // the bytes before startAt, for Verify
}

type memBlob struct {
	head   transport.BlobHeader
	prefix []byte
	data   []byte
	start  int64
	closed bool
	abort  bool
}

func (b *memBlob) Write(p []byte) (int, error) { b.data = append(b.data, p...); return len(p), nil }
func (b *memBlob) Close() error                { b.closed = true; return nil }
func (b *memBlob) Offset() int64               { return b.start + int64(len(b.data)) }

func (s *recSink) Log(_ context.Context, l protocol.Log) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, l)
	return nil
}
func (s *recSink) ArtifactRef(_ context.Context, a protocol.ArtifactRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts = append(s.artifacts, a)
	return nil
}
func (s *recSink) Result(_ context.Context, r protocol.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, r)
	return nil
}
func (s *recSink) CacheRef(_ context.Context, r protocol.CacheRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refs = append(s.refs, r)
	return nil
}
func (s *recSink) Blob(_ context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blobs == nil {
		s.blobs = map[string]*memBlob{}
	}
	b := &memBlob{head: h, start: s.startAt, prefix: s.prefix}
	s.blobs[h.Stream] = b
	return b, nil
}

func (s *recSink) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.logs))
	for i, l := range s.logs {
		out[i] = l.Text
	}
	return out
}

// agentRig is the node end of a loopback "ssh" connection.
type agentRig struct {
	state  string
	runner session.Runner
	hooks  []session.JobHook
	tap    func(session.Carrier) session.Carrier
	hbMS   int
	done   chan error
	killed chan struct{}
	once   sync.Once
}

func newAgentRig(t *testing.T, runner session.Runner) *agentRig {
	return &agentRig{state: t.TempDir(), runner: runner, hooks: []session.JobHook{}, done: make(chan error, 1), killed: make(chan struct{})}
}

// starter returns the Starter whose "ssh" runs a real agent session on pipes.
// It records the target and command it was given in got.
func (a *agentRig) starter(t *testing.T, got *startArgs) Starter {
	return func(ctx context.Context, tgt remote.Target, command string) (*Conn, error) {
		if got != nil {
			got.target, got.command = tgt, command
		}
		coordToAgentR, coordToAgentW := io.Pipe()
		agentToCoordR, agentToCoordW := io.Pipe()
		errR, errW := io.Pipe()
		table, err := slots.Open(a.state, 2)
		if err != nil {
			return nil, err
		}
		actx, cancel := context.WithCancel(context.Background())
		go func() {
			// The agent carrier writes its preamble into the pipe at once, so it must not be
			// built on the goroutine that has not yet handed the Conn (and its reader) back.
			err := a.serve(actx, table, coordToAgentR, agentToCoordW)
			_ = agentToCoordW.Close()
			_ = errW.Close()
			a.done <- err
		}()
		kill := func() error {
			a.once.Do(func() { close(a.killed) })
			cancel()
			_ = coordToAgentR.Close()
			_ = agentToCoordW.Close()
			return nil
		}
		return &Conn{Stdin: coordToAgentW, Stdout: agentToCoordR, Stderr: errR,
			Wait: func() error { <-a.done; return nil }, Kill: kill}, nil
	}
}

type startArgs struct {
	target  remote.Target
	command string
}

func testNode(t *testing.T) (model.Capability, string) {
	t.Helper()
	home := t.TempDir()
	pin := remote.PinnedHostKeys{Path: filepath.Join(home, "known_hosts")}
	if err := pin.Add("nself-ci-node1", "10.0.0.5 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDummyKeyForTestsOnlyDummyKeyForTestsOnly000"); err != nil {
		t.Fatal(err)
	}
	var n model.Capability
	n.Identity.ID = "node1"
	n.Identity.Transport = Kind
	op := "operator"
	n.Identity.Ownership.Value = &op
	n.SecretsEligible = true
	n.Identity.SSH = &model.CapabilitySSH{Hostname: "10.0.0.5", Port: 2222, User: "ci"}
	return n, home
}

func testLease() protocol.Lease {
	return protocol.Lease{LeaseID: "lease-1", AttemptID: "a1", Epoch: 4, Token: "opaque", CompatMode: protocol.Compat15,
		Job: model.Job{Kind: "static"}, Secrets: []protocol.SecretRef{}, ExpiresAt: "2099-01-01T00:00:00Z"}
}

func evidence() json.RawMessage { return json.RawMessage(`{"schema":"test","ok":true}`) }

func okRunner(lines ...string) session.Runner {
	return session.RunnerFunc(func(ctx context.Context, j session.JobRun) (session.JobResult, error) {
		for _, l := range lines {
			j.Output([]byte(l + "\n"))
		}
		return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: evidence()}, nil
	})
}

func newDriver(t *testing.T, home string, a *agentRig, got *startArgs) *Driver {
	t.Helper()
	return &Driver{Home: home, Start: a.starter(t, got), SSHVersion: func(context.Context) (remote.Version, error) { return remote.Version{Major: 9, Minor: 6}, nil },
		NoRenew: true, Heartbeat: time.Second, AgentTTL: 25 * time.Second, CancelGrace: 3 * time.Second, HelloTimeout: 10 * time.Second}
}

func mustExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}

func (a *agentRig) serve(ctx context.Context, table *slots.Table, in io.Reader, out io.Writer) error {
	car, err := session.NewStreamCarrier(in, out, protocol.CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}, true)
	if err != nil {
		return err
	}
	if a.tap != nil {
		car = a.tap(car)
	}
	s, err := session.New(session.Config{Carrier: car, Key: fakeKey{}, Slots: table, Runner: a.runner, StateDir: a.state, Hooks: a.hooks,
		Coordinator: session.Coordinator{ID: "coord", RootFingerprint: "fp"}, Facts: session.Facts{AgentVersion: "1.5.0", MachineID: "m1"}})
	if err != nil {
		return err
	}
	return s.Run(ctx)
}
