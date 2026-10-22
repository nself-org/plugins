//go:build scenarios

package scenarios

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func s10UnequalRuntimeFairness(t *testing.T) {
	st := openStore(t)
	for _, e := range []struct {
		project string
		runtime int64
	}{{"alpha", 400000}, {"beta", 100000}} {
		err := st.TelRecordOutcome(context.Background(), store.TelOutcome{AttemptID: "estimate-" + e.project, Project: e.project, JobKey: "build", RunnerClass: "local", Result: "passed", RuntimeMs: e.runtime, TS: 1}, func(store.TelEstimator, bool) store.TelEstimator {
			return store.TelEstimator{Project: e.project, JobKey: "build", RunnerClass: "local", RuntimeMs: e.runtime, N: 100, Updated: 1}
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	d := newDispatcher(t, dispatch.Config{Store: st, World: liveWorld(t, st, nil, s10Runner("local", "local", 1))})
	var q []dispatch.Queued
	for i, project := range []string{"alpha", "beta", "alpha", "beta", "alpha", "beta"} {
		_, j := s10Job(t, st, project, "")
		j.AttemptID = fmt.Sprintf("%s%d", project, i)
		q = append(q, dispatch.Queued{Job: j, Seq: uint64(i + 1), EnqueuedAt: time.Now()})
	}
	ordered, _ := d.Plan(sched.World{}, q)
	var got []string
	for _, e := range ordered {
		got = append(got, e.Job.Sched.Project)
	}
	if strings.Join(got, " ") != "alpha beta beta beta alpha alpha" {
		t.Fatalf("runtime-weighted order: %v", got)
	}
}
