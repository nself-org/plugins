package run

// Purpose: D-1040 regression. The dispatch hook's world must describe the
// local runner with measured facts (planview.LocalCapability + MarkPersonalLocal,
// the declaration `ci plan` uses), not a permissive placeholder, so placement
// itself refuses work the machine cannot isolate.

import (
	"path/filepath"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestDispatchWorldLocalDeclarationIsMeasuredNotPermissive(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	pol := sched.Policy{Preset: "balanced"}
	sess := &Session{Store: st, RunID: "r1", Capacity: func() ciexec.Capacity {
		return ciexec.Capacity{CPUs: 4, MemoryMB: 8192, FreeMemoryMB: 8192, BatteryPercent: -1}
	}}
	rd := &runDispatch{sess: sess, row: store.PipelineRow{ID: "r1", Project: "project", SourceTrust: "owner"}, pol: &pol}
	w, _, err := rd.world(t.Context(), sched.Pipeline{ID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Runners) != 1 || w.Runners[0].ID != "local" {
		t.Fatalf("runners = %+v", w.Runners)
	}
	d := w.Runners[0].Decl
	if d.Isolation != "process" || len(d.Accepts) != 1 || d.Accepts[0] != "owner" {
		t.Fatalf("local declaration is not the probe-derived one: isolation %q accepts %v", d.Isolation, d.Accepts)
	}
	if !d.PersonalOwnNode {
		t.Fatal("the personal coordinator's own node lost the owner exception")
	}
	job := func(key string, tc model.TrustClass) sched.Job {
		return sched.Job{Key: key, ID: key, Name: key, Pipeline: sched.Pipeline{ID: "r1", Support: []string{platform()}},
			Requirements: model.PlacementRequirements{Platform: platform(), CPU: 1, MemMB: 128}, Trust: tc, Network: "internet",
			TimeoutMs: 60000, PersonalOwnRun: true, Project: "project"}
	}
	if got := sched.Place(w, []sched.Job{job("own", "owner")}); len(got.Placements) != 1 {
		t.Fatalf("an owner job must stay placeable on the local runner: %+v", got.Explanations)
	}
	if got := sched.Place(w, []sched.Job{job("pr", "untrusted")}); len(got.Placements) != 0 {
		t.Fatalf("an untrusted job was placed on a process-isolation host: %+v", got.Placements)
	}
}
