package pipeline

// Purpose: the coordinator's own rendering of the include file, which hostedev compares byte for byte
//          with the file at the pipeline's commit (Epic D11).
// Inputs:  the template version the file declares and the file bytes (read only for the agent commit
//          it names).
// Outputs: the bytes this build would generate for that version and commit.
// Constraints: the agent commit must be on the allowlist templates/gitlab/agent_pins.yaml (a commit
//          outside it is rendered with the newest allowed commit, so the bytes differ and the pipeline
//          is refused as workflow.edited); a file that names no commit is refused as template.unknown.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

// DefaultRendering renders the pinned include file of version.
func DefaultRendering(_ context.Context, version int, file []byte) ([]byte, error) {
	commits, err := gitlabtmpl.SourceCommits()
	if err != nil || len(commits) == 0 {
		return nil, api.NotEvidence(hostedev.ReasonTemplate)
	}
	declared := gitlabtmpl.AgentCommitOf(file)
	if declared == "" {
		return nil, api.NotEvidence(hostedev.ReasonTemplate)
	}
	use := commits[len(commits)-1]
	for _, c := range commits {
		if c == declared {
			use = c
		}
	}
	return gitlabtmpl.Render(gitlabtmpl.Options{Version: version, AgentCommit: use})
}
