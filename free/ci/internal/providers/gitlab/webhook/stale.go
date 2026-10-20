package webhook

// Purpose: replay defence beyond NODE-10's 72 h dedup window (Epic D8, D11): a delivery whose branch,
// tag or merge request head no longer points at the payload revision, or whose job is no longer
// waiting, is stale. Each check is one API call with a 5 s budget.
// Inputs:  verified facts for a push, tag, merge request or job, and the object id the payload named.
// Outputs: (current, known). known is false on any API failure (no credential, timeout, 404 which can
//          also mean "no access", bad JSON): the trigger then continues with HeadCurrent unknown and
//          the trust classifier caps it.
// Constraints: only a successful read that shows a different id marks a delivery stale. A tag also
//          resolves here for a tag push that carries no checkout_sha.

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// staleBudget is the whole time one check may take.
var staleBudget = 5 * time.Second

// glRef is a branch or tag as GitLab reports it; Protected is only on a branch.
type glRef struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	Commit    struct {
		ID string `json:"id"`
	} `json:"commit"`
}

// branchOf reads a branch of a project (the funnel escapes the name into one path segment).
func branchOf(ctx context.Context, c *rest.Client, project, branch string) (glRef, int, bool) {
	var out glRef
	status, err := get(ctx, c, api.BranchPath, projectParams(project, "branch", branch), &out)
	return out, status, err == nil && out.Commit.ID != ""
}

// tagCommit returns the commit a tag points at (an annotated tag is already dereferenced by GitLab).
func tagCommit(ctx context.Context, c *rest.Client, project, tag string) (string, bool) {
	var out glRef
	if _, err := get(ctx, c, tagPath, projectParams(project, "tag", tag), &out); err != nil {
		return "", false
	}
	sha := strings.ToLower(out.Commit.ID)
	return sha, isHex40(sha)
}

// headCurrent asks whether the delivery still describes the current head. want is the revision the
// payload named for the ref.
func (s *Source) headCurrent(ctx context.Context, f model.TriggerFacts, want string) (current, known bool) {
	ctx, cancel := context.WithTimeout(ctx, staleBudget)
	defer cancel()
	c, err := s.api(ctx)
	if err != nil {
		return false, false
	}
	switch f.Kind {
	case "push":
		got, _, ok := branchOf(ctx, c, f.Repo, strings.TrimPrefix(f.Ref, "refs/heads/"))
		return ok && strings.EqualFold(got.Commit.ID, want), ok
	case "tag":
		got, ok := tagCommit(ctx, c, f.Repo, strings.TrimPrefix(f.Ref, "refs/tags/"))
		return ok && strings.EqualFold(got, want), ok
	case "pull_request":
		var mr struct {
			SHA string `json:"sha"`
		}
		params := projectParams(f.Repo, "iid", strconv.Itoa(f.PRNumber))
		if _, err = get(ctx, c, mrPath, params, &mr); err != nil || mr.SHA == "" {
			return false, false
		}
		return strings.EqualFold(mr.SHA, want), true
	}
	return false, false
}

// resolveTag returns the commit a tag points at, within the stale budget.
func (s *Source) resolveTag(ctx context.Context, project, tag string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, staleBudget)
	defer cancel()
	c, err := s.api(ctx)
	if err != nil {
		return "", false
	}
	return tagCommit(ctx, c, project, tag)
}

// jobPending asks whether a job is still waiting for a runner (one call, the stale budget). A captured
// pending event can be replayed after NODE-10's dedup window; a demand for a job that already ran must
// not start a runner. known is false on any failure.
func (s *Source) jobPending(ctx context.Context, project string, job int64) (pending, known bool) {
	ctx, cancel := context.WithTimeout(ctx, staleBudget)
	defer cancel()
	c, err := s.api(ctx)
	if err != nil {
		return false, false
	}
	var j struct {
		Status string `json:"status"`
	}
	if _, err = get(ctx, c, jobPath, projectParams(project, "job", itoa(job)), &j); err != nil || j.Status == "" {
		return false, false
	}
	// created, pending, waiting_for_resource and scheduled have not run; a job that ran or runs is replayed
	switch j.Status {
	case "running", "success", "failed", "canceled", "canceling", "skipped", "manual":
		return false, true
	}
	return true, true
}
