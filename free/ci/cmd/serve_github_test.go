package main

// Purpose: prove TEAM startup installs GitHub webhook and reconciliation dependencies.
// Inputs: operator policy rows, signed deliveries and a loopback fixture forge.
// Outputs: durable intake rows and reconciliation marks from the running coordinator.
// Constraints: no external network, credentials or dispatches; all defaults restored after shutdown.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/reconcile"
	ghwebhook "github.com/nself-org/plugins/free/ci/internal/providers/github/webhook"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const githubServeRev = "0123456789abcdef0123456789abcdef01234567"

// startGitHubServe seeds real operator rows and starts the production TEAM entrypoint.
func startGitHubServe(t *testing.T, values map[string]string, allowed string, forge http.Handler) (*store.Store, string) {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_ALLOWED_REPOS", "")
	t.Setenv("GITHUB_WEBHOOK_SECRET", "")
	dir := serveFixture(t, quietHost())
	st, err := cirun.OpenStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for key, value := range values {
		err := st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject,
			Key: key, Value: json.RawMessage(value), Actor: "test"}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewTLSServer(forge)
	t.Cleanup(srv.Close)
	e := reconcile.Default()
	oldAPI := e.NewAPI
	e.NewAPI = func(context.Context) (*rest.Client, error) {
		return &rest.Client{BaseURL: srv.URL, AllowPrivateBase: true, Provider: "github", UserAgent: "serve-test",
			TLSConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig,
			Auth:      func(context.Context) (string, error) { return "fixture-token", nil }}, nil
	}
	t.Cleanup(func() { e.NewAPI = oldAPI })
	ctx, cancel := context.WithCancel(context.Background())
	exit, bound := make(chan int, 1), make(chan string, 1)
	hook := func(c *coordinator.Config) {
		c.Listen = func(network, _ string) (net.Listener, error) {
			ln, err := net.Listen(network, "127.0.0.1:0")
			if err == nil {
				bound <- "http://" + ln.Addr().String()
			}
			return ln, err
		}
	}
	go func() {
		exit <- serveV15(ctx, []string{"--secret", "shared-fixture-secret", "--workdir", filepath.Join(dir, "wd"), "--data-dir", dir,
			"--allowed-repos", allowed, "--agent-addr", "", "--enroll-addr", ""}, hook)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case code := <-exit:
			if code != 0 {
				t.Errorf("serve exit = %d", code)
			}
		case <-time.After(15 * time.Second):
			t.Error("serve did not stop")
		}
	})
	select {
	case addr := <-bound:
		return st, addr
	case <-time.After(15 * time.Second):
		t.Fatal("serve never listened")
		return nil, ""
	}
}

func TestServeGitHubPerRepoSecret(t *testing.T) {
	for _, tc := range []struct {
		name, secret string
		status, rows int
	}{
		{"per-repo signature enqueues delivery", "repo-fixture-secret", http.StatusAccepted, 1},
		{"shared signature cannot authenticate configured repository", "shared-fixture-secret", http.StatusForbidden, 0},
		{"invalid signature is refused", "wrong-secret", http.StatusForbidden, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CI_SERVE_REPO_SECRET", "repo-fixture-secret")
			st, addr := startGitHubServe(t, map[string]string{
				ghwebhook.KeyWebhookSecrets: `{"owner/repo":{"env":"CI_SERVE_REPO_SECRET","id":42}}`,
			}, "owner/repo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "fixture offline", http.StatusForbidden) }))
			body := `{"repository":{"full_name":"owner/repo","id":42},"client_payload":{"revision":"` + githubServeRev + `"}}`
			mac := hmac.New(sha256.New, []byte(tc.secret))
			_, _ = mac.Write([]byte(body))
			req, err := http.NewRequest(http.MethodPost, addr+"/webhook", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-GitHub-Event", "repository_dispatch")
			req.Header.Set("X-GitHub-Delivery", "serve-per-repo")
			req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.status, raw)
			}
			rows, err := st.IntakeList(context.Background(), 10)
			if err != nil || len(rows) != tc.rows {
				t.Fatalf("intake = %v, %v; want %d rows", rows, err, tc.rows)
			}
			if len(rows) == 1 && (rows[0].Project != "owner/repo" || rows[0].Revision != githubServeRev || !strings.Contains(string(rows[0].Facts), `"auth":"per-repo"`)) {
				t.Fatalf("unexpected intake facts: %+v", rows[0])
			}
		})
	}
}

