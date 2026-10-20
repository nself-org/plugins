package runners

// Purpose: Deregister and Orphans, the runner teardown (Epic D11).
// Inputs:  a RunnerRef (the numeric runner id in ID, the project in Scope.Repo); a cutoff time.
// Outputs: nil when the runner is gone and proven absent; the refs of idle runners older than the cutoff.
// Constraints: only runners this package made are touched: a runner whose description does not start with
//          nself-glr- is E707 deregister.not_ours. Deregister reads the runner first (404 means it is
//          already gone), deletes it (a 404 on the delete also means gone), then reads it again: anything
//          but a 404 is E707 deregister_unverified. Orphans lists the project's project_type runners,
//          keeps those whose description carries a birth time older than the cutoff, and drops any that
//          still has a running job (a job may legitimately outlive the cutoff, up to the six hour limit);
//          a runner whose jobs cannot be read is left alone and tried again on the next pass.

import (
	"context"
	"net/url"
	"regexp"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
)

var runnerNumber = regexp.MustCompile(`^[0-9]{1,18}$`)

type runnerView struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Paused      bool   `json:"paused"`
	RunnerType  string `json:"runner_type"`
}

// runnerRead is GET of one runner: found is false on 404.
func (r *Registrar) runnerRead(ctx context.Context, id string) (v runnerView, found bool, err error) {
	resp, err := r.c.Do(ctx, "GET", runnerPath, map[string]string{"runner": id}, nil, &v)
	if resp.Status == 404 {
		return runnerView{}, false, nil
	}
	return v, err == nil, err
}

// ours reads a runner and requires it to be one this package created.
func (r *Registrar) ours(ctx context.Context, op string, ref providers.RunnerRef) (runnerView, bool, error) {
	if !runnerNumber.MatchString(ref.ID) {
		return runnerView{}, false, e702("runner.id")
	}
	v, found, err := r.runnerRead(ctx, ref.ID)
	if err != nil {
		return runnerView{}, false, runnerErr(err, op)
	}
	if !found {
		return runnerView{}, false, nil
	}
	if _, ok := runnerBirth(v.Description); !ok {
		return runnerView{}, false, e707(op + ".not_ours")
	}
	return v, true, nil
}

// Deregister deletes the runner and verifies it is gone.
func (r *Registrar) Deregister(ctx context.Context, ref providers.RunnerRef) error {
	_, found, err := r.ours(ctx, "deregister", ref)
	if err != nil || !found {
		return err
	}
	resp, err := r.c.Do(ctx, "DELETE", runnerPath, map[string]string{"runner": ref.ID}, nil, nil)
	if err != nil && resp.Status != 404 {
		return runnerErr(err, "deregister")
	}
	_, still, err := r.runnerRead(ctx, ref.ID)
	if pe, ok := err.(*providers.Error); ok && pe.Class != providers.Config {
		return err
	}
	if err != nil || still {
		return e707("deregister_unverified")
	}
	return nil
}

// Orphans lists the runners of scope that are older than before and idle. A zero before means OrphanAge ago.
func (r *Registrar) Orphans(ctx context.Context, scope providers.RunnerScope, before time.Time) ([]providers.RunnerRef, error) {
	p, err := project(scope)
	if err != nil {
		return nil, err
	}
	if before.IsZero() {
		before = r.now().Add(-OrphanAge)
	}
	all, err := api.List[runnerView](ctx, r.c, projectRunnersPath, p.Params(), url.Values{"type": {"project_type"}})
	if err != nil {
		return nil, runnerErr(err, "list")
	}
	var out []providers.RunnerRef
	for _, x := range all {
		born, ok := runnerBirth(x.Description)
		if !ok || x.ID <= 0 || !born.Before(before) {
			continue
		}
		busy, err := r.hasRunningJob(ctx, itoa(x.ID))
		if err != nil || busy {
			continue // a job may still hold it, or it cannot be told: not an orphan yet
		}
		out = append(out, providers.RunnerRef{ID: itoa(x.ID), Scope: scope})
	}
	return out, nil
}

// hasRunningJob reports whether the runner is running a job right now.
func (r *Registrar) hasRunningJob(ctx context.Context, id string) (bool, error) {
	var jobs []struct {
		ID int64 `json:"id"`
	}
	q := url.Values{"status": {"running"}, "per_page": {"1"}}
	if _, err := r.c.DoQuery(ctx, "GET", runnerJobsPath, map[string]string{"runner": id}, q, nil, &jobs); err != nil {
		return false, err
	}
	return len(jobs) > 0, nil
}
