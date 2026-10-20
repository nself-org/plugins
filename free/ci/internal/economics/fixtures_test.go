package economics

// Shared fixtures: a private store, a spend policy built the way the loader
// builds it (operator scope), queued attempts, admission requests and an Env
// with an injected clock. All values are synthetic.

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// clock is a settable wall clock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) set(v time.Time) {
	c.mu.Lock()
	c.t = v
	c.mu.Unlock()
}

func at(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}

func openStore(t testing.TB, clk *clock) *store.Store {
	t.Helper()
	opts := store.Options{}
	if clk != nil {
		opts.Clock = clk.now
	}
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path, opts)
	if err != nil {
		t.Fatal(err)
	}
	dbPaths.Store(st, path)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// spendPolicy resolves kv at operator scope, then applies planted at the given
// non-operator scope kinds, as the real loader does.
func spendPolicy(t testing.TB, kv map[string]string, planted ...map[string]string) Policy {
	t.Helper()
	scope := func(kind trust.Source, m map[string]string) trust.Scope {
		v := map[string]json.RawMessage{}
		for k, raw := range m {
			v[k] = json.RawMessage(raw)
		}
		return trust.Scope{Kind: kind, Values: v}
	}
	scopes := []trust.Scope{scope(trust.SourceProject, kv)}
	for _, pl := range planted {
		scopes = append(scopes, scope(trust.SourceRevision, pl))
	}
	reg, err := trust.NewRegistry(policy.Default.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	eff, digest, err := reg.Merge("", scopes...)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{}
	names := map[string]bool{}
	for _, s := range policy.Default.Keys() {
		names[s.Key] = true
	}
	for _, sc := range scopes {
		for k := range sc.Values {
			names[k] = true
		}
	}
	for k := range names {
		if v, _, ok := eff.Value(k); ok {
			values[k] = v
		}
	}
	p, err := schedpolicy.Spend(&policy.Resolved{Effective: eff, Values: values, Digest: digest})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// paidKV is a plausible paid operator policy for provider "cloudco": 3.6 million micro per hour, a
// one-minute increment, so a 60 s job costs 60 000 micro.
func paidKV(extra map[string]string) map[string]string {
	kv := map[string]string{
		schedpolicy.KeyPaidEnabled: `true`, schedpolicy.KeyBudgetDay: `1000000`, schedpolicy.KeyBudgetMonth: `5000000`,
		"economics.rates.cloudco": `{"class":"metered","rate_per_hour_micro":3600000,"billing_increment":"1m"}`,
	}
	for k, v := range extra {
		kv[k] = v
	}
	return kv
}

func envFor(st *store.Store, p Policy, clk *clock, coordinator string) Env {
	return Env{Store: st, Spend: StaticSpend(p), Now: clk.now, Coordinator: coordinator}
}

var rowSeq atomic.Int64

// queueAttempt creates pipeline pipe (revision rev), a job and its queued attempt.
func queueAttempt(t testing.TB, st *store.Store, pipe, rev, project string) string {
	t.Helper()
	ctx := context.Background()
	_ = st.CreatePipeline(ctx, store.PipelineRow{ID: pipe, Revision: rev, Project: project, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "cfg1", SelectionMode: "full", Status: "running"})
	id := pipe + ".job" + strconv.FormatInt(rowSeq.Add(1), 10)
	if err := st.CreateJob(ctx, store.JobRow{ID: "job-" + id, PipelineID: pipe, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "job-" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	return id
}

// request is an admission request for attempt on a runner of class and provider.
func request(attempt, class, provider string, timeoutMs, cost int64) dispatch.Request {
	return dispatch.Request{AttemptID: attempt, Placement: sched.Placement{JobKey: attempt, RunnerID: "r-" + provider, CostMicro: cost, CPU: 1, MemMB: 256},
		Runner: sched.Runner{ID: "r-" + provider, Class: class, Provider: provider}, Job: sched.Job{Key: attempt, TimeoutMs: timeoutMs}, Local: true,
		Host: "host-" + attempt, CapacityCPU: 8, CapacityMemMB: 8192}
}

func place(ctx context.Context, st *store.Store, r dispatch.Request) error {
	_, err := dispatch.Admit(ctx, st, r)
	return err
}

func refusalOf(t testing.TB, err error) *dispatch.Refusal {
	t.Helper()
	var ref *dispatch.Refusal
	if !errors.As(err, &ref) {
		t.Fatalf("want a refusal, got %v", err)
	}
	return ref
}

var dbPaths sync.Map

// dbPathOf is the database file of a store opened by openStore.
func dbPathOf(st *store.Store) string { v, _ := dbPaths.Load(st); return v.(string) }
