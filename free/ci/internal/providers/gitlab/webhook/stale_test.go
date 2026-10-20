package webhook

// Purpose: stale-delivery tests (P7-HOST-21 acceptance line 2, "stale-delivery rejection as for
// GitHub"): a delivery whose branch, tag, merge request head or job has moved on is declined with E706
// stale; an API failure lets the trigger continue with HeadCurrent unknown.
// Inputs:  the recorded deliveries and the fake API with edited routes.
// Outputs: pass or fail.
// Constraints: the stale budget is shortened for the timeout case; the variable is restored.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// TestStaleDelivery proves a replayed delivery is declined once the API shows the ref, tag, merge
// request head or job has moved, and that an unreadable answer never blocks a trigger.
func TestStaleDelivery(t *testing.T) {
	const branch = "/projects/acme%2Fapp/repository/branches/main"
	const tag = "/projects/acme%2Fapp/repository/tags/v1.0.0"
	const mr = "/projects/acme%2Fapp/merge_requests/7"
	const job = "/projects/acme%2Fapp/jobs/9001"
	stale := `{"name":"main","protected":true,"commit":{"id":"` + shaB + `"}}`

	t.Run("branch moved", func(t *testing.T) {
		rg := newRig(t, both())
		rg.gl.set(branch, 200, stale)
		_, err := rg.signed("Push Hook", fixture(t, "push"))
		wantDecline(t, "push for an old head", err, reasonStale, "trigger.stale")
	})
	t.Run("tag moved", func(t *testing.T) {
		rg := newRig(t, both())
		rg.gl.set(tag, 200, `{"name":"v1.0.0","commit":{"id":"`+shaB+`"}}`)
		_, err := rg.signed("Tag Push Hook", fixture(t, "tag_push"))
		wantDecline(t, "tag pointing elsewhere", err, reasonStale, "trigger.stale")
	})
	t.Run("merge request head moved", func(t *testing.T) {
		rg := newRig(t, both())
		rg.gl.set(mr, 200, `{"iid":7,"sha":"`+shaB+`","source_project_id":1001,"target_project_id":1001}`)
		_, err := rg.signed("Merge Request Hook", fixture(t, "mr_open"))
		wantDecline(t, "MR with a newer head", err, reasonStale, "trigger.stale")
	})
	t.Run("job already ran", func(t *testing.T) {
		for _, status := range []string{"running", "success", "failed", "canceled"} {
			rg := newRig(t, both())
			rg.gl.set(job, 200, `{"id":9001,"status":"`+status+`"}`)
			_, err := rg.signed("Job Hook", fixture(t, "job_pending_nself"))
			wantDecline(t, "job "+status, err, reasonStale, "trigger.stale")
		}
		rg := newRig(t, both())
		rg.gl.set(job, 200, `{"id":9001,"status":"waiting_for_resource"}`)
		if f, err := rg.signed("Job Hook", fixture(t, "job_pending_nself")); err != nil || f.Kind != "runner_demand" {
			t.Fatalf("a job that has not run is still a demand: %+v %v", f, err)
		}
	})
	t.Run("current head sets the fact", func(t *testing.T) {
		rg := newRig(t, both())
		f, err := rg.signed("Push Hook", fixture(t, "push"))
		if err != nil || f.HeadCurrent.Value == nil || !*f.HeadCurrent.Value || f.HeadCurrent.Source != "api" {
			t.Fatalf("HeadCurrent = %+v err %v", f.HeadCurrent, err)
		}
	})
	t.Run("unknown answers continue", func(t *testing.T) {
		for name, mutate := range map[string]func(*rig){
			"api down":     func(r *rig) { r.gl.failAll() },
			"404":          func(r *rig) { r.gl.drop(branch) },
			"not json":     func(r *rig) { r.gl.set(branch, 200, `<html>`) },
			"empty commit": func(r *rig) { r.gl.set(branch, 200, `{"name":"main"}`) },
			"no credential": func(r *rig) {
				r.src.NewAPI = func(context.Context) (*rest.Client, error) { return nil, errors.New("E700 no credential") }
			},
		} {
			rg := newRig(t, both())
			mutate(rg)
			f, err := rg.signed("Push Hook", fixture(t, "push"))
			if err != nil || f.HeadCurrent.Value != nil {
				t.Fatalf("%s: want a trigger with HeadCurrent unknown, got %+v err %v", name, f.HeadCurrent, err)
			}
		}
	})
	t.Run("a slow API is cut at the budget", func(t *testing.T) {
		was := staleBudget
		staleBudget = 200 * time.Millisecond
		defer func() { staleBudget = was }()
		rg := newRig(t, both())
		rg.gl.slow(branch, 3*time.Second)
		start := time.Now()
		f, err := rg.signed("Push Hook", fixture(t, "push"))
		if err != nil || f.HeadCurrent.Value != nil {
			t.Fatalf("slow API: %+v err %v", f.HeadCurrent, err)
		}
		if time.Since(start) > 2*time.Second {
			t.Fatalf("the stale check ran %v, over its budget", time.Since(start))
		}
	})
}
