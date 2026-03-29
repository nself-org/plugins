package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncService orchestrates syncing data from GitHub to the local database.
type SyncService struct {
	pool   *pgxpool.Pool
	client *GitHubClient
	config *Config
	acctID string
}

// NewSyncService creates a new sync service.
func NewSyncService(pool *pgxpool.Pool, client *GitHubClient, config *Config, accountID string) *SyncService {
	if accountID == "" {
		accountID = "primary"
	}
	return &SyncService{
		pool:   pool,
		client: client,
		config: config,
		acctID: accountID,
	}
}

// SyncAll syncs all resources: orgs, repos, and per-repo resources.
func (s *SyncService) SyncAll(ctx context.Context) *SyncResult {
	start := time.Now()
	result := &SyncResult{Success: true}

	// Sync organizations
	if s.config.Org != "" {
		if err := s.syncOrganizations(ctx, result); err != nil {
			result.Errors = append(result.Errors, "orgs: "+err.Error())
		}
	}

	// Sync repositories
	repos, err := s.syncRepositories(ctx, result)
	if err != nil {
		result.Errors = append(result.Errors, "repos: "+err.Error())
		result.Success = len(result.Errors) == 0
		result.Duration = time.Since(start).Milliseconds()
		return result
	}

	// Sync per-repo resources
	for _, repo := range repos {
		parts := strings.SplitN(repo.FullName, "/", 2)
		if len(parts) != 2 {
			continue
		}
		owner, name := parts[0], parts[1]

		s.syncBranches(ctx, owner, name, repo.ID, result)
		s.syncIssues(ctx, owner, name, repo.ID, result)
		s.syncPullRequests(ctx, owner, name, repo.ID, result)
		s.syncCommits(ctx, owner, name, repo.ID, result)
		s.syncReleases(ctx, owner, name, repo.ID, result)
		s.syncWorkflows(ctx, owner, name, repo.ID, result)
		s.syncWorkflowRuns(ctx, owner, name, repo.ID, result)
		s.syncDeployments(ctx, owner, name, repo.ID, result)
		s.syncCollaborators(ctx, owner, name, repo.ID, result)
	}

	// Sync teams (org-level)
	if s.config.Org != "" {
		s.syncTeams(ctx, s.config.Org, result)
	}

	// Get final stats
	stats, err := GetSyncStats(ctx, s.pool)
	if err == nil {
		result.Stats = *stats
	}

	result.Success = len(result.Errors) == 0
	result.Duration = time.Since(start).Milliseconds()
	return result
}

// SyncResource syncs a specific resource type.
func (s *SyncService) SyncResource(ctx context.Context, resource string) *SyncResult {
	start := time.Now()
	result := &SyncResult{Success: true}

	switch resource {
	case "organizations", "orgs":
		if err := s.syncOrganizations(ctx, result); err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
	case "repositories", "repos":
		if _, err := s.syncRepositories(ctx, result); err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
	case "teams":
		if s.config.Org != "" {
			s.syncTeams(ctx, s.config.Org, result)
		}
	default:
		// For per-repo resources, sync across all repos
		repos, err := s.getTrackedRepos(ctx)
		if err != nil {
			result.Errors = append(result.Errors, "getting repos: "+err.Error())
		} else {
			for _, repo := range repos {
				parts := strings.SplitN(repo.FullName, "/", 2)
				if len(parts) != 2 {
					continue
				}
				owner, name := parts[0], parts[1]

				switch resource {
				case "branches":
					s.syncBranches(ctx, owner, name, repo.ID, result)
				case "issues":
					s.syncIssues(ctx, owner, name, repo.ID, result)
				case "pulls", "pull_requests":
					s.syncPullRequests(ctx, owner, name, repo.ID, result)
				case "commits":
					s.syncCommits(ctx, owner, name, repo.ID, result)
				case "releases":
					s.syncReleases(ctx, owner, name, repo.ID, result)
				case "workflows":
					s.syncWorkflows(ctx, owner, name, repo.ID, result)
				case "workflow_runs", "runs":
					s.syncWorkflowRuns(ctx, owner, name, repo.ID, result)
				case "deployments":
					s.syncDeployments(ctx, owner, name, repo.ID, result)
				case "collaborators":
					s.syncCollaborators(ctx, owner, name, repo.ID, result)
				default:
					result.Errors = append(result.Errors, "unknown resource: "+resource)
				}
			}
		}
	}

	stats, err := GetSyncStats(ctx, s.pool)
	if err == nil {
		result.Stats = *stats
	}

	result.Success = len(result.Errors) == 0
	result.Duration = time.Since(start).Milliseconds()
	return result
}

