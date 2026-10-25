package run

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// A note for one job's check must not land on a same-named check of another job.
func TestMarkHistoryFlakyMatchesJobAndCheck(t *testing.T) {
	defer func(h FlakyHistoryHook) { FlakyHistory = h }(FlakyHistory)
	note := &model.Flaky{Source: "history", Passes: 1, Fails: 1}
	FlakyHistory = func(context.Context, *store.Store, model.Evidence) map[string]*model.Flaky {
		return map[string]*model.Flaky{model.FlakyKey("a", "unit"): note}
	}
	ev := model.Evidence{Checks: []model.Check{{ID: "unit", JobID: "a"}, {ID: "unit", JobID: "b"}}}
	markHistoryFlaky(context.Background(), nil, &ev)
	if ev.Checks[0].Flaky != note || ev.Checks[1].Flaky != nil {
		t.Fatalf("flaky marks: a=%+v b=%+v", ev.Checks[0].Flaky, ev.Checks[1].Flaky)
	}
}
