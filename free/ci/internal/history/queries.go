package history

// Purpose: the read side of run history: `ci runs list`.
// Inputs:  the store, the repository root and git access (for freshness), a limit.
// Outputs: one Entry per run, newest first, with result, duration, revision,
// binding and freshness, and whether stored logs remain.
// Constraints: read-only; nothing here runs a job; the result, binding and
// duration come from the run's evidence when it has one, and read as "unknown"
// (never as a pass) when it does not; freshness is computed against the
// checkout at Root as it is now and is "unknown" with a reason when it cannot
// be; the checkout's inputs are read once for the whole listing.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// MaxLimit bounds one listing.
const MaxLimit = 1000

// Entry is one run in the history.
type Entry struct {
	ID              string  `json:"id"`
	Revision        string  `json:"revision"`
	Trigger         string  `json:"trigger"`
	Status          string  `json:"status"`
	Result          string  `json:"result"`
	StartedAt       string  `json:"started_at"`
	DurationMS      *int64  `json:"duration_ms"`
	Binding         string  `json:"binding"`
	BindingReason   *string `json:"binding_reason"`
	Freshness       string  `json:"freshness"`
	FreshnessReason *string `json:"freshness_reason"`
	LogBytes        int64   `json:"log_bytes"`
}

// Env is what a listing needs.
type Env struct {
	Store *store.Store
	Root  string           // the repository whose HEAD freshness is judged against; "" = unknown
	Git   evidence.Capture // nil: the exec funnel
	Home  string           // state directory holding the logs
}

// List returns up to limit runs, newest first.
func List(ctx context.Context, env Env, limit int) ([]Entry, error) {
	if env.Store == nil {
		return nil, nil
	}
	if limit < 1 {
		limit = 20
	}
	limit = min(limit, MaxLimit)
	var rows []store.PipelineRow
	for before := int64(0); len(rows) < limit; {
		page, err := env.Store.History(ctx, before, min(limit-len(rows), 100))
		if err != nil {
			return nil, err
		}
		if len(page.Pipelines) == 0 {
			break
		}
		rows, before = append(rows, page.Pipelines...), page.NextCreatedAt
	}
	ages, err := env.Store.PipelineAges(ctx)
	if err != nil {
		return nil, err
	}
	when := map[string]store.PipelineAge{}
	for _, a := range ages {
		when[a.ID] = a
	}
	sizes := map[string]int64{}
	if infos, lerr := logs.ListRuns(env.Home); lerr == nil {
		for _, i := range infos {
			sizes[i.Run] = i.Bytes
		}
	}
	fresh := newFreshness(ctx, env)
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, entryFor(ctx, env, r, when[r.ID], sizes[r.ID], fresh))
	}
	return out, nil
}

func entryFor(ctx context.Context, env Env, r store.PipelineRow, age store.PipelineAge, logBytes int64, fresh *freshness) Entry {
	e := Entry{ID: r.ID, Revision: r.Revision, Trigger: r.Trigger, Status: r.Status, Result: "unknown", Binding: "unknown",
		Freshness: "unknown", StartedAt: logs.Stamp(unixNano(age.CreatedAt)), LogBytes: logBytes}
	if age.FinishedAt > 0 {
		d := (age.FinishedAt - age.CreatedAt) / 1e6
		e.DurationMS = &d
	}
	docs, err := evidence.ForRun(ctx, env.Store, r)
	if err != nil || len(docs) == 0 {
		why := "no evidence was recorded for this run"
		e.FreshnessReason = &why
		return e
	}
	ev := docs[0].Evidence
	e.Result, e.Binding, e.BindingReason = string(ev.Result), string(ev.Binding.State), reasonOf(ev.Binding.Reason)
	if ev.Timings.DurationMS != nil {
		e.DurationMS = ev.Timings.DurationMS
	}
	f := fresh.of(ev)
	e.Freshness, e.FreshnessReason = string(f.State), f.Reason
	return e
}

func reasonOf(r *model.BindingReason) *string {
	if r == nil {
		return nil
	}
	s := string(*r)
	return &s
}

func unixNano(n int64) time.Time { return time.Unix(0, n) }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// freshness judges evidence against the checkout's inputs, read once.
type freshness struct {
	cur   model.InputDigests
	dirty bool
	why   string // why every answer is unknown; "" when the inputs were read
}

func newFreshness(ctx context.Context, env Env) *freshness {
	if env.Root == "" {
		return &freshness{why: "no repository to compare with"}
	}
	if _, err := os.Stat(env.Root); err != nil {
		return &freshness{why: "the repository is not readable here"}
	}
	cur, err := evidence.InputsAt(ctx, env.Git, env.Root, "HEAD")
	if err != nil {
		return &freshness{why: fmt.Sprintf("current inputs unreadable: %s", firstLine(err.Error()))}
	}
	return &freshness{cur: cur, dirty: evidence.ReadWorktree(ctx, env.Git, env.Root).Dirty}
}

func (f *freshness) of(ev model.Evidence) model.Freshness {
	if f.why != "" {
		why := f.why
		return model.Freshness{State: "unknown", Reason: &why}
	}
	got := attest.Fresh(ev, f.cur)
	if got.State == "current" && f.dirty {
		why := "the worktree has uncommitted changes"
		return model.Freshness{State: "stale", Reason: &why}
	}
	return got
}
