package jit

// Purpose: Restriction, the verified org-runner-group check (Epic D6, review F7).
// Inputs:  a scope (the owner is read from scope.Repo) and a runner group id.
// Outputs: Verdict; Restriction returns the providers.Restriction view of it.
// Constraints: ProtectedOnly is true only when ALL of these hold, each read from the API: the group
//          allows no public repositories; it is restricted_to_workflows; it lists at least one
//          workflow; every listed workflow is pinned to @refs/heads/<branch> (a concrete branch, not
//          a tag, a bare name, a wildcard or a SHA) and the branch protection API says that branch of
//          that repository (the group's own organization) is protected. Anything unreadable, or a
//          reply missing a field, is "not verified", never "verified". Only organization runner
//          groups can be proven: at repo scope the answer is always "not proven" (a repo-level
//          runner is not governed by a group).
//          ProtectedOnly describes the GROUP, not one job: GitHub hands a queued job to any runner
//          whose labels match, so every job the group can admit (visibility, reusable workflows) must
//          pass TRUST-13's fact rule, not only the demand that minted the runner (Epic note, HOST-04).
//          This says nothing about the code a protected workflow checks out: a pull_request_target or
//          workflow_run workflow, or one that checks out a fork head, still runs fork code on the
//          runner. TRUST-13 decides that from RunEvent, HeadRepo and Fork.

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// Verdict is the full answer; Reason is empty only when ProtectedOnly is true.
type Verdict struct {
	Verified      bool // the group was read from the API and judged
	ProtectedOnly bool
	Reason        string
	Workflows     []string
}

// AllowsPublic and Restricted are pointers: a reply that omits either field is not proof of anything.
type groupReply struct {
	AllowsPublic  *bool    `json:"allows_public_repositories"`
	Restricted    *bool    `json:"restricted_to_workflows"`
	WorkflowsList []string `json:"selected_workflows"`
}

func notProven(reason string, verified bool, wf []string) Verdict {
	return Verdict{Verified: verified, Reason: reason, Workflows: wf}
}

// RestrictionVerdict reads the group and the branch protection of every pinned workflow.
func (r *Registrar) RestrictionVerdict(ctx context.Context, scope providers.RunnerScope, groupID int64) (Verdict, error) {
	t, err := r.resolve(scope)
	if err != nil {
		return Verdict{}, err
	}
	if groupID <= 0 {
		return notProven("no_group", false, nil), nil
	}
	if r.set.Scope == ScopeRepo {
		// Runner groups govern organization runners. A repo-level runner serves every workflow of its
		// repository, fork pull requests included, whatever the org's default group says.
		return notProven("repo_scope_unrestricted", false, nil), nil
	}
	var g groupReply
	_, err = r.c.Do(ctx, "GET", "/orgs/{org}/actions/runner-groups/{group}",
		map[string]string{"org": t.owner, "group": strconv.FormatInt(groupID, 10)}, nil, &g)
	if err != nil {
		if pe, ok := err.(*providers.Error); ok && pe.Status == 404 {
			return notProven("group_unreadable", false, nil), nil
		}
		return Verdict{}, err
	}
	switch {
	case g.AllowsPublic == nil || g.Restricted == nil:
		return notProven("group_fields_missing", false, nil), nil
	case *g.AllowsPublic:
		return notProven("public_repositories_allowed", true, g.WorkflowsList), nil
	case !*g.Restricted:
		return notProven("not_restricted_to_workflows", true, g.WorkflowsList), nil
	case len(g.WorkflowsList) == 0:
		return notProven("no_selected_workflows", true, g.WorkflowsList), nil
	}
	checked := map[string]bool{}
	for _, entry := range g.WorkflowsList {
		owner, repo, branch, ok := pinnedBranch(entry)
		if !ok {
			return notProven("workflow_not_branch_pinned", true, g.WorkflowsList), nil
		}
		if !strings.EqualFold(owner, t.owner) {
			// Protection on another owner's repository is theirs to set and say nothing about ours.
			return notProven("workflow_foreign_owner", true, g.WorkflowsList), nil
		}
		key := owner + "/" + repo + "@" + branch
		if checked[key] {
			continue
		}
		protected, err := r.branchProtected(ctx, owner, repo, branch)
		if err != nil {
			return Verdict{}, err
		}
		if !protected {
			return notProven("workflow_branch_unprotected", true, g.WorkflowsList), nil
		}
		checked[key] = true
	}
	return Verdict{Verified: true, ProtectedOnly: true, Workflows: g.WorkflowsList}, nil
}

// Restriction implements providers.RunnerRegistrar. An API failure is an error, which a caller must
// read as "not protected-only".
func (r *Registrar) Restriction(ctx context.Context, scope providers.RunnerScope, groupID int64) (providers.Restriction, error) {
	v, err := r.RestrictionVerdict(ctx, scope, groupID)
	return providers.Restriction{ProtectedOnly: v.ProtectedOnly, Workflows: v.Workflows}, err
}

var (
	branchName   = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	workflowPath = regexp.MustCompile(`^\.github/workflows/[A-Za-z0-9._-]+\.ya?ml$`)
)

// pinnedBranch splits "owner/repo/.github/workflows/<file>.yml@refs/heads/<branch>". The ref is cut at
// the LAST "@", the path may hold none, the file must sit in .github/workflows, and the branch must be
// a plain branch name (no "..", "//", leading or trailing "/", "@" or "%").
func pinnedBranch(entry string) (owner, repo, branch string, ok bool) {
	i := strings.LastIndexByte(entry, '@')
	if i < 0 {
		return "", "", "", false
	}
	path, ref := entry[:i], entry[i+1:]
	branch, isHead := strings.CutPrefix(ref, "refs/heads/")
	parts := strings.SplitN(path, "/", 3)
	if !isHead || len(parts) < 3 || parts[0] == "" || parts[1] == "" || !workflowPath.MatchString(parts[2]) {
		return "", "", "", false
	}
	if !branchName.MatchString(branch) || strings.Contains(branch, "..") || strings.Contains(branch, "//") ||
		strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") || strings.HasSuffix(branch, ".lock") {
		return "", "", "", false
	}
	return parts[0], parts[1], branch, true
}

// branchProtected is true when the branch protection API answers 200; 404 (no rule, no right to see
// it, a rules-set-only rule, or a branch name with a slash that GitHub does not accept escaped) is
// "not protected", which errs on the safe side, and any other failure is an error.
func (r *Registrar) branchProtected(ctx context.Context, owner, repo, branch string) (bool, error) {
	_, err := r.c.Do(ctx, "GET", "/repos/{owner}/{repo}/branches/{branch}/protection",
		map[string]string{"owner": owner, "repo": repo, "branch": branch}, nil, nil)
	if err == nil {
		return true, nil
	}
	if pe, ok := err.(*providers.Error); ok && pe.Status == 404 {
		return false, nil
	}
	return false, err
}
