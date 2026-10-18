package hygiene

// Purpose: SweepOrphans removes the roots and labelled resources of attempts that no live lease
// owns (a crashed agent or coordinator left them).
// Inputs:  the attempt ids of live leases.
// Outputs: a SweepReport; E681 when something could not be removed.
// Constraints: a live attempt is never touched; a root younger than Host.MinAge is skipped; only
// entries with a safe attempt name and resources carrying the attempt label are considered.

import (
	"context"
	"os"
	"sort"
	"time"
)

// SweepReport says what a sweep did.
type SweepReport struct {
	Removed []string         `json:"removed,omitempty"`
	Skipped []string         `json:"skipped,omitempty"`
	Failed  map[string]error `json:"-"`
}

// SweepOrphans cleans every attempt that is not in liveLeases.
func (h *Host) SweepOrphans(ctx context.Context, liveLeases []string) (SweepReport, error) {
	rep := SweepReport{Failed: map[string]error{}}
	live := make(map[string]bool, len(liveLeases))
	for _, id := range liveLeases {
		live[id] = true
	}
	young := map[string]bool{}
	candidates := map[string]bool{}
	if h.Base != "" {
		entries, err := os.ReadDir(h.Base)
		if err != nil && !os.IsNotExist(err) {
			return rep, err
		}
		for _, e := range entries {
			name := e.Name()
			if !ValidAttempt(name) || live[name] {
				continue
			}
			if h.MinAge > 0 {
				if info, err := e.Info(); err == nil && time.Since(info.ModTime()) < h.MinAge {
					young[name] = true
					rep.Skipped = append(rep.Skipped, name)
					continue
				}
			}
			candidates[name] = true
		}
	}
	for _, kind := range Kinds {
		found, err := h.runtime().List(ctx, kind, "")
		if err != nil {
			rep.Failed["list "+string(kind)] = err
			continue
		}
		for _, r := range found {
			if !live[r.Attempt] && !young[r.Attempt] {
				candidates[r.Attempt] = true
			}
		}
	}
	names := make([]string, 0, len(candidates))
	for n := range candidates {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		if err := h.Cleanup(ctx, name); err != nil {
			rep.Failed[name] = err
			continue
		}
		rep.Removed = append(rep.Removed, name)
	}
	if len(rep.Failed) > 0 {
		return rep, leakError(ReasonCleanupFailed, "%d orphans could not be removed", len(rep.Failed))
	}
	return rep, nil
}
