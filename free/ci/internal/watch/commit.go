package watch

// Purpose: the decision for one head (P7-AUTO-11): gate it, resolve its result context, read its status
//          history, honour a final result, a live claim and a hosted use or wait, and claim it when this
//          host is ready and may take it.
// Inputs:  a Head, readiness, whether this pass already ran a commit.
// Outputs: a Decision; the claim through report.Ledger.PostClaim; the run through run.go.
// Constraints: only statuses of the trusted creators count (claims.Inputs.Trusted); the retry count is
//          the number of trusted `error` results on the result context minus one, so an error is retried
//          watch.retry_max times watch.retry_gap apart and then stays final; a failure is final at once;
//          a claim is read back before the run and the run starts only while the claim holds.

import (
	"context"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/claims"
	"github.com/nself-org/plugins/free/ci/internal/hostedtier"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// ResidualContext carries the residual checks of a hosted use on a pinned repository.
const ResidualContext = report.Context + "/residual"

// hostedUse is the hosted decision a run must respect.
type hostedUse struct {
	use      bool
	failed   bool // the hosted result was red
	residual []string
}

func (w *Watcher) resultContext(ctx context.Context, h Head) string {
	if w.C.StatusContext != "" {
		return w.C.StatusContext
	}
	owner, repo, _ := strings.Cut(h.Repo, "/")
	if w.P.ResultContext == nil {
		return report.WatchContext
	}
	return w.P.ResultContext(ctx, owner, repo, "")
}

// inputs builds the claim-ledger inputs of h from its status history.
func (w *Watcher) inputs(h Head, list []report.LedgerStatus, providerNow time.Time, resultCtx string) claims.Inputs {
	st := make([]claims.Status, 0, len(list))
	for _, s := range list {
		st = append(st, claims.Status{Context: s.Context, State: string(s.State), Creator: s.Creator, Description: s.Description, CreatedAt: s.CreatedAt})
	}
	if providerNow.IsZero() {
		providerNow = w.now()
	}
	w.mu.Lock()
	first, ok := w.firstSeen[key(h)]
	if !ok {
		first = providerNow
		w.firstSeen[key(h)] = first
	}
	w.mu.Unlock()
	return claims.Inputs{Now: providerNow, Statuses: st, Me: claims.Me{Host: w.C.Host, Incarnation: w.C.Incarnation},
		Trusted: w.C.Trusted, RunLimit: w.C.RunLimit, Preferred: w.C.Preferred, Hosts: w.C.Hosts, Step: w.C.Step,
		FirstSeen: first, RetryMax: w.C.RetryMax, RetryGap: w.C.RetryGap, ResultContext: resultCtx, ClaimContext: report.ClaimContext}
}

// history reads the status history of h; ok is false when the ledger could not be read.
func (w *Watcher) history(ctx context.Context, h Head, resultCtx string) (claims.Inputs, error) {
	owner, repo, _ := strings.Cut(h.Repo, "/")
	list, pnow, err := w.P.Ledger.ListStatuses(ctx, owner, repo, h.SHA)
	if err != nil {
		return claims.Inputs{}, err
	}
	return w.inputs(h, list, pnow, resultCtx), nil
}

// finals counts the trusted final results on the result context.
func finals(in claims.Inputs, ctxName string) int {
	n := 0
	for _, s := range in.Statuses {
		if trusted(in, s.Creator) && s.Context == ctxName && (s.State == "success" || s.State == "failure") {
			n++
		}
	}
	return n
}

// consider decides what to do with h and, when it takes h, runs it.
func (w *Watcher) consider(ctx context.Context, h Head, ready, ran bool, rep *PassReport) Decision {
	d := Decision{Repo: h.Repo, SHA: h.SHA, Action: "skip"}
	reason, code, isolated := w.gate(ctx, h)
	if reason != "" {
		d.Reason, d.Code = reason, code
		w.log("skip", map[string]any{"repo": h.Repo, "sha": h.SHA, "reason": reason})
		return d
	}
	resultCtx := w.resultContext(ctx, h)
	d.Context = resultCtx
	in, err := w.history(ctx, h, resultCtx)
	if err != nil {
		d.Reason = SkipError
		rep.Errors = append(rep.Errors, h.Repo+"@"+short(h.SHA)+": status history unreadable")
		return d
	}
	if in.Final() {
		w.noteDuplicate(h, in, resultCtx, rep)
		d.Reason = SkipFinal
		return d
	}
	hu := w.hosted(ctx, h, in.FirstSeen)
	switch {
	case hu == nil:
		d.Reason = SkipHosted
		return d
	case hu.use && resultCtx != report.Context: // pinned: the Actions check stands, the residual is advisory
		resultCtx, in.ResultContext, d.Context = ResidualContext, ResidualContext, ResidualContext
		if in.Final() {
			d.Reason = SkipFinal
			return d
		}
		if len(hu.residual) == 0 {
			d.Reason = SkipHostedUse
			return d
		}
	}
	dec := claims.Decide(in)
	switch dec.Kind {
	case claims.Skip:
		d.Reason = SkipFinal
		return d
	case claims.Lost:
		rep.Held++
		d.Reason = SkipClaimed
		return d
	case claims.Wait:
		rep.Queued++
		d.Reason = SkipWait
		return d
	}
	if !ready {
		rep.Queued++
		d.Reason = SkipNotReady
		return d
	}
	if ran {
		rep.Queued++
		d.Reason = SkipBusy
		return d
	}
	if !dec.Holds {
		owner, repo, _ := strings.Cut(h.Repo, "/")
		if _, err := w.P.Ledger.PostClaim(ctx, report.ClaimStatus(owner, repo, h.SHA, w.C.Host, w.C.Incarnation)); err != nil {
			d.Reason = SkipError
			rep.Errors = append(rep.Errors, h.Repo+"@"+short(h.SHA)+": claim not posted")
			return d
		}
		if in, err = w.history(ctx, h, resultCtx); err != nil || !claims.Decide(in).Holds {
			d.Reason = SkipLost
			return d
		}
	}
	return w.run(ctx, h, Job{Head: h, Context: resultCtx, Only: hostedResidual(hu), Isolated: isolated}, hu, rep)
}

// hosted returns the hosted decision of h, or nil when the hosted run is still inside its timers.
func (w *Watcher) hosted(ctx context.Context, h Head, pushed time.Time) *hostedUse {
	if w.P.Hosted == nil {
		return &hostedUse{}
	}
	r := w.P.Hosted.Decide(ctx, h.Repo, h.SHA, pushed)
	switch r.Decision {
	case hostedtier.Wait:
		return nil
	case hostedtier.Use:
		return &hostedUse{use: true, failed: r.Reason == hostedtier.ReasonFailure, residual: r.Residual}
	}
	return &hostedUse{}
}

func hostedResidual(hu *hostedUse) []string {
	if hu == nil || !hu.use {
		return nil
	}
	return hu.residual
}

// noteDuplicate reports, once per commit, a second final result (two hosts inside the fence window).
func (w *Watcher) noteDuplicate(h Head, in claims.Inputs, ctxName string, rep *PassReport) {
	if n := finals(in, ctxName); n > 1 {
		w.mu.Lock()
		seen := w.dups[key(h)]
		w.dups[key(h)] = true
		w.mu.Unlock()
		if !seen {
			rep.Duplicates += n - 1
			w.log("duplicate", map[string]any{"repo": h.Repo, "sha": h.SHA, "finals": n})
		}
	}
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
