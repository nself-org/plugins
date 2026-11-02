package policy

// Purpose: verify invocation branch snapshots preserve policy answers and retry
// failures while leaving mutable operator and revision scopes live.
// Inputs: real local git origins, real policy rows, counting runners.
// Outputs: byte-identical digests and exactly one successful fetch per snapshot.
// Constraints: no network; independent contexts must never share answers.

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestBranchSnapshotEquivalentAndInvocationScoped(t *testing.T) {
	f := newFixture(t)
	git(t, f.work, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	want, err := readBranch(context.Background(), f.runner, f.work)
	if err != nil {
		t.Fatal(err)
	}
	fetches := 0
	run := func(ctx context.Context, args ...string) (string, int, error) {
		if slices.Contains(args, "fetch") {
			fetches++
		}
		return f.runner(ctx, args...)
	}
	ctx := WithBranchSnapshot(context.Background())
	for range 3 {
		got, err := branchForRun(WithBranchSnapshot(ctx), run, f.work)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("snapshot = %+v, %v; plain = %+v", got, err, want)
		}
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d, want 1", fetches)
	}
	next := f.push(baseConfig + "policy:\n  trust:\n    network: none\n")
	b, err := branchForRun(WithBranchSnapshot(context.Background()), run, f.work)
	if err != nil || b.Tip != next || fetches != 2 {
		t.Fatalf("next invocation = %+v, %v; fetches %d", b, err, fetches)
	}
}

func TestBranchSnapshotLeavesMutablePolicyScopesFresh(t *testing.T) {
	f := newFixture(t)
	st := testStore(t)
	r := resolver(t, f, st)
	repo := Repo{Root: f.work, Project: "proj", Visibility: "private", Class: "owner"}
	rev := Revision{Dir: f.work}
	ctx := WithBranchSnapshot(context.Background())
	want, err := r.Resolve(context.Background(), repo, rev)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Resolve(ctx, repo, rev)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot changed resolved policy:\n got %+v %v\nwant %+v", got, err, want)
	}
	f.setWorktree(baseConfig + "policy:\n  trust:\n    network: none\n")
	got, err = r.Resolve(ctx, repo, rev)
	if err != nil || got.Values["trust.network"] != "none" || got.Digest == want.Digest {
		t.Fatalf("revision tightening was hidden: %+v %v", got, err)
	}
	put(t, st, "user", LocalSubject, "trust.isolation_min", `"container"`)
	updated, err := r.Resolve(ctx, repo, rev)
	if err != nil || updated.Values["trust.isolation_min"] != "container" || updated.Digest == got.Digest {
		t.Fatalf("operator tightening was hidden: %+v %v", updated, err)
	}
}

func TestBranchSnapshotDoesNotCacheFailuresOrStaleTips(t *testing.T) {
	f := newFixture(t)
	git(t, f.work, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	for _, mode := range []string{"spawn", "fetch", "tip"} {
		t.Run(mode, func(t *testing.T) {
			broken, calls := true, 0
			run := func(ctx context.Context, args ...string) (string, int, error) {
				calls++
				if broken {
					switch {
					case mode == "spawn":
						return "", -1, errors.New("cannot start git")
					case mode == "fetch" && slices.Contains(args, "fetch"):
						return "fetch failed", 128, nil
					case mode == "tip" && slices.Contains(args, "rev-parse"):
						return "", 128, nil
					}
				}
				return f.runner(ctx, args...)
			}
			ctx := WithBranchSnapshot(context.Background())
			_, _ = branchForRun(ctx, run, f.work)
			before := calls
			broken = false
			b, err := branchForRun(ctx, run, f.work)
			if err != nil || b.Fetch != FetchFresh || calls == before {
				t.Fatalf("failure was retained: %+v %v; calls %d -> %d", b, err, before, calls)
			}
		})
	}
}

func TestBranchSnapshotNoOriginDoesNotEscapeInvocation(t *testing.T) {
	f := newFixture(t)
	git(t, f.work, "remote", "remove", "origin")
	first, err := branchForRun(WithBranchSnapshot(context.Background()), f.runner, f.work)
	if err != nil || first.Name != "" {
		t.Fatalf("no origin = %+v, %v", first, err)
	}
	git(t, f.work, "remote", "add", "origin", f.origin)
	next, err := branchForRun(WithBranchSnapshot(context.Background()), f.runner, f.work)
	if err != nil || next.Fetch != FetchFresh || next.Tip == "" {
		t.Fatalf("new invocation retained no-origin: %+v, %v", next, err)
	}
}
