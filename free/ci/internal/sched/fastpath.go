package sched

import "github.com/nself-org/plugins/free/ci/internal/model"

func fastPath(w World, jobs []Job, runners []Runner) model.FastPath {
	local := false
	remote := false
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
	var allLocal, hybrid int64
	known := true
	for _, j := range jobs {
		bestLocal := int64(0)
		bestHybrid := int64(0)
		for _, r := range runners {
			if len(eligibility(w, j, r, false)) != 0 {
				continue
			}
			s := sample(w, j, r)
			if s.RuntimeMs == nil {
				known = false
			}
			run, _ := runtime(w, j, r)
			if location(r) == "local" {
				if bestLocal == 0 || run < bestLocal {
					bestLocal = run
				}
			} else {
				n, _ := Add(run, coldStart(r))
				n, _ = Add(n, transfer(w, j, r))
				if bestHybrid == 0 || n < bestHybrid {
					bestHybrid = n
				}
			}
		}
		if bestLocal > 0 {
			allLocal, _ = Add(allLocal, bestLocal)
		}
		if bestHybrid == 0 || bestLocal > 0 && bestLocal < bestHybrid {
			bestHybrid = bestLocal
		}
		hybrid, _ = Add(hybrid, bestHybrid)
	}
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
