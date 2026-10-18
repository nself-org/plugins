package attest_test

// Purpose: shared fixtures for the attest tests: one good evidence document,
// the gate it satisfies and helpers to bend either.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const goodRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func strp(s string) *string { return &s }

// goodEvidence is bound, clean, passing, full selection, coordinator-recorded.
func goodEvidence() model.Evidence {
	checks := []model.Check{
		{ID: "vet", JobID: "build", Kind: "quality", Tool: "go", Required: true, Substantive: true, Result: "pass"},
		{ID: "test", JobID: "build", Kind: "test", Tool: "go", Required: true, Substantive: true, Result: "pass"},
		{ID: "lint", JobID: "lint", Kind: "quality", Tool: "golangci-lint", Required: true, Substantive: true, Result: "pass"},
	}
	return model.Evidence{
		Schema: "ci.evidence/v1", Revision: goodRevision, RepoID: "repo-1", RunID: "run-1",
		Pipeline: model.PipelineRef{ID: "p1", Name: "default", Kind: "default"}, Trigger: "local", SourceTrust: "owner",
		PolicyDigest: "policy-1", WorktreeState: "clean", Binding: model.Binding{State: "bound"},
		InputDigests: model.InputDigests{Config: strings.Repeat("c", 64),
			Lockfiles: []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("1", 64)}, {Path: "pnpm-lock.yaml", SHA256: strings.Repeat("2", 64)}},
			Generated: []model.DigestRef{{Path: "gen/api.go", SHA256: strings.Repeat("3", 64)}}},
		Runner:       model.RunnerRef{ID: "mac-mini.local", Provider: "local", Platform: "darwin/arm64", Isolation: "process"},
		Producer:     model.Producer{Kind: "coordinator-recorded", Version: "1"},
		ToolVersions: map[string]string{"go": "1.27.2"}, Checks: checks, Artifacts: []model.Artifact{}, Retries: []model.Retry{},
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"},
		Result:    "pass", Counts: model.Counts{Pass: 3, Total: 3},
		Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
	}
}

// goodGate accepts goodEvidence.
func goodGate(ev model.Evidence) attest.Gate {
	cur := ev.InputDigests
	return attest.Gate{RepoID: ev.RepoID, Revision: ev.Revision, PolicyDigest: ev.PolicyDigest, RequiredJobs: []string{"build", "lint"},
		MinTrust: "internal", Class: attest.ClassDefault, Current: &cur}
}

func marshal(t testing.TB, ev model.Evidence) []byte {
	t.Helper()
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func hasReason(rs []attest.Reason, want attest.Reason) bool {
	for _, r := range rs {
		if r == want {
			return true
		}
	}
	return false
}

func sameReasons(got, want []attest.Reason) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

type fakeVerifier struct {
	err    error
	panics bool
	calls  int
}

func (f *fakeVerifier) VerifyEvidence([]byte, model.Evidence) error {
	f.calls++
	if f.panics {
		panic("boom")
	}
	return f.err
}

// signedRemote turns ev into a signed-remote document.
func signedRemote(ev model.Evidence) model.Evidence {
	ev.Producer.Kind = "signed-remote"
	ev.Signature = model.Signature{Kind: "runner-ed25519", KeyID: strp("ci-node-0123456789abcdef"), Value: strp("c2ln")}
	return ev
}
