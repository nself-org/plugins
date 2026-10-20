package webhook

// Purpose: providers.TrustFacts for GitHub (Epic D8, TRUST-13's hook through providers.RefetchFacts).
// A payload is a claim; this fills the facts the API can state: actor permission, whether the actor is
// a bot, the head repository and fork status, first-time contributor, protected ref, visibility, SHA
// reachability, whether the head is still current and the changed files. For a runner demand it reads
// the workflow run instead (event, head repository, actor; review F7).
// Inputs:  facts from Verify (or a later enrichment) and the operator's GitHub credential.
// Outputs: the same facts with api-sourced fields set. A call that fails leaves its fields as they were
//          (never invented); when every call fails the original facts come back with the first error.
// Constraints: no classification (TRUST-13) and no dedup. Calls go through the REST funnel, which takes
//          no query string, so first-time rests on GitHub's own author_association (it already counts
//          prior commits to the repository); NONE counts as first-time. Changed files come from the
//          compare API, which lists at most 300 files: 300 or more is unknown (nil).

import (
	"context"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// maxChangedFiles is the compare API's file cap; a full page cannot be told from a truncated one.
const maxChangedFiles = 300

// fetch carries one Refetch.
type fetch struct {
	ctx      context.Context
	c        *rest.Client
	f        model.TriggerFacts
	now      time.Time
	calls    int
	okCalls  int
	firstErr error
}

// Refetch implements providers.TrustFacts.
func (s *Source) Refetch(ctx context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
	if f.Source != Name || !coordinator.ValidProject(f.Repo) {
		return f, nil
	}
	c, err := s.api(ctx)
	if err != nil {
		return f, err
	}
	r := &fetch{ctx: ctx, c: c, f: f, now: s.Now()}
	if f.Kind == "runner_demand" {
		r.demand()
	} else {
		r.change()
	}
	if r.calls > 0 && r.okCalls == 0 {
		return f, r.firstErr
	}
	return r.f, nil
}

// call performs one GET and reports whether it succeeded.
func (r *fetch) call(template string, params map[string]string, out any) (int, bool) {
	r.calls++
	status, err := get(r.ctx, r.c, template, params, out)
	if err != nil {
		if r.firstErr == nil {
			r.firstErr = err
		}
		return status, false
	}
	r.okCalls++
	return status, true
}

func (r *fetch) params(extra ...string) map[string]string {
	p := repoParams(r.f.Repo)
	for i := 0; i+1 < len(extra); i += 2 {
		p[extra[i]] = extra[i+1]
	}
	return p
}

// demand fills the facts of a workflow_job runner demand from the workflow run (review F7): the run's
// event, its head repository (and so whether it is a fork) and its actor.
func (r *fetch) demand() {
	id := ""
	if r.f.ProviderRunID.Value != nil {
		id = *r.f.ProviderRunID.Value
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return
	}
	var run struct {
		Event          string  `json:"event"`
		Actor          ghUser  `json:"actor"`
		Repository     ghRepo  `json:"repository"`
		HeadRepository *ghRepo `json:"head_repository"`
	}
	if _, ok := r.call("/repos/{owner}/{repo}/actions/runs/{run}", r.params("run", id), &run); !ok {
		return
	}
	if run.Event != "" {
		r.f.RunEvent = fact(run.Event, "api", r.now)
	}
	if h := run.HeadRepository; h != nil && h.FullName != "" && h.ID > 0 && run.Repository.ID > 0 {
		r.f.HeadRepo = fact(h.FullName, "api", r.now)
		r.f.Fork = fact(h.ID != run.Repository.ID, "api", r.now) // another repository, not "a repository that is a fork"
	}
	if run.Actor.Login != "" {
		r.f.Actor = fact(run.Actor.Login, "api", r.now)
	}
}
