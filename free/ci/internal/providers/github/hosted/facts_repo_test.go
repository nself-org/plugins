package hosted_test

// Purpose: the one AttemptFacts source accepts the repository shape of the node's own provider (D-1345):
// a nested GitLab project path for a gitlab node, only owner/name for a github-hosted node.
// Constraints: no network; the gitlab check is registered by the gitlab pipeline package.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

func TestFactsAcceptTheRepoShapeOfTheNodeProvider(t *testing.T) {
	runtime.RegisterRepoCheck("test-nested", func(r string) bool { return r == "group/sub/project" })
	t.Cleanup(func() { runtime.RegisterRepoCheck("test-nested", nil) })
	node := func(provider, repo string) model.Capability {
		var c model.Capability
		c.Identity.Provider = provider
		c.Identity.Labels = []string{hosted.LabelRepo + repo, hosted.LabelRunner + "tag"}
		return c
	}
	cases := []struct {
		provider, repo string
		ok             bool
	}{
		{"test-nested", "group/sub/project", true},
		{"test-nested", "acme/widgets", false},    // the provider's own rule decides
		{hosted.ProviderID, "acme/widgets", true}, // GitHub: owner/name
		{hosted.ProviderID, "group/sub/project", false},
		{"unregistered", "group/sub/project", false}, // no check: held to the GitHub shape
		{"unregistered", "acme/widgets", true},
	}
	for _, c := range cases {
		f, err := hosted.Facts(context.Background(), node(c.provider, c.repo), protocol.Lease{})
		if (err == nil) != c.ok || (c.ok && f.Repo != c.repo) {
			t.Errorf("%s %q: facts %+v, err %v, want ok=%v", c.provider, c.repo, f, err, c.ok)
		}
	}
}
