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
	"os"
	"os/user"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// RepoVisibility reports the visibility of owner/repo. Nil, or any value other
// than attest.Private, means the publication profile applies.
var RepoVisibility func(owner, repo string) attest.Visibility

func init() { RegisterPublisher(attestPublisher{}) }

var (
	localOnce            sync.Once
	localHosts, localUsr []string
)

// localNames are this machine's host name and account name: literals the
// publication profile must remove from log text even when they carry no
// private suffix. Failures to read them leave the lists short, never fail a run.
func localNames() (hosts, users []string) {
	localOnce.Do(func() {
		if h, err := os.Hostname(); err == nil && h != "" {
			localHosts = append(localHosts, h)
			if short, _, ok := strings.Cut(h, "."); ok && short != "" {
				localHosts = append(localHosts, short)
			}
		}
		if u, err := user.Current(); err == nil && u.Username != "" {
			localUsr = append(localUsr, u.Username)
		}
	})
	return localHosts, localUsr
}

type attestPublisher struct{}

func (attestPublisher) Publish(_ context.Context, r PublishRequest) (report.Status, error) {
	vis := attest.Public
	if RepoVisibility != nil {
		vis = RepoVisibility(r.Owner, r.Repo)
	}
	hosts, users := localNames()
	p := attest.ProfileFor(attest.RepoFacts{Visibility: vis, RepoID: r.Run.Evidence.RepoID, Hostnames: hosts, Usernames: users})
	published := attest.PublishWith(r.Run.Evidence, p)
	r.Run.Evidence = published
	if len(published.Checks) > 0 {
		r.Projection = model.Projection(published)
	}
	s := defaultStatus(r)
	// the description ends with the digest of the unpublished evidence: a reader
	// cannot rebuild it from the published document, which is why it is only a pointer
	s.Description = report.Truncate(p.Text(s.Description))
	s.TargetURL = p.URL(s.TargetURL)
	return s, nil
}
