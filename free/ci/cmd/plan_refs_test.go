package main

// Purpose: compare batched remote queries to the original individual probes.
// Inputs: real git refs (including prefix siblings and symbolic refs) and a
// failed-listing seam. Outputs: identical remote facts and classifications.
// Constraints: no network; existing revision tests remain unchanged.

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/plan"
)

func TestPlanRemoteBatchMatchesIndividualQueries(t *testing.T) {
	for _, refs := range [][]string{nil, {"main"}, {"master", "trunk"}, {"main", "master", "trunk"}, {"main/topic", "master2", "trunkish"}} {
		t.Run(strings.Join(refs, "+"), func(t *testing.T) {
			root := t.TempDir()
			git(t, root, "init", "-q", "-b", "feature")
			git(t, root, "commit", "-q", "--allow-empty", "-m", "fixture")
			for _, name := range refs {
				git(t, root, "update-ref", "refs/remotes/origin/"+name, "HEAD")
			}
			probe := func(args ...string) (string, bool) { return planGit(context.Background(), root, args...) }
			out, ok := probe("for-each-ref", "--format=%(refname)", "refs/remotes/origin/main", "refs/remotes/origin/master", "refs/remotes/origin/trunk")
			var want []string
			for _, name := range []string{"main", "master", "trunk"} {
				if _, ok := probe("rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+name); ok {
					want = append(want, name)
				}
			}
			for _, batchOK := range []bool{ok, false} {
				got := planRemoteNames(strings.TrimSpace(out), batchOK, probe)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("batch success=%v: names %v, legacy %v", batchOK, got, want)
				}
				for _, detached := range []bool{false, true} {
					for _, tag := range []bool{false, true} {
						a := plan.BranchFacts{Branch: "feature", Detached: detached, ExactTag: tag, Remotes: got}
						b := a
						b.Remotes = want
						ad, ar := plan.ClassifyRevision(a)
						bd, br := plan.ClassifyRevision(b)
						if ad != bd || ar != br {
							t.Fatal("classification differs from individual probes")
						}
					}
				}
			}
		})
	}
}

func TestPlanRemoteBatchSymbolicRefs(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "feature")
	git(t, root, "commit", "-q", "--allow-empty", "-m", "fixture")
	git(t, root, "symbolic-ref", "refs/remotes/origin/main", "refs/heads/feature")
	git(t, root, "symbolic-ref", "refs/remotes/origin/master", "refs/heads/missing")
	probe := func(args ...string) (string, bool) { return planGit(context.Background(), root, args...) }
	out, ok := probe("for-each-ref", "--format=%(refname)", "refs/remotes/origin/main", "refs/remotes/origin/master", "refs/remotes/origin/trunk")
	var want []string
	for _, name := range []string{"main", "master", "trunk"} {
		if _, ok := probe("rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+name); ok {
			want = append(want, name)
		}
	}
	if got := planRemoteNames(strings.TrimSpace(out), ok, probe); !reflect.DeepEqual(got, want) {
		t.Fatalf("symbolic refs = %v, legacy %v", got, want)
	}
}
