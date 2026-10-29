//go:build integration

package sim

// Purpose: TestWriteLoad, the TEAM store write-load leg (P7-NODE-17): 50 agents renew a lease every 10 s
//          while the webhook intake takes 20 deliveries per second, for -write-load-for (10 minutes), against
//          one durable state.db. It asserts zero 503s and zero failed renewals and records the p99 write latency.
// Inputs:  -write-load-for, -perf-json; INTEGRATION=1.
// Outputs: metric store.write_p99_ms (ms): the p99 of the intake write latency (the webhook handler answers 2xx
//          only after the trigger is stored) in each 10 s window of the run, so p50/p95/max are taken over the
//          windows. The renew latency and the totals are logged.
// Constraints: in-process (the real coordinator Handler, lease engine and store); no Docker, no network beyond
//          the loopback of the test; deliveries are open-loop (one goroutine per tick), so a slow store shows up as
//          latency and concurrency, not as a lower rate.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	loadAgents    = 50
	loadBeat      = 10 * time.Second
	loadHooksPerS = 20
	loadWindow    = 10 * time.Second
	loadSecret    = "write-load-secret"
)

type stamp struct {
	at time.Time
	ms float64
}

type loadResult struct {
	mu      sync.Mutex
	hooks   []stamp
	renews  []stamp
	codes   map[int]int
	renewEr int
}

func (r *loadResult) hook(at time.Time, ms float64, code int) {
	r.mu.Lock()
	r.hooks = append(r.hooks, stamp{at, ms})
	r.codes[code]++
	r.mu.Unlock()
}

func (r *loadResult) renew(at time.Time, ms float64, err error) {
	r.mu.Lock()
	r.renews = append(r.renews, stamp{at, ms})
	if err != nil {
		r.renewEr++
	}
	r.mu.Unlock()
}

// windowP99 groups stamps into windows of loadWindow from start and returns each window's p99.
func windowP99(s []stamp, start time.Time) []float64 {
	by := map[int][]float64{}
	for _, v := range s {
		by[int(v.at.Sub(start)/loadWindow)] = append(by[int(v.at.Sub(start)/loadWindow)], v.ms)
	}
	keys := make([]int, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	var out []float64
	for _, k := range keys {
		sort.Float64s(by[k])
		out = append(out, quantile(by[k], 0.99))
	}
	return out
}

func allMS(s []stamp) []float64 {
	out := make([]float64, 0, len(s))
	for _, v := range s {
		out = append(out, v.ms)
	}
	sort.Float64s(out)
	return out
}

func grantLeases(t *testing.T, st *store.Store, eng *lease.Engine) (ids []string, epochs []int64) {
	t.Helper()
	ctx := context.Background()
	err := st.CreatePipeline(ctx, store.PipelineRow{ID: "pload", Revision: "rev", Project: "o/r", Trigger: "local", SourceTrust: "owner",
		PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < loadAgents; i++ {
		a := fmt.Sprintf("load-a%02d", i)
		if err = st.CreateJob(ctx, store.JobRow{ID: "job-" + a, PipelineID: "pload", Name: a, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
			t.Fatal(err)
		}
		if err = st.CreateAttempt(ctx, store.AttemptRow{ID: a, JobID: "job-" + a, N: 1}, ""); err != nil {
			t.Fatal(err)
		}
		lid := fmt.Sprintf("load-l%02d", i)
		epoch, err := eng.Grant(ctx, "host", store.Demand{AttemptID: a, LeaseID: lid, RunnerID: fmt.Sprintf("node-%02d", i), CPU: 1, MemMB: 1,
			CapacityCPU: 1000, CapacityMemMB: 1000, TTL: eng.Config.TTL}, 1)
		if err != nil {
			t.Fatalf("grant %s: %v", lid, err)
		}
		ids, epochs = append(ids, lid), append(epochs, epoch)
	}
	return ids, epochs
}

func sign(body []byte) string {
	m := hmac.New(sha256.New, []byte(loadSecret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func hookRequest(n int) *http.Request {
	body, _ := json.Marshal(map[string]any{"ref": "refs/heads/main", "after": fmt.Sprintf("%040x", n+1),
		"repository": map[string]string{"full_name": "o/r", "clone_url": "https://github.com/o/r.git"}})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", fmt.Sprintf("load-%d", n))
	req.Header.Set("X-Hub-Signature-256", sign(body))
	return req
}

func TestWriteLoad(t *testing.T) {
	integration(t)
	dur := *writeLoadFor
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	eng := lease.New(st, nil, lease.Config{})
	ids, epochs := grantLeases(t, st, eng)
	c, err := coordinator.New(coordinator.Config{Store: st, Secret: loadSecret, AllowedRepos: []string{"o/r"}, PolicyDigest: "pd"})
	if err != nil {
		t.Fatal(err)
	}
	h := c.Handler(true)
	res := &loadResult{codes: map[int]int{}}
	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()
	start := time.Now()
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select { // spread the agents over one beat, as a fleet that started at different moments
			case <-time.After(time.Duration(i) * loadBeat / loadAgents):
			case <-ctx.Done():
				return
			}
			tick := time.NewTicker(loadBeat)
			defer tick.Stop()
			for {
				t0 := time.Now()
				err := eng.Renew(context.Background(), ids[i], epochs[i], 0)
				res.renew(t0, ms(time.Since(t0)), err)
				select {
				case <-tick.C:
				case <-ctx.Done():
					return
				}
			}
		}(i)
	}
	tick := time.NewTicker(time.Second / loadHooksPerS)
	defer tick.Stop()
	for n := 0; ; n++ {
		select {
		case <-ctx.Done():
			wg.Wait()
			finishLoad(t, res, start, dur)
			return
		case <-tick.C:
		}
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			t0 := time.Now()
			h.ServeHTTP(w, hookRequest(n))
			res.hook(t0, ms(time.Since(t0)), w.Code)
		}(n)
	}
}

func finishLoad(t *testing.T, res *loadResult, start time.Time, dur time.Duration) {
	t.Helper()
	res.mu.Lock()
	defer res.mu.Unlock()
	if n := res.codes[http.StatusServiceUnavailable]; n != 0 {
		t.Errorf("%d deliveries were answered 503", n)
	}
	for code, n := range res.codes {
		if code != http.StatusAccepted {
			t.Errorf("%d deliveries were answered %d, want 202", n, code)
		}
	}
	if res.renewEr != 0 {
		t.Errorf("%d lease renewals failed", res.renewEr)
	}
	wins := windowP99(res.hooks, start)
	hooks, renews := allMS(res.hooks), allMS(res.renews)
	t.Logf("write load %s: %d deliveries (codes %v), %d renewals, delivery p50 %.2f p99 %.2f max %.2f ms, renew p99 %.2f ms, %d windows",
		dur, len(hooks), res.codes, len(renews), quantile(hooks, 0.50), quantile(hooks, 0.99), quantile(hooks, 1), quantile(renews, 0.99), len(wins))
	if len(hooks) < int(dur/time.Second)*loadHooksPerS*9/10 {
		t.Errorf("only %d deliveries were sent in %s; the rate fell short", len(hooks), dur)
	}
	recordPerf("write-load", "store.write_p99_ms", "ms", fmt.Sprintf("p99 of the intake write per %s window; %d agents at %s, %d deliveries/s, %s", loadWindow, loadAgents, loadBeat, loadHooksPerS, dur), wins)
}
