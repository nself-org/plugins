package policy

// Purpose: verify immutable policy config reads keep their bytes and failure
// behavior when shared by an invocation.
// Inputs: committed config objects and a counting real git runner.
// Outputs: equivalent policies, independent returned bytes and bounded spawns.
// Constraints: failed object reads are always retried.

import (
	"context"
	"reflect"
	"slices"
	"testing"
)

func TestPolicyAtSnapshotEquivalentAndIndependent(t *testing.T) {
	f := newFixture(t)
	sha := f.push(baseConfig + "policy:\n  trust:\n    network: none\n")
	git(t, f.work, "fetch", "-q", "origin")
	want, err := readPolicyAt(context.Background(), f.runner, f.work, sha)
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	run := func(ctx context.Context, args ...string) (string, int, error) {
		reads++
		return f.runner(ctx, args...)
	}
	ctx := WithBranchSnapshot(context.Background())
	for range 3 {
		got, err := policyAt(ctx, run, f.work, sha)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("snapshot = %v, %v; want %v", got, err, want)
		}
		got["trust.network"][0] = '!'
		delete(got, "trust.network")
	}
	if reads != 2 { // one regular-file listing and one show
		t.Fatalf("immutable config spawned %d git calls, want 2", reads)
	}
}

func TestPolicyAtSnapshotRetriesObjectFailure(t *testing.T) {
	f := newFixture(t)
	sha := git(t, f.work, "rev-parse", "HEAD")
	broken := true
	run := func(ctx context.Context, args ...string) (string, int, error) {
		if broken && slices.Contains(args, "ls-tree") {
			return "cannot read tree", 128, nil
		}
		return f.runner(ctx, args...)
	}
	ctx := WithBranchSnapshot(context.Background())
	if _, err := policyAt(ctx, run, f.work, sha); err == nil {
		t.Fatal("object error was hidden")
	}
	broken = false
	if _, err := policyAt(ctx, run, f.work, sha); err != nil {
		t.Fatalf("object failure was retained: %v", err)
	}
}
