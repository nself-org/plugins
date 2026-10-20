//go:build integration

package reliability

// Purpose: the coordinator side of the lab: the real store, PKI, token gate, lease engine, agent listener and agent
//          transport driver, wired the way cmd/transport_agent.go wires them, plus the expiry sweeper.
// Inputs:  timings (heartbeat, coordinator TTL) and the address nodes reach the host on.
// Outputs: a listener that can be taken down and brought up again on the same store (kill and restart), and the
//          times of the agent acks it has seen.
// Constraints: tests only; fake synthetic values; the listener is always closed by t.Cleanup.

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

// timing is a coordinator profile. The agent fences at coordinator_ttl - 2 x heartbeat after its last ack.
type timing struct{ heartbeat, ttl time.Duration }

// agentTTL is when an agent that hears nothing kills its job.
func (p timing) agentTTL() time.Duration { return p.ttl - 2*p.heartbeat }

// Profiles: fast keeps each test under a minute; outage keeps the heartbeat short so a 20 s coordinator
// outage starts at most a heartbeat after the last ack, and its agent_ttl (56 s) leaves a 20 s outage a wide
// margin on a loaded machine (twenty agents and their containers share one host).
var (
	fast   = timing{heartbeat: time.Second, ttl: 6 * time.Second}      // agent_ttl 4 s
	outage = timing{heartbeat: 2 * time.Second, ttl: 60 * time.Second} // agent_ttl 56 s
)

type coord struct {
	t    *testing.T
	home string
	st   *store.Store
	ca   *pki.CA
	eng  *lease.Engine
	tok  *pki.Tokens
	ips  []net.IP
	p    timing
	port int
	bind string

	mu        sync.Mutex
	srv       *agentapi.Server
	done      chan error
	acks      map[string]time.Time
	nAcks     atomic.Int64
	stopSweep func()
	ackHook   atomic.Pointer[func(node, leaseID string)] // the drain hears every ack that names a lease
	gen       int                                        // how many times the coordinator has been started
	logMu     sync.Mutex
	log       []string // the listener's own log lines (refusals carry their E66x code)
}

// newCoord opens the store and the PKI under home and starts the listener on bind ("0.0.0.0:0" for a free port).
func newCoord(t *testing.T, home string, ips []net.IP, p timing) *coord {
	t.Helper()
	c := &coord{t: t, home: home, ips: append([]net.IP{net.ParseIP("127.0.0.1")}, ips...), p: p, acks: map[string]time.Time{}, bind: "0.0.0.0:0"}
	var err error
	if c.st, err = store.Open(home+"/state.db", store.Options{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.st.Close() })
	if c.ca, err = pki.New(context.Background(), pki.Options{Store: c.st, DataDir: home}); err != nil {
		t.Fatal(err)
	}
	if c.tok, err = c.ca.Tokens(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.eng = lease.New(c.st, registry.New(c.st), lease.Config{Heartbeat: p.heartbeat, TTL: p.ttl})
	return c
}

// up starts the listener (on the port of the first start when it is a restart) and the sweeper, wires the
// agent driver and gives the leases of the dead listener their restart grace, as `ci serve` does at start.
func (c *coord) up() {
	c.t.Helper()
	gate := &agent.Gate{Tokens: c.tok, Bindings: c.st, Leases: c.st}
	drv := agent.Default
	drv.Wire(agent.Wiring{Gate: gate, Minter: c.tok, Leases: c.st, LogDir: c.home + "/agent-logs", Logf: c.logf})
	drv.Poll, drv.CancelGrace = 50*time.Millisecond, 5*time.Second
	leaf := &agentapi.LeafSource{CA: c.ca, ID: "coord-reliability", IPs: c.ips}
	cfg := agentapi.Config{CA: c.ca, Guard: pki.NewGuard(c.st), Cert: leaf.Get, Leases: c.eng, Sink: drv.Sink(), Source: drv, Store: c.st,
		Admit: drv.Admit, Blobs: drv.Blobs(), Heartbeat: c.p.heartbeat, CoordinatorTTL: c.p.ttl, PollTimeout: 2 * time.Second,
		SourcePoll: 50 * time.Millisecond, GuardEvery: 100 * time.Millisecond, Logf: c.logf, OnAck: c.onAck}
	srv, err := agentapi.New(cfg)
	if err != nil {
		c.t.Fatal(err)
	}
	addr := c.bind
	if c.port != 0 {
		addr = net.JoinHostPort("0.0.0.0", itoa(c.port))
	}
	var ln net.Listener
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if ln, err = net.Listen("tcp", addr); err == nil {
			break
		}
	}
	if err != nil {
		c.t.Fatalf("listen %s: %v", addr, err)
	}
	c.port = ln.Addr().(*net.TCPAddr).Port
	drv.SetCancel(srv.Cancel)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	c.mu.Lock()
	c.srv, c.done = srv, done
	c.mu.Unlock()
	c.gen++ // a restarted `ci serve` registers as a new coordinator: that is what gives the old leases their grace
	if _, err = c.eng.Start(context.Background(), "coord-reliability-"+itoa(c.gen), c.incarnation()); err != nil {
		c.t.Fatalf("restart grace: %v", err)
	}
	c.startSweeper()
}

