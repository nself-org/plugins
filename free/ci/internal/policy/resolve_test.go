package policy

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func resolver(t *testing.T, f *fixture, s Reader) *Resolver {
	t.Helper()
	return &Resolver{Keys: testKeys(t), Store: s, Run: f.runner}
}

// A fork PR (untrusted source) whose worktree ci.yaml widens every guarded key.
func TestPlantedPRIgnored(t *testing.T) {
	f := newFixture(t)
	r := resolver(t, f, nil)
	repo := Repo{Root: f.work, Project: "proj", Visibility: "private"} // no class: untrusted
	base, err := r.Resolve(context.Background(), repo, Revision{Dir: f.work})
	if err != nil {
		t.Fatal(err)
	}
	f.setWorktree(baseConfig + "policy:\n  trust:\n    privacy_zone: hosted-allowed\n    providers: [evil]\n    network: lan\n    secrets: [project, deploy]\n    isolation_min: process\n")
	got, err := r.Resolve(context.Background(), repo, Revision{Dir: f.work})
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != base.Digest || !reflect.DeepEqual(got.Values, base.Values) || !reflect.DeepEqual(got.Sources, base.Sources) {
		t.Fatalf("planted PR changed the policy:\nbase %v\n got %v", base.Values, got.Values)
	}
	ig := ignoredKeys(got)
	for _, key := range []string{"trust.privacy_zone", "trust.providers", "trust.network", "trust.secrets", "trust.isolation_min"} {
		if !strings.HasPrefix(ig[key], "E671") {
			t.Errorf("%s not ignored with E671: %q", key, ig[key])
		}
	}
	for _, c := range got.Ignored {
		if c.Scope != SourceRevision {
			t.Errorf("ignored scope %s", c.Scope)
		}
	}
	if len(base.Ignored) != 0 {
		t.Errorf("base ignored %v", base.Ignored)
	}
}

// The protected branch is read at the fetched tip, not at the checkout.
func TestBranchFetchApplies(t *testing.T) {
	f := newFixture(t)
	r := resolver(t, f, nil)
	repo := Repo{Root: f.work, Project: "proj", Visibility: "private", Class: "owner"}
	before, err := r.Resolve(context.Background(), repo, Revision{Dir: f.work})
	if err != nil {
		t.Fatal(err)
	}
	if before.Fetch != FetchFresh || before.PinnedCommit == "" {
		t.Fatalf("before: fetch %s pin %q", before.Fetch, before.PinnedCommit)
	}
	tip := f.push(baseConfig + "policy:\n  trust:\n    network: restricted\n    approval:\n      first_time: true\n")
	after, err := r.Resolve(context.Background(), repo, Revision{Dir: f.work}) // the worktree still has the old file
	if err != nil {
		t.Fatal(err)
	}
	if after.PinnedCommit != tip || after.Fetch != FetchFresh {
		t.Fatalf("pin %s fetch %s, want %s fresh", after.PinnedCommit, after.Fetch, tip)
	}
	if after.Values["trust.network"] != "restricted" || after.Sources["trust.network"] != SourceProtectedBranch || after.Values["trust.approval.first_time"] != true {
		t.Fatalf("branch policy not applied: %v %v", after.Values, after.Sources)
	}
	if after.Digest == before.Digest {
		t.Fatal("digest did not change")
	}
	// Widening on the protected branch is not a planted-PR case: it fails closed (E670).
	s := testStore(t)
	put(t, s, "project", "proj", "trust.network", `"none"`)
	r.Store = s
	if _, err := r.Resolve(context.Background(), repo, Revision{}); err == nil || !strings.Contains(err.Error(), "E670") {
		t.Fatalf("branch widening over a project row: %v", err)
	}
}

