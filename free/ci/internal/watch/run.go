package watch

// Purpose: one claimed attempt (P7-AUTO-11): run it under the SIGTERM drain, read the ledger back, and
//          post the one result, aggregating the residual checks of a hosted use.
// Inputs:  the claimed Head, its Job and the hosted decision.
// Outputs: a Decision with Action run; the result status through report.Reporter; on a pinned
//          repository a residual failure also raises the notifier failure alert.
// Constraints: a cancelled parent context waits watch.drain_sec for the attempt, then cancels it, so the
//          attempt ends cancelled or finished and the coordinator releases its lease; the result is posted
//          only while this host's claim still holds (claims.MayPublish), else the attempt is fenced and
//          nothing is posted; a cancelled attempt ends as `error` so its claim is closed, not orphaned;
//          an unpinned hosted use posts nself-ci as the aggregate (failure when either part failed); a
//          pinned one posts nself-ci/residual and never touches the Actions result.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/claims"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// postTimeout bounds the read-back and the result post after a drain cancelled the pass.
const postTimeout = 30 * time.Second

// run executes j for h and posts its result.
func (w *Watcher) run(ctx context.Context, h Head, j Job, hu *hostedUse, rep *PassReport) Decision {
	d := Decision{Repo: h.Repo, SHA: h.SHA, Action: "run", Context: j.Context}
	w.log("run", map[string]any{"repo": h.Repo, "sha": h.SHA, "context": j.Context, "residual": len(j.Only)})
	var out Outcome
	if hu != nil && hu.use && len(j.Only) == 0 {
		out = Outcome{State: report.Success, Description: "hosted result"} // nothing residual: the aggregate is the hosted result
	} else {
		out = w.attempt(ctx, j)
	}
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), postTimeout)
	defer cancel()
	in, err := w.history(pctx, h, j.Context)
	if err != nil || !claims.MayPublish(in) {
		d.Reason = "fenced"
		w.log("fenced", map[string]any{"repo": h.Repo, "sha": h.SHA})
		return d
	}
	state, desc := w.aggregate(out, hu, j.Context)
	attempt := 1
	for _, s := range in.Statuses {
		if s.Context == j.Context && s.State == "error" && trusted(in, s.Creator) {
			attempt++
		}
	}
	desc = fmt.Sprintf("%s host=%s attempt=%d", desc, w.C.Host, attempt)
	owner, repo, _ := strings.Cut(h.Repo, "/")
	st := report.Status{Owner: owner, Repo: repo, Revision: h.SHA, State: state, Description: report.Truncate(desc), StatusContext: j.Context}
	if err := w.P.Reporter.Post(pctx, st); err != nil {
		d.Reason = "post_failed"
		rep.Errors = append(rep.Errors, h.Repo+"@"+short(h.SHA)+": result not posted")
		return d
	}
	d.State = string(state)
	if j.Context == ResidualContext && state != report.Success && w.P.Alert != nil {
		w.P.Alert(pctx, h, "residual checks "+string(state)+" on "+short(h.SHA))
	}
	w.log("result", map[string]any{"repo": h.Repo, "sha": h.SHA, "context": j.Context, "state": string(state), "attempt": attempt})
	return d
}

func trusted(in claims.Inputs, creator string) bool {
	for _, t := range in.Trusted {
		if t != "" && t == creator {
			return true
		}
	}
	return false
}

// aggregate turns the attempt into the posted state. On the result context of a hosted use the hosted
// conclusion is folded in; elsewhere the attempt stands.
func (w *Watcher) aggregate(out Outcome, hu *hostedUse, ctxName string) (report.State, string) {
	state, desc := out.State, out.Description
	if !state.Valid() || state == report.Pending {
		state = report.Error
	}
	if out.Cancelled {
		state, desc = report.Error, "cancelled: the watcher stopped"
	}
	if hu == nil || !hu.use || ctxName == ResidualContext {
		return state, desc
	}
	switch {
	case hu.failed:
		return report.Failure, "hosted run failed"
	case state == report.Success:
		return report.Success, "hosted run and residual checks passed"
	case state == report.Failure:
		return report.Failure, "residual checks failed"
	}
	return state, desc
}

// attempt runs j; a SIGTERM (ctx cancelled) waits watch.drain_sec before the attempt is cancelled.
func (w *Watcher) attempt(ctx context.Context, j Job) Outcome {
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		w.log("drain", map[string]any{"repo": j.Head.Repo, "sha": j.Head.SHA, "drain_sec": w.C.Drain.Seconds()})
		select {
		case <-done:
		case <-w.after(w.C.Drain):
			cancel()
		}
	}()
	out, err := w.P.Runner.Run(runCtx, j)
	if runCtx.Err() != nil {
		out.Cancelled = true
	}
	if err != nil && !out.Cancelled {
		w.log("run_error", map[string]any{"repo": j.Head.Repo, "sha": j.Head.SHA, "error": err.Error()})
		return Outcome{State: report.Error, Description: "infrastructure error"}
	}
	return out
}
