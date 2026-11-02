package policy

// Purpose: share one protected-branch snapshot across an invocation's policy
// readers, avoiding repeated fetches of the same tip.
// Inputs: a run context, repository root and process runner.
// Outputs: the branch and fetch state first successfully observed in that run.
// Constraints: operator rows and revision policy are never cached; errors are
// retried. The cache dies with its context and is never shared between runs.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
)

type branchSnapshotKey struct{}

type branchSnapshot struct {
	mu       sync.Mutex
	branches map[string]Branch
	policies map[string]map[string]json.RawMessage
}

// WithBranchSnapshot gives one invocation a stable protected-branch tip. Nested
// callers keep the existing snapshot; a new invocation starts with a new context.
func WithBranchSnapshot(ctx context.Context) context.Context {
	if _, ok := ctx.Value(branchSnapshotKey{}).(*branchSnapshot); ok {
		return ctx
	}
	return context.WithValue(ctx, branchSnapshotKey{}, &branchSnapshot{
		branches: map[string]Branch{}, policies: map[string]map[string]json.RawMessage{},
	})
}

// branchForRun reads once per repository when an invocation installed a snapshot.
// Serializing the first reads also prevents duplicate concurrent fetches.
func branchForRun(ctx context.Context, run Runner, root string) (Branch, error) {
	s, ok := ctx.Value(branchSnapshotKey{}).(*branchSnapshot)
	if !ok {
		return readBranch(ctx, run, root)
	}
	key, err := filepath.Abs(root)
	if err != nil {
		return readBranch(ctx, run, root)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.branches[key]; ok {
		return b, nil
	}
	b, err := readBranch(ctx, run, root)
	if err == nil && ctx.Err() == nil && (b.Name == "" || b.Fetch == FetchFresh && b.Tip != "") {
		s.branches[key] = b
	}
	return b, err
}

// policyAt shares successful reads of immutable committed config bytes. Root
// remains part of the key because a monorepo's relative config path matters.
func policyAt(ctx context.Context, run Runner, root, sha string) (map[string]json.RawMessage, error) {
	s, ok := ctx.Value(branchSnapshotKey{}).(*branchSnapshot)
	if !ok || !fullSHA.MatchString(sha) {
		return readPolicyAt(ctx, run, root, sha)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return readPolicyAt(ctx, run, root, sha)
	}
	key := abs + "\x00" + sha
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.policies[key]; ok {
		return clonePolicy(p), nil
	}
	p, err := readPolicyAt(ctx, run, root, sha)
	if err == nil && ctx.Err() == nil {
		s.policies[key] = clonePolicy(p)
	}
	return p, err
}

func clonePolicy(in map[string]json.RawMessage) map[string]json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(map[string]json.RawMessage, len(in))
	for key, value := range in {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}
