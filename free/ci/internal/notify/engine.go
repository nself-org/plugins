package notify

// Purpose: the rules and the delivery loop (P7-AUTO-18). OnRun turns one finished run into
// at most one event per kind and sends each to every sink:
//   failure      result fail; once per (repo, sha, context), so a rerun of the same sha with the
//                same result stays quiet
//   error_stuck  result error (an infrastructure error that outlived its retries); same key rule
//   recovery     result pass after a fail or error on the same (repo, ref, context), once per sha
//   digest       MaybeDigest: the first run to end on a new UTC day sends one summary
// Inputs: a Run, Config pieces (sinks, quiet kinds, digest on/off), a State, a clock.
// Outputs: notes for the run summary; delivery counters in State.
// Constraints: delivery never fails or changes a run: every error is a note, a sink panic is
// caught, and the whole delivery is bounded by Budget (sinks still running then are
// cancelled and counted as failed). A message no sink took is released, so a rerun may send
// it again; one that some sink took is not. A cancelled run changes nothing. Deterministic:
// the clock and the timer are injected.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// DefaultBudget bounds one delivery round (the command sink alone may take CommandTimeout).
const DefaultBudget = CommandTimeout + 2*time.Second

// Run is the finished run the engine judges. Result is the evidence result: pass, fail,
// error or cancelled.
type Run struct {
	Repo, Ref, Revision, Context, RunID, EvidenceDigest, Result string
	Counts                                                      model.Counts
	FailedJobs                                                  []string
}

// Engine decides and delivers. The zero value of Now, Budget and After is production.
type Engine struct {
	Sinks       []Notifier
	Quiet       map[Kind]bool
	DailyDigest bool
	State       State
	Now         func() time.Time
	Budget      time.Duration
	After       func(time.Duration) <-chan time.Time
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) after(d time.Duration) <-chan time.Time {
	if e.After != nil {
		return e.After(d)
	}
	return time.After(d)
}

// OnRun handles one finished run and returns notes.
func (e *Engine) OnRun(ctx context.Context, r Run) (notes []string) {
	if len(e.Sinks) == 0 || e.State == nil {
		return nil
	}
	switch r.Result {
	case "pass", "fail", "error":
	default:
		return nil
	}
	if r.Revision == "" {
		return nil
	}
	// A run with no ref (a detached checkout) can alert but has no red-to-green history.
	prev := ""
	if r.Ref != "" {
		var err error
		if prev, err = e.State.Observe(ctx, r.Repo, r.Ref, r.Context, r.Revision, r.Result); err != nil {
			return []string{"notify: state unavailable, nothing sent: " + firstLine(err.Error())}
		}
	}
	var kind Kind
	switch {
	case r.Result == "fail":
		kind = KindFailure
	case r.Result == "error":
		kind = KindErrorStuck
	case prev == "fail" || prev == "error":
		kind = KindRecovery
	}
	timedOut := false
	if kind != "" && !e.Quiet[kind] {
		var n []string
		var taken bool
		n, taken, timedOut = e.send(ctx, r, e.event(kind, r))
		notes = append(notes, n...)
		if kind == KindRecovery && !taken && r.Ref != "" {
			// nobody heard the recovery: put the red result back so the next green run sends it again
			if _, err := e.State.Observe(context.WithoutCancel(ctx), r.Repo, r.Ref, r.Context, r.Revision, prev); err != nil {
				notes = append(notes, "notify: last result not restored: "+firstLine(err.Error()))
			}
		}
	}
	if timedOut { // a hung sink already used the run's delivery budget: the digest waits for the next run
		return notes
	}
	return append(notes, e.MaybeDigest(ctx)...)
}

// send claims, delivers and releases the claim when no sink took the message. taken is
// false when the message was not sent (already claimed counts as taken).
func (e *Engine) send(ctx context.Context, r Run, ev Event) (notes []string, taken, timedOut bool) {
	won, err := e.State.Claim(ctx, r.Repo, r.Revision, r.Context, ev.Kind)
	if err != nil {
		return []string{"notify: state unavailable, nothing sent: " + firstLine(err.Error())}, false, false
	}
	if !won {
		return nil, true, false
	}
	ok, notes, timedOut := e.deliver(ctx, ev)
	if ok == 0 {
		// the run's context may be spent by now (its delivery budget), so the release must not depend on it
		if err = e.State.Release(context.WithoutCancel(ctx), r.Repo, r.Revision, r.Context, ev.Kind); err != nil {
			notes = append(notes, "notify: claim not released: "+firstLine(err.Error()))
		}
	}
	return notes, ok > 0, timedOut
}

