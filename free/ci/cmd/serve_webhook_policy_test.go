package main

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/webhook"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestServeWebhookPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, value, env string
		id               int64
	}{
		{"named", `{"o/r":"TEST_REPO_SECRET"}`, "TEST_REPO_SECRET", 0},
		{"pinned", `{"o/r":{"env":"TEST_PINNED_SECRET","id":42}}`, "TEST_PINNED_SECRET", 42},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NSELF_V15", "1")
			dir := serveFixture(t, quietHost())
			st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			err = st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: webhook.KeyWebhookSecrets, Value: []byte(tc.value), Actor: "test"}, nil)
			_ = st.Close()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			checked := false
			code := serveV15(ctx, []string{"--insecure", "--workdir", filepath.Join(dir, "wd"), "--data-dir", dir}, func(c *coordinator.Config) {
				got, err := webhook.New().Secrets(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if got["o/r"] != (webhook.RepoSecret{Env: tc.env, ID: tc.id}) {
					t.Fatalf("coordinator did not install repository policy: %v", got)
				}
				checked = true
				c.Listen = func(network, addr string) (net.Listener, error) {
					l, e := net.Listen(network, "127.0.0.1:0")
					cancel()
					return l, e
				}
			})
			if code != 0 || !checked {
				t.Fatalf("serve = %d, checked = %v", code, checked)
			}
			got, err := webhook.New().Secrets(context.Background())
			if err != nil || len(got) != 0 {
				t.Fatalf("policy retained after shutdown: %v %v", got, err)
			}
		})
	}
}
