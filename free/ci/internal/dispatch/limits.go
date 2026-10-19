package dispatch

// Purpose: the `limits` admission step (Epic D8): pipeline, job, runner,
// provider, project, user, team and global concurrency, read inside the
// admission transaction. A level at or over its limit refuses with limit.<level>.
// Inputs:  View.Counts (leased, running and finalizing attempts, from the store)
// and Request.Limits from the effective policy; 0 means no limit.
// Outputs: nil or a *Refusal naming every level that is full.
// Constraints: the runner limit is the lower of the policy limit and the runner's
// own slots, and a runner whose slots are known to be zero admits nothing; every
// level is a hard limit, the foreground floor (X17) is the local admission policy
// and eligibility rule 9, never a waiver here (an unenforced limit is no limit).

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func init() {
	if err := RegisterStep("limits", limitsStep); err != nil {
		panic(err)
	}
}

type level struct {
	reason       model.PlacementReason
	count, limit int
}

func limitsStep(_ context.Context, v View) error {
	r, c := v.Request, v.Counts
	if r.SlotsKnown && r.RunnerSlots <= 0 {
		return &Refusal{Reason: model.LimitRunner, Detail: "runner has no slots"}
	}
	runnerLimit := lowest(r.Limits.Runner, r.RunnerSlots)
	levels := []level{
		{model.LimitPipeline, c.Pipeline, r.Limits.Pipeline},
		{model.LimitJob, c.Job, r.Limits.Job},
		{model.LimitRunner, c.Runner, runnerLimit},
		{model.LimitProvider, c.Provider, lowest(r.Limits.Provider, r.ProviderLimit)},
		{model.LimitProject, c.Project, r.Limits.Project},
		{model.LimitUser, c.User, r.Limits.User},
		{model.LimitTeam, c.Team, r.Limits.Team},
		{model.LimitGlobal, c.Global, r.Limits.Global},
	}
	var full []string
	first := model.PlacementReason("")
	for _, l := range levels {
		if l.limit > 0 && l.count >= l.limit {
			if first == "" {
				first = l.reason
			}
			full = append(full, string(l.reason))
		}
	}
	if first == "" {
		return nil
	}
	return &Refusal{Reason: first, Detail: strings.Join(full, ", ") + " at its limit"}
}

// lowest returns the smaller positive of a and b; 0 means unlimited.
func lowest(a, b int) int {
	switch {
	case a <= 0:
		return max(b, 0)
	case b <= 0:
		return a
	}
	return min(a, b)
}
