package dispatch

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestFairRuntimeCost(t *testing.T) {
	for _, tc := range []struct {
		name    string
		samples int64
		want    string
	}{
		{"fallback", 0, "a1 b1 a2 b2 a3 b3"},
		{"insufficient", minSamples - 1, "a1 b1 a2 b2 a3 b3"},
		{"unequal runtime", minSamples, "a1 b1 b2 b3 a2 a3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := openStore(t)
			for _, e := range []struct {
				project string
				ms      int64
			}{{"alpha", 400000}, {"beta", 100000}} {
				if tc.samples == 0 {
					continue
				}
				err := st.TelRecordOutcome(context.Background(), store.TelOutcome{AttemptID: "o-" + e.project, Project: e.project, JobKey: "build", RunnerClass: "local", Result: "passed", RuntimeMs: e.ms, TS: 1}, func(store.TelEstimator, bool) store.TelEstimator {
					return store.TelEstimator{Project: e.project, JobKey: "build", RunnerClass: "local", RuntimeMs: e.ms, N: tc.samples, Updated: 1}
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			clock := newClock()
			d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil), Clock: clock.Now})
			var q []Queued
			for _, e := range []struct{ project, id string }{{"alpha", "a1"}, {"beta", "b1"}, {"alpha", "a2"}, {"beta", "b2"}, {"alpha", "a3"}, {"beta", "b3"}} {
				q = append(q, queuedAt(clock, e.project, e.id, "", ""))
			}
			if got := ids(d.cfg.Orderer.Order(q)); got != tc.want {
				t.Fatalf("order %s, want %s", got, tc.want)
			}
		})
	}
}
