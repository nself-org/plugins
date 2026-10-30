// Package sched provides local CI overhead measurement primitives.
package sched

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// MetricNames returns the ordered CI overhead metrics, measured in milliseconds.
func MetricNames() []string {
	return []string{
		"ci.startup", "ci.detect", "ci.plan", "ci.store_open", "ci.exec_overhead",
		"ci.service_start_warm", "ci.service_start_cold",
		"ci.e2e_tiny.v14", "ci.e2e_tiny.v15",
		"ci.e2e_medium.v14", "ci.e2e_medium.v15",
	}
}

// Observation retains raw samples as well as their derived statistics.
type Observation struct {
	Name    string    `json:"name"`
	Unit    string    `json:"unit"`
	Samples []float64 `json:"samples"`
	Median  float64   `json:"median"`
	P95     float64   `json:"p95"`
}

// Summarize computes the median and nearest-rank p95 without modifying samples.
func Summarize(name string, samples []float64) (Observation, error) {
	if len(samples) == 0 {
		return Observation{}, fmt.Errorf("%s: no samples", name)
	}
	sorted := append([]float64(nil), samples...)
	for _, v := range sorted {
		if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return Observation{}, fmt.Errorf("%s: invalid duration", name)
		}
	}
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	median := sorted[mid]
	if len(sorted)%2 == 0 {
		median = sorted[mid-1]/2 + sorted[mid]/2
	}
	return Observation{
		Name: name, Unit: "ms", Samples: append([]float64(nil), samples...),
		Median: median, P95: sorted[int(math.Ceil(float64(len(sorted))*0.95))-1],
	}, nil
}

// Probe separates untimed setup and cleanup from the timed operation.
// Cold service setup must evict its dedicated image; Run must include the pull
// and wait for service readiness. Warm setup must ensure the image is present.
// Process probes must wait for process exit and return nonzero exits as errors.
type Probe struct {
	Prepare func(context.Context) error
	Run     func(context.Context) error
	Cleanup func(context.Context) error
}

// Measure records ten samples after two warmups, alternating legacy and V15.
// Every probe is mandatory: missing service or legacy measurements fail closed.
// A caller must impose a context deadline and isolate all probe state.
func Measure(ctx context.Context, probes map[string]Probe) ([]Observation, error) {
	names := MetricNames()
	for _, name := range names {
		if probes[name].Run == nil {
			return nil, fmt.Errorf("%s: probe required", name)
		}
	}
	samples := make(map[string][]float64, len(names))
	for round := 0; round < 12; round++ {
		order := append([]string(nil), names...)
		if round%2 != 0 {
			order[7], order[8] = order[8], order[7]
			order[9], order[10] = order[10], order[9]
		}
		for _, name := range order {
			elapsed, err := measureOne(ctx, probes[name])
			if err != nil {
				return nil, fmt.Errorf("%s round %d: %w", name, round, err)
			}
			if round >= 2 {
				samples[name] = append(samples[name], float64(elapsed)/float64(time.Millisecond))
			}
		}
	}
	var result []Observation
	for _, name := range names {
		observation, err := Summarize(name, samples[name])
		if err != nil {
			return nil, err
		}
		result = append(result, observation)
	}
	return result, nil
}

func measureOne(ctx context.Context, p Probe) (elapsed time.Duration, err error) {
	if err = ctx.Err(); err != nil {
		return 0, err
	}
	if p.Cleanup != nil {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			cleanupErr := p.Cleanup(cleanupCtx)
			if err == nil {
				err = cleanupErr
			}
		}()
	}
	if p.Prepare != nil {
		if err = p.Prepare(ctx); err != nil {
			return 0, err
		}
	}
	start := time.Now()
	err = p.Run(ctx)
	elapsed = time.Since(start)
	if err == nil {
		err = ctx.Err()
	}
	return elapsed, err
}
