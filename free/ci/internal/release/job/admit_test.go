package job_test

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
)

// TestReleaseAdmissionDepsStale: evidence whose lockfile changed since it was
// recorded is refused with E715 deps_stale (the named test of the Ticket).
func TestReleaseAdmissionDepsStale(t *testing.T) {
	s := goodState()
	if e := job.Evaluate(job.BuildFacts(s)); e != nil {
		t.Fatalf("current passing evidence was refused: %v", e)
	}
	changed := digests()
	changed.Lockfiles[0].SHA256 = strings.Repeat("9", 64)
	s.Now = &changed
	e := job.Evaluate(job.BuildFacts(s))
	if got := reasonOf(t, e); got != job.DepsStale {
		t.Fatalf("reason = %q, want %q (%v)", got, job.DepsStale, e)
	}
	if !strings.Contains(e.Message, string(attest.ReasonStale)) {
		t.Fatalf("the message hides the attest reason: %s", e.Message)
	}
}

// TestReleaseAdmissionReasons: every way the deps evidence falls short has its
// own E715 reason, and nothing else is admitted.
func TestReleaseAdmissionReasons(t *testing.T) {
	cases := []struct {
		name string
		bend func(*job.RunState)
		want string
	}{
		{"current and passing", func(*job.RunState) {}, ""},
		{"a dep failed", func(s *job.RunState) { s.Outcomes[0].Status = "failed" }, job.DepsFailing},
		{"a dep was skipped", func(s *job.RunState) { s.Outcomes[1].Status = "skipped" }, job.DepsFailing},
		{"a dep was cancelled", func(s *job.RunState) { s.Outcomes[1].Status = "cancelled" }, job.DepsFailing},
		{"config changed", func(s *job.RunState) { s.Now.Config = strings.Repeat("d", 64) }, job.DepsStale},
		{"lockfile added", func(s *job.RunState) {
			s.Now.Lockfiles = append(s.Now.Lockfiles, model.DigestRef{Path: "Cargo.lock", SHA256: strings.Repeat("4", 64)})
		}, job.DepsStale},
		{"no digester answered", func(s *job.RunState) { s.Now = nil }, job.DepsStale},
		{"dirty worktree", func(s *job.RunState) { s.Clean = false }, job.DepsUnbound},
		{"dep removed in the revision", func(s *job.RunState) { s.Outcomes = s.Outcomes[1:] }, job.DepsJobsMismatch},
		{"no outcomes at all", func(s *job.RunState) { s.Outcomes = nil }, job.DepsJobsMismatch},
		{"release job without deps", func(s *job.RunState) { s.Deps = nil }, job.DepsJobsMismatch},
		{"empty dep name", func(s *job.RunState) { s.Deps = []string{""} }, job.DepsJobsMismatch},
		{"ad hoc pipeline", func(s *job.RunState) { s.Pipeline.Kind = "adhoc" }, job.Adhoc},
		{"drill pipeline", func(s *job.RunState) { s.Pipeline.Kind = "drill" }, job.Adhoc},
		{"named pipeline", func(s *job.RunState) { s.Pipeline.Kind = "named" }, ""},
		{"internal source", func(s *job.RunState) { s.SourceTrust = "internal" }, ""},
		{"collaborator source", func(s *job.RunState) { s.SourceTrust = "collaborator" }, job.Trust},
		{"untrusted source", func(s *job.RunState) { s.SourceTrust = "untrusted" }, job.Trust},
		{"unknown source", func(s *job.RunState) { s.SourceTrust = "stranger" }, job.DepsInvalid},
		{"affected selection", func(s *job.RunState) { s.Selection.Mode = "affected" }, job.SelectionPartial},
		{"no revision", func(s *job.RunState) { s.Revision = "" }, job.DepsInvalid},
		{"another policy", func(s *job.RunState) { s.PolicyDigest = "" }, job.DepsMismatch},
		{"no repo", func(s *job.RunState) { s.RepoID = "" }, job.DepsMismatch},
	}
	for _, c := range cases {
		s := goodState()
		c.bend(&s)
		if got := reasonOf(t, job.Evaluate(job.BuildFacts(s))); got != c.want {
			t.Errorf("%s: reason %q, want %q", c.name, got, c.want)
		}
	}
}

