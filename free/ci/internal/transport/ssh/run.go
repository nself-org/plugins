package ssh

// Purpose: Run, the lifecycle of one lease over one ssh session: bundle,
// connect, handshake, serve, close.
// Inputs:  model.Capability, protocol.Lease, transport.Sink.
// Outputs: nil after the agent's result reached the Sink; otherwise a *Error
// (host key E655, transport E656 incl. session lost) or the context's error.
// Constraints: the session always ends with the ssh process reaped; cancel
// sends a protocol cancel first and closes the session only after a grace
// period, and closing stdin makes the agent kill the job's process group.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

type frame struct {
	m   protocol.Message
	err error
}

// conn state of one running session.
type sess struct {
	d      *Driver
	node   model.Capability
	lease  protocol.Lease
	sink   transport.Sink
	conn   *Conn
	car    session.Carrier
	stderr *tailBuffer

	sendMu sync.Mutex
	seq    uint64

	frames     chan frame
	stop       chan struct{}
	readerDone chan struct{}
	errDone    <-chan struct{}
	blobs      map[string]*blobState
	blobCount  int             // blobs opened in this session
	committed  map[string]bool // sha256 of blobs received whole and verified
	lastAlive  atomic.Int64    // unix nanos of the last frame that proves the agent works on this lease
	blobBytes  int64           // bytes announced in this session

	helloTimedOut atomic.Bool // the hello timer killed ssh
}

// Run implements transport.Driver.
func (d *Driver) Run(ctx context.Context, node model.Capability, lease protocol.Lease, sink transport.Sink) error {
	if d.Renew == nil && !d.NoRenew {
		return newErr(CodeTransport, ReasonNotConfigured, "the ssh transport has no lease renewal wired (Driver.Renew): a lease would expire under a running job")
	}
	if d.Personal && d.Secrets != nil && len(lease.Secrets) > 0 && !secretsEligible(node) {
		return newErr(CodeTransport, ReasonSecretsRefused, "node %s is not an operator-owned, uid-separated node (secrets_eligible); it will not be sent secret values", node.Identity.ID)
	}
	tgt, err := d.target(ctx, node)
	if err != nil {
		return err
	}
	bundle := ""
	if lease.Checkout.Revision != "" {
		var cleanup func()
		if bundle, cleanup, err = d.makeBundle(ctx, &lease); err != nil {
			return err
		}
		defer cleanup()
		if d.afterBundle != nil {
			d.afterBundle(bundle)
		}
	}
	conn, err := d.start()(ctx, tgt, d.command(node, lease))
	if err != nil {
		return newErr(CodeTransport, ReasonConnect, "cannot start ssh for node %s: %v", node.Identity.ID, err)
	}
	s := &sess{d: d, node: node, lease: lease, sink: sink, conn: conn, frames: make(chan frame, 64),
		stop: make(chan struct{}), readerDone: make(chan struct{}), blobs: map[string]*blobState{}, committed: map[string]bool{}}
	s.stderr, s.errDone = drain(conn.Stderr)
	defer s.close()
	if err := s.handshake(ctx); err != nil {
		return err
	}
	return s.serve(ctx, bundle)
}

