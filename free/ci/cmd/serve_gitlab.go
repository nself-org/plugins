package main

// Purpose: the GitLab-provider wiring of the V15 coordinator start (D-1346): it points the GitLab pipeline
//          provider at the coordinator's store, so the intent of a running pipeline outlives a restart, and
//          gives the GitLab commit status the phase of the run, so a status reaches running and canceled.
// Inputs:  the open coordinator store.
// Outputs: serveGitLab installs pipeline.UseStore and status.UsePhase; the returned func removes them.
//          The refetch of trigger facts needs no call here: the GitLab webhook package installs its own
//          source with pipeline.Default().UseTrustFacts when it loads.
// Constraints: call once at coordinator start, before the first dispatch, next to serveHosted. The phase
//          comes from the run row alone (running -> running, cancelled -> canceled); a run the store does
//          not know, or a read that fails, refines nothing, so a status is never made greener than it is.

import (
	"context"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	glstatus "github.com/nself-org/plugins/free/ci/internal/providers/gitlab/status"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// gitlabPhaseTimeout bounds the store read of one status post.
const gitlabPhaseTimeout = 5 * time.Second

// serveGitLab installs the GitLab-provider wiring and returns the func that removes it.
func serveGitLab(st *store.Store) (undo func()) {
	pipeline.UseStore(st)
	glstatus.UsePhase(gitlabPhaseFrom(st))
	return func() {
		glstatus.UsePhase(nil)
		pipeline.ReleaseStore()
	}
}

// gitlabPhaseFrom reads the phase of a status from the newest run of its project and revision.
func gitlabPhaseFrom(st *store.Store) func(report.Status) glstatus.Phase {
	return func(s report.Status) glstatus.Phase {
		ctx, cancel := context.WithTimeout(context.Background(), gitlabPhaseTimeout)
		defer cancel()
		runs, err := st.PipelinesByRevision(ctx, s.Revision) // newest first
		if err != nil {
			return glstatus.PhaseNone
		}
		project := s.Owner + "/" + s.Repo
		for _, r := range runs {
			if !strings.EqualFold(r.Project, project) {
				continue
			}
			switch r.Status {
			case "running":
				return glstatus.PhaseRunning
			case "cancelled":
				return glstatus.PhaseCanceled
			}
			return glstatus.PhaseNone
		}
		return glstatus.PhaseNone
	}
}
