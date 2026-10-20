package usage_test

// Purpose: shared fixtures for the usage tests: a fake GitHub billing API on a local TLS server that
// fails the test on any request outside the one allowed endpoint, a source wired to it, and a ledger
// store. Inputs: synthetic values only (accounts acme and octo, a token that exists nowhere).
// Outputs: helpers; no test lives here. Constraints: no real network.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/usage"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var today = time.Date(2026, 10, 19, 15, 4, 5, 0, time.UTC)

// reply is what the fake answers for one allowed path.
type reply struct {
	status int
	file   string // testdata file, "" for an empty body
	body   string // inline body, used when file is empty
	header map[string]string
}

type fakeBilling struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	routes  map[string]reply // allowed path -> reply
	calls   []string         // "path?query"
	unknown []string         // requests outside the routes
}

func newFakeBilling(t *testing.T, routes map[string]reply) *fakeBilling {
	f := &fakeBilling{t: t, routes: routes}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	t.Cleanup(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.unknown) > 0 {
			t.Errorf("requests outside the allowed billing usage endpoint: %v", f.unknown)
		}
	})
	return f
}

func (f *fakeBilling) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rep, ok := f.routes[r.URL.Path]
	if !ok || r.Method != http.MethodGet || strings.Contains(r.URL.Path, "summary") || strings.Contains(r.URL.Path, "billing/actions") {
		f.unknown = append(f.unknown, r.Method+" "+r.URL.Path)
		http.Error(w, `{"message":"fixture server: unexpected request"}`, 599)
		return
	}
	f.calls = append(f.calls, r.URL.Path+"?"+r.URL.Query().Encode())
	for k, v := range rep.header {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(rep.status)
	if rep.body != "" {
		_, _ = w.Write([]byte(rep.body))
	}
	if rep.file != "" {
		body, err := os.ReadFile(filepath.Join("testdata", rep.file))
		if err != nil {
			f.t.Fatal(err)
		}
		_, _ = w.Write(body)
	}
}

func (f *fakeBilling) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeBilling) client() *rest.Client {
	return &rest.Client{BaseURL: f.srv.URL, AllowPrivateBase: true, APIVersion: "2022-11-28", UserAgent: "usage-test",
		TLSConfig: f.srv.Client().Transport.(*http.Transport).TLSClientConfig,
		Auth:      func(context.Context) (string, error) { return "synthetic-test-token-0001", nil }}
}

func (f *fakeBilling) source(t *testing.T, account string) *usage.GitHub {
	t.Helper()
	acct, err := usage.ParseAccount(account)
	if err != nil {
		t.Fatal(err)
	}
	src, err := usage.New(usage.Config{Client: f.client(), Account: acct})
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func openLedger(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func ledgerCtx(st *store.Store) context.Context {
	return economics.WithEnv(context.Background(), economics.Env{Store: st, Now: func() time.Time { return today }, Coordinator: "usage-test"})
}

// recorder keeps every line it passes to the real ledger writer.
type recorder struct {
	mu    sync.Mutex
	lines []economics.Usage
}

func (r *recorder) record(ctx context.Context, u economics.Usage) error {
	r.mu.Lock()
	r.lines = append(r.lines, u)
	r.mu.Unlock()
	return economics.RecordUsage(ctx, u)
}

func (r *recorder) keys() map[string]economics.Usage {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := map[string]economics.Usage{}
	for _, l := range r.lines {
		m[l.Key] = l
	}
	return m
}

// githubLine reads the github provider line of the report `ci policy usage` prints.
func githubLine(t *testing.T, st *store.Store) economics.ProviderLine {
	t.Helper()
	rep, err := economics.BuildReport(context.Background(), st, schedpolicy.SpendPolicy{}, today)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range rep.Providers {
		if p.Provider == usage.Provider {
			return p
		}
	}
	t.Fatalf("no github provider line in the report: %+v", rep.Providers)
	return economics.ProviderLine{}
}

func economicsPolicy() schedpolicy.SpendPolicy { return schedpolicy.SpendPolicy{} }
