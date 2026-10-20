package reconcile

// Purpose: poll-mode runner demand (Epic D19, review F7/F8/F9): list the queued and in-progress workflow
// runs of each allowlisted repository, read their jobs, and hand every job that is itself queued and asks
// for [self-hosted, <providers.github.jit.label>] to the registered DemandHandler as a runner_demand
// trigger, so JIT runners on the operator's own nodes can carry the required nself-ci job without a
// public webhook.
// Inputs:  GET /repos/{o}/{r}/actions/runs?status=queued|in_progress, .../runs/{id}/jobs; marks and ETags.
// Outputs: one runner_demand Delivery (source github, facts sourced api, auth per-repo) per job, once.
// Constraints: every list is read conditionally and processed only when it changed since the last pass
//          that finished (a 304 costs no primary quota); a run whose jobs were read at its current
//          updated_at is not read again, so a tick cut short by its bound still moves on; an idle repository costs one request on most
//          ticks (the queued list), and the in_progress list is read on ticks where something is queued,
//          where the last read found runs, and on every inProgressEvery-th tick; at most MaxDemandCalls
//          requests per repository per tick; a job is handed over at most once (mark per job id), also
//          when the handler declines it; the facts carry the run's event, head repository and fork flag
//          (HOST-19 and TRUST-13 classify on them) and no Ref; demand polling is shared with the head scan
//          only through the quota stop and the hourly ledger, which reserves it 40%. In webhook mode this
//          Reconciler does nothing.

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// Bounds of one demand tick.
const (
	MaxDemandCalls  = 6 // requests per repository per tick: two lists and up to four job lists
	runsPerPage     = 30
	inProgressEvery = 4 // an idle repository's in_progress list is read on every 4th tick
)

// Demand is the poll-mode demand Reconciler of an Engine.
type Demand struct{ e *Engine }

var (
	_ coordinator.Reconciler = (*Demand)(nil)
	_ coordinator.Everyer    = (*Demand)(nil)
)

// Demand returns the demand Reconciler of e.
func (e *Engine) Demand() *Demand { return &Demand{e} }

// Name implements coordinator.Reconciler.
func (*Demand) Name() string { return NameDemand }

// Every implements coordinator.Everyer: the operator's poll interval (default 30 s, at least 15 s).
func (d *Demand) Every() time.Duration {
	if st, err := d.e.settings(context.Background()); err == nil {
		return st.PollInterval
	}
	return DefaultPollInterval
}

type ghRun struct {
	ID        int64  `json:"id"`
	Event     string `json:"event"`
	HeadSHA   string `json:"head_sha"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
	Actor     struct {
		Login string `json:"login"`
	} `json:"actor"`
	Repository struct {
		ID      int64 `json:"id"`
		Private *bool `json:"private"` // absent is unknown, never public
	} `json:"repository"`
	HeadRepository *struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"head_repository"`
}

type ghJob struct {
	ID     int64    `json:"id"`
	Status string   `json:"status"`
	Labels []string `json:"labels"`
}

type demandState struct {
	InProgress bool `json:"in_progress"`
}

// Reconcile implements coordinator.Reconciler.
func (d *Demand) Reconcile(ctx context.Context, enq coordinator.Enqueuer) error {
	if !compat.V15() {
		return nil
	}
	e := d.e
	st, err := e.settings(ctx)
	if err != nil || st.Ingress != IngressPoll {
		return err
	}
	if providers.DemandHandlerFor(Source) == nil { // nobody to hand a demand to yet (P7-HOST-19): read nothing, consume nothing
		return nil
	}
	if _, blocked := e.blockedUntil(ctx); blocked {
		return nil
	}
	repos, err := e.Repos(ctx)
	if err != nil || len(repos) == 0 {
		return err
	}
	var errs []error
	defer func() { _ = e.flush(ctx) }()
	for _, repo := range repos {
		err := e.pollRepo(ctx, enq, repo, st)
		var quota *QuotaStop
		switch {
		case err == nil:
		case errors.As(err, &quota):
			return errors.Join(append(errs, err)...)
		case errors.Is(err, ErrBudget):
			return errors.Join(errs...)
		default:
			errs = append(errs, err)
		}
		if ctx.Err() != nil {
			break
		}
	}
	if len(errs) == 0 {
		e.noteOK(ctx)
	}
	return errors.Join(errs...)
}

// tick counts the demand ticks of repo.
func (e *Engine) tick(repo string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ticks == nil {
		e.ticks = map[string]int{}
	}
	e.ticks[lower(repo)]++
	return e.ticks[lower(repo)]
}

type runList struct {
	WorkflowRuns []ghRun `json:"workflow_runs"`
}

