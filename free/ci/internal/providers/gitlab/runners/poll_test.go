package runners_test

// Purpose: the GitLab pending-job poll tests (D-1389): a demand the handler declined for a cap, or before
// the ledger was bound, is offered again on the next tick and runs once a slot is free.
// Inputs:  a TLS fake of GET /projects/:id/jobs, a real store behind the ledger, a recording Enqueuer.
// Outputs: pass or fail.
// Constraints: no live network; the ledger clock is the test's own so a pending slot can expire.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type pendingJob struct {
	ID   int64
	Tags []string
	SHA  string
}

// fakePending serves GET /projects/:id/jobs?scope[]=pending for one project.
type fakePending struct {
	srv   *httptest.Server
	mu    sync.Mutex
	jobs  []pendingJob
	calls []string
}

func newFakePending(t *testing.T) *fakePending {
	f := &fakePending{}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePending) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.EscapedPath()+"?"+r.URL.RawQuery)
	if r.Method != "GET" || r.URL.EscapedPath() != "/projects/acme%2Fwidgets/jobs" || r.URL.Query().Get("scope[]") != "pending" {
		w.WriteHeader(404)
		return
	}
	rows := []map[string]any{} // GitLab lists the newest job first
	for i := len(f.jobs) - 1; i >= 0; i-- {
		j := f.jobs[i]
		rows = append(rows, map[string]any{"id": j.ID, "status": "pending", "tag_list": j.Tags, "ref": "main",
			"commit": map[string]any{"id": j.SHA}, "pipeline": map[string]any{"id": 3000 + j.ID}})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rows)
}

func (f *fakePending) set(jobs ...pendingJob) { f.mu.Lock(); f.jobs = jobs; f.mu.Unlock() }

type recordingEnq struct{ got []coordinator.Delivery }

func (e *recordingEnq) Enqueue(_ context.Context, d coordinator.Delivery) (coordinator.Outcome, error) {
	e.got = append(e.got, d)
	return coordinator.Outcome{Kind: store.IntakeEnqueued}, nil
}

func (e *recordingEnq) demands() []string {
	var out []string
	for _, d := range e.got {
		var f model.TriggerFacts
		if json.Unmarshal(d.Facts, &f) == nil && f.ProviderJobID.Value != nil {
			out = append(out, *f.ProviderJobID.Value)
		}
	}
	return out
}

func joined(s []string) string { return strings.Join(s, ",") }

func pollerFor(f *fakePending, maxPending int) *runners.Poller {
	return &runners.Poller{
		Client: func(context.Context) (*rest.Client, error) {
			c := (&fakeGL{srv: f.srv}).client(runnerToken)
			return c, nil
		},
		Projects: func(context.Context) ([]string, error) { return []string{testProject}, nil },
		Handler:  handler(maxPending),
	}
}

// TestPollOffersACappedDemandAgainOnceASlotFrees is the D-1389 regression: with a cap of one the second
// pending job is declined E703 and, with no poll, stayed pending until GitLab's stuck-job timeout. The poll
// offers it again, and it runs after the first demand's slot expires.
func TestPollOffersACappedDemandAgainOnceASlotFrees(t *testing.T) {
	now := epoch
	t.Setenv("NSELF_V15", "1")
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	runnerjob.Use(runnerjob.NewLedger(st, func() time.Time { return now }))
	t.Cleanup(func() { runnerjob.Use(nil); _ = st.Close() })

	f := newFakePending(t)
	p := pollerFor(f, 1)
	ctx := context.Background()
	f.set(
		pendingJob{ID: 5001, Tags: []string{"nself"}, SHA: simSHA},
		pendingJob{ID: 5002, Tags: []string{"nself"}, SHA: simSHA},
		pendingJob{ID: 5003, Tags: []string{"other"}, SHA: simSHA},
		pendingJob{ID: 5004, Tags: []string{"nself", "ubuntu-latest"}, SHA: simSHA},
	)

	enq := &recordingEnq{}
	if err = p.Reconcile(ctx, enq); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if got := joined(enq.demands()); got != "5001" {
		t.Fatalf("tick 1 offered %q, want only the oldest nself job 5001 (5002 is capped, 5003 and 5004 are not ours)", got)
	}
	if !strings.Contains(joined(f.calls), "per_page=") {
		t.Fatalf("the list is not paged: %v", f.calls)
	}

	// 5001 keeps its slot while it waits: the capped job is offered again and declined again, not lost
	enq = &recordingEnq{}
	if err = p.Reconcile(ctx, enq); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if got := joined(enq.demands()); got != "5001" {
		t.Fatalf("tick 2 offered %q, want 5001 again and 5002 still capped", got)
	}

	// 5001 left the queue (a runner took it elsewhere) and its pending slot ran out
	now = now.Add(runnerjob.PendingTTL + time.Minute)
	f.set(pendingJob{ID: 5002, Tags: []string{"nself"}, SHA: simSHA})
	enq = &recordingEnq{}
	if err = p.Reconcile(ctx, enq); err != nil {
		t.Fatalf("tick 3: %v", err)
	}
	if got := joined(enq.demands()); got != "5002" {
		t.Fatalf("tick 3 offered %q, want the formerly capped job 5002 now that a slot is free", got)
	}
	d := enq.got[0]
	if d.Source != "gitlab" || d.Project != testProject || d.Revision != simSHA || d.TriggerClass != "runner_demand" || d.Synthetic == nil ||
		d.DeliveryID != "poll:job:"+strconv.Itoa(5002) {
		t.Fatalf("delivery %+v", d)
	}
}

// TestPollDoesNothingBeforeV15 keeps the poll dormant: no request leaves the process.
func TestPollDoesNothingBeforeV15(t *testing.T) {
	t.Setenv("NSELF_V15", "")
	f := newFakePending(t)
	f.set(pendingJob{ID: 5001, Tags: []string{"nself"}, SHA: simSHA})
	enq := &recordingEnq{}
	if err := pollerFor(f, 2).Reconcile(context.Background(), enq); err != nil || len(f.calls) != 0 || len(enq.got) != 0 {
		t.Fatalf("err %v calls %v offered %d", err, f.calls, len(enq.got))
	}
}
