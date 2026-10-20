//go:build integration

package remote

// Purpose: the node end of the simulation tests: a real agent session (the same
// internal/agent/session code the shipped agent runs) with an artifacts JobHook
// and a Runner that writes the job's outputs into a pinned workspace and
// collects them with Stage, exactly as cmd/agent does around the executor
// (cmd/agent/artifacts_hook.go has its own tests). The Carrier it speaks over
// is wrapped so a test can corrupt, cut or record the frames.
// Inputs:  the job outputs (path -> bytes), fault switches.
// Outputs: the frames the node sent, for the golden comparison.
// Constraints: fake synthetic bytes only; bounded waits.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

type node struct {
	t     *testing.T
	files map[string][]byte
	job   model.Job

	// fault switches
	flipChunk  int                   // flip a byte of the Nth chunk frame sent (1-based, 0 = never)
	cutChunk   int                   // fail the Nth chunk frame once, as a cut connection does (0 = never)
	dieAtChunk int                   // kill the carrier at the Nth chunk frame, as a lost ssh session does (0 = never)
	extraRef   *protocol.ArtifactRef // sent after the real refs
	cut        int
	failRefAt  int // fail the Nth artifact-ref once (1-based, 0 = never), as a cut connection does between two refs
	refs       int
	onChunk    func(k int) // runs before the Nth chunk is handed to the carrier

	mu     sync.Mutex
	stage  *Stage
	frames []protocol.Message
	chunks int
}

func (n *node) hooks() []session.JobHook {
	return []session.JobHook{{Name: "artifacts",
		Before: func(_ context.Context, l protocol.Lease, root session.JobRoot) (session.JobSetup, error) {
			st, err := NewStage(filepath.Join(root.Dir, "artifacts-stage"), l.LeaseID)
			if st != nil {
				st.WaitOffset = 3 * time.Second // a cancelled lease gets no answers: do not wait the default
			}
			n.mu.Lock()
			n.stage = st
			n.mu.Unlock()
			return session.JobSetup{}, err
		},
		After: func(ctx context.Context, _ protocol.Lease, _ session.JobRoot, out session.Outcome, up session.Uploader) error {
			n.mu.Lock()
			st := n.stage
			n.mu.Unlock()
			defer func() { _ = st.Close() }()
			if out.State != "passed" {
				return nil
			}
			if err := st.Ship(ctx, up.(session.OffsetSource).Offsets(), &cutter{n: n, Uploader: up}); err != nil {
				return fmt.Errorf("%w: %v", session.ErrHookFatal, err) // as cmd/agent/artifacts_hook.go does
			}
			if n.extraRef != nil {
				return up.ArtifactRef(*n.extraRef)
			}
			return nil
		}}}
}

// cutter fails the configured chunk once, like a send on a dying connection.
type cutter struct {
	n *node
	session.Uploader
}

func (c *cutter) ArtifactRef(a protocol.ArtifactRef) error {
	c.n.mu.Lock()
	c.n.refs++
	fail := c.n.failRefAt != 0 && c.n.refs == c.n.failRefAt
	c.n.mu.Unlock()
	if fail {
		return errors.New("connection cut")
	}
	return c.Uploader.ArtifactRef(a)
}

func (c *cutter) Chunk(ch protocol.Chunk) error {
	c.n.mu.Lock()
	c.n.chunks++
	cut := c.n.cutChunk != 0 && c.n.chunks == c.n.cutChunk && c.n.cut == 0
	if cut {
		c.n.cut++
	}
	k, hook := c.n.chunks, c.n.onChunk
	c.n.mu.Unlock()
	if hook != nil {
		hook(k)
	}
	if cut {
		return errors.New("connection cut")
	}
	return c.Uploader.Chunk(ch)
}

func (n *node) runner() session.Runner {
	return session.RunnerFunc(func(ctx context.Context, j session.JobRun) (session.JobResult, error) {
		dir := n.t.TempDir()
		for p, b := range n.files {
			full := filepath.Join(dir, p)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return session.JobResult{}, err
			}
			if err := os.WriteFile(full, b, 0o644); err != nil {
				return session.JobResult{}, err
			}
		}
		ws, err := os.OpenRoot(dir)
		if err != nil {
			return session.JobResult{}, err
		}
		defer func() { _ = ws.Close() }()
		n.mu.Lock()
		st := n.stage
		n.mu.Unlock()
		if err = st.Collect(ctx, ws, n.job, redact.New()); err != nil {
			return session.JobResult{Outcome: session.Outcome{State: "failed", FailureClass: "infra"}, Evidence: json.RawMessage(`{}`)}, err
		}
		return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: json.RawMessage(`{"schema":"test","ok":true}`)}, nil
	})
}

// tap wraps the node's carrier: it records what the node sends and injects the configured faults.
func (n *node) tap(c session.Carrier) session.Carrier { return &tapCarrier{Carrier: c, n: n} }

type tapCarrier struct {
	session.Carrier
	n *node
}

func (c *tapCarrier) Send(m protocol.Message) error {
	n := c.n
	n.mu.Lock()
	if ch, ok := m.Body.(protocol.Chunk); ok {
		if k := n.flipChunk; k != 0 && n.countChunksLocked() == k {
			b := []byte(ch.Data)
			b[len(b)/2] ^= 0x01
			ch.Data = string(b)
			m.Body = ch
		}
	}
	die := n.dieAtChunk != 0 && m.Type == "chunk" && n.countChunksLocked() == n.dieAtChunk
	n.frames = append(n.frames, m)
	n.mu.Unlock()
	if die {
		_ = c.Carrier.Close()
		return io.ErrClosedPipe
	}
	return c.Carrier.Send(m)
}

// countChunksLocked is the 1-based position of the chunk frame being sent now.
func (n *node) countChunksLocked() int {
	k := 0
	for _, f := range n.frames {
		if f.Type == "chunk" {
			k++
		}
	}
	return k + 1
}

func (n *node) sent() []protocol.Message {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]protocol.Message(nil), n.frames...)
}
