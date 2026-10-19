package run

// Purpose: pins the dispatch hook's placement inputs (D-1042): a run maps a job
// to a sched.Job with the mapping `ci plan` uses (planview), and its world
// counts the per-job limit over the run's own jobs, not every active attempt.

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestSchedJobSharesPlanMapping(t *testing.T) {
	job := model.Job{Kind: "release", Run: []string{"true"}, Secrets: []string{"deploy/TOKEN", "deploy/OTHER", "none/X"}}
	rd := &runDispatch{sess: &Session{RunID: "run1"}, row: store.PipelineRow{ID: "run1", Project: "/proj", SourceTrust: "internal"}}
	got := rd.schedJob(AttemptSpec{Job: &job, N: 2, Foreground: true}, "run1.jobA.a2", "run1.jobA", 1, 256)
	want, _ := planview.SchedJobs(planview.Context{PipelineID: "run1", Project: "/proj", DefaultPlatform: platform(), Foreground: true},
		[]planview.PlannedJob{{ID: "run1.jobA", Name: "run1.jobA", Job: job}})
	if len(want) != 1 {
		t.Fatalf("plan mapping: %+v", want)
	}
	w := want[0]
	if got.TimeoutMs != w.TimeoutMs || got.Priority != w.Priority || !reflect.DeepEqual(got.SecretClasses, w.SecretClasses) ||
		got.Pipeline.Release != w.Pipeline.Release || got.Pipeline.Band != w.Pipeline.Band || got.Network != w.Network {
		t.Fatalf("run mapping diverges from the plan's:\n run  %+v\n plan %+v", got, w)
	}
	if got.Key != "run1.jobA.a2" || got.ID != "run1.jobA" || got.Attempt != 2 || got.Trust != "internal" || got.Requirements.Platform != platform() {
		t.Fatalf("run-specific fields: %+v", got)
	}
}

func TestDispatchWorldCountsJobsOfTheRun(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: func() time.Time { return time.Unix(100, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	row := store.PipelineRow{ID: "run1", Project: "/proj", Revision: "rev", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}
	if err = st.CreatePipeline(ctx, row); err != nil {
		t.Fatal(err)
	}
	sess := newSession()
	sess.Store, sess.RunID, sess.Capacity = st, row.ID, func() ciexec.Capacity {
		return ciexec.Capacity{CPUs: 10, MemoryMB: 1000, FreeMemoryMB: 1000, BatteryPercent: -1, PluggedIn: true}
	}
	// three jobs of the run, one active attempt each: every job is at 1, the host at 3
	for _, n := range []string{"a", "b", "c"} {
		id := row.ID + "." + n
		if err = st.CreateJob(ctx, store.JobRow{ID: id, PipelineID: row.ID, Name: n, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
			t.Fatal(err)
		}
		sess.bind(&model.Job{Kind: "test", Run: []string{"echo", n}}, id)
		if err = st.CreateAttempt(ctx, store.AttemptRow{ID: id + ".a1", JobID: id, N: 1}, ""); err != nil {
			t.Fatal(err)
		}
		if err = st.AdmitAndLease(ctx, "host", store.Demand{AttemptID: id + ".a1", RunnerID: "local", LeaseID: "lease" + n, CPU: 1, MemMB: 1, CapacityCPU: 10, CapacityMemMB: 1000}); err != nil {
			t.Fatal(err)
		}
	}
	rd := &runDispatch{sess: sess, row: row, pol: &sched.Policy{}}
	w, _, err := rd.world(ctx, sched.Pipeline{ID: row.ID, Support: []string{runtime.GOOS + "/" + runtime.GOARCH}})
	if err != nil {
		t.Fatal(err)
	}
	if w.Counts.Global != 3 {
		t.Fatalf("global active = %d, want 3", w.Counts.Global)
	}
	if w.Counts.Job != 1 {
		t.Fatalf("per-job active = %d, want 1 (the busiest job of the run, not every active attempt)", w.Counts.Job)
	}
}
