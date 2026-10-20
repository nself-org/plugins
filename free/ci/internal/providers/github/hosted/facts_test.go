package hosted_test

// Purpose: tests of the AttemptFacts source for a repository path that is not owner/name (D-1345).
// Inputs:  nodes built with the labels a provider's Describe sets.
// Outputs: pass or fail.
// Constraints: no network; the gitlab provider is linked in so its repository check is registered.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	_ "github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
)

func nodeWith(provider, repo string) model.Capability {
	var c model.Capability
	c.Identity.Labels = []string{"nself.provider=" + provider, "nself.repo=" + repo, "nself.runner=shared"}
	return c
}

// TestFactsNestedRepoByProvider proves a GitLab subgroup project passes the facts source while a GitHub
// node with the same label still needs owner/name.
func TestFactsNestedRepoByProvider(t *testing.T) {
	ctx := context.Background()
	l := protocol.Lease{AttemptID: "att-1", Epoch: 1}
	f, err := hosted.Facts(ctx, nodeWith("gitlab", "grp/sub/widgets"), l)
	if err != nil || f.Repo != "grp/sub/widgets" {
		t.Fatalf("gitlab nested project: %+v %v", f, err)
	}
	if _, err = hosted.Facts(ctx, nodeWith("github-hosted", "grp/sub/widgets"), l); err == nil {
		t.Fatal("a nested path was accepted for a GitHub node")
	}
	if _, err = hosted.Facts(ctx, nodeWith("gitlab", "widgets"), l); err == nil {
		t.Fatal("a one-segment path was accepted for a GitLab node")
	}
	if _, err = hosted.Facts(ctx, nodeWith("github-hosted", "acme/widgets"), l); err != nil {
		t.Fatalf("github repo: %v", err)
	}
}