func TestSpendKeyOperatorOnly(t *testing.T) {
	f := newFixture(t)
	s := testStore(t)
	repo := Repo{Root: f.work, Project: "proj", Visibility: "private", Class: "owner"}
	f.push(baseConfig + "policy:\n  economics:\n    paid_enabled: capped\n")
	f.setWorktree(baseConfig + "policy:\n  economics:\n    paid_enabled: off\n")
	r := resolver(t, f, s)
	got, err := r.Resolve(context.Background(), repo, Revision{Dir: f.work})
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["economics.paid_enabled"] != "on" || got.Sources["economics.paid_enabled"] != SourceDefault {
		t.Fatalf("spend key taken from the branch or revision: %v %v", got.Values, got.Sources)
	}
	srcs := map[Source]bool{}
	for _, ig := range got.Ignored {
		if ig.Key == "economics.paid_enabled" && strings.HasPrefix(ig.Reason, "E671") {
			srcs[ig.Scope] = true
		}
	}
	if !srcs[SourceProtectedBranch] || !srcs[SourceRevision] {
		t.Fatalf("ignored scopes %v", srcs)
	}
	put(t, s, "project", "proj", "economics.paid_enabled", `"capped"`)
	got, err = r.Resolve(context.Background(), repo, Revision{Dir: f.work})
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["economics.paid_enabled"] != "capped" || got.Sources["economics.paid_enabled"] != SourceProject {
		t.Fatalf("project row not applied: %v %v", got.Values, got.Sources)
	}
}

