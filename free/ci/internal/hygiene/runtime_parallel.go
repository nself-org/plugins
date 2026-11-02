package hygiene

// Purpose: collect Finalize's independent runtime listings concurrently, then apply the serial
// probe and error policy before publishing a batch snapshot.
// Inputs: a runtime and the context of the first batch listing.
// Outputs: the same ordered, per-kind snapshots as a serial batch.
// Constraints: only ExecRuntime is parallelized; injected runtimes retain their serial contract.
// All listing workers are joined before returning, including on cancellation or a listing error.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// ConcurrentBatchRuntime is BatchRuntime with a lazy parallel snapshot for ExecRuntime. Delaying
// collection until the first List keeps runs with no managed attempts from querying the runtime.
func ConcurrentBatchRuntime(inner Runtime) Runtime {
	b := BatchRuntime(inner).(*batchRuntime)
	b.parallel = true
	return b
}

type listingResult struct {
	items   []exec.Labelled
	err     error
	queried bool
}

// listKinds runs one worker per binary/kind pair (six with the default binaries). Workers only
// collect command results: cache writes and info fallbacks happen later in the original order.
func (r *ExecRuntime) listKinds(ctx context.Context) map[exec.ResourceKind]*snapshot {
	bins := r.binaries()
	results := make([][]listingResult, len(Kinds))
	var workers sync.WaitGroup
	for i, kind := range Kinds {
		results[i] = make([]listingResult, len(bins))
		for j, bin := range bins {
			if state, hit := r.cachedProbe(bin); hit && state != runtimeUp {
				continue
			}
			workers.Add(1)
			go func(i, j int, bin string, kind exec.ResourceKind) {
				defer workers.Done()
				results[i][j].items, results[i][j].err = exec.RuntimeListLabelled(ctx, bin, kind, "")
				results[i][j].queried = true
			}(i, j, bin, kind)
		}
	}
	workers.Wait()
	return r.reduceKinds(ctx, bins, results)
}

// reduceKinds replays the serial kind/binary traversal. A cached down/missing state suppresses
// subsequent results just as it suppressed their commands before; successful speculative listings
// cannot overwrite it. Each kind still stops at its first per-binary error.
func (r *ExecRuntime) reduceKinds(ctx context.Context, bins []string, results [][]listingResult) map[exec.ResourceKind]*snapshot {
	snap := make(map[exec.ResourceKind]*snapshot, len(Kinds))
	for i, kind := range Kinds {
		j := 0
		query := func(ctx context.Context, bin string, kind exec.ResourceKind, attempt string) ([]exec.Labelled, error) {
			// Cached absent/down binaries never query; skip their slots in the collected results.
			for j < len(bins) && bins[j] != bin {
				j++
			}
			result := results[i][j]
			j++
			if !result.queried {
				// A negative probe may expire while the other workers run. Apply the normal
				// listing path rather than interpreting a skipped slot as an empty success.
				return exec.RuntimeListLabelled(ctx, bin, kind, attempt)
			}
			return result.items, result.err
		}
		items, err := r.listUsing(ctx, kind, "", query)
		snap[kind] = &snapshot{items: items, err: err}
	}
	return snap
}