// --- Internal sync methods ---------------------------------------------------

func (s *SyncService) syncOrganizations(ctx context.Context, result *SyncResult) error {
	log.Printf("[github:sync] syncing organizations")
	orgs, err := s.client.ListOrganizations()
	if err != nil {
		return err
	}

	for _, o := range orgs {
		org := &Organization{
			ID:              o.ID,
			SourceAccountID: s.acctID,
			NodeID:          o.NodeID,
			Login:           o.Login,
			Name:            o.Name,
			Description:     o.Description,
			Company:         o.Company,
			Blog:            o.Blog,
			Location:        o.Location,
			Email:           o.Email,
			TwitterUsername: o.TwitterUsername,
			IsVerified:      o.IsVerified,
			HTMLURL:         o.HTMLURL,
			AvatarURL:       o.AvatarURL,
			PublicRepos:     o.PublicRepos,
			PublicGists:     o.PublicGists,
			Followers:       o.Followers,
			Following:       o.Following,
			Type:            o.Type,
			Plan:            o.Plan,
			CreatedAt:       o.CreatedAt,
			UpdatedAt:       o.UpdatedAt,
		}
		if err := UpsertOrganization(ctx, s.pool, org); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("org %s: %v", o.Login, err))
		}
	}
	return nil
}

func (s *SyncService) syncRepositories(ctx context.Context, result *SyncResult) ([]Repository, error) {
	log.Printf("[github:sync] syncing repositories")
	var ghRepos []ghRepo
	var err error

	if s.config.Org != "" {
		ghRepos, err = s.client.ListRepositories(s.config.Org)
	} else {
		ghRepos, err = s.client.ListUserRepositories()
	}
	if err != nil {
		return nil, err
	}

	// Filter to configured repos if specified
	if len(s.config.Repos) > 0 {
		allowed := make(map[string]bool)
		for _, r := range s.config.Repos {
			allowed[r] = true
		}
		var filtered []ghRepo
		for _, r := range ghRepos {
			if allowed[r.FullName] || allowed[r.Name] {
				filtered = append(filtered, r)
			}
		}
		ghRepos = filtered
	}

	var repos []Repository
	for _, r := range ghRepos {
		ownerType := r.Owner.Type
		topics := &r.Topics
		repo := &Repository{
			ID:              r.ID,
			SourceAccountID: s.acctID,
			NodeID:          r.NodeID,
			Name:            r.Name,
			FullName:        r.FullName,
			OwnerLogin:      r.Owner.Login,
			OwnerType:       &ownerType,
			Private:         r.Private,
			Description:     r.Description,
			Fork:            r.Fork,
			URL:             &r.URL,
			HTMLURL:         &r.HTMLURL,
			CloneURL:        &r.CloneURL,
			SSHURL:          &r.SSHURL,
			Homepage:        r.Homepage,
			Language:        r.Language,
			DefaultBranch:   r.DefaultBranch,
			Size:            r.Size,
			StargazersCount: r.StargazersCount,
			WatchersCount:   r.WatchersCount,
			ForksCount:      r.ForksCount,
			OpenIssuesCount: r.OpenIssuesCount,
			Topics:          topics,
			Visibility:      r.Visibility,
			Archived:        r.Archived,
			Disabled:        r.Disabled,
			HasIssues:       r.HasIssues,
			HasProjects:     r.HasProjects,
			HasWiki:         r.HasWiki,
			HasPages:        r.HasPages,
			HasDownloads:    r.HasDownloads,
			HasDiscussions:  r.HasDiscussions,
			AllowForking:    r.AllowForking,
			IsTemplate:      r.IsTemplate,
			License:         r.License,
			PushedAt:        r.PushedAt,
			CreatedAt:       r.CreatedAt,
			UpdatedAt:       r.UpdatedAt,
		}
		if err := UpsertRepository(ctx, s.pool, repo); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("repo %s: %v", r.FullName, err))
		}
		repos = append(repos, *repo)
	}

	result.Stats.Repositories = len(repos)
	return repos, nil
}

