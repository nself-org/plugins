package run

// Purpose: let a run take the artifacts of a remote attempt (P7-CACHE-04, Epic
// CACHE D4/D7). The run's remoteSink is one object for every lease of a run and
// transport.BlobHeader names no lease, so each delivery gets a per-lease view of
// it (bindLeaseSink): blobs and artifact-refs of a job that declares outputs go
// to the coordinator's artifact handler (internal/artifacts/remote), everything
// else stays with remoteSink. The record intake runs while the attempt is
// running; the existing AttemptFinalize hook (artifacts_hook.go) then verifies
// and commits it, so a remote attempt passes only with verified artifacts.
// Inputs:  a dispatch.Delivery (attempt, lease, lease epoch, job) and the
// frames the carrier driver gives its Sink.
// Outputs: blobs in the project's artifact realm, staged artifact records,
// declarations written once per lease (at the first blob, ref or result).
// Constraints: a job that declares nothing, a hosted lease and a stream other
// than `artifact` keep the old behaviour (bytes discarded, refs ignored); a
// handler that cannot be built refuses the frames instead of dropping them, so
// a declared artifact cannot be silently lost; the lease epoch is fenced
// through the lease engine before every write.

import (
	"context"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/artifacts/remote"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// bindLeaseSink returns the view of sink for one delivery; a sink that does not bind is returned as is.
func bindLeaseSink(sink transport.Sink, d dispatch.Delivery) transport.Sink {
	if b, ok := sink.(interface {
		bindLease(dispatch.Delivery) transport.Sink
	}); ok {
		return b.bindLease(d)
	}
	return sink
}

func (s *remoteSink) bindLease(d dispatch.Delivery) transport.Sink {
	if d.Hosted || !declares(d.Job) {
		return s
	}
	ra := s.attempt(d.AttemptID)
	if ra == nil {
		return s
	}
	pol, err := records.PolicyFromEnv(os.Getenv)
	if err != nil {
		return refusedSink{s, err}
	}
	home := records.StoreDir(s.home)
	pol.StorePath, pol.Team = home, os.Getenv("NSELF_CI_COORDINATOR_MODE") == "team"
	blobs, err := cas.New(home)
	if err != nil {
		return refusedSink{s, err}
	}
	h := &remote.Handler{Ledger: s.st, Blobs: blobs, Policy: pol}
	l := h.Bind(remote.Binding{AttemptID: d.AttemptID, LeaseID: d.LeaseID, Epoch: d.Epoch, Job: d.Job,
		Fence: func(ctx context.Context) error { return s.eng.Fence(ctx, d.LeaseID, d.Epoch) },
		Ready: func(ctx context.Context) error { return s.markRunning(ctx, ra) }})
	return leaseSink{remoteSink: s, l: l}
}

// leaseSink is the Sink of one lease of a job that declares outputs.
type leaseSink struct {
	*remoteSink
	l *remote.Lease
}

func (s leaseSink) Blob(ctx context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	if h.Stream != transfer.StreamArtifact {
		return s.remoteSink.Blob(ctx, h)
	}
	return s.l.Blob(ctx, h)
}

func (s leaseSink) ArtifactRef(ctx context.Context, a protocol.ArtifactRef) error {
	return s.l.ArtifactRef(ctx, a)
}

// Result writes the declarations before the attempt leaves running, so an attempt whose node
// delivered nothing fails its finalize (a declared artifact has no collected file).
func (s leaseSink) Result(ctx context.Context, r protocol.Result) error {
	if err := s.l.Declare(ctx); err != nil {
		return err
	}
	return s.remoteSink.Result(ctx, r)
}

// refusedSink refuses the artifact frames of a lease whose handler could not be built.
type refusedSink struct {
	*remoteSink
	err error
}

func (s refusedSink) Blob(_ context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	if h.Stream != transfer.StreamArtifact {
		return &discardBlob{}, nil
	}
	return nil, s.err
}
func (s refusedSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return s.err }
func (s refusedSink) Result(context.Context, protocol.Result) error           { return s.err }
