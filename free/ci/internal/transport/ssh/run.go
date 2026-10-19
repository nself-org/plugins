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
}

// Run implements transport.Driver.
func (d *Driver) Run(ctx context.Context, node model.Capability, lease protocol.Lease, sink transport.Sink) error {
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
	}
	conn, err := d.start()(ctx, tgt, d.command(lease))
	if err != nil {
		return newErr(CodeTransport, ReasonConnect, "cannot start ssh for node %s: %v", node.Identity.ID, err)
	}
	s := &sess{d: d, node: node, lease: lease, sink: sink, conn: conn, frames: make(chan frame, 64),
		stop: make(chan struct{}), readerDone: make(chan struct{}), blobs: map[string]*blobState{}}
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
	l.Checkout.BundleDigest, l.Checkout.Size = digest, size
	return path, cleanup, nil
}

// handshake skips shell noise, reads hello and answers welcome (or reject).
func (s *sess) handshake(ctx context.Context) error {
	done := make(chan struct{})
	defer close(done)
	timer := time.AfterFunc(s.d.helloTimeout(), func() { _ = s.conn.Kill() })
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
	personal := s.d.Personal && len(s.lease.Secrets) > 0 && s.d.Secrets != nil
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

// connectFailure classifies an ssh process that ended before hello.
func (s *sess) connectFailure(cause error) error {
	_ = s.conn.Stdin.Close()
	waited := make(chan error, 1)
	go func() { waited <- s.conn.Wait() }()
	var werr error
	select {
	case werr = <-waited:
	case <-time.After(2 * time.Second):
		_ = s.conn.Kill()
		werr = <-waited
	}
	select {
	case <-s.errDone:
	case <-time.After(time.Second):
	}
	e := classify(s.node.Identity.ID, s.stderr.String(), werr)
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
// process group), reap ssh, and stop the reader.
func (s *sess) close() {
	close(s.stop)
	if s.car != nil {
		_ = s.car.Close()
	}
	_ = s.conn.Stdin.Close()
	if s.car != nil {
		select {
		case <-s.readerDone:
		case <-time.After(3 * time.Second):
			_ = s.conn.Kill()
			<-s.readerDone
		}
	}
	select {
	case <-s.errDone:
	case <-time.After(2 * time.Second):
		_ = s.conn.Kill()
		<-s.errDone
	}
	waited := make(chan struct{})
	go func() { _ = s.conn.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(3 * time.Second):
		_ = s.conn.Kill()
		<-waited
	}
}