func (s *SyncService) syncBranches(ctx context.Context, owner, repo string, repoID int64, result *SyncResult) {
	branches, err := s.client.ListBranches(owner, repo)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("branches %s/%s: %v", owner, repo, err))
		return
	}

	for _, b := range branches {
		branch := &Branch{
			ID:              fmt.Sprintf("%d:%s", repoID, b.Name),
			SourceAccountID: s.acctID,
			RepoID:          &repoID,
			Name:            b.Name,
			SHA:             b.Commit.SHA,
			Protected:       b.Protected,
		}
		if err := UpsertBranch(ctx, s.pool, branch); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("branch %s/%s:%s: %v", owner, repo, b.Name, err))
		}
	}
	result.Stats.Branches += len(branches)
}

func (s *SyncService) syncIssues(ctx context.Context, owner, repo string, repoID int64, result *SyncResult) {
	issues, err := s.client.ListIssues(owner, repo, "all")
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("issues %s/%s: %v", owner, repo, err))
		return
	}

	count := 0
	for _, i := range issues {
		// Skip pull requests (GitHub issues API includes them)
		if i.PullRequest != nil {
			continue
		}

		var userLogin *string
		var userID *int64
		if i.User != nil {
			userLogin = &i.User.Login
			userID = &i.User.ID
		}
		labels := &i.Labels
		assignees := &i.Assignees
		htmlURL := i.HTMLURL

		issue := &Issue{
			ID:              i.ID,
			SourceAccountID: s.acctID,
			NodeID:          &i.NodeID,
			RepoID:          &repoID,
			Number:          i.Number,
			Title:           i.Title,
			Body:            i.Body,
			State:           i.State,
			StateReason:     i.StateReason,
			Locked:          i.Locked,
			UserLogin:       userLogin,
			UserID:          userID,
			Labels:          labels,
			Assignees:       assignees,
			Milestone:       i.Milestone,
			Comments:        i.Comments,
			Reactions:       i.Reactions,
			HTMLURL:         &htmlURL,
			ClosedAt:        i.ClosedAt,
			CreatedAt:       i.CreatedAt,
			UpdatedAt:       i.UpdatedAt,
		}
		if err := UpsertIssue(ctx, s.pool, issue); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("issue %s/%s#%d: %v", owner, repo, i.Number, err))
		}
		count++
	}
	result.Stats.Issues += count
}

func (s *SyncService) syncPullRequests(ctx context.Context, owner, repo string, repoID int64, result *SyncResult) {
	prs, err := s.client.ListPullRequests(owner, repo, "all")
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("pulls %s/%s: %v", owner, repo, err))
		return
	}

	for _, p := range prs {
		var userLogin *string
		var userID *int64
		if p.User != nil {
			userLogin = &p.User.Login
			userID = &p.User.ID
		}

		var mergedBy *string
		if p.MergedBy != nil {
			mergedBy = &p.MergedBy.Login
		}

		var headRepoID *int64
		if p.Head.Repo != nil {
			headRepoID = &p.Head.Repo.ID
		}

		headRef := p.Head.Ref
		headSHA := p.Head.SHA
		baseRef := p.Base.Ref
		baseSHA := p.Base.SHA
		labels := &p.Labels
		assignees := &p.Assignees
		reviewers := &p.Reviewers
		htmlURL := p.HTMLURL
		diffURL := p.DiffURL

		pr := &PullRequest{
			ID:              p.ID,
			SourceAccountID: s.acctID,
			NodeID:          &p.NodeID,
			RepoID:          &repoID,
			Number:          p.Number,
			Title:           p.Title,
			Body:            p.Body,
			State:           p.State,
			Draft:           p.Draft,
			Locked:          p.Locked,
			UserLogin:       userLogin,
			UserID:          userID,
			HeadRef:         &headRef,
			HeadSHA:         &headSHA,
			HeadRepoID:      headRepoID,
			BaseRef:         &baseRef,
			BaseSHA:         &baseSHA,
			Merged:          p.Merged,
			Mergeable:       p.Mergeable,
			MergeableState:  p.MergeableState,
			MergedByLogin:   mergedBy,
			MergedAt:        p.MergedAt,
			MergeCommitSHA:  p.MergeCommitSHA,
			Labels:          labels,
			Assignees:       assignees,
			Reviewers:       reviewers,
			MilestonePR:     p.Milestone,
			CommentCount:    p.Comments,
			ReviewComments:  p.ReviewComments,
			Commits:         p.Commits,
			Additions:       p.Additions,
			Deletions:       p.Deletions,
			ChangedFiles:    p.ChangedFiles,
			HTMLURL:         &htmlURL,
			DiffURL:         &diffURL,
			ClosedAt:        p.ClosedAt,
			CreatedAt:       p.CreatedAt,
			UpdatedAt:       p.UpdatedAt,
		}
		if err := UpsertPullRequest(ctx, s.pool, pr); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("pr %s/%s#%d: %v", owner, repo, p.Number, err))
		}
	}
	result.Stats.PullRequests += len(prs)
}

