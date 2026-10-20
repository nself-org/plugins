//go:build integration

package remote

// Purpose: the coordinator's Sink of the simulation: the same calls the run's
// leaseSink makes (internal/run/remote_blob_hook.go), over the handler under
// test and the real lease engine.

import (
	"context"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

type simSink struct {
	r       *rig
	mu      sync.Mutex
	logs    []string
	errs    []error
	results int
}

func (r *rig) sink() *simSink {
	s := &simSink{r: r}
	r.snk = s
	return s
}

func (s *simSink) Log(ctx context.Context, l protocol.Log) error {
	if err := s.r.lease.b.Ready(ctx); err != nil { // the run's sink moves a leased attempt to running on the first sign of life
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, l.Text)
	return nil
}

func (s *simSink) Result(ctx context.Context, res protocol.Result) error {
	if err := s.r.lease.Declare(ctx); err != nil {
		return err
	}
	if err := s.r.eng.Result(ctx, res.LeaseID, int64(res.Epoch)); err != nil {
		return err
	}
	s.mu.Lock()
	s.results++
	s.mu.Unlock()
	return nil
}

func (s *simSink) ArtifactRef(ctx context.Context, a protocol.ArtifactRef) error {
	err := s.r.lease.ArtifactRef(ctx, a)
	if err != nil {
		s.note(err)
	}
	return err
}
func (s *simSink) CacheRef(context.Context, protocol.CacheRef) error { return nil }

func (s *simSink) Blob(ctx context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	if h.Stream != transfer.StreamArtifact {
		return nil, transfer.Failf(transfer.ReasonProtocol, "unexpected stream")
	}
	w, err := s.r.lease.Blob(ctx, h)
	if err != nil {
		s.note(err)
		return nil, err
	}
	return &noteWriter{w: w.(*transfer.Writer), s: s}, nil
}

// note remembers a refusal the driver would only report as a rejected blob.
func (s *simSink) note(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errs = append(s.errs, err)
}

// saw reports whether a recorded refusal mentions text.
func (s *simSink) saw(text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.errs {
		if strings.Contains(e.Error(), text) {
			return true
		}
	}
	return false
}

// refused is how many recorded refusals carry the E714 reason.
func (s *simSink) refused(reason string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.errs {
		if transfer.IsReason(e, reason) {
			n++
		}
	}
	return n
}

// noteWriter passes everything to the handler's writer and records its errors.
type noteWriter struct {
	w *transfer.Writer
	s *simSink
}

func (n *noteWriter) Write(p []byte) (int, error) {
	c, err := n.w.Write(p)
	if err != nil {
		n.s.note(err)
	}
	return c, err
}
func (n *noteWriter) Close() error {
	err := n.w.Close()
	if err != nil {
		n.s.note(err)
	}
	return err
}
func (n *noteWriter) Offset() int64 { return n.w.Offset() }
func (n *noteWriter) Abort() error  { return n.w.Abort() }
func (n *noteWriter) Verify(want string) error {
	err := n.w.Verify(want)
	if err != nil {
		n.s.note(err)
	}
	return err
}

func (s *simSink) nResults() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.results
}
