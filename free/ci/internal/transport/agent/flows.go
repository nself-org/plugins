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

// healthy says a connection lasted long enough for the backoff to start over.
func healthy(lasted time.Duration) bool { return lasted >= healthyAfter }

// permanent wraps an error that ends the carrier (the session has been told).
type permanent struct{ error }

// capBackoff bounds a reconnect delay by a tenth of the agent_ttl the coordinator announced: an agent that is
// running a job must be back well before it fences itself, so an outage shorter than agent_ttl is survived even
// when the exponential delay has grown to its 60 s ceiling (D6, P7-NODE-11). ttl 0 (no welcome yet) leaves d.
func capBackoff(d, ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return d
	}
	return min(d, max(BackoffMin, ttl/10))
}

func (c *Carrier) ttl() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentTTL
}

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
	rehello := false
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
		forgotten := errors.As(err, &he) && he.noSession() || errors.Is(err, errResync)
		if forgotten {
			c.mu.Lock()
			c.sess = ""
			c.mu.Unlock()
		}
		if healthy(time.Since(began)) {
			bo.Reset()
		}
		if forgotten && !rehello {
			// The coordinator is up but forgot the session (it restarted): the hello it needs can go at once.
			// Waiting out another delay here would spend agent_ttl twice after a short outage (P7-NODE-11).
			// Once only: a second refusal in a row backs off as usual.
			rehello = true
			c.logf("pull: the coordinator has no session for this agent; saying hello again")
			continue
		}
		rehello = false
		d := capBackoff(bo.Next(), c.ttl())
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
	switch b := m.Body.(type) {
	case *protocol.Welcome:
		if b.AgentTTLMS > 0 {
			c.mu.Lock()
			c.agentTTL = time.Duration(b.AgentTTLMS) * time.Millisecond
			c.mu.Unlock()
		}
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
		if err = c.registerToken(ctx, *l); err != nil {
			return err
		}
		if err = c.deliver(ctx, *m); err != nil {
			return err
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
