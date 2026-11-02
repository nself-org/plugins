package main

// Purpose: exercise startup dependency wiring without sockets, including its cleanup.
// Inputs: operator rows and a fixture API factory that stops before network access.
// Outputs: each reconciler reaches the factory only with both store and policy installed.
// Constraints: uses the same startup helper as TEAM serve; no external credentials.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/reconcile"
	ghwebhook "github.com/nself-org/plugins/free/ci/internal/providers/github/webhook"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestServeGitHubStartupDependencies(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_ALLOWED_REPOS", "")
	_, _, _ = newServeV15Flags()
	st := serveTestStore(t)
	for key, value := range map[string]string{
		ghwebhook.KeyWebhookSecrets: `{"owner/repo":"CI_SERVE_REPO_SECRET"}`,
		reconcile.KeyPollInterval:   `"47s"`,
	} {
		if err := st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject,
			Key: key, Value: json.RawMessage(value), Actor: "test"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	e := reconcile.Default()
	oldAPI := e.NewAPI
	stop := errors.New("fixture API reached")
	e.NewAPI = func(context.Context) (*rest.Client, error) { return nil, stop }
	t.Cleanup(func() { e.NewAPI = oldAPI })
	undo := serveHosted(st, t.TempDir(), nil)
	t.Cleanup(func() {
		undo()
		if ghwebhook.Default.HasSecrets() || e.Demand().Every() != reconcile.DefaultPollInterval {
			t.Error("startup cleanup retained GitHub policy")
		}
	})
	if !ghwebhook.Default.HasSecrets() {
		t.Error("GitHub webhook did not receive operator policy")
	}
	if got := e.Demand().Every(); got != 47*time.Second {
		t.Errorf("demand interval = %s, want 47s before Run", got)
	}
	for _, r := range []coordinator.Reconciler{e.GitHub(), e.Demand()} {
		t.Run(r.Name(), func(t *testing.T) {
			if err := r.Reconcile(context.Background(), nil); !errors.Is(err, stop) {
				t.Fatalf("reconciler did not reach fixture API with store and policy: %v", err)
			}
		})
	}
}
