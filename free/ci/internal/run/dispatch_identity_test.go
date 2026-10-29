package run

// Purpose: the scheduler job carries the run's user and team (D-1044), so the user and team limits
// count the attempts of that user and team, not every hook attempt as one bucket.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestSchedJobCarriesUserAndTeam(t *testing.T) {
	job := model.Job{Kind: "test", Run: []string{"true"}}
	rd := &runDispatch{sess: &Session{RunID: "run1", User: "mia", Team: "acme"}, row: store.PipelineRow{ID: "run1", Project: "acme/web", SourceTrust: "owner"}}
	got := rd.schedJob(AttemptSpec{Job: &job, N: 1, Foreground: true}, "run1.a.a1", "run1.a", 1, 256)
	if got.User != "mia" || got.Team != "acme" {
		t.Fatalf("job identity = %q, %q; want mia, acme", got.User, got.Team)
	}
	rd.sess = &Session{RunID: "run1"}
	if got = rd.schedJob(AttemptSpec{Job: &job, N: 1, Foreground: true}, "run1.a.a1", "run1.a", 1, 256); got.User != "" || got.Team != "" {
		t.Fatalf("an unnamed run must keep the shared bucket, got %q, %q", got.User, got.Team)
	}
}
