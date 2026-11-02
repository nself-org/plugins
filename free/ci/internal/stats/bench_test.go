package stats

// Purpose: BenchmarkCompute100k (P7-SCHED-20): the cost of one stats read on the 100 000-attempt fixture, so a
// regression in the read or the aggregation shows as a number before TestStatsQueryPlan trips its CPU budget.
// Inputs:  buildFixture(b, 80000): 80 000 jobs and 100 000 attempts, built once outside the timer.
// Outputs: ns/op, B/op and allocs/op of Compute, and cpu-ms/op (user plus system CPU of the process per run).
// Constraints: run with go test -run xxx -bench Compute100k -benchmem ./internal/stats/ ; no assertions.

import (
	"context"
	"testing"
	"time"
)

func BenchmarkCompute100k(b *testing.B) {
	st := buildFixture(b, 80000)
	ctx := context.Background()
	o := Options{Now: fixtureNow, Since: 24 * time.Hour, Global: true}
	b.ReportAllocs()
	b.ResetTimer()
	start := cpuTime()
	for i := 0; i < b.N; i++ {
		if _, err := Compute(ctx, st, o); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64((cpuTime()-start).Milliseconds())/float64(b.N), "cpu-ms/op")
}
