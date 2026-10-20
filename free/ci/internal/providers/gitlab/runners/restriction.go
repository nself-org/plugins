package runners

// Purpose: Restriction, the proof that lets a runner be registered ref_protected (Epic D11, review F6).
// Inputs:  the project scope and, in place of GitHub's runner group id, the GitLab job id of the demand
//          (GitLab has no runner groups). Zero means no job is named, which proves nothing.
// Outputs: Restriction{ProtectedOnly: true} only when the job's ref is a branch or tag that is protected
//          in the project; otherwise ProtectedOnly false. A transient failure is returned as the error and
//          the caller registers not_protected.
// Constraints: the answer is read from the API at the moment of the call (the job read for the ref, then
//          protected_branches or protected_tags by that exact name); a merge request ref
//          (refs/merge-requests/N/head, the shape of a fork's job) and a wildcard-only protection read as
//          unprotected. A 404 means not protected; a body that does not parse means not protected. The
//          check never widens what a runner may do: ref_protected only narrows it.

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
)

type jobView struct {
	Ref     string   `json:"ref"`
	Tag     bool     `json:"tag"`
	Status  string   `json:"status"`
	TagList []string `json:"tag_list"`
	Runner  *struct {
		ID int64 `json:"id"`
	} `json:"runner"`
}

// Assessment is what Assess proved about a demand's job.
type Assessment struct {
	ProtectedOnly bool
	// Tags are the job's own tags that the operator configured, in the operator's order; empty when the job
	// could not be read or asks for none of them.
	Tags []string
}

// Restriction implements providers.RunnerRegistrar.
func (r *Registrar) Restriction(ctx context.Context, scope providers.RunnerScope, jobID int64) (providers.Restriction, error) {
	a, err := r.Assess(ctx, scope, jobID)
	return providers.Restriction{ProtectedOnly: a.ProtectedOnly}, err
}

// Assess reads the job once and answers both questions. A transient failure is the error; anything the API
// cannot prove reads as unprotected with no tags.
func (r *Registrar) Assess(ctx context.Context, scope providers.RunnerScope, jobID int64) (Assessment, error) {
	if jobID <= 0 {
		return Assessment{}, nil
	}
	p, err := project(scope)
	if err != nil {
		return Assessment{}, err
	}
	var job jobView
	if _, err = r.c.Do(ctx, "GET", jobPath, p.With("job", itoa(jobID)), nil, &job); err != nil {
		if api.IsStatus(err, 404) {
			return Assessment{}, nil
		}
		return Assessment{}, runnerErr(err, "restriction")
	}
	out := Assessment{Tags: r.demandTags(job.TagList)}
	if job.Ref == "" || strings.HasPrefix(job.Ref, "refs/") {
		return out, nil
	}
	path := protectedBranchPath
	if job.Tag {
		path = protectedTagPath
	}
	var prot struct {
		Name string `json:"name"`
	}
	resp, err := r.c.Do(ctx, "GET", path, p.With("name", job.Ref), nil, &prot)
	if err != nil {
		if resp.Status == 404 {
			return out, nil
		}
		return out, runnerErr(err, "restriction")
	}
	// a 200 whose body is not the protection of this exact ref (a proxy's login page, a wildcard answer) proves nothing
	out.ProtectedOnly = prot.Name == job.Ref
	return out, nil
}

// demandTags keeps the configured tags a job asks for; it is empty unless the job asks for the demand tag.
func (r *Registrar) demandTags(asked []string) []string {
	want := make(map[string]bool, len(asked))
	for _, t := range asked {
		want[t] = true
	}
	var out []string
	for _, t := range r.set.Tags {
		if want[t] {
			out = append(out, t)
		}
	}
	if !want[DemandTag] {
		return nil
	}
	return out
}
