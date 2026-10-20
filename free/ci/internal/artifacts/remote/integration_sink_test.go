//go:build integration

package remote

// Purpose: the coordinator's Sink of the simulation: the same calls the run's
// leaseSink makes (internal/run/remote_blob_hook.go), over the handler under
// test and the real lease engine.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

type simSink struct {
	r       *rig
	mu      sync.Mutex
	logs    []string
	results int
}

func (r *rig) sink() *simSink { return &simSink{r: r} }

func (s *simSink) Log(_ context.Context, l protocol.Log) error {
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
	return s.r.lease.ArtifactRef(ctx, a)
}
func (s *simSink) CacheRef(context.Context, protocol.CacheRef) error { return nil }

func (s *simSink) Blob(ctx context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	if h.Stream != transfer.StreamArtifact {
		return nil, transfer.Failf(transfer.ReasonProtocol, "unexpected stream")
	}
	w, err := s.r.lease.Blob(ctx, h)
	if err != nil {
		s.r.t.Logf("sink: blob %s refused: %v", h.SHA256[:12], err)
	}
	return w, err
}

func (s *simSink) nResults() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.results
}
