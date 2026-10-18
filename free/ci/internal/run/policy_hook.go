package run

// Purpose: the real PolicyResolver (P7-TRUST-27): it resolves the effective
// policy through internal/policy and hands the digest to the pipeline row and
// the evidence. It replaces the built-in defaults of policy.go.
// Inputs: a PolicyRequest (the repository root and the per-run flag values). Outputs: a PolicyResult.
// Constraints: registered from init(); git runs through the exec funnel
// (report.Capture); the operator rows come from the default state.db, because
// PolicyRequest carries no store path (handoff: Coordinator.policy should pass
// Opts.Home); the local run is classed owner and the repository visibility
// is unknown, which keeps hosted execution off.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// PolicyHook resolves the policy for a run. The zero value uses the default
// store, the process-wide key set and report.Capture.
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
		open = func() (*store.Store, error) { return OpenStore("") }
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
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGit(ctx, res.Run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root, RunValues: r.Values})
	if err != nil {
		return PolicyResult{}, err
	}
	return PolicyResult{Digest: string(got.Digest)}, nil
}
