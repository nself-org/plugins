package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func UpsertOrganization(ctx context.Context, pool *pgxpool.Pool, o *Organization) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_organizations (
			id, source_account_id, node_id, login, name, description, company, blog,
			location, email, twitter_username, is_verified, html_url, avatar_url,
			public_repos, public_gists, followers, following, type,
			total_private_repos, owned_private_repos, plan, created_at, updated_at, synced_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			node_id = EXCLUDED.node_id,
			login = EXCLUDED.login,
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			company = EXCLUDED.company,
			blog = EXCLUDED.blog,
			location = EXCLUDED.location,
			email = EXCLUDED.email,
			twitter_username = EXCLUDED.twitter_username,
			is_verified = EXCLUDED.is_verified,
			html_url = EXCLUDED.html_url,
			avatar_url = EXCLUDED.avatar_url,
			public_repos = EXCLUDED.public_repos,
			public_gists = EXCLUDED.public_gists,
			followers = EXCLUDED.followers,
			following = EXCLUDED.following,
			type = EXCLUDED.type,
			total_private_repos = EXCLUDED.total_private_repos,
			owned_private_repos = EXCLUDED.owned_private_repos,
			plan = EXCLUDED.plan,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, o.ID, o.SourceAccountID, o.NodeID, o.Login, o.Name, o.Description,
		o.Company, o.Blog, o.Location, o.Email, o.TwitterUsername, o.IsVerified,
		o.HTMLURL, o.AvatarURL, o.PublicRepos, o.PublicGists, o.Followers,
		o.Following, o.Type, o.TotalPrivateRepos, o.OwnedPrivateRepos,
		o.Plan, o.CreatedAt, o.UpdatedAt)
	return err
}

// UpsertRepository inserts or updates a repository record.
// Size-cap exception: single DB operation — 63L scan loop with struct mapping; splitting would fragment a single SQL query across files.
func UpsertRepository(ctx context.Context, pool *pgxpool.Pool, r *Repository) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_repositories (
			id, source_account_id, node_id, name, full_name, owner_login, owner_type,
			private, description, fork, url, html_url, clone_url, ssh_url, homepage,
			language, languages, default_branch, size, stargazers_count, watchers_count,
			forks_count, open_issues_count, topics, visibility, archived, disabled,
			has_issues, has_projects, has_wiki, has_pages, has_downloads, has_discussions,
			allow_forking, is_template, license, pushed_at, created_at, updated_at, synced_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27,
			$28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			node_id = EXCLUDED.node_id,
			name = EXCLUDED.name,
			full_name = EXCLUDED.full_name,
			owner_login = EXCLUDED.owner_login,
			owner_type = EXCLUDED.owner_type,
			private = EXCLUDED.private,
			description = EXCLUDED.description,
			fork = EXCLUDED.fork,
			url = EXCLUDED.url,
			html_url = EXCLUDED.html_url,
			clone_url = EXCLUDED.clone_url,
			ssh_url = EXCLUDED.ssh_url,
			homepage = EXCLUDED.homepage,
			language = EXCLUDED.language,
			languages = EXCLUDED.languages,
			default_branch = EXCLUDED.default_branch,
			size = EXCLUDED.size,
			stargazers_count = EXCLUDED.stargazers_count,
			watchers_count = EXCLUDED.watchers_count,
			forks_count = EXCLUDED.forks_count,
			open_issues_count = EXCLUDED.open_issues_count,
			topics = EXCLUDED.topics,
			visibility = EXCLUDED.visibility,
			archived = EXCLUDED.archived,
			disabled = EXCLUDED.disabled,
			has_issues = EXCLUDED.has_issues,
			has_projects = EXCLUDED.has_projects,
			has_wiki = EXCLUDED.has_wiki,
			has_pages = EXCLUDED.has_pages,
			has_downloads = EXCLUDED.has_downloads,
			has_discussions = EXCLUDED.has_discussions,
			allow_forking = EXCLUDED.allow_forking,
			is_template = EXCLUDED.is_template,
			license = EXCLUDED.license,
			pushed_at = EXCLUDED.pushed_at,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, r.ID, r.SourceAccountID, r.NodeID, r.Name, r.FullName, r.OwnerLogin,
		r.OwnerType, r.Private, r.Description, r.Fork, r.URL, r.HTMLURL,
		r.CloneURL, r.SSHURL, r.Homepage, r.Language, r.Languages,
		r.DefaultBranch, r.Size, r.StargazersCount, r.WatchersCount,
		r.ForksCount, r.OpenIssuesCount, r.Topics, r.Visibility,
		r.Archived, r.Disabled, r.HasIssues, r.HasProjects, r.HasWiki,
		r.HasPages, r.HasDownloads, r.HasDiscussions, r.AllowForking,
		r.IsTemplate, r.License, r.PushedAt, r.CreatedAt, r.UpdatedAt)
	return err
}

// UpsertBranch inserts or updates a branch record.
func UpsertBranch(ctx context.Context, pool *pgxpool.Pool, b *Branch) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_branches (id, source_account_id, repo_id, name, sha, protected, protection_enabled, protection, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			repo_id = EXCLUDED.repo_id,
			name = EXCLUDED.name,
			sha = EXCLUDED.sha,
			protected = EXCLUDED.protected,
			protection_enabled = EXCLUDED.protection_enabled,
			protection = EXCLUDED.protection,
			updated_at = NOW()
	`, b.ID, b.SourceAccountID, b.RepoID, b.Name, b.SHA, b.Protected, b.ProtectionEnabled, b.Protection)
	return err
}

// UpsertIssue inserts or updates an issue record.
