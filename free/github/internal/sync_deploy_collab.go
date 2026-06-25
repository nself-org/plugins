package internal

import (
	"context"
	"encoding/json"
	"fmt"
)

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
