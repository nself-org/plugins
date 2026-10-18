package run

// Purpose: install attest.Publish as the Publisher (P7-TRUST-11). The status
// that goes to a code host is built from the published evidence, so it holds
// nothing the publication profile removes.
// Inputs:  a PublishRequest from the finish step.
// Outputs: the status to post (description and target_url scrubbed).
// Constraints: fail closed: a repository whose visibility is unknown is
// treated as public; RepoVisibility is the one seam a host sets (a private
// repository gets the unscrubbed text).

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// RepoVisibility reports the visibility of owner/repo. Nil, or any value other
// than attest.Private, means the publication profile applies.
var RepoVisibility func(owner, repo string) attest.Visibility

func init() { RegisterPublisher(attestPublisher{}) }

type attestPublisher struct{}

func (attestPublisher) Publish(_ context.Context, r PublishRequest) (report.Status, error) {
	vis := attest.Public
	if RepoVisibility != nil {
		vis = RepoVisibility(r.Owner, r.Repo)
	}
	published := attest.Publish(r.Run.Evidence, vis)
	r.Run.Evidence = published
	if len(published.Checks) > 0 {
		r.Projection = model.Projection(published)
	}
	s := defaultStatus(r)
	p := attest.ProfileFor(attest.RepoFacts{Visibility: vis, RepoID: published.RepoID})
	s.Description = report.Truncate(p.Text(s.Description))
	s.TargetURL = p.URL(s.TargetURL)
	return s, nil
}
