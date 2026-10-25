package history

// Purpose: retention. Remove the oldest runs beyond the policy, their stored
// logs with them, so the store and the log files stay consistent.
// Inputs:  the store, the state directory, a Policy and the run ids to keep.
// Outputs: what was (or, in a dry run, would be) removed.
// Constraints: a run that is still running, and every id in Keep, is never
// removed; a run goes when it is beyond the newest KeepRuns, older than
// MaxAge, or (oldest first) while the stored logs exceed MaxLogBytes; store
// rows go first and only the log directories of runs whose rows are really
// gone are deleted, so a crash between the two leaves an orphan log directory
// (no run row), never a run row without its logs recorded as present; orphan
// log directories older than OrphanGrace are swept on every prune; the
// run-start prune removes at most runStartPruneMax runs.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// OrphanGrace is how old a log directory without a run row must be before it
// is swept (a run being created writes its row first, so this is a margin).
const OrphanGrace = time.Hour

// runStartPruneMax bounds one run-start prune; the rest go at the next runs.
const runStartPruneMax = 100

// PruneParams selects what Prune does.
type PruneParams struct {
	Home   string
	Policy Policy
	Keep   []string // run ids never removed (the running pipeline)
	DryRun bool
	Now    time.Time // zero: time.Now()
	// MaxRuns bounds how many runs one call removes, oldest first (0: no bound); the
	// run-start prune uses it so the first prune of a long history spreads over runs.
	MaxRuns int
}

// PruneResult reports a prune.
type PruneResult struct {
	Runs       []string `json:"runs"`        // run rows removed, oldest first
	Orphans    []string `json:"orphans"`     // log directories removed that had no run row
	FreedBytes int64    `json:"freed_bytes"` // stored log bytes removed
	DryRun     bool     `json:"dry_run"`
}

// Prune applies the policy.
func Prune(ctx context.Context, st *store.Store, p PruneParams) (PruneResult, error) {
	now := p.Now
	if now.IsZero() {
		now = time.Now()
	}
	res := PruneResult{DryRun: p.DryRun, Runs: []string{}, Orphans: []string{}}
	ages, err := st.PipelineAges(ctx) // newest first
	if err != nil {
		return res, err
	}
	infos, err := logs.ListRuns(p.Home)
	if err != nil {
		return res, err
	}
	size := map[string]int64{}
	modified := map[string]time.Time{}
	var total int64
	for _, i := range infos {
		size[i.Run], modified[i.Run] = i.Bytes, i.Modified
		total += i.Bytes
	}
	keep := map[string]bool{}
	for _, k := range p.Keep {
		keep[k] = true
	}
	known := map[string]bool{}
	for _, a := range ages {
		known[a.ID] = true
	}
	orphans := []string{}
	for _, i := range infos {
		if !known[i.Run] && !keep[i.Run] && now.Sub(modified[i.Run]) > OrphanGrace {
			orphans = append(orphans, i.Run)
		}
	}
	for _, id := range orphans {
		total -= size[id] // swept anyway: they must not push real runs out
	}
	victim := map[string]bool{}
	oldestFirst := []string{}
	for i, a := range ages {
		if a.Status == "running" || keep[a.ID] {
			continue
		}
		created := time.Unix(0, a.CreatedAt)
		if i >= p.Policy.KeepRuns || now.Sub(created) > p.Policy.MaxAge() {
			victim[a.ID] = true
			total -= size[a.ID]
		}
	}
	for i := len(ages) - 1; i >= 0; i-- { // the size cap takes the oldest that remain
		a := ages[i]
		if total <= p.Policy.MaxLogBytes {
			break
		}
		if a.Status == "running" || keep[a.ID] || victim[a.ID] || size[a.ID] == 0 {
			continue
		}
		victim[a.ID] = true
		total -= size[a.ID]
	}
	for i := len(ages) - 1; i >= 0; i-- {
		if victim[ages[i].ID] {
			oldestFirst = append(oldestFirst, ages[i].ID)
		}
	}
	if p.MaxRuns > 0 && len(oldestFirst) > p.MaxRuns {
		oldestFirst = oldestFirst[:p.MaxRuns]
	}
	res.Runs, res.Orphans = oldestFirst, orphans
	for _, id := range append(append([]string(nil), oldestFirst...), orphans...) {
		res.FreedBytes += size[id]
	}
	if p.DryRun {
		return res, nil
	}
	if _, err = st.DeletePipelines(ctx, oldestFirst); err != nil {
		return PruneResult{DryRun: false}, err
	}
	left, err := st.PipelineAges(ctx)
	if err != nil {
		return res, err
	}
	still := map[string]bool{}
	for _, a := range left {
		still[a.ID] = true
	}
	res.Runs, res.FreedBytes = []string{}, 0
	for _, id := range oldestFirst {
		if still[id] {
			continue // the store refused it (it started running meanwhile)
		}
		res.Runs = append(res.Runs, id)
	}
	for _, id := range append(append([]string(nil), res.Runs...), orphans...) {
		if err = logs.RemoveRun(p.Home, id); err != nil {
			return res, err
		}
		res.FreedBytes += size[id]
	}
	return res, nil
}

func init() {
	logs.AtRunStart = func(ctx context.Context, s *run.Session) {
		if s == nil || s.Store == nil {
			return
		}
		home := s.StateDir()
		pol, err := LoadPolicy(home)
		if err != nil {
			// an unusable retention file must not turn into default limits that delete more than the user allowed
			fmt.Fprintf(os.Stderr, "note: %s is unusable (%v); skipping the automatic prune\n", ConfigPath(home), err)
			return
		}
		_, _ = Prune(ctx, s.Store, PruneParams{Home: home, Policy: pol, Keep: []string{s.RunID}, MaxRuns: runStartPruneMax})
	}
}
