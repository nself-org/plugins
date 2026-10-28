package run

// Purpose: the real PolicyResolver (P7-TRUST-27): it resolves the effective
// policy through internal/policy and hands the digest to the pipeline row and
// the evidence. It replaces the built-in defaults of policy.go.
// Inputs: a PolicyRequest (the repository root). Outputs: a PolicyResult.
// Constraints: the digest is the policy in force without the per-run values (they
// steer placement only; evidence verification re-resolves without them); registered from init(); git runs through the exec funnel
// (report.Capture); the operator rows come from PolicyRequest.Store (the
// coordinator's state.db; "" = the default state.db); the policy resolves at the class the run is
// admitted at (PolicyRequest.Class; "" = the operator's own checkout, owner) and the repository
// visibility is unknown unless the host's RepoVisibility says private, which keeps hosted execution off.

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// PolicyHook resolves the policy for a run. The zero value uses the store of
// the request (the default one when it names none), the process-wide key set
// and report.Capture.
type PolicyHook struct {
	OpenStore func() (*store.Store, error)
	Capture   Capture
	Keys      *policy.Keyset
}

func init() { RegisterPolicy(PolicyHook{}) }

// Resolve implements PolicyResolver.
func (h PolicyHook) Resolve(ctx context.Context, r PolicyRequest) (PolicyResult, error) {
	open := h.OpenStore
	if open == nil {
		open = func() (*store.Store, error) { return OpenStore(r.Store) }
	}
	capture := h.Capture
	if capture == nil {
		capture = report.Capture
	}
	st, err := open()
	if err != nil {
		return PolicyResult{}, err
	}
	defer func() { _ = st.Close() }()
	root := r.Root
	if root == "" {
		root = "."
	}
	res := policy.Resolver{Keys: h.Keys, Store: st, Run: policy.Runner(capture)}
	vis := r.Visibility
	if vis != "private" {
		vis = "unknown"
	}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGitCached(ctx, res.Run, root), Visibility: vis, Class: classOrOwner(r.Class)}, policy.Revision{Dir: root, Run: r.Run})
	if err != nil {
		return PolicyResult{}, err
	}
	out := PolicyResult{Digest: string(got.BaseDigest)}
	if p, qp, err := schedulingOf(&got.Effective); err == nil { // a refusal is left to the dispatcher, which reports it on the first job
		out.Placement, out.Queue = &p, &qp
	}
	for _, ig := range got.Ignored {
		if ig.Scope == policy.SourceRun {
			out.IgnoredRun = append(out.IgnoredRun, ig.Key+": "+ig.Reason)
		} else {
			out.Notes = append(out.Notes, ig.Key+": "+ig.Reason)
		}
	}
	if got.Fetch == policy.FetchStale {
		out.Notes = append(out.Notes, "protected-branch tip is stale: the fetch of origin failed, the last fetched tip applies")
	}
	if v, ok := got.Values["trust.secrets"]; ok {
		out.SecretClasses, out.HasSecretClasses = secrets.AllowlistOfValue(v)
	}
	return out, nil
}

// classOrOwner is the class a policy resolves at: the run's own, else owner (the local operator's checkout).
func classOrOwner(c model.TrustClass) model.TrustClass {
	if c == "" {
		return "owner"
	}
	return c
}

// policyVisibility is "private" when the host's RepoVisibility seam says the "owner/name" repository is
// private, else "unknown". Public is never reported: the seam answers public for a repository it does not
// know, and "public" would open the hosted-allowed zone.
func policyVisibility(repo string) string {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || RepoVisibility == nil || RepoVisibility(owner, name) != attest.Private {
		return "unknown"
	}
	return "private"
}

// schedulingOf is the placement and queue policy of an effective policy.
func schedulingOf(eff *policy.Effective) (sched.Policy, schedpolicy.QueuePolicy, error) {
	p, err := schedpolicy.Placement(eff)
	if err != nil {
		return sched.Policy{}, schedpolicy.QueuePolicy{}, err
	}
	qp, err := schedpolicy.Queue(eff)
	return p, qp, err
}
