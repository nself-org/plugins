package session

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Carrier moves decoded protocol frames. A carrier owns framing, the stream
// sequence checks (protocol.Encoder and Decoder) and any reconnect; the session
// owns message content and assigns Seq on Send. Recv returns io.EOF when the
// peer closed cleanly. Send must not block for ever on a peer that has stopped
// reading. Close flushes what was sent, then unblocks a pending Recv.
type Carrier interface {
	Send(protocol.Message) error
	Recv() (protocol.Message, error)
	Close() error
}

// Stream carrier timings; variables so tests can shorten them.
var (
	// sendTimeout is how long a Send waits for room in the write queue.
	sendTimeout = 30 * time.Second
	// flushGrace is how long Close waits for queued frames to be written.
	flushGrace = 5 * time.Second
)

const queueDepth = 256

// errStalled reports a peer that stopped reading.
var errStalled = errors.New("carrier: the peer is not reading")

// queueWriter hands each Write (one protocol frame) to a writer goroutine, so a
// blocked pipe stalls only that goroutine: closing a blocking os.File does not
// interrupt a write already in progress, and a caller must never wait on it.
type queueWriter struct {
	q       chan []byte
	closing chan struct{}
	dead    chan struct{} // closed when the underlying write failed
	drained chan struct{} // closed when the writer goroutine has exited
	once    sync.Once
	err     error
}

func newQueueWriter(w io.Writer) *queueWriter {
	qw := &queueWriter{q: make(chan []byte, queueDepth), closing: make(chan struct{}), dead: make(chan struct{}), drained: make(chan struct{})}
	go func() {
		defer close(qw.drained)
		for {
			select {
			case b := <-qw.q:
				if _, err := w.Write(b); err != nil {
					qw.err = err
					close(qw.dead)
					return
				}
			case <-qw.closing:
				for { // flush what is already queued, then stop
					select {
					case b := <-qw.q:
						if _, err := w.Write(b); err != nil {
							return
						}
					default:
						return
					}
				}
			}
		}
	}()
	return qw
}

func (qw *queueWriter) Write(p []byte) (int, error) {
	b := append([]byte(nil), p...)
	timer := time.NewTimer(sendTimeout)
	defer timer.Stop()
	select {
	case <-qw.dead:
		return 0, qw.err
	case <-qw.closing:
		return 0, io.ErrClosedPipe
	default:
	}
	select {
	case qw.q <- b:
		return len(p), nil
	case <-qw.dead:
		return 0, qw.err
	case <-qw.closing:
		return 0, io.ErrClosedPipe
	case <-timer.C:
		return 0, errStalled
	}
}

// stop closes the queue and waits up to flushGrace for it to drain.
func (qw *queueWriter) stop() {
	qw.once.Do(func() { close(qw.closing) })
	select {
	case <-qw.drained:
	case <-time.After(flushGrace):
	}
}

type streamCarrier struct {
	enc    *protocol.Encoder
	dec    *protocol.Decoder
	qw     *queueWriter
	mu     sync.Mutex
	closer []io.Closer
}

// NewStreamCarrier is carrier A: protocol frames over a reader and a writer
// (stdin and stdout of `serve --stdio`). With preamble true it writes the
// NSELF-CI-AGENT/1 line before anything else, so the coordinator can skip
// shell noise ahead of hello.
func NewStreamCarrier(r io.Reader, w io.Writer, policy protocol.CarrierPolicy, preamble bool) (Carrier, error) {
	return newStreamCarrier(r, w, policy, preamble, 0, 0)
}

// NewResumedStreamCarrier continues a session whose agent-to-coordinator
// direction is at sendSeq and coordinator-to-agent at recvSeq (pair it with
// Config.Resume.SendSeq == sendSeq).
func NewResumedStreamCarrier(r io.Reader, w io.Writer, policy protocol.CarrierPolicy, sendSeq, recvSeq uint64) (Carrier, error) {
	return newStreamCarrier(r, w, policy, false, sendSeq, recvSeq)
}

func newStreamCarrier(r io.Reader, w io.Writer, policy protocol.CarrierPolicy, preamble bool, sendSeq, recvSeq uint64) (Carrier, error) {
	if preamble {
		if _, err := io.WriteString(w, protocol.Preamble); err != nil {
			return nil, err
		}
	}
	c := &streamCarrier{qw: newQueueWriter(w)}
	var err error
	if c.enc, err = protocol.NewResumedEncoder(c.qw, sendSeq, sendSeq); err != nil {
		return nil, err
	}
	if c.dec, err = protocol.NewResumedDecoder(r, policy, recvSeq, recvSeq); err != nil {
		return nil, err
	}
	if cl, ok := r.(io.Closer); ok {
		c.closer = append(c.closer, cl)
	}
	return c, nil
}

func (c *streamCarrier) Send(m protocol.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(m)
}

func (c *streamCarrier) Recv() (protocol.Message, error) { return c.dec.Decode() }

// Close flushes queued frames (bounded), then closes the reader.
func (c *streamCarrier) Close() error {
	c.qw.stop()
	var first error
	for _, cl := range c.closer {
		if err := cl.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
