// Package agent is the pull transport, carrier B (Epic D5, D6): the coordinator side that hands a lease to the
// node's agent over the mTLS listener (internal/coordinator/agentapi), and the agent side that keeps the
// outbound connection (client.go, carrier.go).
//
// Purpose: driver.go is the `agent` transport Driver: it signs a single-job token for the lease, queues the lease
//
//	for the node's long-poll, waits for the result, and cancels the job when the run is cancelled.
//
// Inputs:  a model.Capability (the node), a protocol.Lease, a transport.Sink.
// Outputs: the lease on the node's long-poll (Next), calls on the run's Sink (sink.go), E656 on a lost lease.
// Constraints: the token in the lease is signed for this lease, node, attempt and epoch and expires at job
//
//	timeout + 5 minutes; a lease carries secret references only, never values (X8); Run ends when the
//	result was accepted, when the run is cancelled and the cancel grace passed, or when the lease is
//	gone from the store (expired or the attempt ended).
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// Kind is the transport driver name (capability identity.transport).
const Kind = "agent"

// DefaultJobTimeout caps the token when a job declares no timeout.
const DefaultJobTimeout = 30 * time.Minute

// maxQueued bounds the undelivered leases of one node.
const maxQueued = 1024

// Minter signs job tokens (pki.Tokens).
type Minter interface {
	Mint(cl pki.TokenClaims, jobTimeout time.Duration) (string, error)
}

// Driver is the pull transport on the coordinator.
type Driver struct {
	// Gate vets lease-scoped calls (agentapi.Config.Admit) and spends a token when its result is accepted.
	Gate *Gate
	// Minter signs the lease's job token. Run refuses to start without it.
	Minter Minter
	// LogDir holds the durable per-lease logs (JSONL with line_seq); empty forwards logs without a copy.
	LogDir string
	// Cancel asks the node's agent to stop a lease (agentapi.Server.Cancel); nil means no control stream.
	Cancel func(node, leaseID, reason string) error
	// Leases reads the lease row, to notice a lease that expired or an attempt that ended.
	Leases LeaseView
	// CancelGrace is how long Run waits for the cancelled job's result (default 5 s).
	CancelGrace time.Duration
	// Poll is the period of the lease liveness check (default 1 s).
	Poll time.Duration
	// Logf receives operational notes (never a token or a value).
	Logf func(format string, a ...any)

	mu        sync.Mutex
	queues    map[string][]protocol.Lease // by node id, oldest first
	routes    map[string]*route           // by lease id
	finished  map[string]bool             // lease ids whose Run returned, bounded
	order     []string
	lastPrune time.Time
	pending   map[string]map[string]string // node -> lease id -> reason: cancels that found no control stream
}

// Wiring is what the coordinator's composition root sets once its store and CA exist.
type Wiring struct {
	Gate   *Gate
	Minter Minter
	Leases LeaseView
	LogDir string
	Logf   func(format string, a ...any)
}

// Wire installs the wiring; it is safe while runs are in flight.
func (d *Driver) Wire(w Wiring) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Gate, d.Minter, d.Leases, d.LogDir, d.Logf = w.Gate, w.Minter, w.Leases, w.LogDir, w.Logf
}

// SetCancel installs the function that asks an agent to stop a lease (agentapi.Server.Cancel).
func (d *Driver) SetCancel(fn func(node, leaseID, reason string) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Cancel = fn
}

// cfg is a consistent snapshot of the settings.
type cfg struct {
	gate   *Gate
	minter Minter
	leases LeaseView
	logDir string
	logf   func(string, ...any)
	cancel func(node, leaseID, reason string) error
}

func (d *Driver) cfg() cfg {
	d.mu.Lock()
	defer d.mu.Unlock()
	return cfg{gate: d.Gate, minter: d.Minter, leases: d.Leases, logDir: d.LogDir, logf: d.Logf, cancel: d.Cancel}
}

// Default is the driver registered for kind "agent"; the composition root sets its fields.
var Default = &Driver{}

func init() {
	if err := transport.Register(Default); err != nil {
		panic(err)
	}
}

// Kind implements transport.Driver.
func (*Driver) Kind() string { return Kind }

type route struct {
	node, leaseID, jti string
	sink               transport.Sink
	done               chan struct{}
	once               sync.Once
	lw                 *LogWriter
}

func (r *route) finish() { r.once.Do(func() { close(r.done) }) }

func (d *Driver) logf(format string, a ...any) {
	if f := d.cfg().logf; f != nil {
		f(format, a...)
	}
}

func (d *Driver) grace() time.Duration {
	if d.CancelGrace > 0 {
		return d.CancelGrace
	}
	return 5 * time.Second
}

func (d *Driver) poll() time.Duration {
	if d.Poll > 0 {
		return d.Poll
	}
	return time.Second
}

// jobTimeout is the timeout a job declares, or the default.
func jobTimeout(j model.Job) time.Duration {
	if t, err := time.ParseDuration(j.Timeout); err == nil && t > 0 {
		return t
	}
	return DefaultJobTimeout
}

func newJTI() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// pruneEvery is how often Run clears expired token bindings.
const pruneEvery = 10 * time.Minute

// prune removes expired token bindings, at most once per pruneEvery.
func (d *Driver) prune(ctx context.Context, g *Gate) {
	if g == nil {
		return
	}
	now := time.Now()
	d.mu.Lock()
	due := now.Sub(d.lastPrune) >= pruneEvery
	if due {
		d.lastPrune = now
	}
	d.mu.Unlock()
	if due {
		if _, err := g.Prune(ctx, now); err != nil {
			d.logf("agent transport: pruning token bindings: %v", err)
		}
	}
}

// Run delivers lease to the node's agent and returns when the job is over.
func (d *Driver) Run(ctx context.Context, node model.Capability, lease protocol.Lease, sink transport.Sink) error {
	c := d.cfg()
	if c.minter == nil || c.leases == nil {
		return fail(ReasonNotConfigure, "the agent transport has no token minter or lease view wired")
	}
	nodeID := node.Identity.ID
	jti := newJTI()
	tok, err := c.minter.Mint(pki.TokenClaims{JTI: jti, JobID: lease.AttemptID, AttemptID: lease.AttemptID, Revision: lease.Checkout.Revision, RunnerID: nodeID, Epoch: int64(lease.Epoch)}, jobTimeout(lease.Job))
	if err != nil {
		return fail(ReasonNotConfigure, "cannot sign the job token: %v", err)
	}
	lease.Token = tok
	d.prune(ctx, c.gate)
	rt := &route{node: nodeID, leaseID: lease.LeaseID, jti: jti, sink: sink, done: make(chan struct{})}
	if err = d.enqueue(rt, lease); err != nil {
		return err
	}
	defer d.release(rt)
	tick := time.NewTicker(d.poll())
	defer tick.Stop()
	for {
		select {
		case <-rt.done:
			return nil
		case <-ctx.Done():
			return d.cancelRun(rt)
		case <-tick.C:
			if gone, why := d.leaseGone(ctx, rt); gone {
				return fail(ReasonLeaseLost, "lease %s on node %s ended before a result arrived (%s)", rt.leaseID, rt.node, why)
			}
		}
	}
}
