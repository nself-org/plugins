package sched

import "github.com/nself-org/plugins/free/ci/internal/model"

func sample(w World, j Job, r Runner) Sample {
	if m := w.Estimates[r.ID]; m != nil {
		return m[j.Key]
	}
	return Sample{}
}
func runtime(w World, j Job, r Runner) (int64, string) {
	s := sample(w, j, r)
	if s.RuntimeMs != nil {
		return *s.RuntimeMs, confidence(s.Confidence)
	}
	for _, local := range w.Runners {
		if local.Location != "local" {
			continue
		}
		ls := sample(w, j, local)
		if ls.RuntimeMs != nil && local.Capability.Resources.CPU.Value != nil && r.Capability.Resources.CPU.Value != nil {
			lc := int64(*local.Capability.Resources.CPU.Value)
			rc := int64(*r.Capability.Resources.CPU.Value)
			if rc > 0 {
				n, _ := Mul(*ls.RuntimeMs, lc)
				n /= rc
				if n < 1 {
					n = 1
				}
				return n, "estimated"
			}
		}
	}
	if j.TimeoutMs > 0 {
		return j.TimeoutMs / 2, "unknown"
	}
	return 60000, "unknown"
}
func confidence(v string) string {
	if v == "known" || v == "estimated" {
		return v
	}
	return "unknown"
}
func location(r Runner) string {
	if r.Location != "" {
		return r.Location
	}
	if r.Capability.Location.Kind.Value != nil {
		return *r.Capability.Location.Kind.Value
	}
	return "remote"
}
func coldStart(r Runner) int64 {
	switch location(r) {
	case "local":
		return 0
	case "lan":
		return 2000
	case "hosted":
		return 60000
	case "ephemeral":
		return 120000
	}
	return 5000
}
func transfer(w World, j Job, r Runner) int64 {
	if location(r) == "local" {
		return 0
	}
	b := int64(50000000)
	if s := sample(w, j, r); s.CheckoutBytes != nil {
		b = *s.CheckoutBytes
	}
	rate := int64(2500000)
	if location(r) == "lan" {
		rate = 12500000
	}
	n, _ := Mul(b, 1000)
	return n / rate
}
func cacheValue(w World, j Job, r Runner) (int64, string) {
	if m := w.Cache[r.ID]; m != nil {
		if fact, ok := m[j.Key]; ok && fact.MissBytes != nil {
			rate := int64(2500000)
			if location(r) == "lan" {
				rate = 12500000
			}
			n, _ := Mul(*fact.MissBytes, 1000)
			return n / rate, confidence(fact.Confidence)
		}
	}
	return 0, "unknown"
}
func component(value, weight int64, conf string) model.Component {
	return model.Component{ValueMs: value, WeightPermille: weight, Confidence: conf}
}
