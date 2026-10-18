package policy_test

// Purpose: the PolicyResolver hook records the same digest as Resolve.
// Constraints: external test package, because run imports policy.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func capture(ctx context.Context, argv ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if err != nil && !asExit(err, &ee) {
		return string(out), -1, err
	}
	if ee != nil {
		return string(out), ee.ExitCode(), nil
	}
	return string(out), 0, nil
}

func asExit(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	*target = ee
	return ok
}

func TestResolveShapeDigest(t *testing.T) {
	root := t.TempDir()
	if out, exit, err := capture(context.Background(), "git", "init", "-q", "-b", "main", root); err != nil || exit != 0 {
		t.Fatalf("git init: %s %v", out, err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".nself/ci.yaml"), []byte("version: 1\npolicy:\n  trust:\n    network: restricted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(db, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	err = st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: "trust.isolation_min", Value: []byte(`"container"`), Actor: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := policy.Resolver{Store: st, Run: capture}
	got, err := r.Resolve(context.Background(), policy.Repo{Root: root, Project: policy.ProjectID(root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Digest) != 64 || got.Values["trust.network"] != "restricted" || got.Sources["trust.network"] != policy.SourceRevision || got.Values["trust.isolation_min"] != "container" {
		t.Fatalf("shape: %+v", got)
	}
	if _, _, ok := got.Effective.Value("trust.privacy_zone"); !ok || got.Fetch != policy.FetchNone {
		t.Fatalf("effective/fetch: %v", got.Fetch)
	}
	hook := run.PolicyHook{OpenStore: func() (*store.Store, error) { return store.Open(db, store.Options{}) }, Capture: capture}
	res, err := hook.Resolve(context.Background(), run.PolicyRequest{Root: root, Pipeline: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Digest != string(got.Digest) {
		t.Fatalf("hook digest %s != resolve digest %s", res.Digest, got.Digest)
	}
	_ = st.Close()
}

// D-0984: per-run flag values reach the digest through PolicyRequest.Values.
func TestHookPassesRunValues(t *testing.T) {
	root := t.TempDir()
	if out, exit, err := capture(context.Background(), "git", "init", "-q", "-b", "main", root); err != nil || exit != 0 {
		t.Fatalf("git init: %s %v", out, err)
	}
	db := filepath.Join(t.TempDir(), "state.db")
	open := func() (*store.Store, error) { return store.Open(db, store.Options{}) }
	st, err := open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hook := run.PolicyHook{OpenStore: open, Capture: capture}
	plain, err := hook.Resolve(context.Background(), run.PolicyRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]json.RawMessage{"trust.network": json.RawMessage(`"none"`)}
	flagged, err := hook.Resolve(context.Background(), run.PolicyRequest{Root: root, Values: values})
	if err != nil {
		t.Fatal(err)
	}
	if flagged.Digest == plain.Digest {
		t.Fatal("the run values did not change the effective policy digest")
	}
	r := policy.Resolver{Store: st, Run: capture}
	want, err := r.Resolve(context.Background(), policy.Repo{Root: root, Project: policy.ProjectIDGit(context.Background(), capture, root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root, RunValues: values})
	if err != nil {
		t.Fatal(err)
	}
	if flagged.Digest != string(want.Digest) || want.Sources["trust.network"] != policy.SourceRun || want.Values["trust.network"] != "none" {
		t.Fatalf("hook digest %s, resolve %s, source %s", flagged.Digest, want.Digest, want.Sources["trust.network"])
	}
}
