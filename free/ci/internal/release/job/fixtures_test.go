package job_test

// Purpose: shared fixtures for the release job tests: a run state whose deps
// passed on a clean, committed tree, and helpers to bend it.

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
)

const revision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func digests() model.InputDigests {
	return model.InputDigests{Config: strings.Repeat("c", 64),
		Lockfiles: []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("1", 64)}, {Path: "pnpm-lock.yaml", SHA256: strings.Repeat("2", 64)}},
		Generated: []model.DigestRef{{Path: "gen/api.go", SHA256: strings.Repeat("3", 64)}}}
}

// goodState: build and lint passed on a clean tree at revision, inputs unchanged.
func goodState() job.RunState {
	bound, now := digests(), digests()
	return job.RunState{RunID: "run-1", RepoID: "repo-1", Revision: revision, PolicyDigest: "policy-1", ConfigDigest: bound.Config,
		SourceTrust: "owner", Trigger: "local", Pipeline: model.PipelineRef{ID: "run-1", Name: "release", Kind: "default"},
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"},
		Deps:      []string{"build", "lint"},
		Outcomes:  []job.Dep{{Name: "build", Status: "passed", Required: true}, {Name: "lint", Status: "passed", Required: true}, {Name: "publish", Status: "queued"}},
		Clean:     true, Bound: &bound, Now: &now}
}

func reasonOf(t testing.TB, e *job.Error) string {
	t.Helper()
	if e == nil {
		return ""
	}
	if e.Code != job.CodeNotAdmitted {
		t.Fatalf("code = %s, want E715", e.Code)
	}
	return e.Reason
}
