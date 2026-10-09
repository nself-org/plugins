package sched

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"sort"
)

// Place selects runners in caller job order, reserving each selected slot privately.
func Place(w World, jobs []Job) Result {
	w.Counts.Runners = copyCounts(w.Counts.Runners)
	w.Counts.Providers = copyCounts(w.Counts.Providers)
	runners := append(append([]Runner(nil), w.Runners...), w.Hosted...)
	sort.Slice(runners, func(i, j int) bool { return runners[i].ID < runners[j].ID })
	result := Result{Placements: []Placement{}, Explanations: []Explanation{}, FastPath: fastPath(w, jobs, runners)}
	for _, j := range jobs {
		x := Explanation{JobKey: j.Key, Alternatives: []model.Alternative{}, Reasons: []model.PlacementReasonDetail{}}
		if !has(j.Pipeline.Support, j.Requirements.Platform) {
			result.ErrorCode = "E690"
			x.Status = "unplaceable"
			x.Reasons = append(x.Reasons, reason(model.PlatformMismatch, "E690: outside pipeline support"))
			result.Explanations = append(result.Explanations, x)
			continue
		}
		staticCandidates := 0
		for _, r := range runners {
			rs := eligibility(w, j, r, false)
			static := true
			for _, q := range rs {
				pr := model.PlacementReason(q.Code)
				if pr.Static() || !pr.Valid() {
					static = false
				}
			}
			if static {
				staticCandidates++
			}
		}
		var best *Placement
		bestScore := int64(0)
		bestIndex := -1
		approvalOnly := false
		noStaticCandidate := true
		for i, r := range runners {
			anti := j.Attempt > 1 && staticCandidates > 1
			rs := eligibility(w, j, r, anti)
			alt := model.Alternative{RunnerID: r.ID, Provider: r.Provider, Eligible: len(rs) == 0, Reasons: rs}
			if len(rs) == 0 {
				sc, comps := score(w, j, r)
				alt.Score = &sc
				alt.Components = comps
				localEligible := false
				if result.FastPath.Taken {
					for _, candidate := range runners {
						if location(candidate) == "local" && len(eligibility(w, j, candidate, anti)) == 0 {
							localEligible = true
							break
						}
					}
				}
				if !result.FastPath.Taken || !localEligible || location(r) == "local" {
					if best == nil || sc < bestScore {
						p := Placement{JobKey: j.Key, RunnerID: r.ID, CPU: j.Requirements.CPU, MemMB: j.Requirements.MemMB}
						cost, _ := worstCost(j.TimeoutMs, w.Costs[r.ID])
						if r.Class == "owned" || r.Class == "local" {
							cost = 0
						}
						p.CostMicro = cost
						best = &p
						bestScore = sc
						bestIndex = i
					}
				}
			}
			onlyApproval := len(rs) > 0
			staticFailure := false
			for _, q := range rs {
				if q.Code != string(model.ApprovalPending) {
					onlyApproval = false
				}
				pr := model.PlacementReason(q.Code)
				if pr.Static() || !pr.Valid() {
					staticFailure = true
				}
			}
			if !staticFailure {
				noStaticCandidate = false
			}
			if onlyApproval {
				approvalOnly = true
			}
			x.Alternatives = append(x.Alternatives, alt)
		}
		if best != nil {
			x.Status = "placed"
			result.Placements = append(result.Placements, *best)
			reserve(&runners[bestIndex], *best)
			w.Counts.Pipeline++
			w.Counts.Job++
			w.Counts.Runner++
			w.Counts.Provider++
			if w.Counts.Runners == nil {
				w.Counts.Runners = make(map[string]int)
			}
			if w.Counts.Providers == nil {
				w.Counts.Providers = make(map[string]int)
			}
			w.Counts.Runners[runners[bestIndex].ID]++
			w.Counts.Providers[runners[bestIndex].Provider]++
			w.Counts.Project++
			w.Counts.User++
			w.Counts.Team++
			w.Counts.Global++
		} else if approvalOnly {
			x.Status = "awaiting-approval"
		} else if w.OpenRegistry && noStaticCandidate {
			x.Status = "waiting"
			x.Reasons = append(x.Reasons, reason(model.PlatformNoneAvailable, "registry open"))
		} else if noStaticCandidate {
			x.Status = "unplaceable"
		} else {
			x.Status = "waiting"
		}
		if x.Status == "waiting" && len(x.Reasons) == 0 {
			x.Reasons = append(x.Reasons, reason(model.CapacityBusy, "currently unavailable"))
		}
		result.Explanations = append(result.Explanations, x)
	}
	return result
}

func copyCounts(source map[string]int) map[string]int {
	out := make(map[string]int, len(source))
	for k, v := range source {
		out[k] = v
	}
	return out
}
