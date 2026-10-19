package ssh

// Purpose: one started ssh process as the transport sees it, and the bounded
// capture of its stderr.
// Inputs:  a remote.Target and the constant agent command.
// Outputs: Conn (pipes plus Wait and Kill).
// Constraints: ssh starts only through cli sdk/go/remote Start (argv, the D4
// option set, the environment allowlist); the agent's stderr is never mixed
// into the frame stream.

import (
	"context"
	"io"
	"sync"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Conn is a started remote command.
type Conn struct {
	Stdin  io.WriteCloser
	Stdout io.Reader
	Stderr io.Reader
	Wait   func() error
	Kill   func() error
}

// Starter starts command on t. The default uses remote.Start; tests supply an in-process peer.
type Starter func(ctx context.Context, t remote.Target, command string) (*Conn, error)

func startRemote(ctx context.Context, t remote.Target, command string) (*Conn, error) {
	s, err := remote.Start(ctx, t, command)
	if err != nil {
		return nil, err
	}
	return &Conn{Stdin: s.Stdin, Stdout: s.Stdout, Stderr: s.Stderr, Wait: s.Wait, Kill: s.Kill}, nil
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// drain copies r into a tailBuffer until EOF and reports when it is done.
func drain(r io.Reader) (*tailBuffer, <-chan struct{}) {
	tb := &tailBuffer{max: 4096}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(tb, r)
	}()
	return tb, done
}
