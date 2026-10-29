package pools

// Purpose: the explanation of a project's effective set (`ci nodes pools explain`): every runner a pool
//          term drops, for a run of record and for a dev run, with its pool.excluded detail.
// Inputs:  an Input (Dev is ignored: both run kinds are resolved) and the notes of FromPolicy.
// Outputs: Explanation, the data of `explain --json`.
// Constraints: pure; the excluded rows keep the order of Input.Runners.

import "github.com/nself-org/plugins/free/ci/internal/model"

// Explanation is the effective set of one project and why each other runner is out.
type Explanation struct {
	Project      string        `json:"project"`
	Pools        []Pool        `json:"pools"`
	Eligible     []string      `json:"eligible"`
	Excluded     []ExcludedRow `json:"excluded"`
	Unauthorized []string      `json:"unauthorized"`
	DevExcluded  []ExcludedRow `json:"dev_excluded"`
	Notes        []string      `json:"notes"`
}

// ExcludedRow is one runner the pool terms drop.
type ExcludedRow struct {
	RunnerID string                        `json:"runner_id"`
	Pools    []string                      `json:"pools"`
	Reasons  []model.PlacementReasonDetail `json:"reasons"`
}

// Explain resolves in for a run of record and for a dev run.
func Explain(in Input, notes []string) Explanation {
	in.Dev = false
	rec := Resolve(in)
	in.Dev = true
	dev := Resolve(in)
	return Explanation{Project: in.Project, Pools: rec.Pools, Eligible: rec.Eligible, Excluded: rows(in.Runners, rec),
		Unauthorized: rec.Unauthorized, DevExcluded: rows(in.Runners, dev), Notes: append([]string{}, notes...)}
}

func rows(runners []Runner, res Result) []ExcludedRow {
	out := []ExcludedRow{}
	for _, r := range runners {
		if why, ok := res.Excluded[r.ID]; ok {
			out = append(out, ExcludedRow{RunnerID: r.ID, Pools: append([]string{}, Memberships(r)...), Reasons: why})
		}
	}
	return out
}