func (s *SyncService) syncCommits(ctx context.Context, owner, repo string, repoID int64, result *SyncResult) {
	commits, err := s.client.ListCommits(owner, repo)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("commits %s/%s: %v", owner, repo, err))
		return
	}

	for _, c := range commits {
		var authorLogin, committerLogin *string
		if c.Author != nil {
			authorLogin = &c.Author.Login
		}
		if c.Committer != nil {
			committerLogin = &c.Committer.Login
		}

		msg := c.Commit.Message
		authorName := c.Commit.Author.Name
		authorEmail := c.Commit.Author.Email
		committerName := c.Commit.Committer.Name
		committerEmail := c.Commit.Committer.Email
		treeSHA := c.Commit.Tree.SHA
		parents := &c.Parents
		htmlURL := c.HTMLURL
		reason := c.Commit.Verification.Reason

		var additions, deletions, total int
		if c.Stats != nil {
			additions = c.Stats.Additions
			deletions = c.Stats.Deletions
			total = c.Stats.Total
		}

		commit := &Commit{
			SHA:                c.SHA,
			SourceAccountID:    s.acctID,
			NodeID:             &c.NodeID,
			RepoID:             &repoID,
			Message:            &msg,
			AuthorName:         &authorName,
			AuthorEmail:        &authorEmail,
			AuthorLogin:        authorLogin,
			AuthorDate:         c.Commit.Author.Date,
			CommitterName:      &committerName,
			CommitterEmail:     &committerEmail,
			CommitterLogin:     committerLogin,
			CommitterDate:      c.Commit.Committer.Date,
			TreeSHA:            &treeSHA,
			Parents:            parents,
			CommitAdditions:    additions,
			CommitDeletions:    deletions,
			Total:              total,
			HTMLURL:            &htmlURL,
			Verified:           c.Commit.Verification.Verified,
			VerificationReason: &reason,
		}
		if err := UpsertCommit(ctx, s.pool, commit); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("commit %s/%s:%s: %v", owner, repo, c.SHA[:7], err))
		}
	}
	result.Stats.Commits += len(commits)
}

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

func (s *SyncService) syncDeployments(ctx context.Context, owner, repo string, repoID int64, result *SyncResult) {
	deployments, err := s.client.ListDeployments(owner, repo)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("deployments %s/%s: %v", owner, repo, err))
		return
	}

	for _, d := range deployments {
		var creatorLogin *string
		if d.Creator != nil {
			creatorLogin = &d.Creator.Login
		}

		sha := d.SHA
		ref := d.Ref
		task := d.Task
		env := d.Environment

		dep := &Deployment{
			ID:                    d.ID,
			SourceAccountID:       s.acctID,
			NodeID:                &d.NodeID,
			RepoID:                &repoID,
			SHA:                   &sha,
			Ref:                   &ref,
			Task:                  &task,
			Environment:           &env,
			Description:           d.Description,
			CreatorLogin:          creatorLogin,
			ProductionEnvironment: d.ProductionEnvironment,
			TransientEnvironment:  d.TransientEnvironment,
			Payload:               d.Payload,
			CreatedAt:             d.CreatedAt,
			UpdatedAt:             d.UpdatedAt,
		}
		if err := UpsertDeployment(ctx, s.pool, dep); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("deployment %s/%s:%d: %v", owner, repo, d.ID, err))
		}
	}
	result.Stats.Deployments += len(deployments)
}