func TestServeGitHubReconciliation(t *testing.T) {
	for _, ingress := range []string{"poll", "webhook"} {
		t.Run(ingress, func(t *testing.T) {
			forge := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("ETag", `"fixture"`)
				switch r.URL.Path {
				case "/repos/owner/repo":
					fmt.Fprint(w, `{"id":42,"full_name":"owner/repo","default_branch":"main","visibility":"public"}`)
				case "/repos/owner/repo/pulls":
					fmt.Fprint(w, `[]`)
				case "/repos/owner/repo/branches":
					fmt.Fprintf(w, `[{"name":"main","protected":true,"commit":{"sha":%q}}]`, githubServeRev)
				case "/repos/owner/repo/commits/" + githubServeRev + "/status":
					fmt.Fprint(w, `{"statuses":[]}`)
				case "/repos/owner/repo/actions/runs":
					fmt.Fprint(w, `{"workflow_runs":[]}`)
				case "/repos/owner/repo/hooks":
					fmt.Fprint(w, `[{"id":7,"active":true,"events":["push"],"config":{"url":"https://ci.example/webhook"}}]`)
				case "/repos/owner/repo/hooks/7/deliveries":
					fmt.Fprintf(w, `[{"id":8,"guid":"missed","delivered_at":%q,"status_code":500}]`, time.Now().UTC().Format(time.RFC3339))
				case "/repos/owner/repo/hooks/7/deliveries/8/attempts":
					if r.Method != http.MethodPost {
						t.Errorf("redelivery method = %s", r.Method)
					}
					w.WriteHeader(http.StatusAccepted)
					fmt.Fprint(w, `{}`)
				default:
					t.Errorf("unexpected forge request: %s", r.URL)
					http.NotFound(w, r)
				}
			})
			st, _ := startGitHubServe(t, map[string]string{
				ghwebhook.KeyWebhookSecrets: `{"owner/repo":"CI_SERVE_REPO_SECRET"}`,
				reconcile.KeyIngress:        fmt.Sprintf("%q", ingress),
				reconcile.KeyPollInterval:   `"47s"`,
				reconcile.KeyHookURL:        `"https://ci.example"`,
			}, "", forge)
			// serve applies the operator's settings after it starts listening: wait for them, bounded
			settled := time.Now().Add(5 * time.Second)
			got := reconcile.Default().Demand().Every()
			for got != 47*time.Second && time.Now().Before(settled) {
				time.Sleep(10 * time.Millisecond)
				got = reconcile.Default().Demand().Every()
			}
			if got != 47*time.Second {
				t.Fatalf("demand interval = %s, want operator's 47s", got)
			}
			key := "enq|owner/repo|" + githubServeRev
			if ingress == "webhook" {
				key = "redeliver|owner/repo|7:missed"
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				_, ok, err := st.ProviderMarkGet(context.Background(), "github-reconcile", key)
				_, polled, perr := st.ProviderFactGet(context.Background(), "github", "owner/repo", "etag:runs-queued")
				if err != nil || perr != nil {
					t.Fatalf("read reconciliation state: %v, %v", err, perr)
				}
				if ok && (ingress == "webhook" || polled) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("startup did not persist %s (mark=%v, demand poll=%v)", key, ok, polled)
				}
				time.Sleep(10 * time.Millisecond)
			}
			rows, err := st.IntakeList(context.Background(), 10)
			want := 1
			if ingress == "webhook" {
				want = 0
			}
			if err != nil || len(rows) != want {
				t.Fatalf("intake rows = %v, %v; want %d", rows, err, want)
			}
		})
	}
}
