package sched

import "github.com/nself-org/plugins/free/ci/internal/model"

var componentNames = [9]string{"wait", "runtime", "cold_start", "transfer", "cache", "contention", "cost", "scarcity", "reliability"}

func score(w World, j Job, r Runner) (int64, map[string]model.Component) {
	p := preset(w.Policy.Preset)
	s := sample(w, j, r)
	run, runConf := runtime(w, j, r)
	vals := [9]int64{}
	confs := [9]string{"estimated", runConf, "estimated", "estimated", "unknown", "estimated", "estimated", "estimated", "estimated"}
	if s.WaitMs != nil {
		vals[0] = *s.WaitMs
		confs[0] = confidence(s.Confidence)
	} else if r.QueuedAhead > 0 {
		n, _ := Mul(r.QueuedAhead, run)
		slots := int64(r.Avail.Slots)
		if slots < 1 {
			slots = 1
		}
		vals[0] = n / slots
	}
	vals[1] = run
	vals[2] = coldStart(r)
	vals[3] = transfer(w, j, r)
	if s.ColdStartMs != nil {
		vals[2] = *s.ColdStartMs
		confs[2] = confidence(s.Confidence)
	}
	if s.TransferMs != nil {
		vals[3] = *s.TransferMs
		confs[3] = confidence(s.Confidence)
	}
	vals[4], confs[4] = cacheValue(w, j, r)
	if s.CacheMs != nil {
		vals[4] = *s.CacheMs
		confs[4] = confidence(s.Confidence)
	}
	if r.Active {
		vals[5] = 30000
	}
	if s.ContentionMs != nil {
		vals[5] = *s.ContentionMs
		confs[5] = confidence(s.Confidence)
	}
	cost, _ := worstCost(j.TimeoutMs, w.Costs[r.ID])
	if s.CostMicro != nil {
		cost = *s.CostMicro
	}
	if r.Class == "local" || r.Class == "owned" {
		cost = 0
	}
	vals[6], _ = Mul(cost, p.MsPerCostMicro)
	if s.ScarcityMs != nil {
		vals[7] = *s.ScarcityMs
		confs[7] = confidence(s.Confidence)
	} else if r.Class == "included" {
		vals[7] = run / 2
	}
	vals[8] = run / 20
	if s.InfraFailurePermille != nil {
		n, _ := Mul(run, *s.InfraFailurePermille)
		vals[8] = n / 1000
		confs[8] = confidence(s.Confidence)
	}
	if s.ReliabilityMs != nil {
		vals[8] = *s.ReliabilityMs
		confs[8] = confidence(s.Confidence)
	}
	comps := make(map[string]model.Component, 9)
	total := int64(0)
	for i, name := range componentNames {
		comps[name] = component(vals[i], p.Weights[i], confs[i])
		term, _ := Mul(vals[i], p.Weights[i])
		term /= 1000
		total, _ = Add(total, term)
	}
	total, _ = Add(total, p.ClassBiasMs[classIndex(r.Class)])
	return total, comps
}