func (s *SyncService) syncTeams(ctx context.Context, org string, result *SyncResult) {
	teams, err := s.client.ListTeams(org)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("teams %s: %v", org, err))
		return
	}

	for _, t := range teams {
		var parentID *int64
		if t.Parent != nil {
			parentID = &t.Parent.ID
		}

		privacy := t.Privacy
		permission := t.Permission
		htmlURL := t.HTMLURL

		team := &Team{
			ID:              t.ID,
			SourceAccountID: s.acctID,
			NodeID:          &t.NodeID,
			OrgLogin:        org,
			Name:            t.Name,
			Slug:            t.Slug,
			Description:     t.Description,
			Privacy:         &privacy,
			Permission:      &permission,
			ParentID:        parentID,
			MembersCount:    t.MembersCount,
			ReposCount:      t.ReposCount,
			HTMLURL:         &htmlURL,
			CreatedAt:       t.CreatedAt,
			UpdatedAt:       t.UpdatedAt,
		}
		if err := UpsertTeam(ctx, s.pool, team); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("team %s/%s: %v", org, t.Slug, err))
		}
	}
	result.Stats.Teams += len(teams)
}

func (s *SyncService) syncCollaborators(ctx context.Context, owner, repo string, repoID int64, result *SyncResult) {
	collabs, err := s.client.ListCollaborators(owner, repo)
	if err != nil {
		// 403 is expected for non-org repos, skip silently
		return
	}

	for _, c := range collabs {
		colType := c.Type
		roleName := c.RoleName

		collab := &Collaborator{
			ID:              c.ID,
			SourceAccountID: s.acctID,
			RepoID:          &repoID,
			Login:           c.Login,
			Type:            &colType,
			SiteAdmin:       c.SiteAdmin,
			Permissions:     c.Permissions,
			RoleName:        &roleName,
		}
		if err := UpsertCollaborator(ctx, s.pool, collab); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("collaborator %s/%s:%s: %v", owner, repo, c.Login, err))
		}
	}
	result.Stats.Collaborators += len(collabs)
}

// getTrackedRepos returns all synced repositories from the database.
func (s *SyncService) getTrackedRepos(ctx context.Context) ([]Repository, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, source_account_id, node_id, name, full_name, owner_login, owner_type,
			private, description, fork, url, html_url, clone_url, ssh_url, homepage,
			language, languages, default_branch, size, stargazers_count, watchers_count,
			forks_count, open_issues_count, topics, visibility, archived, disabled,
			has_issues, has_projects, has_wiki, has_pages, has_downloads, has_discussions,
			allow_forking, is_template, license, pushed_at, created_at, updated_at, synced_at
		FROM np_github_repositories
		ORDER BY full_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []Repository
	for rows.Next() {
		var r Repository
		if err := rows.Scan(
			&r.ID, &r.SourceAccountID, &r.NodeID, &r.Name, &r.FullName,
			&r.OwnerLogin, &r.OwnerType, &r.Private, &r.Description, &r.Fork,
			&r.URL, &r.HTMLURL, &r.CloneURL, &r.SSHURL, &r.Homepage,
			&r.Language, &r.Languages, &r.DefaultBranch, &r.Size,
			&r.StargazersCount, &r.WatchersCount, &r.ForksCount,
			&r.OpenIssuesCount, &r.Topics, &r.Visibility, &r.Archived,
			&r.Disabled, &r.HasIssues, &r.HasProjects, &r.HasWiki,
			&r.HasPages, &r.HasDownloads, &r.HasDiscussions, &r.AllowForking,
			&r.IsTemplate, &r.License, &r.PushedAt, &r.CreatedAt,
			&r.UpdatedAt, &r.SyncedAt,
		); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// toRawMessage converts any value to a *json.RawMessage.
func toRawMessage(v interface{}) *json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	raw := json.RawMessage(b)
	return &raw
}
