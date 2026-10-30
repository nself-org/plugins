package runtime

// Purpose: a world built for a run that can place no hosted runner (world.RunUse.NoHosted, P7-CI-46) runs no
//          credential lookup and no Describe and gets no hosted runner; without the hint the same provider
//          is described. Inputs: the shared fake provider. Outputs: pass or fail. Constraints: no network.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func TestContributorNoHostedSkipsCredentialAndDescribe(t *testing.T) {
	p := useProvider(t)
	DefaultContributor().SetTargets(func(context.Context) ([]providers.Query, error) {
		return []providers.Query{{Project: "o/a", Repo: "o/a"}}, nil
	})
	w := sched.World{Hosted: []sched.Runner{{ID: "stale"}}}
	if err := DefaultContributor().Fill(world.WithRunUse(context.Background(), world.RunUse{NoHosted: true}), &w); err != nil {
		t.Fatal(err)
	}
	if len(w.Hosted) != 0 || len(p.queries()) != 0 || len(DefaultContributor().Skipped()) != 0 {
		t.Fatalf("hosted %+v, described %+v, skipped %+v: want none", w.Hosted, p.queries(), DefaultContributor().Skipped())
	}
	DefaultContributor().mu.Lock()
	creds := len(DefaultContributor().creds)
	DefaultContributor().mu.Unlock()
	if creds != 0 {
		t.Fatalf("a credential was resolved for a run that cannot use it (%d)", creds)
	}
	if err := DefaultContributor().Fill(context.Background(), &w); err != nil {
		t.Fatal(err)
	}
	if len(w.Hosted) == 0 || len(p.queries()) != 1 {
		t.Fatalf("without the hint: hosted %d, described %+v", len(w.Hosted), p.queries())
	}
}