func (c *coord) incarnation() int64 {
	inc, _ := c.st.HighestIncarnation(context.Background())
	return inc
}

func (c *coord) onAck(node, leaseID string) {
	c.nAcks.Add(1)
	if h := c.ackHook.Load(); h != nil {
		(*h)(node, leaseID)
	}
	c.mu.Lock()
	c.acks[node] = time.Now()
	c.mu.Unlock()
}

// lastAck is when the node's last lease ack arrived (zero: never).
func (c *coord) lastAck(node string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.acks[node]
}

// url is the base URL nodes dial.
func (c *coord) url(host string) string { return "https://" + net.JoinHostPort(host, itoa(c.port)) }

// connected is the set of node ids with a live session.
func (c *coord) connected() []string {
	c.mu.Lock()
	srv := c.srv
	c.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.NodesConnected()
}

// down takes the listener away without a drain: every connection is closed at once and nothing is sent. The
// run loop, the store and the lease rows stay (the coordinator process keeps its runs; only the agents lose it).
func (c *coord) down() {
	c.t.Helper()
	c.stopSweeper()
	c.mu.Lock()
	srv, done := c.srv, c.done
	c.srv = nil
	c.mu.Unlock()
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	<-done
}

func (c *coord) startSweeper() { c.stopSweep = c.eng.StartSweeper(200*time.Millisecond, c.t.Logf) }

func (c *coord) stopSweeper() {
	if c.stopSweep != nil {
		c.stopSweep()
		c.stopSweep = nil
	}
}

// logf records a line of the listener or the driver (and shows it in the test log).
func (c *coord) logf(format string, a ...any) {
	line := fmt.Sprintf(format, a...)
	c.logMu.Lock()
	c.log = append(c.log, line)
	c.logMu.Unlock()
	c.t.Log(line)
}

// logged returns the logged lines that contain every one of subs.
func (c *coord) logged(subs ...string) []string {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	var out []string
next:
	for _, l := range c.log {
		for _, s := range subs {
			if !strings.Contains(l, s) {
				continue next
			}
		}
		out = append(out, l)
	}
	return out
}

// cancel asks the agent that holds a lease to stop it (agentapi.Server.Cancel); the live listener answers.
func (c *coord) cancel(node, leaseID, reason string) error {
	c.mu.Lock()
	srv := c.srv
	c.mu.Unlock()
	if srv == nil {
		return fmt.Errorf("the listener is down")
	}
	return srv.Cancel(node, leaseID, reason)
}

// countAttempts sets *n to the number of attempt rows of a job.
func (c *coord) countAttempts(n *int, jobID string) {
	c.t.Helper()
	for i := 0; ; i++ {
		_, _, ok, err := c.st.AttemptAt(context.Background(), jobID, i+1)
		if err != nil {
			c.t.Fatal(err)
		}
		if !ok {
			*n = i
			return
		}
	}
}
