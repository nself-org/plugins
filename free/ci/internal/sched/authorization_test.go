package sched

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestReviewProjectAuthorizationDerived(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Job, *Runner)
	}{
		{"discovered", func(_ *Job, r *Runner) { r.Capability.Lifecycle.Discovered = true }},
		{"other-project", func(j *Job, _ *Runner) { j.Project = "other" }},
		{"empty-project", func(j *Job, _ *Runner) { j.Project = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, j := fixtureRunner("r", "lan"), fixtureJob("j")
			tc.edit(&j, &r)
			r.Capability.Lifecycle.Eligible = true
			got := Place(fixtureWorld(r), []Job{j})
			if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.NodeDiscovered)) {
				t.Fatalf("unauthorized project placed: %+v", got)
			}
		})
	}
	r, j := fixtureRunner("r", "lan"), fixtureJob("j")
	r.Capability.Lifecycle.Eligible = false // Stored status is last-known, not authority.
	if got := Place(fixtureWorld(r), []Job{j}); len(got.Placements) != 1 {
		t.Fatalf("authorized project refused on stale status: %+v", got)
	}
}

func TestReviewMissingCapabilityFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Runner)
	}{
		{"zero-value", func(r *Runner) { r.Capability = model.Capability{} }},
		{"missing-schema-only", func(r *Runner) { r.Capability.Schema = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := fixtureRunner("r", "lan")
			tc.edit(&r)
			got := Place(fixtureWorld(r), []Job{fixtureJob("j")})
			if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.NodeDiscovered)) {
				t.Fatalf("missing capability placed: %+v", got)
			}
		})
	}
}

func TestReviewHostedGrantRequired(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Runner)
	}{
		{"location", func(r *Runner) { r.Decl.Hosted = false }},
		{"declaration", func(r *Runner) {
			r.Location = "lan"
			r.Capability.Location.Kind.Value = ptr("lan")
			r.Decl.Hosted = true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := fixtureRunner("r", "hosted")
			r.Capability.Lifecycle.AuthorizedProjects = nil
			tc.edit(&r)
			got := Place(fixtureWorld(r), []Job{fixtureJob("j")})
			if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, "privacy.hosted_not_authorized") {
				t.Fatalf("hosted runner without a grant placed: %+v", got)
			}
		})
	}
}

func TestReviewProtocolRequiresV1AndCompatibleMinimum(t *testing.T) {
	for _, tc := range []struct {
		name     string
		min      int
		versions []int
	}{
		{"v1-absent-min-zero", 0, []int{2}},
		{"v1-absent-min-one", 1, []int{2}},
		{"min-two-with-v1", 2, []int{1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := fixtureRunner("r", "lan")
			r.Capability.Protocol.Min, r.Capability.Protocol.Versions = tc.min, tc.versions
			got := Place(fixtureWorld(r), []Job{fixtureJob("j")})
			if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.NodeProtocol)) {
				t.Fatalf("unsupported protocol placed: %+v", got)
			}
		})
	}
}
