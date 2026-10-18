package run

// D-0970: the join between the coordinator and the attest Publisher. The
// run tests cover postStatus with a liar Publisher and the attest tests cover
// the Publisher alone; these drive postStatus with the real Publisher
// installed and read what the reporter was handed.

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// leakyJobs has an allowed failure whose id is a private address; the status
// summary lists allowed failures by id.
func leakyJobs() map[string]model.Job {
	probe := sh("exit 1")
	probe.Required, probe.AllowFailure = no(), true
	return map[string]model.Job{"a": sh("true"), "probe-10.0.0.7": probe}
}

// RepoVisibility is a package seam: the join tests run serially.
func postedThroughAttest(t *testing.T, vis func(owner, repo string) attest.Visibility) report.Status {
	t.Helper()
	old := RepoVisibility
	RepoVisibility = vis
	t.Cleanup(func() { RepoVisibility = old })
	r := newRig(t, leakyJobs(), gitFake{})
	r.C.Hooks.Publisher = attestPublisher{}
	if res := r.run(t); res.Exit != 0 {
		t.Fatalf("exit %d\n%s", res.Exit, r.Out)
	}
	p := r.Rep.all()
	if len(p) != 1 {
		t.Fatalf("posted %+v\n%s", p, r.Out)
	}
	return p[0]
}

// The coordinator hands the attest Publisher's scrubbed status to the
// reporter: a public (or unknown) repository gets no private address, the
// state stays success, the description keeps the evidence digest suffix and
// the target_url is empty.
func TestAttestPublisherStatusDeliveredScrubbed(t *testing.T) {
	for name, vis := range map[string]func(string, string) attest.Visibility{
		"unset":   nil,
		"unknown": func(string, string) attest.Visibility { return "" },
		"public":  func(string, string) attest.Visibility { return attest.Public },
	} {
		s := postedThroughAttest(t, vis)
		if strings.Contains(s.Description, "10.0.0.7") {
			t.Fatalf("%s: delivered description leaks: %q", name, s.Description)
		}
		if s.State != report.Success || s.TargetURL != "" || s.Owner != "acme" || s.Repo != "widgets" || s.Revision != testSHA {
			t.Fatalf("%s: delivered status %+v", name, s)
		}
		if !strings.Contains(s.Description, "allowed failures:") || !strings.Contains(s.Description, " (evidence ") {
			t.Fatalf("%s: description lost its shape: %q", name, s.Description)
		}
	}
}

// A private repository gets the unscrubbed text through the same join.
func TestAttestPublisherStatusDeliveredPrivateKeepsText(t *testing.T) {
	s := postedThroughAttest(t, func(string, string) attest.Visibility { return attest.Private })
	if !strings.Contains(s.Description, "probe-10.0.0.7") || s.State != report.Success {
		t.Fatalf("delivered status %+v", s)
	}
}
