package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type fakeOutcome struct {
	kind   model.JobKind
	change OutcomeChange
	seen   *OutcomeFacts
}

func (f fakeOutcome) Wants(j model.Job) bool { return j.Kind == f.kind }
func (f fakeOutcome) Outcome(_ context.Context, facts OutcomeFacts) (OutcomeChange, bool) {
	if f.seen != nil {
		*f.seen = facts
	}
	return f.change, true
}

func result(state string) ciexec.AttemptResult {
	var r ciexec.AttemptResult
	r.Attempt.State = model.JobState(state)
	return r
}

func TestJudgeTurnsAPassIntoAFailure(t *testing.T) {
	var seen OutcomeFacts
	h := fakeOutcome{kind: "drill", seen: &seen, change: OutcomeChange{Fail: true, Err: errors.New("E640: bad"), Findings: 2, Excerpt: "why"}}
	rs := &runState{hooks: Hooks{Outcomes: []OutcomeHook{h}}}
	out := &outputBuffer{}
	out.write([]byte("hello"))
	res, o := result("passed"), JobOutcome{}
	rs.judge(context.Background(), AttemptInfo{Job: model.Job{Kind: "drill"}}, out, &res, &o)
	if res.Attempt.State != "failed" || res.Attempt.FailureClass != "code" || res.Err == nil || o.Findings != 2 || o.Excerpt != "why" {
		t.Fatalf("res %+v outcome %+v", res.Attempt, o)
	}
	if !seen.Passed || seen.Output != "hello" {
		t.Fatalf("facts %+v", seen)
	}
}

func TestJudgeLeavesOthersAlone(t *testing.T) {
	h := fakeOutcome{kind: "drill", change: OutcomeChange{Fail: true}}
	rs := &runState{hooks: Hooks{Outcomes: []OutcomeHook{h}}}
	out := &outputBuffer{}
	for name, tc := range map[string]struct {
		state string
		job   model.JobKind
	}{"other kind": {"passed", "test"}, "cancelled": {"cancelled", "drill"}} {
		res, o := result(tc.state), JobOutcome{}
		rs.judge(context.Background(), AttemptInfo{Job: model.Job{Kind: tc.job}}, out, &res, &o)
		if string(res.Attempt.State) != tc.state {
			t.Errorf("%s: state became %s", name, res.Attempt.State)
		}
	}
	// A failed attempt keeps failing; the hook only adds findings.
	res, o := result("failed"), JobOutcome{}
	h.change = OutcomeChange{Fail: true, Findings: 1}
	rs.hooks.Outcomes = []OutcomeHook{h}
	rs.judge(context.Background(), AttemptInfo{Job: model.Job{Kind: "drill"}}, out, &res, &o)
	if res.Attempt.State != "failed" || o.Findings != 1 {
		t.Errorf("failed attempt: %+v %+v", res.Attempt, o)
	}
	if rs.wantsOutput(model.Job{Kind: "test"}) || !rs.wantsOutput(model.Job{Kind: "drill"}) {
		t.Error("wantsOutput must follow the hook's Wants")
	}
}

func TestOutputBufferKeepsTheTail(t *testing.T) {
	var b outputBuffer
	b.write([]byte(strings.Repeat("a", OutcomeOutputCap)))
	b.write([]byte("tail"))
	if len(b.text()) != OutcomeOutputCap || !strings.HasSuffix(b.text(), "tail") {
		t.Fatalf("len=%d", len(b.text()))
	}
	for i := 0; i < 3*OutcomeOutputCap/4096; i++ { // many small writes stay bounded
		b.write([]byte(strings.Repeat("b", 4096)))
	}
	b.write([]byte("end"))
	if !b.truncated || len(b.buf) > 2*OutcomeOutputCap || len(b.text()) != OutcomeOutputCap || !strings.HasSuffix(b.text(), "end") {
		t.Fatalf("truncated=%v buf=%d text=%d", b.truncated, len(b.buf), len(b.text()))
	}
}

type panicOutcome struct{}

func (panicOutcome) Wants(j model.Job) bool { return j.Kind == "drill" }
func (panicOutcome) Outcome(context.Context, OutcomeFacts) (OutcomeChange, bool) {
	panic("boom")
}

func TestJudgeFailsAnAttemptWhenTheHookPanics(t *testing.T) {
	rs := &runState{hooks: Hooks{Outcomes: []OutcomeHook{panicOutcome{}}}}
	res, o := result("passed"), JobOutcome{}
	rs.judge(context.Background(), AttemptInfo{Job: model.Job{Kind: "drill"}}, &outputBuffer{}, &res, &o)
	if res.Attempt.State != "failed" || res.Attempt.FailureClass != "infra" || res.Err == nil {
		t.Fatalf("a hook that cannot judge must not let the attempt pass: %+v %v", res.Attempt, res.Err)
	}
}
