package selftest

// Purpose: the tidy steps that need the run store: the CI-39 retention prune of run history
// and logs, and the cache prune with the operator's byte cap.
// Constraints: Plan only reads; Apply performs the real prune. The history prune is its own
// planner (a dry run of history.Prune), so the list and the removal use one rule.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/history"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// HistoryStep prunes run rows and logs by the retention policy.
func HistoryStep(st *store.Store, home string, pol history.Policy, now func() time.Time) Step {
	prune := func(ctx context.Context, dry bool) (history.PruneResult, error) {
		return history.Prune(ctx, st, history.PruneParams{Home: home, Policy: pol, DryRun: dry, Now: now()})
	}
	return Step{
		Name: "run history",
		Plan: func(ctx context.Context) ([]Item, error) {
			res, err := prune(ctx, true)
			if err != nil {
				return nil, err
			}
			var items []Item
			for _, id := range res.Runs {
				items = append(items, Item{Step: "run history", ID: "run " + id})
			}
			for _, id := range res.Orphans {
				items = append(items, Item{Step: "run history", ID: "log dir " + id})
			}
			if len(items) > 0 && res.FreedBytes > 0 { // the freed log bytes belong to the step as a whole
				items[0].Bytes = res.FreedBytes
			}
			return items, nil
		},
		Apply: func(ctx context.Context, _ []Item) error { _, err := prune(ctx, false); return err },
	}
}

// CacheCapEnv returns a Getenv that applies the operator's cache cap (MB) on top of next; a cap
// of 0 leaves the cache's own cap.
func CacheCapEnv(capMB int64, next func(string) string) func(string) string {
	return func(k string) string {
		if k == "NSELF_CI_CACHE_MAX_BYTES" && capMB > 0 {
			return strconv.FormatInt(capMB<<20, 10)
		}
		return next(k)
	}
}

// CacheStep prunes the build cache. es must have been built with CacheCapEnv, so Caps reports the
// cap Apply uses; the plan lists the least recently used entries past the age and byte caps.
func CacheStep(be entries.Backend, es *entries.Store, budget time.Duration, now func() time.Time) Step {
	return Step{
		Name: "cache",
		Plan: func(ctx context.Context) ([]Item, error) {
			all, err := be.CacheEntries(ctx, "")
			if err != nil {
				return nil, err
			}
			maxBytes, maxAge := es.Caps()
			sort.SliceStable(all, func(i, j int) bool { return all[i].LastUsedAt < all[j].LastUsedAt })
			cutoff, total, dropped := now().Add(-maxAge).UnixNano(), int64(0), int64(0)
			for _, m := range all {
				total += m.Bytes()
			}
			var items []Item
			for _, m := range all {
				if m.LastUsedAt < cutoff || total-dropped > maxBytes {
					items = append(items, Item{Step: "cache", ID: fmt.Sprintf("%s %.12s", m.Spec, m.Key), Bytes: m.Bytes()})
					dropped += m.Bytes()
				}
			}
			return items, nil
		},
		Apply: func(ctx context.Context, _ []Item) error {
			res, err := es.Prune(ctx, budget)
			if err == nil && res.More {
				return fmt.Errorf("more cache entries remain; run the tidy again")
			}
			return err
		},
	}
}