// makeBundle packs lease.Checkout.Revision and records its digest and size in the lease.
func (d *Driver) makeBundle(ctx context.Context, l *protocol.Lease) (string, func(), error) {
	dir, err := os.MkdirTemp("", "nself-ci-bundle-")
	if err != nil {
		return "", nil, newErr(CodeTransport, ReasonCheckout, "cannot stage the checkout bundle: %v", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	repo := d.RepoDir
	if repo == "" {
		if repo, err = os.Getwd(); err != nil {
			cleanup()
			return "", nil, newErr(CodeTransport, ReasonCheckout, "no repository directory")
		}
	}
	path := filepath.Join(dir, "checkout.bundle")
	digest, size, err := checkout.Create(ctx, d.Git, repo, l.Checkout.Revision, path)
	if err != nil {
		cleanup()
		e := newErr(CodeTransport, ReasonCheckout, "%v", err)
		e.Err = err
		return "", nil, e
	}
	if size > checkout.MaxBundle {
		cleanup()
		return "", nil, newErr(CodeTransport, ReasonCheckout, "the checkout bundle is %d bytes; a node accepts at most %d", size, checkout.MaxBundle)
	}
	l.Checkout.BundleDigest, l.Checkout.Size = digest, size
	return path, cleanup, nil
}

// handshake skips shell noise, reads hello and answers welcome (or reject).
func (s *sess) handshake(ctx context.Context) error {
	done := make(chan struct{})
	defer close(done)
	timer := time.AfterFunc(s.d.helloTimeout(), func() { s.helloTimedOut.Store(true); _ = s.conn.Kill() })
	defer timer.Stop()
	go func() {
		select {
		case <-ctx.Done():
			_ = s.conn.Kill()
		case <-done:
		}
	}()
	fail := func(err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.connectFailure(err)
	}
	r, err := protocol.SkipPreamble(s.conn.Stdout)
	if err != nil {
		return fail(err)
	}
	personal := s.d.sendsSecrets(s.node, s.lease)
	policy := protocol.CarrierPolicy{Carrier: "A", Personal: personal, OperatorOwned: personal}
	if s.car, err = session.NewStreamCarrier(r, s.conn.Stdin, policy, false); err != nil {
		return fail(err)
	}
	go s.readLoop()
	var it frame
	select {
	case it = <-s.frames:
	case <-ctx.Done():
		return ctx.Err()
	}
	if it.err != nil {
		return fail(it.err)
	}
	hello, ok := it.m.Body.(*protocol.Hello)
	if !ok {
		return newErr(CodeTransport, ReasonHandshake, "expected hello from node %s", s.node.Identity.ID)
	}
	var id [8]byte
	_, _ = rand.Read(id[:])
	body, _ := protocol.AcceptHello(*hello, s.node.Identity.AgeRecipient.Value, []int{1}, s.d.minAgent(), hex.EncodeToString(id[:]),
		int(s.d.heartbeat()/time.Millisecond), int(s.d.agentTTL()/time.Millisecond))
	if rej, rejected := body.(protocol.Reject); rejected {
		_ = s.send("reject", rej, false)
		return newErr(rej.Code, ReasonHandshake, "node %s rejected: %s", s.node.Identity.ID, rej.Reason)
	}
	return s.send("welcome", body, false)
}

func (d *Driver) minAgent() string {
	if d.MinAgentVersion != "" {
		return d.MinAgentVersion
	}
	return "0.0.0"
}

// connectFailure classifies an ssh process that ended before hello. The stderr drain is
// finished before Wait, because Wait closes the pipe and would cut off the tail that names
// the host key refusal.
func (s *sess) connectFailure(cause error) error {
	_ = s.conn.Stdin.Close()
	select {
	case <-s.errDone:
	case <-time.After(2 * time.Second):
		_ = s.conn.Kill()
		select {
		case <-s.errDone:
		case <-time.After(time.Second):
		}
	}
	waited := make(chan error, 1)
	go func() { waited <- s.conn.Wait() }()
	var werr error
	select {
	case werr = <-waited:
	case <-time.After(2 * time.Second):
		_ = s.conn.Kill()
		werr = <-waited
	}
	e := classify(s.node.Identity.ID, s.stderr.String(), werr)
	if s.helloTimedOut.Load() && e.Reason == ReasonConnect {
		e = newErr(CodeTransport, ReasonConnectTimeout, "node %s did not answer in time", s.node.Identity.ID)
	}
	if e.Reason == ReasonConnect && e.Err == nil {
		e.Err = cause
	}
	return e
}

func (s *sess) send(kind string, body any, scoped bool) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	m := protocol.Message{V: 1, Type: kind, Seq: s.seq + 1, Body: body}
	if scoped {
		e := s.lease.Epoch
		m.Epoch, m.LeaseID = &e, s.lease.LeaseID
	}
	if err := s.car.Send(m); err != nil {
		return err
	}
	s.seq++
	return nil
}

func (s *sess) readLoop() {
	defer close(s.readerDone)
	for {
		m, err := s.car.Recv()
		if err == nil {
			s.noteAlive(m)
		}
		select {
		case s.frames <- frame{m, err}:
		case <-s.stop:
			return
		}
		if err != nil {
			return
		}
	}
}

// close ends the session: flush, close stdin (the agent then kills the job's
// process group), reap ssh, stop the reader and discard any blob left half written.
func (s *sess) close() {
	close(s.stop)
	if s.car != nil {
		_ = s.car.Close()
	}
	_ = s.conn.Stdin.Close()
	if s.car != nil {
		waitOr(s.readerDone, 3*time.Second, s.conn.Kill)
	}
	waitOr(s.errDone, 2*time.Second, s.conn.Kill)
	waited := make(chan struct{})
	go func() { _ = s.conn.Wait(); close(waited) }()
	waitOr(waited, 3*time.Second, s.conn.Kill)
	for stream, st := range s.blobs {
		s.discard(st)
		delete(s.blobs, stream)
	}
}

// waitOr waits for done; after d it runs kill and waits once more, for as long again.
func waitOr(done <-chan struct{}, d time.Duration, kill func() error) {
	select {
	case <-done:
	case <-time.After(d):
		_ = kill()
		select {
		case <-done:
		case <-time.After(d):
		}
	}
}
