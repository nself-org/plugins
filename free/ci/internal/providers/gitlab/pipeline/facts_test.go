package pipeline_test

// Purpose: the one AttemptFacts source of the transport driver accepts a nested GitLab project path on a
//          gitlab node (D-1345), through the repository check this package registers.
// Inputs:  a node labelled the way Describe labels one.
// Outputs: pass or fail.
// Constraints: no network.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
)

func TestFactsAcceptANestedGitLabProject(t *testing.T) {
	node := func(repo string) model.Capability {
		var c model.Capability
		c.Identity.Provider = pipeline.ProviderID
		c.Identity.Labels = []string{pipeline.LabelRepo + repo, pipeline.LabelRunner + "tag"}
		return c
	}
	for repo, ok := range map[string]bool{"group/sub/project": true, "group/project": true, "project": false, "g//p": false, "a/b/../c": false} {
		f, err := hosted.Facts(context.Background(), node(repo), protocol.Lease{})
		if (err == nil) != ok || (ok && f.Repo != repo) {
			t.Errorf("repo %q: facts %+v, err %v, want ok=%v", repo, f, err, ok)
		}
	}
}
