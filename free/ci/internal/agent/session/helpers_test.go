package session

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// fakeCarrier is a scripted coordinator's end of a carrier: the test pushes
// coordinator frames with push and reads what the agent sent with next.
type fakeCarrier struct {
	in     chan protocol.Message
	out    chan protocol.Message
	closed chan struct{}
	once   sync.Once
	seq    uint64
}

func newFakeCarrier() *fakeCarrier {
	return &fakeCarrier{in: make(chan protocol.Message, 64), out: make(chan protocol.Message, 256), closed: make(chan struct{})}
}

func (c *fakeCarrier) Send(m protocol.Message) error {
	select {
	case <-c.closed:
		return io.ErrClosedPipe
	default:
	}
	c.out <- m
	return nil
}

func (c *fakeCarrier) Recv() (protocol.Message, error) {
	select {
	case m := <-c.in:
		return m, nil
	case <-c.closed:
		return protocol.Message{}, io.EOF
	}
}

func (c *fakeCarrier) Close() error { c.once.Do(func() { close(c.closed) }); return nil }

// push sends a coordinator frame with the next coordinator sequence number.
func (c *fakeCarrier) push(kind string, body any) { c.pushFenced(kind, body, 0, "") }

func (c *fakeCarrier) pushFenced(kind string, body any, epoch uint64, lease string) {
	c.seq++
	m := protocol.Message{V: 1, Type: kind, Seq: c.seq, Body: body, LeaseID: lease}
	if epoch != 0 {
		m.Epoch = &epoch
	}
	// Through the real codec, so the session sees what a carrier would decode.
	var buf bytes.Buffer
	enc, err := protocol.NewResumedEncoder(&buf, c.seq-1, c.seq-1)
	if err != nil {
		panic(err)
	}
	if err := enc.Encode(m); err != nil {
		panic(err)
	}
	dec, _ := protocol.NewResumedDecoder(&buf, testPolicy, c.seq-1, c.seq-1)
	got, err := dec.Decode()
	if err != nil {
		panic(err)
	}
	c.in <- got
}

// next returns the agent's next frame, skipping heartbeats unless wanted.
func (c *fakeCarrier) next(t *testing.T, want string) protocol.Message {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case m := <-c.out:
			if m.Type == want {
				return m
			}
			if m.Type != "heartbeat" && m.Type != "log" {
				t.Fatalf("got %s, want %s", m.Type, want)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", want)
		}
	}
}

// drainOut returns every frame already sent.
func (c *fakeCarrier) drainOut() []protocol.Message {
	var out []protocol.Message
	for {
		select {
		case m := <-c.out:
			out = append(out, m)
		default:
			return out
		}
	}
}

type fakeKey struct{}

func (fakeKey) KeyID() string        { return "k1" }
func (fakeKey) PublicString() string { return "pub" }

// manualClock ticks only when the test says so.
type manualClock struct {
	mu  sync.Mutex
	now time.Time
	c   chan time.Time
}

func newManualClock() *manualClock {
	return &manualClock{now: time.Date(2026, 10, 8, 19, 0, 0, 0, time.UTC), c: make(chan time.Time)}
}
func (m *manualClock) Now() time.Time                 { m.mu.Lock(); defer m.mu.Unlock(); return m.now }
func (m *manualClock) NewTicker(time.Duration) Ticker { return manualTicker{m.c} }
func (m *manualClock) advance(d time.Duration) {
	m.mu.Lock()
	m.now = m.now.Add(d)
	t := m.now
	m.mu.Unlock()
	m.c <- t
}

type manualTicker struct{ c chan time.Time }

func (t manualTicker) C() <-chan time.Time { return t.c }
func (manualTicker) Stop()                 {}

var testPolicy = protocol.CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}

var testWelcome = protocol.Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s1", HeartbeatMS: 10000, AgentTTLMS: 30000}

func testLease(id string, epoch uint64) protocol.Lease {
	return protocol.Lease{LeaseID: id, AttemptID: "a-" + id, Epoch: epoch, Token: "opaque", CompatMode: protocol.Compat15,
		Job: model.Job{Kind: "static"}, Secrets: []protocol.SecretRef{}, ExpiresAt: "2026-10-08T20:00:00Z"}
}

// rig runs one session on a fake carrier.
type rig struct {
	t      *testing.T
	car    *fakeCarrier
	clock  *manualClock
	state  string
	table  *slots.Table
	cancel context.CancelFunc
	done   chan error
	ended  chan struct{}
}

type rigOpt func(*Config)

func startRig(t *testing.T, runner Runner, opts ...rigOpt) *rig {
	t.Helper()
	state := t.TempDir()
	table, err := slots.Open(state, 2)
	if err != nil {
		t.Fatal(err)
	}
	return startRigOn(t, state, table, "coord-a", runner, opts...)
}

func startRigOn(t *testing.T, state string, table *slots.Table, coord string, runner Runner, opts ...rigOpt) *rig {
	t.Helper()
	return startRigCar(t, state, table, coord, runner, newFakeCarrier(), opts...)
}

// startRigWith runs a session on a caller-supplied carrier (r.car stays a
// fakeCarrier view of it for pushing).
func startRigWith(t *testing.T, state string, car interface {
	Carrier
	fake() *fakeCarrier
}, runner Runner, opts ...rigOpt) *rig {
	t.Helper()
	table, err := slots.Open(state, 2)
	if err != nil {
		t.Fatal(err)
	}
	return startRigCar(t, state, table, "coord-a", runner, car, opts...)
}

func startRigCar(t *testing.T, state string, table *slots.Table, coord string, runner Runner, carrier Carrier, opts ...rigOpt) *rig {
	t.Helper()
	fc, _ := carrier.(*fakeCarrier)
	if w, ok := carrier.(interface{ fake() *fakeCarrier }); ok {
		fc = w.fake()
	}
	r := &rig{t: t, car: fc, clock: newManualClock(), state: state, table: table, done: make(chan error, 1), ended: make(chan struct{})}
	cfg := Config{Carrier: carrier, Key: fakeKey{}, Slots: table, Runner: runner, Coordinator: Coordinator{ID: coord, RootFingerprint: "fp-" + coord},
		StateDir: state, Clock: r.clock, Hooks: []JobHook{}, Facts: Facts{AgentVersion: "1.5.0", MachineID: "m1"}}
	for _, o := range opts {
		o(&cfg)
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	t.Cleanup(func() { // stop the session before its temp dirs are removed
		cancel()
		select {
		case <-r.ended:
		case <-time.After(10 * time.Second):
			t.Error("session did not stop")
		}
	})
	go func() { r.done <- s.Run(ctx); close(r.ended) }()
	return r
}

// handshake consumes hello and answers welcome.
func (r *rig) handshake() protocol.Hello {
	h := r.car.next(r.t, "hello").Body.(protocol.Hello)
	r.car.push("welcome", testWelcome)
	return h
}

func (r *rig) wait() error {
	r.t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(10 * time.Second):
		r.t.Fatal("session did not end")
		return nil
	}
}

func (r *rig) lease(l protocol.Lease) { r.car.pushFenced("lease", l, l.Epoch, l.LeaseID) }

func okRunner(ev string) Runner {
	return RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		return JobResult{Outcome: Outcome{State: "passed"}, Evidence: json.RawMessage(ev)}, nil
	})
}
