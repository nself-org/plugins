package webhook

// Purpose: D-1390. An authenticated running Job Hook reaches the OnRunning seam (the runner pause of review
// F6) and is still declined as a delivery; nothing unauthenticated or not running reaches it.
// Inputs:  the job_running and job_pending_nself fixtures through Source.Verify.
// Outputs: pass or fail.
// Constraints: no live network.

import (
	"context"
	"sync"
	"testing"
)

type runSpy struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (s *runSpy) hook(_ context.Context, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies = append(s.bodies, append([]byte(nil), body...))
}

func (s *runSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func TestRunningJobHookReachesOnRunning(t *testing.T) {
	body := fixture(t, "job_running")

	t.Run("a signed running hook calls OnRunning once and is still declined", func(t *testing.T) {
		rg, spy := newRig(t, both()), &runSpy{}
		rg.src.OnRunning = spy.hook
		_, err := rg.signed("Job Hook", body)
		wantDecline(t, "running", err, reasonUnsupported, `job status "running"`)
		if spy.count() != 1 || string(spy.bodies[0]) != string(body) {
			t.Fatalf("OnRunning calls: %d", spy.count())
		}
	})

	t.Run("a bare-token project is served too", func(t *testing.T) {
		rg, spy := newRig(t, Secrets{Tokens: map[string]RepoSecret{"acme/app": {Env: tokenEnv}}}), &runSpy{}
		rg.src.OnRunning = spy.hook
		_, _ = rg.bearer("Job Hook", bareToken, body)
		if spy.count() != 1 {
			t.Fatalf("OnRunning calls: %d", spy.count())
		}
	})

	t.Run("a forged delivery never reaches it", func(t *testing.T) {
		rg, spy := newRig(t, both()), &runSpy{}
		rg.src.OnRunning = spy.hook
		d := delivery{event: "Job Hook", id: "msg-1", ts: rg.now, signSecret: otherSecret}
		if _, err := rg.src.Verify(context.Background(), d.request(body), body); err == nil {
			t.Fatal("a wrongly signed hook must be refused")
		}
		if _, err := rg.bearer("Job Hook", "wrong", body); err == nil {
			t.Fatal("a wrong token must be refused")
		}
		if spy.count() != 0 {
			t.Fatalf("OnRunning calls: %d", spy.count())
		}
	})

	t.Run("a pending demand and other jobs do not reach it", func(t *testing.T) {
		rg, spy := newRig(t, both()), &runSpy{}
		rg.src.OnRunning = spy.hook
		if f, err := rg.signed("Job Hook", fixture(t, "job_pending_nself")); err != nil || f.Kind != "runner_demand" {
			t.Fatalf("pending demand: %+v %v", f, err)
		}
		_, _ = rg.signed("Push Hook", fixture(t, "push"))
		if spy.count() != 0 {
			t.Fatalf("OnRunning calls: %d", spy.count())
		}
	})

	t.Run("no hook installed: the running hook is only declined", func(t *testing.T) {
		rg := newRig(t, both())
		_, err := rg.signed("Job Hook", body)
		wantDecline(t, "running", err, reasonUnsupported, `job status "running"`)
	})
}
