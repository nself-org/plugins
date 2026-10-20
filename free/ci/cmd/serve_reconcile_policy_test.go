package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/reconcile"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestServeReconcilePolicyAndStore(t *testing.T) {
	for _, interval := range []string{"45s", "90s"} {
		t.Run(interval, func(t *testing.T) {
			t.Setenv("NSELF_V15", "1")
			dir := serveFixture(t, quietHost())
			st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			err = st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: reconcile.KeyPollInterval, Value: []byte(`"` + interval + `"`), Actor: "test"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			engine := reconcile.Default()
			oldAPI, oldRepos := engine.NewAPI, engine.Repos
			t.Cleanup(func() { engine.NewAPI, engine.Repos = oldAPI, oldRepos })
			api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
			}))
			defer api.Close()
			engine.Repos = func(context.Context) ([]string, error) { return []string{"o/r"}, nil }
			engine.NewAPI = func(context.Context) (*rest.Client, error) {
				return &rest.Client{BaseURL: api.URL, AllowPrivateBase: true, Provider: "github", TLSConfig: api.Client().Transport.(*http.Transport).TLSClientConfig}, nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			code := serveV15(ctx, []string{"--insecure", "--workdir", filepath.Join(dir, "wd"), "--data-dir", dir}, func(c *coordinator.Config) {
				want, _ := time.ParseDuration(interval)
				if got := engine.Demand().Every(); got != want {
					t.Fatalf("poll interval = %v, want %v", got, want)
				}
				if err := engine.Demand().Reconcile(ctx, nil); err != nil {
					t.Fatalf("demand poll: %v", err)
				}
				if _, ok, err := st.ProviderFactGet(ctx, "github", "_token", "ledger"); err != nil || !ok {
					t.Fatalf("poll ledger missing from coordinator store: %v %v", ok, err)
				}
				c.Listen = func(network, addr string) (net.Listener, error) {
					l, e := net.Listen(network, "127.0.0.1:0")
					cancel()
					return l, e
				}
			})
			if code != 0 {
				t.Fatalf("serve = %d", code)
			}
			if got := engine.Demand().Every(); got != reconcile.DefaultPollInterval {
				t.Fatalf("policy retained: %v", got)
			}
		})
	}
}
