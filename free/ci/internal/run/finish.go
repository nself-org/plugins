package run

// Purpose: the end of a run: record the evidence, finish the pipeline row,
// run AfterRun hooks and finalizers, and post the one `nself-ci` status
// through the outbox (or say in one line why not).
// Inputs:  the job outcomes and the run facts. Outputs: a Result.
// Constraints: unbound evidence (dirty tree, no commit) never posts; the
// outbox posts only when its seq is not older than the last posted for the
// (revision, context); every write uses a context that survives Ctrl-C; a
// hook or post failure is a note, never a changed verdict.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// statusPost is what prepareStatus resolved.
type statusPost struct {
	Reporter    report.Reporter
	Owner, Repo string
	Reason      string // why nothing will be posted ("" when posting)
	Seq         int64
	Outbox      *report.Outbox
	Notes       []string // problems found while preparing (drain failures)
}

type finishIn struct {
	St        *store.Store
	RunID     string
	Rev       Revision
	Source    string
	Policy    PolicyResult
	Hooks     Hooks
	Jobs      []JobOutcome
	Cancelled bool
	Partial   bool // --job or --pipeline: not the whole default gate
	Start     time.Time
	Post      statusPost
	Recovered RecoverReport
	Skipped   []plan.Skipped
	Begin     any // what EvidenceBuilder.Begin returned
}

