package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ListRepositories(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]Repository, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, source_account_id, node_id, name, full_name, owner_login, owner_type,
			private, description, fork, url, html_url, clone_url, ssh_url, homepage,
			language, languages, default_branch, size, stargazers_count, watchers_count,
			forks_count, open_issues_count, topics, visibility, archived, disabled,
			has_issues, has_projects, has_wiki, has_pages, has_downloads, has_discussions,
			allow_forking, is_template, license, pushed_at, created_at, updated_at, synced_at
		FROM np_github_repositories
		ORDER BY updated_at DESC NULLS LAST
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Repository
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
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetRepository returns a single repository by ID.
func GetRepository(ctx context.Context, pool *pgxpool.Pool, id int64) (*Repository, error) {
	var r Repository
	err := pool.QueryRow(ctx, `
		SELECT id, source_account_id, node_id, name, full_name, owner_login, owner_type,
			private, description, fork, url, html_url, clone_url, ssh_url, homepage,
			language, languages, default_branch, size, stargazers_count, watchers_count,
			forks_count, open_issues_count, topics, visibility, archived, disabled,
			has_issues, has_projects, has_wiki, has_pages, has_downloads, has_discussions,
			allow_forking, is_template, license, pushed_at, created_at, updated_at, synced_at
		FROM np_github_repositories WHERE id = $1
	`, id).Scan(
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
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListIssues returns issues with optional filtering and pagination.