// MaybeDigest sends the daily digest when it is on, none was sent today (UTC) and runs
// finished since the last one.
func (e *Engine) MaybeDigest(ctx context.Context) []string {
	if !e.DailyDigest || e.Quiet[KindDigest] || len(e.Sinks) == 0 || e.State == nil {
		return nil
	}
	now := e.now()
	day := now.UTC().Format("2006-01-02")
	last, err := e.State.LastDigest(ctx)
	if err != nil {
		return []string{"notify: digest state unavailable: " + firstLine(err.Error())}
	}
	since := now.Add(-24 * time.Hour)
	if last != 0 {
		if time.Unix(0, last).UTC().Format("2006-01-02") == day {
			return nil
		}
		since = time.Unix(0, last)
	}
	st, err := e.State.DigestStats(ctx, since.UnixNano(), now.UnixNano())
	if err != nil {
		return []string{"notify: digest counts unavailable: " + firstLine(err.Error())}
	}
	if st.Runs == 0 {
		return nil
	}
	won, err := e.State.DigestClaim(ctx, day)
	if err != nil || !won {
		if err != nil {
			return []string{"notify: digest state unavailable: " + firstLine(err.Error())}
		}
		return nil
	}
	ev := Event{Kind: KindDigest, Time: now.UTC().Format(time.RFC3339), Digest: &Summary{
		Since: since.UTC().Format(time.RFC3339), Until: now.UTC().Format(time.RFC3339),
		Runs: st.Runs, Passed: st.Passed, Failed: st.Failed, Errors: st.Errored, Queued: st.Queued}}
	ev.Message = fmt.Sprintf("nself-ci daily digest: %d runs, %d passed, %d failed, %d infrastructure errors, %d queued", st.Runs, st.Passed, st.Failed, st.Errored, st.Queued)
	ok, notes, _ := e.deliver(ctx, ev)
	if ok == 0 {
		if err = e.State.DigestRelease(context.WithoutCancel(ctx), day); err != nil {
			notes = append(notes, "notify: digest claim not released: "+firstLine(err.Error()))
		}
	}
	return notes
}

func (e *Engine) event(kind Kind, r Run) Event {
	c := r.Counts
	ev := Event{Kind: kind, Time: e.now().UTC().Format(time.RFC3339), Repo: r.Repo, Ref: r.Ref, Revision: r.Revision, Context: r.Context,
		RunID: r.RunID, Result: r.Result, EvidenceDigest: r.EvidenceDigest, Counts: &c, FailedJobs: r.FailedJobs}
	if r.RunID != "" {
		ev.Show = "nself ci runs show " + r.RunID
	}
	where := r.Repo
	if r.Ref != "" {
		where += "@" + r.Ref
	}
	if len(r.Revision) >= 12 {
		where += " " + r.Revision[:12]
	}
	where = strings.TrimSpace(where)
	switch kind {
	case KindFailure:
		ev.Message = fmt.Sprintf("nself-ci failed on %s: %d of %d checks failed", where, c.Fail, c.Total)
	case KindErrorStuck:
		ev.Message = fmt.Sprintf("nself-ci hit an infrastructure error on %s and gave up after its retries (%d checks errored)", where, c.Error)
	case KindRecovery:
		ev.Message = fmt.Sprintf("nself-ci is green again on %s", where)
	}
	return ev
}

type outcome struct {
	i  int
	ok bool
}

// deliver sends ev to every sink at once and waits for all of them or for the budget.
// It returns how many sinks took the message, notes, and whether the budget ended the wait.
func (e *Engine) deliver(ctx context.Context, ev Event) (int, []string, bool) {
	ev = ev.Redacted()
	budget := e.Budget
	if budget <= 0 {
		budget = DefaultBudget
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan outcome, len(e.Sinks))
	for i, s := range e.Sinks {
		go func() {
			ok := false
			defer func() {
				_ = recover() // a panicking sink is a failed delivery
				results <- outcome{i, ok}
			}()
			ok = s.Notify(sctx, ev) == nil
		}()
	}
	timer := e.after(budget)
	done := make([]bool, len(e.Sinks))
	took, pending, timedOut := 0, len(e.Sinks), false
	var notes []string
	record := func(i int, ok bool) {
		if err := e.State.RecordDelivery(context.WithoutCancel(ctx), e.Sinks[i].Name(), ok); err != nil {
			notes = append(notes, "notify: delivery count not stored: "+firstLine(err.Error()))
		}
		if !ok {
			notes = append(notes, fmt.Sprintf("notify: sink %s did not take the %s message (E813)", e.Sinks[i].Name(), ev.Kind))
		}
	}
wait:
	for pending > 0 {
		select {
		case o := <-results:
			done[o.i] = true
			pending--
			if o.ok {
				took++
			}
			record(o.i, o.ok)
		case <-timer:
			cancel()
			timedOut = true
			break wait
		}
	}
	for i := range e.Sinks {
		if !done[i] {
			record(i, false)
		}
	}
	return took, notes, timedOut
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
