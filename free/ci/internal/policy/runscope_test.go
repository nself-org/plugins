package policy_test

// Purpose: the per-run scope through the real resolver and the real sched keys
// (P7-AUTO-29, D-0984): `--prefer`, `--runner` and `--max-queue` reach the
// effective policy, a pinned runner beats a different `--runner`, and a run
// with no per-run values keeps its digest.
// Constraints: external package, because schedpolicy imports policy.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func runScopeFixture(t *testing.T, config string) (policy.Resolver, policy.Repo, policy.Revision) {
	t.Helper()
	root := t.TempDir()
	if out, exit, err := capture(context.Background(), "git", "init", "-q", "-b", "main", root); err != nil || exit != 0 {
		t.Fatalf("git init: %s %v", out, err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".nself/ci.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return policy.Resolver{Store: st, Run: capture}, policy.Repo{Root: root, Project: policy.ProjectID(root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root}
}

func runValues(t *testing.T, f schedpolicy.Flags) map[string]json.RawMessage {
	t.Helper()
	v, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRunScopeReachesPolicy(t *testing.T) {
	res, repo, rev := runScopeFixture(t, "version: 1\n")
	base, err := res.Resolve(context.Background(), repo, rev)
	if err != nil {
		t.Fatal(err)
	}
	rev.Run = runValues(t, schedpolicy.Flags{Prefer: "local", Runner: "ms5", MaxQueue: "5m"})
	got, err := res.Resolve(context.Background(), repo, rev)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{schedpolicy.KeyPreset: "prefer-local", schedpolicy.KeyRunner: "ms5", schedpolicy.KeyMaxQueue: int64(300000)} {
		if v := got.Values[key]; v != want || got.Sources[key] != policy.SourceRun {
			t.Errorf("%s = %v from %s, want %v from run", key, v, got.Sources[key], want)
		}
	}
	if got.Digest == base.Digest || len(got.Ignored) != len(base.Ignored) {
		t.Errorf("digest unchanged or values ignored: %v", got.Ignored)
	}
	if got.BaseDigest != base.Digest || base.BaseDigest != base.Digest {
		t.Errorf("BaseDigest must be the digest without the run values: %s vs %s", got.BaseDigest, base.Digest)
	}
	p, err := schedpolicy.Placement(&got.Effective)
	if err != nil || p.Preset != "prefer-local" || p.Overrides.Runner != "ms5" || p.Overrides.MaxQueueMs != 300000 {
		t.Errorf("placement view: %+v %v", p, err)
	}
	rev.Run = nil
	again, err := res.Resolve(context.Background(), repo, rev)
	if err != nil || again.Digest != base.Digest {
		t.Errorf("a run without per-run values changed its digest: %v", err)
	}
}

func TestRunScopeProjectBeatsRun(t *testing.T) {
	res, repo, rev := runScopeFixture(t, "version: 1\n")
	put := func(key, value string) {
		err := res.Store.(*store.Store).ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "project", Subject: repo.Project, Key: key, Value: []byte(value), Actor: "test"}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	put(schedpolicy.KeyRunner, `"ms6"`)
	put(schedpolicy.KeyLocalOnly, `true`)
	put(schedpolicy.KeyMaxQueue, `"2m"`)
	rev.Run = runValues(t, schedpolicy.Flags{Runner: "ms5", MaxQueue: "10m", Prefer: "fastest"})
	got, err := res.Resolve(context.Background(), repo, rev)
	if err != nil {
		t.Fatal(err)
	}
	if got.Values[schedpolicy.KeyRunner] != "ms6" || got.Values[schedpolicy.KeyMaxQueue] != int64(120000) || got.Values[schedpolicy.KeyLocalOnly] != true {
		t.Errorf("run scope loosened the project: %v", got.Values)
	}
	if got.Values[schedpolicy.KeyPreset] != "fastest" {
		t.Errorf("preset preference lost: %v", got.Values[schedpolicy.KeyPreset])
	}
	ignored := map[string]string{}
	for _, ig := range got.Ignored {
		if ig.Scope == policy.SourceRun {
			ignored[ig.Key] = ig.Reason
		}
	}
	if len(ignored) != 2 || !strings.Contains(ignored[schedpolicy.KeyRunner], "E671") || !strings.Contains(ignored[schedpolicy.KeyMaxQueue], "E671") {
		t.Errorf("ignored run values = %v", ignored)
	}
}

// TestRunScopeSpendRefused: a Spend key in the run scope is ignored, never set.
func TestRunScopeSpendRefused(t *testing.T) {
	res, repo, rev := runScopeFixture(t, "version: 1\n")
	base, err := res.Resolve(context.Background(), repo, rev)
	if err != nil {
		t.Fatal(err)
	}
	rev.Run = map[string]json.RawMessage{schedpolicy.KeyPaidEnabled: json.RawMessage(`true`)}
	got, err := res.Resolve(context.Background(), repo, rev)
	if err != nil {
		t.Fatal(err)
	}
	if got.Values[schedpolicy.KeyPaidEnabled] != false || got.Digest != base.Digest {
		t.Errorf("spend key accepted from the run scope: %v", got.Values[schedpolicy.KeyPaidEnabled])
	}
	for _, ig := range got.Ignored {
		if ig.Key == schedpolicy.KeyPaidEnabled && ig.Scope == policy.SourceRun && strings.Contains(ig.Reason, "E671") {
			return
		}
	}
	t.Errorf("the refusal is not reported: %v", got.Ignored)
}
