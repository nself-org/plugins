package signing

// Purpose: the narrow, read-only view of the run store and the blob store the
// signer re-checks every payload against (Epic CACHE D12).
// Inputs:  a Source (the run store: *store.Store satisfies it) and Blobs (the
// content-addressed store: *cas.Store satisfies it).
// Outputs: the facts of one run: its row, its attempts, its stored evidence
// and the verified committed records.
// Constraints: only the methods below are reachable from the check code, so a
// signer process never writes to the store even though the store type could;
// every Source method is a plain read.

import (
	"context"
	"io"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Source reads the facts of a run.
type Source interface {
	PipelineByID(ctx context.Context, id string) (store.PipelineRow, bool, error)
	PipelineJobs(ctx context.Context, pipelineID string) ([]store.RunJob, error)
	PipelineAttempts(ctx context.Context, pipelineID string) ([]store.AttemptView, error)
	EvidenceByRevision(ctx context.Context, revision, inputDigest string) ([]store.EvidenceView, error)
	ArtifactList(ctx context.Context, f records.Filter) ([]records.Record, error)
}

// Blobs reads the stored bytes of a record.
type Blobs interface {
	Open(realm cas.Realm, digest string) (io.ReadCloser, error)
	Stat(realm cas.Realm, digest string) (os.FileInfo, error)
}

// terminal reports whether an attempt state is final (no job of the run can
// still change a record or the evidence).
func terminal(state string) bool {
	return state == "passed" || state == "failed" || state == "cancelled"
}

// CheckTerminal refuses while any attempt of the run is not final. The
// coordinator calls it before it starts the signer; the signer calls it again.
func CheckTerminal(ctx context.Context, src Source, runID string) error {
	_, ok, err := src.PipelineByID(ctx, runID)
	if err != nil {
		return Failf(ReasonSignerFailed, "the run store could not be read")
	}
	if !ok {
		return Failf(ReasonPayloadRejected, "run %q is unknown", echo(runID))
	}
	attempts, err := src.PipelineAttempts(ctx, runID)
	if err != nil {
		return Failf(ReasonSignerFailed, "the run store could not be read")
	}
	if len(attempts) == 0 {
		return Failf(ReasonPayloadRejected, "run %q has no attempt", echo(runID))
	}
	for _, a := range attempts {
		if !terminal(a.State) {
			return Failf(ReasonPayloadRejected, "attempt %s of run %q is still %s: sign after the run ends", echo(a.ID), echo(runID), echo(a.State))
		}
	}
	return nil
}