// pollRepo is one repository's tick.
func (e *Engine) pollRepo(ctx context.Context, enq coordinator.Enqueuer, repo string, st Settings) error {
	s, err := e.session(ctx, kindDemand, MaxDemandCalls)
	if err != nil {
		return err
	}
	n := e.tick(repo)
	queued, qTag, qChanged, err := e.listRuns(ctx, s, repo, "queued")
	if err != nil {
		return err
	}
	var prev demandState
	if _, err = e.getFact(ctx, repo, "demand-state", &prev); err != nil {
		return err
	}
	var running []ghRun
	var rTag string
	rChanged := false
	if len(queued) > 0 || prev.InProgress || n%inProgressEvery == 0 {
		if running, rTag, rChanged, err = e.listRuns(ctx, s, repo, "in_progress"); err != nil {
			return err
		}
		if err = e.putFact(ctx, repo, "demand-state", demandState{InProgress: len(running) > 0 || (!rChanged && prev.InProgress)}); err != nil {
			return err
		}
	}
	complete := true
	var firstErr error
	runs := append(append([]ghRun(nil), queued...), running...)
	for _, run := range runs {
		if err = e.handRun(ctx, s, enq, repo, st, run); err != nil {
			var quota *QuotaStop
			if errors.As(err, &quota) || errors.Is(err, ErrBudget) {
				return err
			}
			complete = false // ErrCallBound or a failure: this list is read again next tick
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if !complete {
		if errors.Is(firstErr, ErrCallBound) {
			return nil // more runs than one tick may read: the rest follow on the next tick
		}
		return firstErr
	}
	if qChanged {
		err = e.setEtag(ctx, repo, "runs-queued", qTag)
	}
	if err == nil && rChanged {
		err = e.setEtag(ctx, repo, "runs-in_progress", rTag)
	}
	return err
}

// listRuns reads one run list conditionally. changed is false for a 304, and the runs are then empty:
// everything in the list was handed over by the pass that stored the ETag.
func (e *Engine) listRuns(ctx context.Context, s *session, repo, status string) (runs []ghRun, etag string, changed bool, err error) {
	old, err := e.etag(ctx, repo, "runs-"+status)
	if err != nil {
		return nil, "", false, err
	}
	var out runList
	q := url.Values{"status": {status}, "per_page": {strconv.Itoa(runsPerPage)}}
	resp, err := s.get(ctx, "/repos/{owner}/{repo}/actions/runs", repoParams(repo), q, old, &out)
	if err != nil {
		return nil, "", false, err
	}
	if resp.Status == 304 {
		return nil, old, false, nil
	}
	etag = resp.ETag
	if len(out.WorkflowRuns) >= runsPerPage { // a full page may hide more runs: never remember it as fully read
		etag = ""
	}
	return out.WorkflowRuns, etag, true, nil
}

// handRun reads the jobs of one run and hands over every new job that asks for this operator's runners.
func (e *Engine) handRun(ctx context.Context, s *session, enq coordinator.Enqueuer, repo string, st Settings, run ghRun) error {
	if run.ID <= 0 || len(run.HeadSHA) != 40 {
		return nil
	}
	runKey := strconv.FormatInt(run.ID, 10) + "@" + run.UpdatedAt
	if done, err := e.marked(ctx, "run", repo, runKey); err != nil || done { // its jobs were read at this update
		return err
	}
	p := repoParams(repo)
	p["run"] = strconv.FormatInt(run.ID, 10)
	var jobs struct {
		Jobs []ghJob `json:"jobs"`
	}
	if _, err := s.get(ctx, "/repos/{owner}/{repo}/actions/runs/{run}/jobs", p, url.Values{"filter": {"latest"}, "per_page": {"100"}}, "", &jobs); err != nil {
		return err
	}
	for _, j := range jobs.Jobs {
		if j.Status != "queued" || !wantsUs(j.Labels, st.JITLabel) {
			continue
		}
		id := strconv.FormatInt(j.ID, 10)
		seen, err := e.marked(ctx, "demand", repo, id)
		if err != nil {
			return err
		}
		if seen {
			continue
		}
		f := demandFacts(repo, e.now(), run, j)
		d, err := delivery(ctx, f)
		switch {
		case errors.Is(err, ErrDeclined):
			// handed over and refused: remembered below so the job is not offered every tick
		case err != nil: // includes ErrHandlerFailed and ErrNoHandler: nothing is marked, the job is offered again
			return err
		default:
			if _, err = enq.Enqueue(ctx, d); err != nil {
				return err
			}
		}
		if err = e.mark(ctx, "demand", repo, id); err != nil {
			return err
		}
	}
	return e.mark(ctx, "run", repo, runKey)
}

// wantsUs reports whether a job's labels include self-hosted and the operator's runner label.
func wantsUs(labels []string, ours string) bool {
	has := func(want string) bool {
		return slices.ContainsFunc(labels, func(l string) bool { return strings.EqualFold(l, want) })
	}
	return ours != "" && has("self-hosted") && has(ours)
}
