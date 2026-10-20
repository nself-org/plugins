package agent

// Purpose: the agent half of carrier B: a session.Carrier over the mTLS pull connection. It keeps the frames
//          the session sent until the coordinator confirms them, reconnects with backoff, resumes the upstream
//          stream by seq, and starts a new transport session (hello again) when the coordinator forgot this one.
// Inputs:  Send (the session's frames, the first is hello), the coordinator's answers (welcome, lease, control
//          frames, acks, cancels).
// Outputs: Recv yields welcome, lease, cancel, drain, upgrade and a heartbeat message carrying the ack of each
//          acknowledged heartbeat (what the agent fence waits for).
// Constraints: frames keep their order; the transport seq of a frame is assigned when it is first written and is
//          renumbered from 1 after a new hello; a frame leaves the buffer only when the coordinator's ack or
//          its resume answer covers it; the first frame of a new transport session is the latest heartbeat,
//          which reconciles the leases the agent kept across the gap; a lease's token is registered with the
//          upstream stream before the session sees the lease; Send never waits longer than the send timeout;
//          Close waits a bounded time for results to be written and acknowledged.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Timings; variables so tests can shorten them.
var (
	sendTimeout = 30 * time.Second
	flushGrace  = 2 * time.Second
)

// maxOutBytes bounds the frames kept for resend.
const maxOutBytes = 16 << 20

// errStalled reports a coordinator that stopped taking frames.
var errStalled = errors.New("carrier: the coordinator is not taking frames")

// CarrierConfig configures a Carrier.
type CarrierConfig struct {
	Client  Options
	Backoff Backoff
	// Sleep waits d or until ctx ends (default: a timer).
	Sleep func(ctx context.Context, d time.Duration) error
	Logf  func(format string, a ...any)
}

type outFrame struct {
	m    protocol.Message
	tseq uint64
	size int
}

// Carrier is session.Carrier over the pull connection.
type Carrier struct {
	cfg    CarrierConfig
	cl     *client
	cancel context.CancelFunc
	inbox  chan protocol.Message
	done   chan struct{}
	wake   chan struct{} // new frame, token change or close: the uploader looks again
	space  chan struct{} // frames were trimmed: a blocked Send looks again

	mu        sync.Mutex
	closed    bool
	hello     []byte
	helloSent bool // the welcome of the first hello reached the session
	sess      string
	out       []*outFrame
	outBytes  int
	nextT     uint64 // last transport seq assigned on this transport session
	confirmed uint64 // last transport seq the coordinator is known to have accepted
	written   uint64 // last transport seq written on the live stream
	lastBeat  *protocol.Message
	tokens    map[string]string
	epochs    map[string]uint64 // epoch of each lease that has a token
	tokenAt   map[string]time.Time
	gen       int // bumped when the token set grows: the upstream stream restarts to carry it
	liveGen   int
	streaming bool          // an upstream stream is open: frames are being written
	up        chan struct{} // closed and replaced when liveGen changes
	err       error         // why the carrier ended, for Recv
}

