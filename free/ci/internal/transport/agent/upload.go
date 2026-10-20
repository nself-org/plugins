package agent

// Purpose: the upstream stream of the pull carrier: POST /v1/agent/stream with the buffered frames written in
//          order, the coordinator's answers read back (acks trim the buffer, cancels and rejects reach the
//          session), resume by seq after a drop, and a restart when a new job token must be carried.
// Inputs:  the carrier's frame buffer, token set and confirmed seq.
// Outputs: frames on the wire with transport seq numbers; Recv messages from the answers.
// Constraints: the claim sent with a stream is the last seq the agent knows was accepted; when the coordinator
//          accepted more (409 resume_seq with its last seq) the agent adopts that and skips those frames, so a
//          frame is never written twice into one transport session; a heartbeat acknowledgement reaches the
//          session as a heartbeat message with its ack field set (the agent fence reads it); a coordinator
//          reject ends the stream and the carrier reconnects.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// upload keeps one upstream stream open, restarting it when the token set changes.
func (c *Carrier) upload(ctx context.Context) error {
	for {
		gen, toks := c.tokenSnapshot()
		if err := c.streamOnce(ctx, gen, toks); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// open posts the stream, adopting the coordinator's position once if it is ahead of the agent's.
func (c *Carrier) open(ctx context.Context, gen int, toks map[string]string) (*http.Response, *io.PipeWriter, error) {
	sess := c.session()
	for try := 0; ; try++ {
		pr, pw := io.Pipe()
		resp, err := c.cl.stream(ctx, sess, c.confirmedSeq(), toks, pr)
		if err == nil {
			return resp, pw, nil
		}
		_ = pr.Close()
		_ = pw.Close()
		var he *HTTPError
		if errors.As(err, &he) && he.Status == http.StatusConflict && he.Reason == "resume_seq" && try < 2 {
			last, perr := strconv.ParseUint(he.LastSeq, 10, 64)
			if perr != nil || !c.adopt(last) {
				return nil, nil, errResync
			}
			continue
		}
		return nil, nil, err
	}
}

// streamOnce runs one stream until the token set changes (nil), the stream ends, or an error.
func (c *Carrier) streamOnce(ctx context.Context, gen int, toks map[string]string) error {
	resp, pw, err := c.open(ctx, gen, toks)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.setLive(gen)
	c.setStreaming(true)
	defer c.setStreaming(false)
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	readDone, writeDone := make(chan error, 1), make(chan error, 1)
	go func() { readDone <- c.readStream(rctx, resp.Body) }()
	go func() { writeDone <- c.writeFrames(rctx, pw, gen) }()
	select {
	case err = <-writeDone: // a restart (nil) or a write failure
		pw.CloseWithError(err)
		select { // a graceful close ends the response; wait for the answers still in flight
		case rerr := <-readDone:
			if err == nil && rerr != nil && !errors.Is(rerr, io.EOF) {
				return rerr
			}
		case <-time.After(5 * time.Second):
		}
		return err
	case err = <-readDone:
		cancel()
		pw.CloseWithError(io.ErrClosedPipe)
		<-writeDone
		if err == nil || errors.Is(err, io.EOF) {
			return errStreamEnded
		}
		return err
	}
}

// writeFrames writes the frames after the confirmed seq in order until the token generation moves on.
func (c *Carrier) writeFrames(ctx context.Context, w io.Writer, gen int) error {
	written := c.confirmedSeq()
	for {
		c.mu.Lock()
		if c.gen != gen {
			c.mu.Unlock()
			return nil
		}
		var f *outFrame
		for _, o := range c.out {
			if o.tseq == 0 || o.tseq > written {
				f = o
				break
			}
		}
		var tseq uint64
		if f != nil {
			if f.tseq == 0 {
				c.nextT++
				f.tseq = c.nextT
			}
			tseq = f.tseq
		}
		m := protocol.Message{}
		if f != nil {
			m = f.m
		}
		c.mu.Unlock()
		if f == nil {
			select {
			case <-c.wake:
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		b, err := encode(m, tseq)
		if err != nil {
			return err
		}
		if _, err = w.Write(b); err != nil {
			return err
		}
		written = tseq
		c.mu.Lock()
		if tseq > c.written {
			c.written = tseq
		}
		c.mu.Unlock()
	}
}

// readStream reads the coordinator's answers of one stream.
func (c *Carrier) readStream(ctx context.Context, body io.Reader) error {
	dec := protocol.NewDecoder(body, protocol.CarrierPolicy{Carrier: "B"})
	for {
		m, err := dec.Decode()
		if err != nil {
			return err
		}
		switch b := m.Body.(type) {
		case *protocol.Ack:
			c.mu.Lock()
			c.trim(b.AckSeq)
			c.mu.Unlock()
			if b.LeaseID != "" {
				continue
			}
			seq := b.AckSeq
			m = protocol.Message{Type: "heartbeat", Ack: &seq, Body: &protocol.Heartbeat{Leases: []protocol.LeaseState{}}}
		case *protocol.Reject:
			return &protocol.Error{Code: b.Code, Reason: b.Reason}
		}
		if err = c.deliver(ctx, m); err != nil {
			return err
		}
	}
}

func (c *Carrier) setStreaming(on bool) {
	c.mu.Lock()
	c.streaming = on
	c.mu.Unlock()
}
