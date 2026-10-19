package jit

// Purpose: FetchApprovalPolicy, the fork-approval fact (Epic D6): GET
//          /repos/{o}/{r}/actions/permissions/fork-pr-contributor-approval is stored as the
//          host-provider fact github/<repo>/fork_pr_approval_policy.
// Inputs:  a scope naming owner/repo.
// Outputs: the policy string; the fact {"approval_policy": ...} observed now.
// Constraints: the doctor warns when JIT is enabled for a public repo and the policy is not
//          all_external_contributors (HOST-12). The value is a short policy name, never a secret.

import (
	"context"
	"encoding/json"
	"regexp"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// The key under which the policy is stored, and the strictest policy.
const (
	FactApprovalPolicy = "fork_pr_approval_policy"
	StrictApproval     = "all_external_contributors"
)

var policyName = regexp.MustCompile(`^[a-z_]{1,64}$`)

// FetchApprovalPolicy reads the policy and records it when a fact sink is configured.
func (r *Registrar) FetchApprovalPolicy(ctx context.Context, scope providers.RunnerScope) (string, error) {
	owner, repo, found := splitRepo(scope.Repo)
	if !found {
		return "", providers.NewError(providers.Config, "E702", "scope.repo", 0, time.Time{})
	}
	var out struct {
		Policy string `json:"approval_policy"`
	}
	if _, err := r.c.Do(ctx, "GET", "/repos/{owner}/{repo}/actions/permissions/fork-pr-contributor-approval",
		map[string]string{"owner": owner, "repo": repo}, nil, &out); err != nil {
		return "", err
	}
	if !policyName.MatchString(out.Policy) {
		return "", providers.NewError(providers.Config, "E702", "approval.policy", 0, time.Time{})
	}
	if r.facts != nil {
		value, _ := json.Marshal(map[string]string{"approval_policy": out.Policy})
		if _, err := r.facts.ProviderFactPut(ctx, store.ProviderFact{
			Provider: factProvider, Scope: scope.Repo, Key: FactApprovalPolicy, Value: value, ObservedAt: r.now(),
		}); err != nil {
			return out.Policy, err
		}
	}
	return out.Policy, nil
}
