package attest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/run"
)

func publishRequest(checkID string) run.PublishRequest {
	ev := model.Evidence{Schema: "ci.evidence/v1", RepoID: "r1", Runner: model.RunnerRef{ID: "build-box.local", Provider: "local"},
		Checks: []model.Check{
			{ID: "ok", JobID: "a", Substantive: true, Required: true, Result: "pass"},
			{ID: checkID, JobID: "b", AllowFailure: true, Result: "fail"},
		}}
	return run.PublishRequest{Owner: "o", Repo: "r", Projection: model.Projection(ev),
		Run: run.RunInfo{Revision: strings.Repeat("a", 40), EvidenceDigest: strings.Repeat("f", 64), Result: "pass", Evidence: ev}}
}

func TestAttestPublisherIsInstalled(t *testing.T) {
	if run.Registered.Publisher == nil || (run.Hooks{}).Resolved().Publisher == nil {
		t.Fatal("importing run does not install the Publisher")
	}
}

// The status description is built from the published projection: a public
// repository (or one of unknown visibility) never posts a private address.
func TestAttestPublisherScrubsPublicStatus(t *testing.T) {
	t.Cleanup(func() { run.RepoVisibility = nil })
	req := publishRequest("probe-10.0.0.7-/Users/ali/x")
	for _, vis := range []func(string, string) attest.Visibility{nil, func(string, string) attest.Visibility { return "" }, func(string, string) attest.Visibility { return attest.Public }} {
		run.RepoVisibility = vis
		s, err := run.Registered.Publisher.Publish(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(s.Description, "10.0.0.7") || strings.Contains(s.Description, "/Users/ali") {
			t.Fatalf("status leaks: %q", s.Description)
		}
		if !strings.Contains(s.Description, "(evidence ffffffffffff)") || s.State != report.Success || s.TargetURL != "" {
			t.Fatalf("status %+v", s)
		}
	}
}

func TestAttestPublisherLeavesPrivateRepoText(t *testing.T) {
	t.Cleanup(func() { run.RepoVisibility = nil })
	run.RepoVisibility = func(string, string) attest.Visibility { return attest.Private }
	s, err := run.Registered.Publisher.Publish(context.Background(), publishRequest("probe-10.0.0.7"))
	if err != nil || !strings.Contains(s.Description, "probe-10.0.0.7") {
		t.Fatalf("%q %v", s.Description, err)
	}
}

func TestAttestPublisherDropsPrivateTargetURL(t *testing.T) {
	// defaultStatus never sets a target_url; the hook still filters one a later change adds
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public})
	if p.URL("http://192.168.1.2/run/1") != "" || p.URL("https://github.com/o/r/actions/runs/1") == "" {
		t.Fatal("target_url filter")
	}
}