func TestOfflineStale(t *testing.T) {
	f := newFixture(t)
	r := resolver(t, f, nil)
	repo := Repo{Root: f.work, Project: "proj", Visibility: "private", Class: "owner"}
	tip := f.push(baseConfig + "policy:\n  trust:\n    network: restricted\n")
	if got, err := r.Resolve(context.Background(), repo, Revision{}); err != nil || got.Fetch != FetchFresh {
		t.Fatalf("online: %v %v", got.Fetch, err)
	}
	git(t, f.work, "remote", "set-url", "origin", f.origin+".gone")
	got, err := r.Resolve(context.Background(), repo, Revision{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Fetch != FetchStale || got.PinnedCommit != tip || got.Values["trust.network"] != "restricted" {
		t.Fatalf("offline: fetch %s pin %s values %v", got.Fetch, got.PinnedCommit, got.Values)
	}
}

func TestNoRemoteDefaults(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main", root)
	write(t, root+"/.nself/ci.yaml", baseConfig+"policy:\n  trust:\n    network: none\n")
	s := testStore(t)
	put(t, s, "user", LocalSubject, "trust.isolation_min", `"container"`)
	r := &Resolver{Keys: testKeys(t), Store: s, Run: testRunner}
	got, err := r.Resolve(context.Background(), Repo{Root: root, Project: "p", Visibility: "private", Class: "owner"}, Revision{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Fetch != FetchNone || got.PinnedCommit != "" {
		t.Fatalf("fetch %s pin %q", got.Fetch, got.PinnedCommit)
	}
	if got.Values["trust.isolation_min"] != "container" || got.Sources["trust.isolation_min"] != SourceUser {
		t.Fatalf("operator row: %v", got.Values)
	}
	if got.Values["trust.network"] != "privileged" || got.Sources["trust.network"] != SourceDefault {
		t.Fatalf("the worktree policy must not apply without a revision scope: %v", got.Values)
	}
	// Not a repository at all: still defaults.
	plain := t.TempDir()
	got, err = r.Resolve(context.Background(), Repo{Root: plain, Visibility: "unknown", Class: "owner"}, Revision{})
	if err != nil || got.Fetch != FetchNone {
		t.Fatalf("plain dir: %v %v", got.Fetch, err)
	}
}

func TestPinUnreachable(t *testing.T) {
	f := newFixture(t)
	repo := Repo{Root: f.work, Project: "proj", Visibility: "private", Class: "owner"}
	older := f.push(baseConfig + "policy:\n  trust:\n    network: lan\n")
	tip := f.push(baseConfig + "policy:\n  trust:\n    network: restricted\n")
	// A commit that exists locally but is not on origin/main.
	write(t, f.work+"/.nself/ci.yaml", baseConfig+"policy:\n  trust:\n    network: none\n")
	git(t, f.work, "checkout", "-q", "-b", "side")
	git(t, f.work, "commit", "-q", "-am", "side")
	side := git(t, f.work, "rev-parse", "HEAD")

	s := testStore(t)
	r := resolver(t, f, s)
	pin := func(sha string) {
		if err := s.ApplyWrite(context.Background(), storePin(sha), nil); err != nil {
			t.Fatal(err)
		}
	}
	pin(older)
	got, err := r.Resolve(context.Background(), repo, Revision{})
	if err != nil || got.PinnedCommit != older || got.Values["trust.network"] != "lan" {
		t.Fatalf("reachable pin: %v pin %s values %v", err, got.PinnedCommit, got.Values)
	}
	pin(side)
	got, err = r.Resolve(context.Background(), repo, Revision{})
	if err != nil {
		t.Fatal(err)
	}
	if got.PinnedCommit != tip || got.Values["trust.network"] != "restricted" {
		t.Fatalf("unreachable pin used: pin %s values %v", got.PinnedCommit, got.Values)
	}
	if reason := ignoredKeys(got)["pinned_commit"]; !strings.HasPrefix(reason, "E672") {
		t.Fatalf("unreachable pin not ignored with E672: %q", reason)
	}
}

func TestMachineInvokedCap(t *testing.T) {
	f := newFixture(t)
	s := testStore(t)
	repo := Repo{Root: f.work, Project: "proj", Visibility: "private", Class: "owner"}
	r := resolver(t, f, s)
	ctx := context.Background()
	got, err := r.Resolve(ctx, repo, Revision{})
	if err != nil || got.Values["trust.machine_invoked_cap"] != "owner" || got.Sources["trust.machine_invoked_cap"] != SourceDefault {
		t.Fatalf("default: %v %v", got.Values["trust.machine_invoked_cap"], err)
	}
	put(t, s, "user", LocalSubject, "trust.machine_invoked_cap", `"collaborator"`)
	got, err = r.Resolve(ctx, repo, Revision{})
	if err != nil || got.Values["trust.machine_invoked_cap"] != "collaborator" || got.Sources["trust.machine_invoked_cap"] != SourceUser {
		t.Fatalf("user value: %v %v", got.Values["trust.machine_invoked_cap"], err)
	}
	f.setWorktree(baseConfig + "policy:\n  trust:\n    machine_invoked_cap: owner\n")
	got, err = r.Resolve(ctx, repo, Revision{Dir: f.work})
	if err != nil || got.Values["trust.machine_invoked_cap"] != "collaborator" || !strings.HasPrefix(ignoredKeys(got)["trust.machine_invoked_cap"], "E671") {
		t.Fatalf("a revision must not raise the cap: %v %v", got.Values["trust.machine_invoked_cap"], err)
	}
	f.setWorktree(baseConfig + "policy:\n  trust:\n    machine_invoked_cap: untrusted\n")
	got, err = r.Resolve(ctx, repo, Revision{Dir: f.work})
	if err != nil || got.Values["trust.machine_invoked_cap"] != "untrusted" || got.Sources["trust.machine_invoked_cap"] != SourceRevision {
		t.Fatalf("a revision may lower the cap: %v %v", got.Values["trust.machine_invoked_cap"], err)
	}
	put(t, s, "project", "proj", "trust.machine_invoked_cap", `"owner"`)
	if _, err := r.Resolve(ctx, repo, Revision{}); err == nil || !strings.Contains(err.Error(), "E670") {
		t.Fatalf("a project value over a user value must be E670: %v", err)
	}
}

func TestUnknownAndInvalidKeys(t *testing.T) {
	f := newFixture(t)
	r := resolver(t, f, nil)
	repo := Repo{Root: f.work, Visibility: "private", Class: "owner"}
	for name, doc := range map[string]string{
		"unknown key":   "policy:\n  trust:\n    no_such: 1\n",
		"null value":    "policy:\n  trust:\n    network:\n",
		"wrong type":    "policy:\n  trust:\n    secrets: project\n",
		"double define": "policy:\n  trust:\n    network: none\n  trust.network: none\n",
	} {
		f.setWorktree(baseConfig + doc)
		if _, err := r.Resolve(context.Background(), repo, Revision{Dir: f.work}); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("%s: %v", name, err)
		}
	}
	f.setWorktree(baseConfig + "policy:\n  sched:\n    anything: 3\n")
	got, err := r.Resolve(context.Background(), repo, Revision{Dir: f.work})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Values["sched.anything"]; ok {
		t.Fatalf("a revision introduced a key with no registered default: %v", got.Values)
	}
	if !strings.HasPrefix(ignoredKeys(got)["sched.anything"], "E671") {
		t.Fatalf("ignored: %v", got.Ignored)
	}
}

func storePin(sha string) store.PolicyWrite {
	return store.PolicyWrite{Op: store.PolicyPinOp, Subject: "proj", SHA: sha, Actor: "test"}
}

// Root may be a subdirectory of the repository (monorepo): the protected branch
// is read at the same relative path, so a PR cannot drop its policy by deleting it.
func TestPlantedPRSubdirRoot(t *testing.T) {
	f := newFixture(t)
	f.pushAt("svc/.nself/ci.yaml", baseConfig+"policy:\n  trust:\n    network: none\n")
	git(t, f.work, "pull", "-q", "origin", "main")
	svc := filepath.Join(f.work, "svc")
	write(t, filepath.Join(svc, ".nself/ci.yaml"), baseConfig) // the PR deletes the key
	r := resolver(t, f, nil)
	got, err := r.Resolve(context.Background(), Repo{Root: svc, Visibility: "private", Class: "owner"}, Revision{Dir: svc})
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["trust.network"] != "none" || got.Sources["trust.network"] != SourceProtectedBranch {
		t.Fatalf("subdirectory policy lost: %v %v", got.Values, got.Sources)
	}
}

// A repository that cannot list the tip's config fails closed instead of reading no policy.
func TestUnreadableTipFailsClosed(t *testing.T) {
	f := newFixture(t)
	r := resolver(t, f, nil)
	broken := func(ctx context.Context, argv ...string) (string, int, error) {
		for _, a := range argv {
			if a == "ls-tree" {
				return "fatal: not a tree object", 128, nil
			}
		}
		return f.runner(ctx, argv...)
	}
	r.Run = broken
	if _, err := r.Resolve(context.Background(), Repo{Root: f.work, Visibility: "private", Class: "owner"}, Revision{}); err == nil || !strings.Contains(err.Error(), "E672") {
		t.Fatalf("unreadable tip: %v", err)
	}
}

// D-0984: the run scope reaches the effective policy and is tighten-only.
func TestRunValuesScope(t *testing.T) {
	f := newFixture(t)
	r := resolver(t, f, nil)
	repo := Repo{Root: f.work, Project: "proj", Visibility: "private", Class: "owner"}
	f.setWorktree(baseConfig + "policy:\n  trust:\n    network: restricted\n")
	base, err := r.Resolve(context.Background(), repo, Revision{Dir: f.work})
	if err != nil {
		t.Fatal(err)
	}
	if base.Values["trust.network"] != "restricted" {
		t.Fatalf("setup: %v", base.Values["trust.network"])
	}
	run := map[string]json.RawMessage{"sched.preset": json.RawMessage(`"prefer-local"`), "trust.network": json.RawMessage(`"lan"`)}
	got, err := r.Resolve(context.Background(), repo, Revision{Dir: f.work, RunValues: run})
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["sched.preset"] != "prefer-local" || got.Sources["sched.preset"] != SourceRun {
		t.Fatalf("preset: %v from %s", got.Values["sched.preset"], got.Sources["sched.preset"])
	}
	if got.Values["trust.network"] != base.Values["trust.network"] || !strings.HasPrefix(ignoredKeys(got)["trust.network"], "E671") {
		t.Fatalf("a run flag loosened trust.network: %v, ignored %v", got.Values["trust.network"], ignoredKeys(got))
	}
	if got.Digest == base.Digest {
		t.Fatal("digest does not bind the run values")
	}
}
