package services

// Purpose: wait until a started service answers on its loopback port. A bare
// TCP connect is not enough: Docker's port proxy accepts connections before the
// service inside is up, then drops them. So after the connect the wait asks
// the service one protocol-level question where the protocol is known
// (Postgres SSLRequest, Redis PING) and otherwise requires the connection to
// stay open for a short moment.
// Inputs:  an address, a probe kind and a deadline. Outputs: nil when ready.
// Constraints: no credentials are sent; the wait stops at the deadline or when
// the context ends.

import (
	"context"
	"io"
	"net"
	"time"
)

// probeKind selects the protocol question.
type probeKind int

const (
	probeTCP probeKind = iota
	probePostgres
	probeRedis
)

// waitReady polls addr until it is ready or the deadline passes. A cancelled
// parent context is returned as such, not as a readiness failure.
func waitReady(ctx context.Context, addr string, kind probeKind, deadline time.Duration) error {
	dctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	var last error
	for {
		last = probeOnce(dctx, addr, kind)
		if last == nil {
			return nil
		}
		select {
		case <-dctx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fail(CodeProvision, "service at %s was not ready within %s: %v", addr, deadline, last)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func probeOnce(ctx context.Context, addr string, kind probeKind) error {
	d := net.Dialer{Timeout: 2 * time.Second}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 16)
	switch kind {
	case probePostgres:
		// SSLRequest: length 8, code 80877103. A live server answers 'S' or 'N'.
		if _, err = c.Write([]byte{0, 0, 0, 8, 0x04, 0xd2, 0x16, 0x2f}); err != nil {
			return err
		}
		n, err := c.Read(buf)
		if err != nil {
			return err
		}
		if n < 1 || (buf[0] != 'S' && buf[0] != 'N') {
			return io.ErrUnexpectedEOF
		}
		return nil
	case probeRedis:
		if _, err = c.Write([]byte("PING\r\n")); err != nil {
			return err
		}
		n, err := c.Read(buf)
		if err != nil {
			return err
		}
		if n < 1 || (buf[0] != '+' && buf[0] != '-') { // +PONG, or -NOAUTH when a password is set
			return io.ErrUnexpectedEOF
		}
		return nil
	}
	// Unknown protocol: the proxy drops a connection with no backend within a
	// few milliseconds, so one that survives 300 ms has a service behind it.
	_ = c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, err = c.Read(buf); err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil
		}
		return err
	}
	return nil
}
