//go:build integration

package health_test

// Purpose: scenario 8, provider outage (P7-HOST-11 acceptance line 1), through the real hosted
//          stack: the GitHub-hosted provider against a fake API that answers 5xx and drops
//          connections, the providers world contributor, the scheduler and the dispatcher, the
//          placement document `ci explain --json` prints, and the durable status outbox.
// Inputs:  the fake API's fault mode, an injected clock for the breaker and the sender.
// Outputs: pass or fail.
// Constraints: no live network; the breaker and the sender share one injected clock.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// apiReporter posts a commit status through the REST funnel, the way the GitHub adapter does.
type apiReporter struct{ api *fakeAPI }

func (r apiReporter) Post(ctx context.Context, s report.Status) error {
	body := map[string]string{"state": string(s.State), "context": report.Context, "description": s.Description}
	_, err := r.api.client().Do(ctx, "POST", "/repos/{owner}/{repo}/statuses/{sha}", map[string]string{"owner": s.Owner, "repo": s.Repo, "sha": s.Revision}, body, nil)
	return err
}

// TestScenario8 proves acceptance line 1. With the fake GitHub API returning 5xx and dropping
// connections: dispatches to the hosted runner are lost and the breaker opens; the placement document
// lists provider.down for the hosted runner; a pipeline job completes on the local runner while the
// provider is down and no request is sent to the failing API beyond the failed dispatches; its final
// status is queued in the outbox and not lost; when the fake recovers, the next answer closes the
// breaker, the outbox drains, and the final state is posted exactly once with the context nself-ci.
func TestScenario8(t *testing.T) {
	ctx := context.Background()
	clk := newClock()
	var sender *report.Sender
	tr := health.New(health.Options{Clock: clk.Now, Drain: func(c context.Context) (int, error) { return sender.Drain(c) }})
	api := newFakeAPI(t)
	s := newStack(t, api, tr)
	tr.SetStore(s.st)
	sender = &report.Sender{Store: s.st, Registry: report.NewRegistry(apiReporter{api}), Now: clk.Now, Rand: func() float64 { return 0.5 }, Pause: time.Millisecond}
	hostedJob := schedJob("probe-job", true)

	if doc := s.explain(hostedJob); strings.Contains(doc, string(model.ProviderDown)) {
		t.Fatalf("a healthy provider must not be listed as down: %s", doc)
	}

	// The outage: 5xx first, then dropped connections (client timeouts and resets).
	api.setMode(mode5xx)
	for i := 0; i < 8 && tr.HealthOf(ctx, provID).State != "down"; i++ {
		if i == 2 {
			api.setMode(modeReset)
		}
		id := "out" + itoa(i)
		aid := s.queue(id, 1)
		if _, err := s.submit(ctx, aid, id, true); err != nil {
			t.Fatalf("hosted dispatch %d: %v", i, err)
		}
		s.waitDeliveryErrors(i + 1)
	}
	h := tr.HealthOf(ctx, provID)
	if h.State != "down" || h.Reason != health.ReasonDown {
		t.Fatalf("health after the outage = %+v", h)
	}
	if doc := s.explain(hostedJob); !strings.Contains(doc, string(model.ProviderDown)) {
		t.Fatalf("`ci explain --json` must list provider.down for the hosted runner: %s", doc)
	}

	// The pipeline completes on the local runner, and the failing API is left alone.
	before := api.total()
	aid := s.queue("build", 1)
	a, err := s.submit(ctx, aid, "build", false)
	if err != nil || !a.Local {
		t.Fatalf("the job must run on the local runner while the provider is down: %+v %v", a, err)
	}
	for _, to := range []model.JobState{"running", "finalizing", "passed"} {
		from, epoch, err := s.st.AttemptStateEpoch(ctx, aid)
		if err != nil {
			t.Fatal(err)
		}
		if from != "passed" {
			if err = s.st.Transition(ctx, aid, from, to, storeDetail(epoch)); err != nil {
				t.Fatalf("%s -> %s: %v", from, to, err)
			}
		}
	}
	if n := api.total() - before; n != 0 {
		t.Fatalf("%d requests reached the failing API while the provider was down; the breaker must stop the retry storm", n)
	}

	// The final status is queued, not lost.
	seq, err := sender.Reserve(ctx, "o", "r", testRev)
	if err != nil {
		t.Fatal(err)
	}
	if err = sender.Complete(ctx, seq, report.Status{Owner: "o", Repo: "r", Revision: testRev, State: report.Success, Description: "all required jobs passed"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := sender.Drain(ctx); n != 0 || len(api.posted()) != 0 {
		t.Fatalf("nothing can be posted during the outage: posted %d, fake saw %v", n, api.posted())
	}

	// Recovery: the fake answers again, the next placement on the hosted runner is the probe.
	api.setMode(modeOK)
	clk.advance(2 * time.Minute)
	probe := s.queue("probe", 1)
	if _, err = s.submit(ctx, probe, "probe", true); err != nil {
		t.Fatalf("the probe slot must let one dispatch through: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for len(api.posted()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	tr.Wait()
	if h = tr.HealthOf(ctx, provID); h.State != "up" {
		t.Fatalf("health after the fake recovered = %+v", h)
	}
	posted := api.posted()
	if len(posted) != 1 || posted[0].State != "success" || posted[0].Context != report.Context || !strings.HasSuffix(posted[0].Path, "/statuses/"+testRev) {
		t.Fatalf("the final state must post once after recovery: %+v", posted)
	}
	if doc := s.explain(hostedJob); strings.Contains(doc, string(model.ProviderDown)) {
		t.Fatalf("a recovered provider must not be listed as down: %s", doc)
	}
	if state, _, _ := s.st.AttemptStateEpoch(ctx, aid); state != "passed" {
		t.Fatalf("the pipeline's job ended %s, want passed", state)
	}
}
