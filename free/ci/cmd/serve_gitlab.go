package main

// Purpose: the GitLab wiring of the V15 coordinator start (D-1346), the GitLab counterpart of serveHosted: it
//          keeps the GitLab dispatch journal in the coordinator's store and lets the commit-status reporter say
//          running and canceled from the run the store holds.
// Inputs:  the open coordinator store.
// Outputs: serveGitLab installs pipeline.UseStore and status.UsePhase; the returned func removes them. The
//          trust-fact refetch (pipeline.Default().UseTrustFacts) is installed by the webhook package's init(),
//          linked in providers_gitlab_webhook.go, so it needs no call here.
// Constraints: call once at coordinator start, before the first dispatch, next to serveHosted. A status whose
//          run the store cannot show (another project, a read error) is refined by nothing: it stays pending or
//          failed, never running or canceled by guess. The hook reads with its own short timeout (the reporter
//          passes no context to it).

import (
	"context"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/status"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// gitlabPhaseTimeout bounds one projection read made while a status is being posted.
const gitlabPhaseTimeout = 5 * time.Second

// serveGitLab installs the GitLab wiring and returns the func that removes it.
func serveGitLab(st *store.Store) (undo func()) {
	pipeline.UseStore(st)
	status.UsePhase(gitlabPhase(st))
	return func() {
		status.UsePhase(nil)
		pipeline.ReleaseStore()
	}
}

// gitlabPhase refines a status with the newest run the store holds for the status's project and revision:
// a pending status is running while an attempt of the run runs, a failed one is canceled when the run was
// cancelled (and nothing in it failed on its own).
func gitlabPhase(st *store.Store) func(report.Status) status.Phase {
	return func(s report.Status) status.Phase {
		ctx, cancel := context.WithTimeout(context.Background(), gitlabPhaseTimeout)
		defer cancel()
		runs, err := st.PipelinesByRevision(ctx, s.Revision) // newest first
		if err != nil {
			return status.PhaseNone
		}
		for _, run := range runs {
			if !strings.EqualFold(run.Project, s.Owner+"/"+s.Repo) {
				continue
			}
			attempts, err := st.PipelineAttempts(ctx, run.ID)
			if err != nil {
				return status.PhaseNone
			}
			var running, cancelled, failed bool
			for _, a := range attempts {
				switch string(a.State) {
				case "running", "finalizing":
					running = true
				case "cancelled":
					cancelled = true
				case "failed":
					failed = true
				}
			}
			switch {
			case s.State == report.Pending && running:
				return status.PhaseRunning
			case (s.State == report.Failure || s.State == report.Error) && (run.Status == "cancelled" || cancelled && !failed):
				return status.PhaseCanceled
			}
			return status.PhaseNone
		}
		return status.PhaseNone
	}
}
