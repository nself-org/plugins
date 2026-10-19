package job_test

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
)

func TestBuildFactsRecordsTheDepsAsCoordinatorEvidence(t *testing.T) {
	f := job.BuildFacts(goodState())
	ev := f.Evidence
	if ev.Producer.Kind != "coordinator-recorded" || ev.Signature.Kind != "coordinator-recorded" || ev.Result != "pass" {
		t.Fatalf("producer %q signature %q result %q", ev.Producer.Kind, ev.Signature.Kind, ev.Result)
	}
	if len(ev.Checks) != 2 || ev.Counts.Pass != 2 || ev.Counts.Total != 2 || ev.Counts.Fail+ev.Counts.Skip+ev.Counts.Error != 0 {
		t.Fatalf("checks %d counts %+v", len(ev.Checks), ev.Counts)
	}
	for _, c := range ev.Checks {
		if c.JobID != c.ID || !c.Required || !c.Substantive || c.Result != "pass" {
			t.Errorf("check %+v", c)
		}
	}
	if ev.Binding.State != "bound" || ev.WorktreeState != "clean" || ev.Binding.Reason != nil {
		t.Fatalf("binding %+v worktree %q", ev.Binding, ev.WorktreeState)
	}
	if f.Revision != revision || f.RepoID != "repo-1" || f.PolicyDigest != "policy-1" || len(f.Deps) != 2 || f.Current == nil {
		t.Fatalf("facts identity %+v", f)
	}
	if ev.InputDigests.Config != digests().Config || len(ev.InputDigests.Lockfiles) != 2 {
		t.Fatalf("inputs %+v", ev.InputDigests)
	}
}

func TestBuildFactsCountsAndBindsWhatItSaw(t *testing.T) {
	s := goodState()
	s.Deps = []string{"build", "lint", "test", "docs"}
	s.Outcomes = []job.Dep{{Name: "build", Status: "failed"}, {Name: "lint", Status: "PASSED"}, {Name: "test", Status: "skipped"}}
	s.Clean, s.Bound = false, nil
	ev := job.BuildFacts(s).Evidence
	if ev.Counts.Pass != 1 || ev.Counts.Fail != 1 || ev.Counts.Skip != 1 || ev.Counts.Total != 3 {
		t.Fatalf("counts %+v", ev.Counts)
	}
	if ev.Result != "fail" {
		t.Fatalf("result %q", ev.Result)
	}
	if ev.Binding.State != "unbound" || ev.Binding.Reason == nil || *ev.Binding.Reason != "dirty" || ev.WorktreeState != "dirty" {
		t.Fatalf("binding %+v", ev.Binding)
	}
	if ev.InputDigests.Config != s.ConfigDigest || ev.InputDigests.Lockfiles == nil || ev.InputDigests.Generated == nil {
		t.Fatalf("without a bound digest the config digest alone is recorded: %+v", ev.InputDigests)
	}
	e := job.Evaluate(job.BuildFacts(s))
	if e == nil || e.Reason != job.DepsMismatch && e.Reason != job.DepsFailing && e.Reason != job.DepsJobsMismatch {
		t.Fatalf("refusal %v", e)
	}
	if got := attest.ReasonUnbound; e.Message == "" || !strings.Contains(e.Message, string(got)) {
		t.Fatalf("every reason must be listed, got %q", e.Message)
	}
}

// TestBuildFactsKeepsTheConfigTheRunUsed: the digest of the config on the run's
// row is the evidence's, so a run that used another config than the tree's is stale.
func TestBuildFactsKeepsTheConfigTheRunUsed(t *testing.T) {
	s := goodState()
	s.ConfigDigest = strings.Repeat("e", 64) // --config pointed outside the tree
	f := job.BuildFacts(s)
	if f.Evidence.InputDigests.Config != s.ConfigDigest {
		t.Fatalf("the run's config digest was replaced: %s", f.Evidence.InputDigests.Config)
	}
	if got := reasonOf(t, job.Evaluate(f)); got != job.DepsStale {
		t.Fatalf("reason %q, want %q", got, job.DepsStale)
	}
	s.Bound = nil
	s.ConfigDigest = ""
	if f = job.BuildFacts(s); f.Evidence.InputDigests.Config != "" || f.Evidence.InputDigests.Lockfiles == nil {
		t.Fatalf("no digests at all: %+v", f.Evidence.InputDigests)
	}
}
