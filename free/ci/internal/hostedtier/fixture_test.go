package hostedtier

// Purpose: fakes for the decision tests: a recorded-fixture funnel that counts requests and a
//          breaker backed by the real HOST-11 tracker on an injected clock.
// Constraints: no network, no sleeps.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/health"
)

var t0 = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

type fakeFunnel struct {
	runs  []CheckRun
	notes map[int64][]string
	err   error
	calls int
}

func (f *fakeFunnel) CheckRuns(context.Context, string) ([]CheckRun, error) {
	f.calls++
	return f.runs, f.err
}

func (f *fakeFunnel) Annotations(_ context.Context, id int64) ([]string, error) {
	f.calls++
	return f.notes[id], nil
}

func run(status, conclusion string, created, started time.Time) CheckRun {
	return CheckRun{ID: 7, Name: DefaultContext, AppID: GitHubActionsAppID, Status: status, Conclusion: conclusion, CreatedAt: created, StartedAt: started}
}

func observer(t *testing.T, f *fakeFunnel, cfg Config, at time.Time) *Observer {
	t.Helper()
	tr := health.New(health.Options{Clock: func() time.Time { return at }})
	return &Observer{Cfg: cfg, Funnel: f, Breaker: NewHealthBreaker(tr, "github-hosted"), Now: func() time.Time { return at }}
}

var errBoom = errors.New("boom")
