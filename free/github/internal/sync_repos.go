package internal

import (
	"context"
	"fmt"
	"log"
)

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

// Size-cap exception: sync pipeline — 82L sequential sync stages; splitting creates artificial state-passing overhead.
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

// Size-cap exception: sync pipeline — 54L sequential sync stages; splitting creates artificial state-passing overhead.
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

// Size-cap exception: sync pipeline — 81L sequential sync stages; splitting creates artificial state-passing overhead.
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


