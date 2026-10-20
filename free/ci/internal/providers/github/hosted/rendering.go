package hosted

// Purpose: the coordinator's own rendering of the dispatch workflow, which hostedev compares byte
//          for byte with the workflow the run used (Epic D5).
// Inputs:  the template version the run's workflow declares and the workflow bytes (read only for the
//          agent commit it names).
// Outputs: the bytes this build would generate for that version and commit.
// Constraints: only a source-mode workflow is verified by default, and only when its agent commit is
//          on the allowlist templates/github/agent_pins.yaml (a commit outside it is rendered with the
//          newest allowed commit, so the bytes differ and the run is refused as workflow.edited); a
//          release-mode workflow needs pins this build does not hold until the first signed agent
//          release exists, so it is refused as template.unknown unless Config.Rendering supplies them.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

// DefaultRendering renders the pinned source-mode workflow of version.
func DefaultRendering(_ context.Context, version int, workflow []byte) ([]byte, error) {
	commits, err := githubtmpl.SourceCommits()
	if err != nil || len(commits) == 0 {
		return nil, notEvidence(hostedev.ReasonTemplate)
	}
	declared := githubtmpl.AgentCommitOf(workflow)
	if declared == "" {
		return nil, notEvidence(hostedev.ReasonTemplate)
	}
	use := commits[len(commits)-1]
	for _, c := range commits {
		if c == declared {
			use = c
		}
	}
	return githubtmpl.Render(githubtmpl.Options{Version: version, AgentSource: githubtmpl.SourceMode, AgentCommit: use})
}
