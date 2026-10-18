package authz_test

// Purpose: shared fixtures for the authz tests: the traversal corpus loader,
// namespace builders and a good evidence document with the gate it satisfies.

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type corpusEntry struct {
	Name   string `json:"name"`
	Link   string `json:"link"`
	Mode   string `json:"mode"`
	Size   int64  `json:"size"`
	Packed int64  `json:"packed"`
}

type corpusCase struct {
	ID      string        `json:"id"`
	Reason  string        `json:"reason"`
	Entries []corpusEntry `json:"entries"`
}

func (c corpusEntry) entry() authz.Entry {
	e := authz.Entry{Name: c.Name, LinkTarget: c.Link, Size: c.Size, Packed: c.Packed}
	switch c.Mode {
	case "file":
		e.Mode = 0o644
	case "dir":
		e.Mode = fs.ModeDir | 0o755
	case "symlink":
		e.Mode = fs.ModeSymlink | 0o777
	case "hardlink":
		e.Mode, e.Hardlink = 0o644, true
	case "fifo":
		e.Mode = fs.ModeNamedPipe | 0o644
	case "device":
		e.Mode = fs.ModeDevice | 0o644
	case "chardev":
		e.Mode = fs.ModeDevice | fs.ModeCharDevice | 0o644
	case "socket":
		e.Mode = fs.ModeSocket | 0o644
	case "setuid":
		e.Mode = fs.ModeSetuid | 0o755
	default:
		panic("corpus: unknown mode " + c.Mode)
	}
	return e
}

func loadCorpus(t testing.TB) []corpusCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "traversal", "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []corpusCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) == 0 {
		t.Fatal("empty corpus")
	}
	return doc.Cases
}

// trustedClasses are the classes that may be a snapshot reader or source.
var trustedClasses = []model.TrustClass{"collaborator", "internal", "owner"}

func mustNS(t testing.TB, class model.TrustClass, prot authz.Protection, head, pr string) authz.Namespace {
	t.Helper()
	ns, err := authz.CacheNamespace("proj-1", class, prot, head, pr)
	if err != nil {
		t.Fatal(err)
	}
	return ns
}

// finalJob is a job at the point a save may commit.
func finalJob(ns authz.Namespace) authz.Job {
	return authz.Job{Namespace: ns, State: authz.StateFinalizing, Passed: true, Capability: authz.Capability{Attempt: "att-1", Epoch: 3}}
}

var currentCap = authz.Capability{Attempt: "att-1", Epoch: 3}

const goodRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// goodEvidence is bound, clean, passing, full selection, coordinator-recorded.
func goodEvidence() model.Evidence {
	checks := []model.Check{
		{ID: "test", JobID: "build", Kind: "test", Tool: "go", Required: true, Substantive: true, Result: "pass"},
		{ID: "lint", JobID: "lint", Kind: "quality", Tool: "golangci-lint", Required: true, Substantive: true, Result: "pass"},
	}
	return model.Evidence{
		Schema: "ci.evidence/v1", Revision: goodRevision, RepoID: "repo-1", RunID: "run-1",
		Pipeline: model.PipelineRef{ID: "p1", Name: "default", Kind: "default"}, Trigger: "local", SourceTrust: "owner",
		PolicyDigest: "policy-1", WorktreeState: "clean", Binding: model.Binding{State: "bound"},
		InputDigests: model.InputDigests{Config: strings.Repeat("c", 64),
			Lockfiles: []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("1", 64)}}},
		Runner:       model.RunnerRef{ID: "mac-mini.local", Provider: "local", Platform: "darwin/arm64", Isolation: "process"},
		Producer:     model.Producer{Kind: "coordinator-recorded", Version: "1"},
		ToolVersions: map[string]string{"go": "1.27.2"}, Checks: checks, Artifacts: []model.Artifact{}, Retries: []model.Retry{},
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"},
		Result:    "pass", Counts: model.Counts{Pass: 2, Total: 2},
		Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
	}
}

// goodGate is the release gate goodEvidence satisfies.
func goodGate(ev model.Evidence) attest.Gate {
	cur := ev.InputDigests
	return attest.Gate{RepoID: ev.RepoID, Revision: ev.Revision, PolicyDigest: ev.PolicyDigest, RequiredJobs: []string{"build", "lint"},
		MinTrust: "internal", Class: attest.ClassRelease, FullSelection: true, Current: &cur}
}
