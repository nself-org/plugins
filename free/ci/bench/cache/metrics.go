package cache

import (
	"math"
	"sort"
)

// Observation summarizes a set of samples for a single metric.
type Observation struct {
	// Name identifies the metric (for example "restore_ms").
	Name string `json:"name"`
	// Unit describes the measurement unit (for example "ms", "MiB/s").
	Unit string `json:"unit"`
	// Count is the number of samples used to compute the summary.
	Count int `json:"count"`
	// Median is the 50th percentile value.
	Median float64 `json:"median"`
	// P95 is the 95th percentile value.
	P95 float64 `json:"p95"`
}

// Summarize computes median and p95 from raw samples. It returns an error when
// samples is empty. The slice is sorted in place.
func Summarize(name, unit string, samples []float64) (Observation, error) {
	if len(samples) == 0 {
		return Observation{}, errNoSamples
	}
	sort.Float64s(samples)
	return Observation{
		Name:   name,
		Unit:   unit,
		Count:  len(samples),
		Median: percentile(samples, 0.5),
		P95:    percentile(samples, 0.95),
	}, nil
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

type errType struct{ msg string }

func (e *errType) Error() string { return e.msg }

var errNoSamples = &errType{msg: "no samples"}
