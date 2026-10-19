package session

import (
	"io"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Carrier moves decoded protocol frames. A carrier owns framing, the stream
// sequence checks (protocol.Encoder and Decoder) and any reconnect; the session
// owns message content and assigns Seq on Send. Recv returns io.EOF when the
// peer closed cleanly. Close unblocks a pending Recv.
type Carrier interface {
	Send(protocol.Message) error
	Recv() (protocol.Message, error)
	Close() error
}

type streamCarrier struct {
	enc    *protocol.Encoder
	dec    *protocol.Decoder
	mu     sync.Mutex
	closer []io.Closer
}

// NewStreamCarrier is carrier A: protocol frames over a reader and a writer
// (stdin and stdout of `serve --stdio`). With preamble true it writes the
// NSELF-CI-AGENT/1 line before anything else, so the coordinator can skip
// shell noise ahead of hello.
func NewStreamCarrier(r io.Reader, w io.Writer, policy protocol.CarrierPolicy, preamble bool) (Carrier, error) {
	if preamble {
		if _, err := io.WriteString(w, protocol.Preamble); err != nil {
			return nil, err
		}
	}
	c := &streamCarrier{enc: protocol.NewEncoder(w), dec: protocol.NewDecoder(r, policy)}
	for _, v := range []any{r, w} {
		if cl, ok := v.(io.Closer); ok {
			c.closer = append(c.closer, cl)
		}
	}
	return c, nil
}

func (c *streamCarrier) Send(m protocol.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(m)
}

func (c *streamCarrier) Recv() (protocol.Message, error) { return c.dec.Decode() }

func (c *streamCarrier) Close() error {
	var first error
	for _, cl := range c.closer {
		if err := cl.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
