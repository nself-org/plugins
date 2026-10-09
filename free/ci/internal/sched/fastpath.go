package sched

import "github.com/nself-org/plugins/free/ci/internal/model"

// fastPath compares list-scheduled completion times over each runner's free slots.
func fastPath(w World, jobs []Job, runners []Runner) model.FastPath {
	local, remote := false, false
	for _, j := range jobs {
		for _, r := range runners {
			if len(eligibility(w, j, r, false)) != 0 {
				continue
			}
			if location(r) == "local" {
				local = true
			} else {
				remote = true
			}
		}
	}
	if !local {
		return model.FastPath{Reason: "none"}
	}
	if !remote {
		return model.FastPath{Taken: true, Reason: "no_remote"}
	}
	allLocal, _ := planMakespan(w, jobs, runners, true)
	hybrid, known := planMakespan(w, jobs, runners, false)
	if !known {
		return model.FastPath{Taken: true, Reason: "unknown_estimates"}
	}
	margin := w.Policy.FastPathMarginMs
	if margin <= 0 {
		margin = 2000
	}
	gain, _ := Add(allLocal, -hybrid)
	if gain <= margin {
		return model.FastPath{Taken: true, Reason: "overhead_exceeds_gain"}
	}
	return model.FastPath{Reason: "none"}
}

// planMakespan schedules ready jobs in caller order on the earliest finishing slot.
func planMakespan(w World, jobs []Job, runners []Runner, localOnly bool) (int64, bool) {
	slots := make(map[string][]int64, len(runners))
	for _, r := range runners {
		if r.Avail.Slots > 0 {
			slots[r.ID] = make([]int64, r.Avail.Slots)
		}
	}
	finish := make(map[string]int64, len(jobs))
	done := make([]bool, len(jobs))
	known := true
	var makespan int64
	for remaining := len(jobs); remaining > 0; {
		progress := false
		for i, j := range jobs {
			if done[i] {
				continue
			}
			ready := int64(0)
			blocked := false
			for _, dep := range j.Deps {
				if _, ok := finish[dep]; !ok && jobInPlan(jobs, dep) {
					blocked = true
					break
				}
				if finish[dep] > ready {
					ready = finish[dep]
				}
			}
			if blocked {
				continue
			}
			localEligible := false
			if localOnly {
				for _, r := range runners {
					if location(r) == "local" && len(eligibility(w, j, r, false)) == 0 {
						localEligible = true
						break
					}
				}
			}
			bestID, bestSlot, bestEnd := "", 0, int64(0)
			for _, r := range runners {
				if localEligible && location(r) != "local" || len(eligibility(w, j, r, false)) != 0 {
					continue
				}
				s := sample(w, j, r)
				if s.RuntimeMs == nil {
					known = false
				}
				duration, _ := runtime(w, j, r)
				if location(r) != "local" {
					duration, _ = Add(duration, coldStart(r))
					duration, _ = Add(duration, transfer(w, j, r))
				}
				for slot, free := range slots[r.ID] {
					start := free
					if ready > start {
						start = ready
					}
					end, _ := Add(start, duration)
					if bestID == "" || end < bestEnd || end == bestEnd && r.ID < bestID {
						bestID, bestSlot, bestEnd = r.ID, slot, end
					}
				}
			}
			if bestID != "" {
				slots[bestID][bestSlot] = bestEnd
				finish[j.Key] = bestEnd
				if bestEnd > makespan {
					makespan = bestEnd
				}
			} else {
				finish[j.Key] = ready
			}
			done[i], remaining, progress = true, remaining-1, true
		}
		if !progress {
			return makespan, false
		}
	}
	return makespan, known
}

func jobInPlan(jobs []Job, key string) bool {
	for _, j := range jobs {
		if j.Key == key {
			return true
		}
	}
	return false
}