// TestGateIsBuiltFromTheRunNotTheEvidence: bending the evidence cannot move the
// gate, and the gate is the release-admission one.
func TestGateIsBuiltFromTheRunNotTheEvidence(t *testing.T) {
	f := job.BuildFacts(goodState())
	f.Evidence.RepoID, f.Evidence.PolicyDigest = "other", "other"
	g := job.Gate(f)
	if g.RepoID != "repo-1" || g.PolicyDigest != "policy-1" || g.Revision != revision {
		t.Fatalf("gate identity came from the evidence: %+v", g)
	}
	if g.Class != attest.ClassDefault || g.MinTrust != "internal" || g.AllowSelfReported || !g.FullSelection || g.Current == nil {
		t.Fatalf("not the release-admission gate: %+v", g)
	}
	if len(g.RequiredJobs) != 2 || g.RequiredJobs[0] != "build" || g.RequiredJobs[1] != "lint" {
		t.Fatalf("required jobs = %v, want the deps in order", g.RequiredJobs)
	}
	if e := job.Evaluate(f); reasonOf(t, e) != job.DepsMismatch {
		t.Fatalf("evidence of another repository was admitted: %v", e)
	}
	deps := []string{"z", "a"}
	job.Gate(job.Facts{Deps: deps})
	if deps[0] != "z" {
		t.Fatal("Gate sorted the caller's slice")
	}
}

// TestEvidenceOfAnotherRunIdentityIsRefused: the gate's identity is the run's,
// so evidence of another revision, repository or policy is a deps_mismatch.
func TestEvidenceOfAnotherRunIdentityIsRefused(t *testing.T) {
	for name, bend := range map[string]func(*job.Facts){
		"revision": func(f *job.Facts) { f.Revision = strings.Repeat("b", 40) },
		"repo":     func(f *job.Facts) { f.RepoID = "repo-2" },
		"policy":   func(f *job.Facts) { f.PolicyDigest = "policy-2" },
	} {
		f := job.BuildFacts(goodState())
		bend(&f)
		if got := reasonOf(t, job.Evaluate(f)); got != job.DepsMismatch {
			t.Errorf("%s: reason %q, want %q", name, got, job.DepsMismatch)
		}
	}
}

// TestSelfReportedAndSignedEvidenceAreRefused: only coordinator-recorded
// evidence admits a release job.
func TestSelfReportedAndSignedEvidenceAreRefused(t *testing.T) {
	f := job.BuildFacts(goodState())
	f.Evidence.Producer.Kind = "self-reported"
	if got := reasonOf(t, job.Evaluate(f)); got != job.SelfReported {
		t.Fatalf("self-reported: reason %q", got)
	}
	f = job.BuildFacts(goodState())
	f.Evidence.Producer.Kind = "signed-remote"
	f.Evidence.Signature = model.Signature{Kind: "runner-ed25519"}
	if got := reasonOf(t, job.Evaluate(f)); got != job.DepsInvalid {
		t.Fatalf("an unverifiable signed document: reason %q", got)
	}
	f = job.BuildFacts(goodState())
	f.Evidence.Schema = "ci.evidence/v9"
	if got := reasonOf(t, job.Evaluate(f)); got != job.DepsInvalid {
		t.Fatalf("a malformed document: reason %q", got)
	}
}

func TestStepRefusesOnlyReleaseAttemptsItHasFactsFor(t *testing.T) {
	st := job.NewStash()
	step := job.Step(st)
	view := func(id string) dispatch.View { return dispatch.View{Request: dispatch.Request{AttemptID: id}} }
	if err := step(context.Background(), view("other")); err != nil {
		t.Fatalf("an attempt with no facts is not a release attempt: %v", err)
	}
	st.Put("good", job.BuildFacts(goodState()))
	bad := goodState()
	bad.Outcomes[0].Status = "failed"
	st.Put("bad", job.BuildFacts(bad))
	if err := step(context.Background(), view("good")); err != nil {
		t.Fatalf("good facts refused: %v", err)
	}
	err := step(context.Background(), view("bad"))
	ref, ok := err.(*dispatch.Refusal)
	if !ok || ref.Reason != "release."+job.DepsFailing || !strings.HasPrefix(ref.Detail, job.CodeNotAdmitted) {
		t.Fatalf("refusal = %#v", err)
	}
	st.Forget("bad")
	if err = step(context.Background(), view("bad")); err != nil {
		t.Fatalf("forgotten facts still refuse: %v", err)
	}
	if _, ok := st.ReleaseFacts("good"); !ok {
		t.Fatal("Forget removed another attempt")
	}
}

func TestRegisterAddsTheStepOnce(t *testing.T) {
	if err := job.Register(job.NewStash()); err != nil {
		t.Fatal(err)
	}
	if err := job.Register(job.NewStash()); err == nil {
		t.Fatal("a second registration of the release step was accepted")
	}
}

// TestPromotableIsNeverUsedForAdmission: the package does not import
// internal/authz, so authz.Promotable cannot be called (it refuses non-release
// evidence by design; D16 uses it on the published release).
func TestPromotableIsNeverUsedForAdmission(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no source files (%v)", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range af.Imports {
			if strings.Contains(imp.Path.Value, "internal/authz") {
				t.Errorf("%s imports %s", f, imp.Path.Value)
			}
		}
		if strings.Contains(string(src), "Promotable(") && f != "admit.go" {
			t.Errorf("%s mentions Promotable(", f)
		}
	}
}
