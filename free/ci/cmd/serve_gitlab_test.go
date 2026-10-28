package main

// Purpose: the GitLab wiring of the coordinator start (D-1346): the commit-status reporter posts running and
//          canceled from the run the store holds, a status of another project or revision is not refined, and
//          the undo restores the plain mapping.
// Inputs:  a real store with one run, the registered GitLab reporter over a recording TLS server.
// Outputs: pass or fail.
// Constraints: no network beyond loopback.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/status"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

const gitlabRev = "0123456789abcdef0123456789abcdef01234567"

// gitlabPoster returns a func that posts a status through the registered GitLab reporter and returns the
// state GitLab was sent.
func gitlabPoster(t *testing.T) func(owner, repo, rev string, s report.State) string {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	_, rep, ok := report.DefaultRegistry.Resolve("https://gitlab.com/o/r.git")
	gl, isGitLab := rep.(*status.Reporter)
	if !ok || !isGitLab {
		t.Fatalf("no registered GitLab reporter: %T", rep)
	}
	var (
		mu   sync.Mutex
		sent string
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b struct{ State string }
		_ = json.Unmarshal(raw, &b)
		mu.Lock()
		sent = b.State
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	gl.Client = func(context.Context) (*rest.Client, error) {
		return &rest.Client{BaseURL: srv.URL + "/api/v4", AllowPrivateBase: true, UserAgent: "serve-gitlab-test", Provider: "gitlab",
			TLSConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig,
			Auth:      func(context.Context) (string, error) { return "synthetic-gitlab-token", nil }}, nil
	}
	t.Cleanup(func() { gl.Client = nil; status.UsePhase(nil) })
	return func(owner, repo, rev string, s report.State) string {
		t.Helper()
		if err := gl.Post(context.Background(), report.Status{Owner: owner, Repo: repo, Revision: rev, State: s, Description: "d"}); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		defer mu.Unlock()
		return sent
	}
}

func seedGitLabRun(t *testing.T, st *store.Store, attemptState string) {
	t.Helper()
	ctx := context.Background()
	if err := st.CreatePipeline(ctx, store.PipelineRow{ID: "glpipe", Project: "o/r", Revision: gitlabRev, Trigger: "push", SourceTrust: "owner", PrivacyZone: "hosted-allowed", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(ctx, store.JobRow{ID: "gljob", PipelineID: "glpipe", Name: "job", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: "glatt", JobID: "gljob", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	step := func(from, to model.JobState, epoch int64, class model.FailureClass) {
		t.Helper()
		if err := st.Transition(ctx, "glatt", from, to, store.TransitionDetail{Epoch: epoch, FailureClass: class}); err != nil {
			t.Fatalf("%s -> %s: %v", from, to, err)
		}
	}
	if attemptState == "queued" {
		return
	}
	if err := storetest.Admit(ctx, st, "glatt"); err != nil {
		t.Fatal(err)
	}
	step("leased", "running", 1, "")
	switch attemptState {
	case "cancelled":
		step("running", "cancelled", 2, "cancelled")
	case "failed":
		step("running", "failed", 2, "test")
	}
}

// TestServeGitLabPhase: with the wiring a pending status of a running run is sent as running and a failed
// status of a cancelled run as canceled; without it, for another project or another revision, nothing is refined.
func TestServeGitLabPhase(t *testing.T) {
	post := gitlabPoster(t)
	for _, tc := range []struct {
		name, attempt string
		state         report.State
		owner, rev    string
		want          string
		unwired       bool
	}{
		{"running run", "running", report.Pending, "o", gitlabRev, "running", false},
		{"queued run stays pending", "queued", report.Pending, "o", gitlabRev, "pending", false},
		{"cancelled run", "cancelled", report.Failure, "o", gitlabRev, "canceled", false},
		{"failed run stays failed", "failed", report.Failure, "o", gitlabRev, "failed", false},
		{"success is never refined", "running", report.Success, "o", gitlabRev, "success", false},
		{"another project is not refined", "running", report.Pending, "x", gitlabRev, "pending", false},
		{"another revision is not refined", "running", report.Pending, "o", strings.Repeat("f", 40), "pending", false},
		{"without the wiring", "running", report.Pending, "o", gitlabRev, "pending", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := serveTestStore(t)
			seedGitLabRun(t, st, tc.attempt)
			if !tc.unwired {
				undo := serveGitLab(st)
				t.Cleanup(undo)
			}
			if got := post(tc.owner, "r", tc.rev, tc.state); got != tc.want {
				t.Fatalf("sent %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("the undo restores the plain mapping", func(t *testing.T) {
		st := serveTestStore(t)
		seedGitLabRun(t, st, "running")
		undo := serveGitLab(st)
		if got := post("o", "r", gitlabRev, report.Pending); got != "running" {
			t.Fatalf("wired: sent %q", got)
		}
		undo()
		if got := post("o", "r", gitlabRev, report.Pending); got != "pending" {
			t.Fatalf("after undo: sent %q", got)
		}
	})
}
