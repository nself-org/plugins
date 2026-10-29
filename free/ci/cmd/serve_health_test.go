package main

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

func TestServePersistsProviderHealth(t *testing.T) {
	st := serveTestStore(t)
	undo := serveHostedWith(st, func(context.Context) (*policy.Effective, error) { return nil, nil }, nil)
	defer undo()
	if report.Default.Store != st {
		t.Fatal("recovery drain has no coordinator store")
	}
	ctx := context.Background()
	for _, id := range []string{"debt1348github", "debt1348gitlab"} {
		t.Run(id, func(t *testing.T) {
			tracker := health.Default()
			for i := 0; i < health.DownAfter; i++ {
				tracker.Observe(rest.Outcome{Provider: id, Status: 503, Class: providers.Outage})
			}
			tracker.Wait()
			saved, ok, err := st.ProviderFactGet(ctx, id, "health", "breaker")
			if err != nil || !ok {
				t.Fatalf("breaker not persisted: exists=%v err=%v", ok, err)
			}
			if len(saved.Value) == 0 {
				t.Fatal("empty breaker")
			}
			restarted := health.New(health.Options{Store: st})
			if got := restarted.HealthOf(ctx, id); got.State != "down" {
				t.Fatalf("restart state=%s", got.State)
			}
			before := tracker.Drains()
			tracker.Observe(rest.Outcome{Provider: id, Status: 200})
			tracker.Wait()
			if tracker.Drains() != before+1 {
				t.Fatal("recovery did not drain")
			}
		})
	}
}
