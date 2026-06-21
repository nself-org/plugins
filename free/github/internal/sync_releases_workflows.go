package internal

import (
	"context"
	"fmt"
)

func (s *SyncService) syncReleases(ctx context.Context, owner, repo string, repoID int64, result *SyncResult) {
	releases, err := s.client.ListReleases(owner, repo)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("releases %s/%s: %v", owner, repo, err))
		return
	}

	for _, r := range releases {
		var authorLogin *string
		if r.Author != nil {
			authorLogin = &r.Author.Login
		}

		htmlURL := r.HTMLURL
		tarball := r.TarballURL
		zipball := r.ZipballURL
		target := r.TargetCommitish

		release := &Release{
			ID:              r.ID,
			SourceAccountID: s.acctID,
			NodeID:          &r.NodeID,
			RepoID:          &repoID,
			TagName:         r.TagName,
			TargetCommitish: &target,
			Name:            r.Name,
			Body:            r.Body,
			Draft:           r.Draft,
			Prerelease:      r.Prerelease,
			AuthorLogin:     authorLogin,
			HTMLURL:         &htmlURL,
			TarballURL:      &tarball,
			ZipballURL:      &zipball,
			Assets:          r.Assets,
			CreatedAt:       r.CreatedAt,
			PublishedAt:     r.PublishedAt,
		}
		if err := UpsertRelease(ctx, s.pool, release); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("release %s/%s:%s: %v", owner, repo, r.TagName, err))
		}
	}
	result.Stats.Releases += len(releases)
}

func (s *SyncService) syncWorkflows(ctx context.Context, owner, repo string, repoID int64, result *SyncResult) {
	workflows, err := s.client.ListWorkflows(owner, repo)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("workflows %s/%s: %v", owner, repo, err))
		return
	}

	for _, w := range workflows {
		badge := w.BadgeURL
		htmlURL := w.HTMLURL

		wf := &Workflow{
			ID:              w.ID,
			SourceAccountID: s.acctID,
			NodeID:          &w.NodeID,
			RepoID:          &repoID,
			Name:            w.Name,
			Path:            w.Path,
			State:           w.State,
			BadgeURL:        &badge,
			HTMLURL:         &htmlURL,
			CreatedAt:       w.CreatedAt,
			UpdatedAt:       w.UpdatedAt,
		}
		if err := UpsertWorkflow(ctx, s.pool, wf); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("workflow %s/%s:%s: %v", owner, repo, w.Name, err))
		}
	}
	result.Stats.Workflows += len(workflows)
}

func (s *SyncService) syncWorkflowRuns(ctx context.Context, owner, repo string, repoID int64, result *SyncResult) {
	runs, err := s.client.ListWorkflowRuns(owner, repo)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("workflow_runs %s/%s: %v", owner, repo, err))
		return
	}

	for _, r := range runs {
		var actorLogin, triggeringActorLogin *string
		if r.Actor != nil {
			actorLogin = &r.Actor.Login
		}
		if r.TriggeringActor != nil {
			triggeringActorLogin = &r.TriggeringActor.Login
		}

		name := r.Name
		headBranch := r.HeadBranch
		headSHA := r.HeadSHA
		event := r.Event
		status := r.Status
		htmlURL := r.HTMLURL
		jobsURL := r.JobsURL
		logsURL := r.LogsURL
		runNum := r.RunNumber
		runAttempt := r.RunAttempt

		run := &WorkflowRun{
			ID:                   r.ID,
			SourceAccountID:      s.acctID,
			NodeID:               &r.NodeID,
			RepoID:               &repoID,
			WorkflowID:           &r.WorkflowID,
			Name:                 &name,
			HeadBranch:           &headBranch,
			HeadSHA:              &headSHA,
			RunNumber:            &runNum,
			RunAttempt:           &runAttempt,
			Event:                &event,
			Status:               &status,
			Conclusion:           r.Conclusion,
			ActorLogin:           actorLogin,
			TriggeringActorLogin: triggeringActorLogin,
			HTMLURL:              &htmlURL,
			JobsURL:              &jobsURL,
			LogsURL:              &logsURL,
			RunStartedAt:         r.RunStartedAt,
			CreatedAt:            r.CreatedAt,
			UpdatedAt:            r.UpdatedAt,
		}
		if err := UpsertWorkflowRun(ctx, s.pool, run); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("workflow_run %s/%s:%d: %v", owner, repo, r.ID, err))
		}
	}
	result.Stats.WorkflowRuns += len(runs)
}
