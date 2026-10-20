package run

import (
	"context"
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// D-1271: the release.sbom entry of a job's run names a check; the executor
// must never start it as a program.
func TestSpecDropsTheReleaseSBOMEntry(t *testing.T) {
	rs := &runState{c: &Coordinator{}, sess: &Session{}}
	job := model.Job{Kind: "release", Run: []string{"sh", "-c", "make dist", "release.sbom"}}
	got := rs.spec(context.Background(), plan.PlannedJob{}, job, "a1").Command
	if want := []string{"sh", "-c", "make dist"}; !slices.Equal(got, want) {
		t.Fatalf("command %q, want %q", got, want)
	}
	if !slices.Equal(job.Run, []string{"sh", "-c", "make dist", "release.sbom"}) {
		t.Errorf("the job's own run was changed: %q", job.Run)
	}
}
