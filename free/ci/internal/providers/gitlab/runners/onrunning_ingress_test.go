package runners_test

// Purpose: D-1390. A running Job Hook that enters the GitLab webhook Source pauses the runner of that job
// (review F6): the ingress, not a test, calls Registrar.OnRunning.
// Inputs:  a running Job Hook body, a bare-token Source wired with runners.RunningObserver, fakeGL.
// Outputs: pass or fail.
// Constraints: no live network; the token is synthetic.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/webhook"
)

func TestIngressRunningHookPausesRunner(t *testing.T) {
	ctx := context.Background()
	f := newFakeGL(t)
	r := f.registrar(clock)
	m, err := r.MintRunner(ctx, runnerReq(), runners.MintOptions{})
	if err != nil {
		t.Fatal(err)
	}
	f.jobs[4001] = fakeJob{Ref: "main", Status: "running", RunnerID: itoa64(m.RunnerID)}
	body := []byte(sprintf(runningHook, m.RunnerID))

	const tokenEnv, token = "SYNTH_GL_TOKEN", "synthetic-bare-token"
	src := &webhook.Source{
		Now:    time.Now,
		Getenv: func(k string) string { return map[string]string{tokenEnv: token}[k] },
		Policy: func(context.Context) (webhook.Secrets, error) {
			return webhook.Secrets{Tokens: map[string]webhook.RepoSecret{testProject: {Env: tokenEnv}}}, nil
		},
		OnRunning: runners.RunningObserver(r),
	}
	send := func(tok string, b []byte) error {
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(b))
		req.Header.Set("X-Gitlab-Event", "Job Hook")
		req.Header.Set("X-Gitlab-Token", tok)
		_, err := src.Verify(ctx, req, b)
		return err
	}

	// a delivery with the wrong token is refused before anything runs: no pause
	if err := send("wrong", body); err == nil || len(f.puts) != 0 {
		t.Fatalf("a forged delivery must be refused and pause nothing: %v puts %v", err, f.puts)
	}
	// the authenticated hook pauses the runner and is still declined as a delivery
	if err := send(token, body); err == nil {
		t.Fatal("a running hook is declined, not enqueued")
	}
	if len(f.puts) != 1 || f.puts[0]["paused"] != true || !f.runners[0].Paused {
		t.Fatalf("the runner must be paused: puts %v", f.puts)
	}
	// a replay pauses nothing twice; a body that is not a running hook does nothing
	_ = send(token, body)
	if len(f.puts) != 1 {
		t.Fatalf("a replay must not pause twice: puts %v", f.puts)
	}
	runners.RunningObserver(r)(ctx, []byte(`nope`))
}