// prepareStatus decides whether this run can post, drains entries earlier
// invocations left unposted, and reserves this run's seq.
func (c *Coordinator) prepareStatus(ctx context.Context, st *store.Store, rev Revision) statusPost {
	switch {
	case c.Opts.NoStatus:
		return statusPost{Reason: report.ReasonNoStatus}
	case rev.SHA == "":
		return statusPost{Reason: report.ReasonNoRevision}
	}
	gh := report.GH{Capture: c.capture()}
	rep, owner, repo, reason := gh.Resolve(ctx, c.Opts.Root)
	if reason != "" {
		return statusPost{Reason: reason}
	}
	if c.Reporter != nil {
		rep = c.Reporter
	}
	p := statusPost{Reporter: rep, Owner: owner, Repo: repo, Outbox: &report.Outbox{Store: st, Reporter: rep, Owner: owner, Repo: repo}}
	if _, err := p.Outbox.Drain(ctx); err != nil {
		p.Notes = append(p.Notes, "earlier status not delivered (kept for the next run): "+firstLineOf(err.Error()))
	}
	if rev.Dirty {
		p.Reporter, p.Reason = nil, report.ReasonUnbound
		return p
	}
	seq, err := p.Outbox.Reserve(ctx, rev.SHA)
	if err != nil {
		p.Reporter, p.Reason = nil, "status outbox unavailable: "+firstLineOf(err.Error())
		return p
	}
	p.Seq = seq
	return p
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// exitFor maps a result to the process exit code (model.ExitCodes, "ci").
func exitFor(r model.Result) int {
	switch r {
	case "pass":
		return 0
	case "fail":
		return 1
	}
	return 2
}

func pipelineStatus(r model.Result) string {
	switch r {
	case "pass":
		return "passed"
	case "fail":
		return "failed"
	}
	return string(r)
}

// conclude records the evidence and posts the status.
func (c *Coordinator) conclude(f finishIn) Result {
	bg, done := background(context.Background())
	defer done()
	trig := c.Plan.Trigger
	if trig == "" {
		trig = "local"
	}
	ev, digest, err := buildEvidence(evidenceInput{RunID: f.RunID, Project: c.Opts.Root, Revision: f.Rev.SHA, ConfigDigest: c.ConfigDigest,
		PolicyDigest: f.Policy.Digest, Source: f.Source, Dirty: f.Rev.Dirty, Cancelled: f.Cancelled, Pipeline: c.Plan.Pipeline, Selection: c.Plan.Selection,
		Trigger: trig, Jobs: f.Jobs, Start: f.Start, End: time.Now()})
	notes := append([]string(nil), f.Post.Notes...)
	notes = append(notes, ignoredRunNotes(f.Policy.IgnoredRun)...)
	if err == nil && f.Hooks.Evidence != nil {
		facts := EvidenceFacts{Root: c.Opts.Root, RunID: f.RunID, Rev: f.Rev, Store: f.St, Git: c.capture(), Jobs: f.Jobs, Job: c.Opts.Job}
		var done model.Evidence
		cerr := guard("evidence builder", func() (e error) { done, e = f.Hooks.Evidence.Complete(bg, f.Begin, facts, ev); return })
		if cerr != nil {
			notes = append(notes, "evidence not completed (it carries no input digests): "+firstLineOf(cerr.Error()))
		} else {
			ev = done
			digest, err = evidenceDigest(ev)
		}
	}
	if err == nil {
		markHistoryFlaky(bg, f.St, &ev)
		digest, err = evidenceDigest(ev)
	}
	for _, e := range f.Recovered.Errors {
		notes = append(notes, "recovery: "+firstLineOf(e.Error()))
	}
	if f.Recovered.Lost > 0 {
		notes = append(notes, fmt.Sprintf("recovered %d attempt(s) of a dead coordinator (%d retryable, %d need an operator)", f.Recovered.Lost, f.Recovered.Retryable, f.Recovered.NeedsOperator))
	}
	if err == nil {
		body, _ := json.Marshal(ev)
		if err = f.St.RecordRunEvidence(bg, "ev-"+f.RunID, f.RunID, f.Rev.SHA, c.ConfigDigest, body); err != nil {
			notes = append(notes, "evidence not stored: "+firstLineOf(err.Error()))
		}
	} else {
		ev.Result = "error"
		notes = append(notes, "evidence not built: "+firstLineOf(err.Error()))
	}
	if err = f.St.FinishPipeline(bg, f.RunID, pipelineStatus(ev.Result)); err != nil {
		notes = append(notes, "run row not finished: "+firstLineOf(err.Error()))
	}
	info := RunInfo{RunID: f.RunID, Revision: f.Rev.SHA, EvidenceDigest: digest, Result: ev.Result, Evidence: ev, Jobs: f.Jobs, Dirty: f.Rev.Dirty}
	// the status goes first and has its own budget: slow hooks must not eat it
	pctx, pdone := context.WithTimeout(context.WithoutCancel(context.Background()), 60*time.Second)
	line, pnotes := c.postStatus(pctx, f, info, ev)
	pdone()
	notes = append(notes, pnotes...)
	for _, h := range f.Hooks.AfterRuns {
		if err = guard("after-run hook", func() error { return h.AfterRun(bg, info) }); err != nil {
			notes = append(notes, "after-run hook: "+firstLineOf(err.Error()))
		}
	}
	for _, fin := range f.Hooks.Finalizers {
		if err = guard("finalizer", func() error { return fin.Finalize(bg, info) }); err != nil {
			notes = append(notes, "finalizer "+fin.Name()+": "+firstLineOf(err.Error()))
		}
	}
	nctx, ndone := context.WithTimeout(bg, 30*time.Second) // a hung sink must not hold the run after its status went out
	notes = append(notes, notifyRun(nctx, f.Hooks, NotifyRequest{Run: info, Status: line, Notes: append([]string(nil), notes...)})...)
	ndone()
	return Result{RunID: f.RunID, Exit: exitFor(ev.Result), Summary: Summary{RunID: f.RunID, Result: ev.Result, Jobs: f.Jobs, Skipped: f.Skipped,
		Status: line, Evidence: digest, Doc: ev, Notes: notes}}
}

func notPosted(reason string) string { return "nself-ci: status not posted: " + reason }

// defaultStatus is the projection plus the evidence digest; no target_url.
func defaultStatus(r PublishRequest) report.Status {
	state := report.Error
	switch r.Run.Result {
	case "pass":
		state = report.Success
	case "fail":
		state = report.Failure
	}
	d, suffix := r.Projection.Summary, ""
	if r.Run.Result == "error" { // the projection counts checks, it cannot tell a broken runner from a failure
		if i := strings.Index(d, ":"); i >= 0 {
			d = "error" + d[i:]
		}
	}
	if n := r.Run.EvidenceDigest; len(n) >= 12 {
		suffix = " (evidence " + n[:12] + ")"
	}
	// the digest is the part a reader cannot rebuild: cut the summary, never the suffix
	if room := report.MaxDescription - utf8.RuneCountInString(suffix); utf8.RuneCountInString(d) > room {
		d = string([]rune(d)[:room-1]) + "…"
	}
	return report.Status{Owner: r.Owner, Repo: r.Repo, Revision: r.Run.Revision, State: state, Description: report.Truncate(d + suffix)}
}

func (c *Coordinator) postStatus(ctx context.Context, f finishIn, info RunInfo, ev model.Evidence) (string, []string) {
	p := f.Post
	var notes []string
	switch {
	case p.Reporter == nil || p.Outbox == nil:
		return notPosted(firstNonEmpty(p.Reason, report.ReasonNoRemote)), nil
	case ev.Binding.State != "bound":
		return notPosted(report.ReasonUnbound), nil
	case f.Partial:
		return notPosted(report.ReasonPartial), nil
	case f.Cancelled:
		return notPosted(report.ReasonCancelled), nil
	}
	req := PublishRequest{Owner: p.Owner, Repo: p.Repo, Run: info, Projection: model.Projection(ev)}
	s := defaultStatus(req)
	if f.Hooks.Publisher != nil {
		var custom report.Status
		err := guard("publisher", func() (e error) { custom, e = f.Hooks.Publisher.Publish(ctx, req); return })
		if err != nil { // the default projection may hold text the publication profile would filter
			return notPosted("publisher failed, nothing posted: " + firstLineOf(err.Error())), []string{"publisher failed: " + firstLineOf(err.Error())}
		}
		s = custom
	}
	s.Owner, s.Repo, s.Revision = p.Owner, p.Repo, f.Rev.SHA
	if s.State == report.Success && ev.Result != "pass" { // a publisher never turns a failed run into success
		s.State = report.Failure
		if ev.Result != "fail" {
			s.State = report.Error
		}
	}
	if err := publishGates(ctx, f.Hooks, req, s); err != nil { // composes with the Publisher's veto above; either one stops the post
		return notPosted("publish gate refused, nothing posted: " + firstLineOf(err.Error())), []string{"publish gate: " + firstLineOf(err.Error())}
	}
	delivered, err := p.Outbox.Complete(ctx, p.Seq, s)
	switch {
	case err != nil:
		return notPosted("could not deliver (kept for the next run): " + firstLineOf(err.Error())), notes
	case !delivered:
		return notPosted("a newer run already posted for this commit"), notes
	}
	notes = append(notes, publishDetails(ctx, f.Hooks, req, s)...)
	return fmt.Sprintf("nself-ci: status posted (%s) for %s", s.State, f.Rev.SHA[:7]), notes
}
