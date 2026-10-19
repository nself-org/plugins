package records

// Purpose: remove expired records and the blobs only they referenced
// (Epic CACHE D6 retention). Automatic at pipeline start inside a 2 s budget,
// and by `nself ci artifacts prune`.
// Inputs:  PruneSpec, the Ledger, the Blobs.
// Outputs: the removed records.
// Constraints: the rows go in one ledger transaction first, then the blobs no
// remaining record references are swept (the blob store keeps an unreferenced
// blob for its one-hour grace, so a blob staged by a running attempt is
// safe). Release-kind records are never touched without Release. A dry run
// changes nothing.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

// AutoPruneBudget is the shared pipeline-start budget of cache and artifact pruning.
const AutoPruneBudget = 2 * time.Second

// DefaultReleaseAge is the age `prune --release` uses when none is given.
const DefaultReleaseAge = 365 * 24 * time.Hour

// Prune applies spec and returns what it removed (or would remove).
func Prune(ctx context.Context, l Ledger, b Blobs, spec PruneSpec) ([]Record, error) {
	if spec.Now.IsZero() {
		spec.Now = time.Now()
	}
	if spec.Release && spec.OlderThan <= 0 {
		spec.OlderThan = DefaultReleaseAge
	}
	removed, err := l.ArtifactPrune(ctx, spec)
	if err != nil || spec.DryRun || len(removed) == 0 {
		return removed, err
	}
	return removed, sweepBlobs(ctx, l, b, spec.Now)
}

// AutoPrune is the pipeline-start prune: expired non-release records only,
// bounded by AutoPruneBudget; an expired budget is not an error.
func AutoPrune(ctx context.Context, l Ledger, b Blobs, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, AutoPruneBudget)
	defer cancel()
	_, err := Prune(ctx, l, b, PruneSpec{Now: now})
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func sweepBlobs(ctx context.Context, l Ledger, b Blobs, now time.Time) error {
	refs, err := l.ArtifactBlobRefs(ctx)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, r := range refs {
		keep[string(cas.ArtifactRealm(r.Project))+"/"+r.SHA256] = true
	}
	return b.Sweep(ctx, "artifacts", func(r cas.Realm, digest string) bool { return keep[string(r)+"/"+digest] }, now)
}
