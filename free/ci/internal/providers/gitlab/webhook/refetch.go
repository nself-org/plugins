package webhook

// Purpose: providers.TrustFacts for GitLab (Epic D11, TRUST-13's hook through providers.RefetchFacts,
// which reaches this source through the "gitlab" provider's UseTrustFacts seam). A payload is a claim;
// this fills the facts the API can state: actor permission (member access level), whether the actor is
// a bot, the head project and fork status, protected branch, visibility, SHA reachability, whether the
// head is still current and the changed files. For a runner demand it reads the pipeline instead (its
// source becomes RunEvent) and, for a merge request pipeline, the merge request's source project.
// Inputs:  facts from Verify (or a later enrichment) and the operator's GitLab credential.
// Outputs: the same facts with api-sourced fields set. A call that fails leaves its fields as they were
//          (never invented); when every call fails the original facts come back with the first error.
// Constraints: no classification (TRUST-13) and no dedup. GitLab has no author association, so
//          FirstTime and AuthorAssociation stay unknown. Changed files come from the merge request
//          diffs or the compare API; more than 300 files is unknown (nil).

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// maxChangedFiles is the most files a change may have for its list to be recorded (Ticket: "MR changes
// <= 300 files"); diffPage is the page size of the diffs route.
const (
	maxChangedFiles = 300
	diffPage        = 100
)

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
	if f.Source != Name || !validName(f.Repo) {
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

// call performs one GET and reports the status and whether it succeeded.
func (r *fetch) call(template string, params map[string]string, q url.Values, out any) (int, bool) {
	r.calls++
	var status int
	var err error
	if q == nil {
		status, err = get(r.ctx, r.c, template, params, out)
	} else {
		status, err = query(r.ctx, r.c, template, params, q, out)
	}
	if err != nil {
		if r.firstErr == nil {
			r.firstErr = err
		}
		return status, false
	}
	r.okCalls++
	return status, true
}

func (r *fetch) params(kv ...string) map[string]string { return projectParams(r.f.Repo, kv...) }

// glPipeline is the part of a pipeline a demand needs.
type glPipeline struct {
	Source string `json:"source"`
	Ref    string `json:"ref"`
	User   struct {
		Username string `json:"username"`
	} `json:"user"`
}

// mrPipelineRef is the ref of a merge request pipeline.
var mrPipelineRef = regexp.MustCompile(`^refs/merge-requests/([0-9]{1,9})/(head|merge|train)$`)

// demand fills the facts of a Job Hook runner demand from the pipeline: its source, its actor and the
// project the code comes from (a merge request pipeline can run code of a fork).
func (r *fetch) demand() {
	id := ""
	if r.f.ProviderRunID.Value != nil {
		id = *r.f.ProviderRunID.Value
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return
	}
	var pl glPipeline
	if _, ok := r.call(api.PipelinePath, r.params("pipeline", id), nil, &pl); !ok {
		return
	}
	if pl.Source != "" {
		r.f.RunEvent = fact(pl.Source, "api", r.now)
	}
	if pl.User.Username != "" {
		r.f.Actor = fact(pl.User.Username, "api", r.now)
	}
	switch m := mrPipelineRef.FindStringSubmatch(pl.Ref); {
	case m != nil:
		r.mergeRequestSource(m[1])
	case pl.Source != "merge_request_event" && pl.Source != "external_pull_request_event":
		// a pipeline that is not about a merge request runs the project's own code
		r.f.HeadRepo = fact(r.f.Repo, "api", r.now)
		r.f.Fork = fact(false, "api", r.now)
	}
}

// mergeRequestSource sets Fork and HeadRepo from the merge request's source and target projects.
func (r *fetch) mergeRequestSource(iid string) {
	var mr glMergeRequest
	if _, ok := r.call(mrPath, r.params("iid", iid), nil, &mr); !ok {
		return
	}
	r.setSource(mr)
}

// glMergeRequest is the part of a merge request the refetch reads.
type glMergeRequest struct {
	SHA             string `json:"sha"`
	SourceProjectID int64  `json:"source_project_id"`
	TargetProjectID int64  `json:"target_project_id"`
}

// setSource records whether the merge request comes from another project and which one. A fork is a
// merge request whose source project is not its target project; the source path needs a read the token
// may not be allowed, in which case Fork stays and HeadRepo stays unknown.
func (r *fetch) setSource(mr glMergeRequest) {
	if mr.SourceProjectID <= 0 || mr.TargetProjectID <= 0 {
		return
	}
	fork := mr.SourceProjectID != mr.TargetProjectID
	r.f.Fork = fact(fork, "api", r.now)
	if !fork {
		r.f.HeadRepo = fact(r.f.Repo, "api", r.now)
		return
	}
	var src struct {
		Path string `json:"path_with_namespace"`
	}
	if _, ok := r.call(api.ProjectPath, projectParams(itoa(mr.SourceProjectID)), nil, &src); ok && validName(src.Path) {
		r.f.HeadRepo = fact(src.Path, "api", r.now)
	}
}
