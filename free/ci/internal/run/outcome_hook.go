package run

// Purpose: let a package judge a finished attempt from what it printed, before
// the attempt is settled in the store (P7-CI-02: a drill that exits 0 but
// prints a failing ci.evidence document must end failed, never passed).
// Inputs:  the attempt, its exit code and the redacted output (bounded).
// Outputs: an optional OutcomeChange that overturns the end state and adds
// findings and an excerpt to the job's check.
// Constraints: a hook sees only attempts it asked for (Wants); output is the
// last OutcomeOutputCap bytes after the redactor; a hook never turns a
// cancelled attempt into anything else; a hook that panics on a job it wants ends that attempt failed (infra).

import (
	"context"
	"sync"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// OutcomeOutputCap bounds the output kept for an OutcomeHook.
const OutcomeOutputCap = 4 << 20

// OutcomeFacts is what an OutcomeHook sees of an attempt that ended passed or failed.
type OutcomeFacts struct {
	Info      AttemptInfo
	Passed    bool
	ExitCode  int
	Output    string
	Truncated bool // Output starts after the first bytes the attempt printed
}

// OutcomeChange is a hook's verdict. Fail ends a passed attempt failed with
// FailureClass (default code) and Err; Findings and Excerpt always apply.
// Fail with a FailureClass on an attempt that already failed as a code
// failure reclassifies it (a tool the environment lacks is infra, not a
// failing test); a timeout or other class is never overwritten.
type OutcomeChange struct {
	Fail         bool
	FailureClass model.FailureClass
	Err          error
	Detail       string
	Findings     int
	Excerpt      string
}

// OutcomeHook judges finished attempts.
type OutcomeHook interface {
	// Wants reports whether the hook needs the output of a job's attempts.
	Wants(job model.Job) bool
	Outcome(ctx context.Context, f OutcomeFacts) (OutcomeChange, bool)
}

// CheckOutcomeHook is an OutcomeHook that selects attempts by the catalogue
// check behind the job (AttemptInfo.Check) instead of by job shape.
type CheckOutcomeHook interface {
	OutcomeHook
	WantsCheck(check string) bool
}

// hookWants reports whether h asks for the output of this attempt's job.
func hookWants(h OutcomeHook, info AttemptInfo) bool {
	if c, ok := h.(CheckOutcomeHook); ok {
		return c.WantsCheck(info.Check)
	}
	return h.Wants(info.Job)
}

// RegisterOutcome adds an OutcomeHook.
func RegisterOutcome(h OutcomeHook) { Registered.Outcomes = append(Registered.Outcomes, h) }

// outputBuffer keeps the last OutcomeOutputCap bytes of an attempt's output.
type outputBuffer struct {
	mu        sync.Mutex
	buf       []byte
	truncated bool
}

func (o *outputBuffer) write(b []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.buf = append(o.buf, b...)
	if len(o.buf) > 2*OutcomeOutputCap { // trim once per cap bytes, not on every write
		o.buf, o.truncated = append([]byte(nil), o.buf[len(o.buf)-OutcomeOutputCap:]...), true
	}
}

func (o *outputBuffer) text() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.buf) > OutcomeOutputCap {
		return string(o.buf[len(o.buf)-OutcomeOutputCap:])
	}
	return string(o.buf)
}

func (rs *runState) wantsOutput(info AttemptInfo) bool {
	for _, h := range rs.hooks.Outcomes {
		if hookWants(h, info) {
			return true
		}
	}
	return false
}

// judge applies the OutcomeHooks to a finished attempt; it edits res and o in
// place and returns whether the end state changed to failed.
func (rs *runState) judge(ctx context.Context, info AttemptInfo, out *outputBuffer, res *ciexec.AttemptResult, o *JobOutcome) {
	state := res.Attempt.State
	if out == nil || (state != "passed" && state != "failed") {
		return
	}
	f := OutcomeFacts{Info: info, Passed: state == "passed", ExitCode: res.ExitCode, Output: out.text(), Truncated: out.truncated || len(out.buf) > OutcomeOutputCap}
	for _, h := range rs.hooks.Outcomes {
		if !hookWants(h, info) {
			continue
		}
		var ch OutcomeChange
		var ok bool
		if err := guard("outcome hook", func() error { ch, ok = h.Outcome(ctx, f); return nil }); err != nil {
			ch, ok = OutcomeChange{Fail: true, FailureClass: "infra", Err: err}, true // a hook that cannot judge never lets the attempt pass
		}
		if !ok {
			continue
		}
		o.Findings, o.Excerpt = ch.Findings, ch.Excerpt
		if ch.Fail && !f.Passed && ch.FailureClass != "" && (res.Attempt.FailureClass == "" || res.Attempt.FailureClass == "code") {
			res.Attempt.FailureClass = ch.FailureClass
			if res.Err == nil {
				o.Detail = ch.Detail
			}
		}
		if ch.Fail && f.Passed {
			res.Attempt.State, res.Attempt.FailureClass = "failed", ch.FailureClass
			if res.Attempt.FailureClass == "" {
				res.Attempt.FailureClass = "code"
			}
			res.Err = ch.Err
			if ch.Err == nil {
				o.Detail = ch.Detail
			}
		}
		return
	}
}
