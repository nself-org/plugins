package sched

import (
	"fmt"
	"testing"
)

// BenchmarkPlace measures complete decisions, including explanations, on a fixed
// world. Fixture creation is excluded; Place must reserve only its private copy.
func BenchmarkPlace(b *testing.B) {
	for _, runners := range []int{1, 10, 100} {
		for _, jobs := range []int{10, 100, 1000} {
			b.Run(fmt.Sprintf("runners=%d/jobs=%d", runners, jobs), func(b *testing.B) {
				rs := make([]Runner, runners)
				for i := range rs {
					rs[i] = fixtureRunner(fmt.Sprintf("runner-%03d", i), "lan")
				}
				w := fixtureWorld(rs...)
				js := make([]Job, jobs)
				for i := range js {
					js[i] = fixtureJob(fmt.Sprintf("job-%04d", i))
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					result := Place(w, js)
					if len(result.Explanations) != jobs || len(result.Placements) != min(runners, jobs) {
						b.Fatal("fixture did not exercise placement and capacity decisions")
					}
				}
			})
		}
	}
}