// NewCarrier starts the carrier; it connects once the session sends hello.
func NewCarrier(cfg CarrierConfig) *Carrier {
	if cfg.Sleep == nil {
		cfg.Sleep = sleepCtx
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Carrier{cfg: cfg, cl: newClient(cfg.Client), cancel: cancel, inbox: make(chan protocol.Message, 64), done: make(chan struct{}),
		wake: make(chan struct{}, 1), space: make(chan struct{}, 1), tokens: map[string]string{}, epochs: map[string]uint64{}, tokenAt: map[string]time.Time{}, up: make(chan struct{})}
	go c.loop(ctx)
	return c
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Carrier) signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// validate runs the codec's checks on m without keeping the bytes.
func validate(m protocol.Message) error {
	enc, err := protocol.NewResumedEncoder(io.Discard, 0, 0)
	if err != nil {
		return err
	}
	m.V, m.Seq = 1, 1
	return enc.Encode(m)
}

func encode(m protocol.Message, seq uint64) ([]byte, error) {
	var b bytes.Buffer
	enc, err := protocol.NewResumedEncoder(&b, seq-1, seq-1)
	if err != nil {
		return nil, err
	}
	m.V, m.Seq = 1, seq
	if err = enc.Encode(m); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Send queues a frame of the session. The hello frame is kept for the handshake; a frame the codec refuses
// returns the codec's *protocol.Error and is not queued.
func (c *Carrier) Send(m protocol.Message) error {
	if err := validate(m); err != nil {
		return err
	}
	if m.Type == "hello" {
		b, err := encode(m, 1)
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.hello = b
		c.mu.Unlock()
		c.signal(c.wake)
		return nil
	}
	b, _ := encode(m, 1)
	f := &outFrame{m: m, size: len(b)}
	timer := time.NewTimer(sendTimeout)
	defer timer.Stop()
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return io.ErrClosedPipe
		}
		if c.outBytes+f.size <= maxOutBytes || len(c.out) == 0 {
			c.out = append(c.out, f)
			c.outBytes += f.size
			if m.Type == "heartbeat" {
				cp := m
				c.lastBeat = &cp
				c.dropRefused(m)
				c.sweepTokens(m, time.Now())
			}
			c.mu.Unlock()
			c.signal(c.wake)
			return nil
		}
		c.mu.Unlock()
		select {
		case <-c.space:
		case <-timer.C:
			return errStalled
		case <-c.done:
			return io.ErrClosedPipe
		}
	}
}

// Recv returns the next coordinator message; io.EOF after Close.
func (c *Carrier) Recv() (protocol.Message, error) {
	m, ok := <-c.inbox
	if ok {
		return m, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return protocol.Message{}, c.err
	}
	return protocol.Message{}, io.EOF
}

// Close waits a bounded time for the written results to be acknowledged while a stream is open, then stops the
// carrier. With no stream (a partition) it returns at once: the agent fence calls Close to end the session and
// kill its jobs, and that must not wait.
func (c *Carrier) Close() error {
	deadline := time.Now().Add(flushGrace)
	for time.Now().Before(deadline) && c.isStreaming() && !c.flushed() {
		select {
		case <-c.done:
			deadline = time.Time{}
		case <-time.After(10 * time.Millisecond):
		}
	}
	c.mu.Lock()
	already := c.closed
	c.closed = true
	c.mu.Unlock()
	if already {
		<-c.done
		return nil
	}
	c.cancel()
	c.signal(c.wake)
	<-c.done
	c.cl.close()
	return nil
}

func (c *Carrier) isStreaming() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.streaming
}

// flushed says every frame was written and no result or reference still waits for its ack.
func (c *Carrier) flushed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, f := range c.out {
		if f.tseq == 0 || f.tseq > c.written {
			return false
		}
		switch f.m.Type {
		case "result", "artifact-ref", "cache-ref":
			return false
		}
	}
	return true
}

// deliver hands a coordinator message to Recv.
func (c *Carrier) deliver(ctx context.Context, m protocol.Message) error {
	m.V = 1
	select {
	case c.inbox <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// trim drops the frames the coordinator confirmed up to seq; the caller holds c.mu.
func (c *Carrier) trim(seq uint64) {
	if seq <= c.confirmed {
		return
	}
	c.confirmed = seq
	keep := c.out[:0]
	for _, f := range c.out {
		if f.tseq != 0 && f.tseq <= seq {
			if f.m.Type == "result" { // the job is over and the coordinator has the result: its token is no longer needed
				delete(c.tokens, f.m.LeaseID)
				delete(c.epochs, f.m.LeaseID)
				delete(c.tokenAt, f.m.LeaseID)
			}
			c.outBytes -= f.size
			continue
		}
		keep = append(keep, f)
	}
	for i := len(keep); i < len(c.out); i++ {
		c.out[i] = nil
	}
	c.out = keep
	c.signal(c.space)
}
