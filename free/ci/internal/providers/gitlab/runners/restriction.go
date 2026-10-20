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
	Ref    string `json:"ref"`
	Tag    bool   `json:"tag"`
	Status string `json:"status"`
	Runner *struct {
		ID int64 `json:"id"`
	} `json:"runner"`
}

// Restriction implements providers.RunnerRegistrar.
func (r *Registrar) Restriction(ctx context.Context, scope providers.RunnerScope, jobID int64) (providers.Restriction, error) {
	if jobID <= 0 {
		return providers.Restriction{}, nil
	}
	p, err := project(scope)
	if err != nil {
		return providers.Restriction{}, err
	}
	var job jobView
	if _, err = r.c.Do(ctx, "GET", jobPath, p.With("job", itoa(jobID)), nil, &job); err != nil {
		if api.IsStatus(err, 404) {
			return providers.Restriction{}, nil
		}
		return providers.Restriction{}, runnerErr(err, "restriction")
	}
	if job.Ref == "" || strings.HasPrefix(job.Ref, "refs/") {
		return providers.Restriction{}, nil
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
			return providers.Restriction{}, nil
		}
		return providers.Restriction{}, runnerErr(err, "restriction")
	}
	// a 200 whose body is not the protection of this exact ref (a proxy's login page, a wildcard answer) proves nothing
	return providers.Restriction{ProtectedOnly: prot.Name == job.Ref}, nil
}
