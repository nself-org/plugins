package main

import (
	"context"
	"os/exec"
	"testing"
)

// planRevision asks git six independent questions concurrently; every answer
// must still land in its own field (D-1008).
func TestPlanRevisionAnswersStayWithTheirQuestion(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "feature"}, {"-c", "user.email=a@b", "-c", "user.name=t", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"},
		{"tag", "v1.0.0"}, {"update-ref", "refs/remotes/origin/main", "HEAD"}, {"update-ref", "refs/remotes/origin/trunk", "HEAD"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	rev := planRevision(context.Background(), repo, "")
	if rev.Branch != "feature" {
		t.Errorf("branch = %q, want feature", rev.Branch)
	}
	if !rev.Release {
		t.Error("an exact tag must mark the revision as a release")
	}
	if rev.DefaultBranch {
		t.Error("feature is not a default branch when origin has main and trunk")
	}
	if !rev.BaseKnown { // origin/main exists, so the merge-base with it is the base
		t.Error("the merge-base with origin/main must give a known base")
	}
}
