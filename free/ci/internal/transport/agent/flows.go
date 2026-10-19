package agent

// Purpose: the connection management of the pull carrier: the handshake, the three flows that run against one
//          transport session (lease long-poll, control stream, upstream stream) and the loop that rebuilds
//          them with backoff after any failure.
// Inputs:  the carrier state (hello frame, buffered frames, tokens).
// Outputs: messages for Recv; the upstream stream; session id and seq bookkeeping.
// Constraints: all three flows share one context: the first failure stops the other two and the loop waits
//          out the backoff; "no session" from the coordinator (it restarted) starts a new transport session,
//          any other failure keeps the session and resumes it; a hello refusal that is a protocol reject
//          (E652, E660 node_key_changed) is delivered to the session and ends the carrier; the lease flow
//          registers the lease's token with the upstream stream before it hands the lease on.

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// healthyAfter is how long a connection must last before the backoff starts over.
const healthyAfter = 30 * time.Second

var (
	errResync      = errors.New("carrier: the coordinator and the agent disagree about the stream position")
	errStreamEnded = errors.New("carrier: the coordinator ended the stream")
	errNoHello     = errors.New("carrier: no hello to send")
)

// permanent wraps an error that ends the carrier (the session has been told).
type permanent struct{ error }

func (c *Carrier) logf(format string, a ...any) { c.cfg.Logf(format, a...) }

func (c *Carrier) session() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sess
}

// loop connects, runs the flows and reconnects until the carrier is closed.
func (c *Carrier) loop(ctx context.Context) {
	defer func() {
		close(c.done)
		close(c.inbox)
	}()
	bo := c.cfg.Backoff
	for {
		if err := c.waitHello(ctx); err != nil {
			return
		}
		began := time.Now()
		err := c.once(ctx)
		if ctx.Err() != nil {
			return
		}
		var perm *permanent
		if errors.As(err, &perm) {
			c.mu.Lock()
			c.err = perm.error
			c.mu.Unlock()
			return
		}
		var he *HTTPError
		if errors.As(err, &he) && he.noSession() || errors.Is(err, errResync) {
			c.mu.Lock()
			c.sess = ""
			c.mu.Unlock()
		}
		if time.Since(began) >= healthyAfter {
			bo.Reset()
		}
		d := bo.Next()
		c.logf("pull: connection lost (%v); retrying in %s", err, d.Round(time.Millisecond))
		if c.cfg.Sleep(ctx, d) != nil {
			return
		}
	}
}

// waitHello blocks until the session sent its hello.
func (c *Carrier) waitHello(ctx context.Context) error {
	for {
		c.mu.Lock()
		have := c.hello != nil
		c.mu.Unlock()
		if have {
			return nil
		}
		select {
		case <-c.wake:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// once runs one connection: handshake if needed, then the three flows until one fails.
func (c *Carrier) once(ctx context.Context) error {
	if c.session() == "" {
		if err := c.handshake(ctx); err != nil {
			return err
		}
	}
	fctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, 3)
	var wg sync.WaitGroup
	for _, f := range []func(context.Context) error{c.pollLeases, c.readControl, c.upload} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- f(fctx)
		}()
	}
	err := <-errs
	cancel()
	wg.Wait()
	if err == nil {
		err = errStreamEnded
	}
	return err
}

// handshake says hello. A welcome opens a transport session; a reject ends the carrier.
func (c *Carrier) handshake(ctx context.Context) error {
	c.mu.Lock()
	frame := c.hello
	c.mu.Unlock()
	if frame == nil {
		return errNoHello
	}
	m, sess, err := c.cl.hello(ctx, frame)
	if err != nil {
		return err
	}
	switch m.Body.(type) {
	case *protocol.Welcome:
	case *protocol.Reject:
		if derr := c.deliver(ctx, m); derr != nil {
			return derr
		}
		return &permanent{errors.New("carrier: the coordinator rejected hello")}
	default:
		return &protocol.Error{Code: protocol.CodeInvalid, Reason: "expected_welcome"}
	}
	if sess == "" {
		return &protocol.Error{Code: protocol.CodeInvalid, Reason: "no_session_header"}
	}
	c.mu.Lock()
	c.sess = sess
	first := !c.helloSent
	c.helloSent = true
	c.resetTransport()
	c.mu.Unlock()
	if first {
		return c.deliver(ctx, m)
	}
	return nil
}

// resetTransport renumbers the buffered frames for a new transport session; the caller holds c.mu. Old
// heartbeats and acks are dropped (stale), and the latest heartbeat goes first so the coordinator reconciles
// the leases this agent kept.
func (c *Carrier) resetTransport() {
	c.nextT, c.confirmed, c.written = 0, 0, 0
	keep := c.out[:0]
	c.outBytes = 0
	for _, f := range c.out {
		if f.m.Type == "heartbeat" || f.m.Type == "ack" {
			continue
		}
		f.tseq = 0
		keep = append(keep, f)
		c.outBytes += f.size
	}
	c.out = keep
	if c.lastBeat != nil {
		b, _ := encode(*c.lastBeat, 1)
		c.out = append([]*outFrame{{m: *c.lastBeat, size: len(b)}}, c.out...)
		c.outBytes += len(b)
	}
}

// pollLeases long-polls for leases and hands each to the session after its token reached the stream.
func (c *Carrier) pollLeases(ctx context.Context) error {
	for {
		m, err := c.cl.lease(ctx, c.session())
		if err != nil {
			return err
		}
		if m == nil {
			continue
		}
		l, ok := m.Body.(*protocol.Lease)
		if !ok {
			return &protocol.Error{Code: protocol.CodeInvalid, Reason: "frame_type"}
		}
		if err = c.registerToken(ctx, l.LeaseID, l.Token); err != nil {
			return err
		}
		if err = c.deliver(ctx, *m); err != nil {
			return err
		}
	}
}

// registerToken records the token of a lease and waits until the upstream stream carries it.
func (c *Carrier) registerToken(ctx context.Context, leaseID, token string) error {
	if token == "" {
		return nil
	}
	c.mu.Lock()
	if c.tokens[leaseID] != token {
		c.tokens[leaseID] = token
		c.gen++
	}
	want := c.gen
	c.mu.Unlock()
	c.signal(c.wake)
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		c.mu.Lock()
		ok, ch := c.liveGen >= want, c.up
		c.mu.Unlock()
		if ok {
			return nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("carrier: the upstream stream did not pick up the job token")
		}
	}
}

// readControl keeps the control stream open and hands its frames to the session.
func (c *Carrier) readControl(ctx context.Context) error {
	fr, closer, err := c.cl.control(ctx, c.session())
	if err != nil {
		return err
	}
	defer closer.Close()
	for {
		m, err := fr.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return errStreamEnded
			}
			return err
		}
		if err = c.deliver(ctx, m); err != nil {
			return err
		}
	}
}

// tokenSnapshot returns the current token generation and a copy of the tokens.
func (c *Carrier) tokenSnapshot() (int, map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make(map[string]string, len(c.tokens))
	for k, v := range c.tokens {
		cp[k] = v
	}
	return c.gen, cp
}

func (c *Carrier) setLive(gen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen > c.liveGen {
		c.liveGen = gen
		close(c.up)
		c.up = make(chan struct{})
	}
}

// adopt takes the coordinator's last accepted seq as confirmed when it lies between what the agent knew and
// what it sent.
func (c *Carrier) adopt(last uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if last < c.confirmed || last > c.nextT {
		return false
	}
	c.trim(last)
	return true
}

func (c *Carrier) confirmedSeq() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.confirmed
}
