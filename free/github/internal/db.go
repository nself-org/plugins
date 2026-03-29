package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with a source account ID for multi-app isolation.
type DB struct {
	Pool            *pgxpool.Pool
	SourceAccountID string
}

// NewDB creates a new DB instance.
func NewDB(pool *pgxpool.Pool, sourceAccountID string) *DB {
	if sourceAccountID == "" {
		sourceAccountID = "primary"
	}
	return &DB{Pool: pool, SourceAccountID: sourceAccountID}
}

// ListRepositories returns repositories with pagination (delegating to the package-level function).
func (db *DB) ListRepositories(ctx context.Context, limit, offset int) ([]Repository, error) {
	return ListRepositories(ctx, db.Pool, limit, offset)
}

// GetRepositoryByFullName returns a single repository by full_name.
func (db *DB) GetRepositoryByFullName(ctx context.Context, fullName string) (*Repository, error) {
	var r Repository
	err := db.Pool.QueryRow(ctx, `
		SELECT id, source_account_id, node_id, name, full_name, owner_login, owner_type,
			private, description, fork, url, html_url, clone_url, ssh_url, homepage,
			language, languages, default_branch, size, stargazers_count, watchers_count,
			forks_count, open_issues_count, topics, visibility, archived, disabled,
			has_issues, has_projects, has_wiki, has_pages, has_downloads, has_discussions,
			allow_forking, is_template, license, pushed_at, created_at, updated_at, synced_at
		FROM np_github_repositories WHERE full_name = $1
	`, fullName).Scan(
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

// ListPullRequests delegates to the package-level function.
func (db *DB) ListPullRequests(ctx context.Context, repoID *int64, state *string, limit, offset int) ([]PullRequest, error) {
	s := ""
	if state != nil {
		s = *state
	}
	return ListPullRequests(ctx, db.Pool, repoID, s, limit, offset)
}

// InsertWebhookEvent delegates to the package-level function.
func (db *DB) InsertWebhookEvent(ctx context.Context, e WebhookEvent) error {
	return InsertWebhookEvent(ctx, db.Pool, &e)
}

// MarkEventProcessed marks a webhook event as processed.
func (db *DB) MarkEventProcessed(ctx context.Context, deliveryID string, errMsg *string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE np_github_webhook_events
		SET processed = TRUE, processed_at = NOW(), error = $2
		WHERE id = $1
	`, deliveryID, errMsg)
	return err
}

// CountRepositories returns the total count of repositories.
func (db *DB) CountRepositories(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_repositories")
}

// CountIssues returns the total count of issues, optionally filtered by state.
func (db *DB) CountIssues(ctx context.Context, state string) (int, error) {
	if state != "" {
		var count int
		err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_github_issues WHERE state = $1`, state).Scan(&count)
		return count, err
	}
	return countTable(ctx, db.Pool, "np_github_issues")
}

// CountPullRequests returns the total count of pull requests, optionally filtered by state.
func (db *DB) CountPullRequests(ctx context.Context, state string) (int, error) {
	if state != "" {
		var count int
		err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_github_pull_requests WHERE state = $1`, state).Scan(&count)
		return count, err
	}
	return countTable(ctx, db.Pool, "np_github_pull_requests")
}

// CountBranches returns the total count of branches.
func (db *DB) CountBranches(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_branches")
}

// CountCommits returns the total count of commits.
func (db *DB) CountCommits(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_commits")
}

// CountReleases returns the total count of releases.
func (db *DB) CountReleases(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_releases")
}

// CountTags returns the total count of tags.
func (db *DB) CountTags(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_tags")
}

// CountMilestones returns the total count of milestones.
func (db *DB) CountMilestones(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_milestones")
}

// CountLabels returns the total count of labels.
func (db *DB) CountLabels(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_labels")
}

// CountWorkflows returns the total count of workflows.
func (db *DB) CountWorkflows(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_workflows")
}

// CountWorkflowRuns returns the total count of workflow runs.
func (db *DB) CountWorkflowRuns(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_workflow_runs")
}

// CountWorkflowJobs returns the total count of workflow jobs.
func (db *DB) CountWorkflowJobs(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_workflow_jobs")
}

// CountCheckSuites returns the total count of check suites.
func (db *DB) CountCheckSuites(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_check_suites")
}

// CountCheckRuns returns the total count of check runs.
func (db *DB) CountCheckRuns(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_check_runs")
}

// CountDeployments returns the total count of deployments.
func (db *DB) CountDeployments(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_deployments")
}

// CountTeams returns the total count of teams.
func (db *DB) CountTeams(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_teams")
}

// CountCollaborators returns the total count of collaborators.
func (db *DB) CountCollaborators(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_collaborators")
}

// CountPRReviews returns the total count of PR reviews.
func (db *DB) CountPRReviews(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_pr_reviews")
}

// CountIssueComments returns the total count of issue comments.
func (db *DB) CountIssueComments(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_issue_comments")
}

// CountPRReviewComments returns the total count of PR review comments.
func (db *DB) CountPRReviewComments(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_pr_review_comments")
}

// CountCommitComments returns the total count of commit comments.
func (db *DB) CountCommitComments(ctx context.Context) (int, error) {
	return countTable(ctx, db.Pool, "np_github_commit_comments")
}

// GetStats returns stats for all synced entity types.
func (db *DB) GetStats(ctx context.Context) (*SyncStats, error) {
	return GetSyncStats(ctx, db.Pool)
}

// Ping verifies database connectivity.
func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

// countTable is a helper that counts all rows in a table.
func countTable(ctx context.Context, pool *pgxpool.Pool, table string) (int, error) {
	var count int
	err := pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
	return count, err
}

// Migrate creates all 23 tables and their indexes.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		-- 1. Organizations
		CREATE TABLE IF NOT EXISTS np_github_organizations (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			login           TEXT NOT NULL,
			name            TEXT,
			description     TEXT,
			company         TEXT,
			blog            TEXT,
			location        TEXT,
			email           TEXT,
			twitter_username TEXT,
			is_verified     BOOLEAN NOT NULL DEFAULT FALSE,
			html_url        TEXT NOT NULL DEFAULT '',
			avatar_url      TEXT NOT NULL DEFAULT '',
			public_repos    INTEGER NOT NULL DEFAULT 0,
			public_gists    INTEGER NOT NULL DEFAULT 0,
			followers       INTEGER NOT NULL DEFAULT 0,
			following       INTEGER NOT NULL DEFAULT 0,
			type            TEXT NOT NULL DEFAULT 'Organization',
			total_private_repos INTEGER,
			owned_private_repos INTEGER,
			plan            JSONB,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_organizations_login ON np_github_organizations (login);
		CREATE INDEX IF NOT EXISTS idx_np_github_organizations_source ON np_github_organizations (source_account_id);

		-- 2. Repositories
		CREATE TABLE IF NOT EXISTS np_github_repositories (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			name            TEXT NOT NULL,
			full_name       TEXT NOT NULL,
			owner_login     TEXT NOT NULL,
			owner_type      TEXT,
			private         BOOLEAN NOT NULL DEFAULT FALSE,
			description     TEXT,
			fork            BOOLEAN NOT NULL DEFAULT FALSE,
			url             TEXT,
			html_url        TEXT,
			clone_url       TEXT,
			ssh_url         TEXT,
			homepage        TEXT,
			language        TEXT,
			languages       JSONB DEFAULT '{}',
			default_branch  TEXT NOT NULL DEFAULT 'main',
			size            INTEGER NOT NULL DEFAULT 0,
			stargazers_count INTEGER NOT NULL DEFAULT 0,
			watchers_count  INTEGER NOT NULL DEFAULT 0,
			forks_count     INTEGER NOT NULL DEFAULT 0,
			open_issues_count INTEGER NOT NULL DEFAULT 0,
			topics          JSONB DEFAULT '[]',
			visibility      TEXT NOT NULL DEFAULT 'public',
			archived        BOOLEAN NOT NULL DEFAULT FALSE,
			disabled        BOOLEAN NOT NULL DEFAULT FALSE,
			has_issues      BOOLEAN NOT NULL DEFAULT TRUE,
			has_projects    BOOLEAN NOT NULL DEFAULT TRUE,
			has_wiki        BOOLEAN NOT NULL DEFAULT TRUE,
			has_pages       BOOLEAN NOT NULL DEFAULT FALSE,
			has_downloads   BOOLEAN NOT NULL DEFAULT TRUE,
			has_discussions BOOLEAN NOT NULL DEFAULT FALSE,
			allow_forking   BOOLEAN NOT NULL DEFAULT FALSE,
			is_template     BOOLEAN NOT NULL DEFAULT FALSE,
			license         JSONB,
			pushed_at       TIMESTAMPTZ,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_repositories_owner ON np_github_repositories (owner_login);
		CREATE INDEX IF NOT EXISTS idx_np_github_repositories_full_name ON np_github_repositories (full_name);
		CREATE INDEX IF NOT EXISTS idx_np_github_repositories_language ON np_github_repositories (language);
		CREATE INDEX IF NOT EXISTS idx_np_github_repositories_source ON np_github_repositories (source_account_id);

		-- 3. Branches
		CREATE TABLE IF NOT EXISTS np_github_branches (
			id              TEXT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			repo_id         BIGINT,
			name            TEXT NOT NULL,
			sha             TEXT NOT NULL,
			protected       BOOLEAN NOT NULL DEFAULT FALSE,
			protection_enabled BOOLEAN NOT NULL DEFAULT FALSE,
			protection      JSONB,
			updated_at      TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_branches_repo ON np_github_branches (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_branches_source ON np_github_branches (source_account_id);

		-- 4. Issues
		CREATE TABLE IF NOT EXISTS np_github_issues (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			number          INTEGER NOT NULL,
			title           TEXT NOT NULL,
			body            TEXT,
			state           TEXT NOT NULL DEFAULT 'open',
			state_reason    TEXT,
			locked          BOOLEAN NOT NULL DEFAULT FALSE,
			user_login      TEXT,
			user_id         BIGINT,
			labels          JSONB DEFAULT '[]',
			assignees       JSONB DEFAULT '[]',
			milestone       JSONB,
			comments        INTEGER NOT NULL DEFAULT 0,
			reactions       JSONB DEFAULT '{}',
			html_url        TEXT,
			closed_at       TIMESTAMPTZ,
			closed_by_login TEXT,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_issues_repo ON np_github_issues (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_issues_state ON np_github_issues (state);
		CREATE INDEX IF NOT EXISTS idx_np_github_issues_user ON np_github_issues (user_login);
		CREATE INDEX IF NOT EXISTS idx_np_github_issues_created ON np_github_issues (created_at);
		CREATE INDEX IF NOT EXISTS idx_np_github_issues_source ON np_github_issues (source_account_id);

		-- 5. Pull Requests
		CREATE TABLE IF NOT EXISTS np_github_pull_requests (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			number          INTEGER NOT NULL,
			title           TEXT NOT NULL,
			body            TEXT,
			state           TEXT NOT NULL DEFAULT 'open',
			draft           BOOLEAN NOT NULL DEFAULT FALSE,
			locked          BOOLEAN NOT NULL DEFAULT FALSE,
			user_login      TEXT,
			user_id         BIGINT,
			head_ref        TEXT,
			head_sha        TEXT,
			head_repo_id    BIGINT,
			base_ref        TEXT,
			base_sha        TEXT,
			merged          BOOLEAN NOT NULL DEFAULT FALSE,
			mergeable       BOOLEAN,
			mergeable_state TEXT,
			merged_by_login TEXT,
			merged_at       TIMESTAMPTZ,
			merge_commit_sha TEXT,
			labels          JSONB DEFAULT '[]',
			assignees       JSONB DEFAULT '[]',
			reviewers       JSONB DEFAULT '[]',
			milestone       JSONB,
			comments        INTEGER NOT NULL DEFAULT 0,
			review_comments INTEGER NOT NULL DEFAULT 0,
			commits         INTEGER NOT NULL DEFAULT 0,
			additions       INTEGER NOT NULL DEFAULT 0,
			deletions       INTEGER NOT NULL DEFAULT 0,
			changed_files   INTEGER NOT NULL DEFAULT 0,
			html_url        TEXT,
			diff_url        TEXT,
			closed_at       TIMESTAMPTZ,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_prs_repo ON np_github_pull_requests (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_prs_state ON np_github_pull_requests (state);
		CREATE INDEX IF NOT EXISTS idx_np_github_prs_user ON np_github_pull_requests (user_login);
		CREATE INDEX IF NOT EXISTS idx_np_github_prs_merged ON np_github_pull_requests (merged);
		CREATE INDEX IF NOT EXISTS idx_np_github_prs_created ON np_github_pull_requests (created_at);
		CREATE INDEX IF NOT EXISTS idx_np_github_prs_source ON np_github_pull_requests (source_account_id);

		-- 6. PR Reviews
		CREATE TABLE IF NOT EXISTS np_github_pr_reviews (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			pull_request_id BIGINT,
			pull_request_number INTEGER NOT NULL DEFAULT 0,
			user_login      TEXT,
			user_id         BIGINT,
			body            TEXT,
			state           TEXT NOT NULL DEFAULT '',
			html_url        TEXT,
			commit_id       TEXT,
			submitted_at    TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_pr_reviews_repo ON np_github_pr_reviews (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_pr_reviews_pr ON np_github_pr_reviews (pull_request_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_pr_reviews_source ON np_github_pr_reviews (source_account_id);

		-- 7. Issue Comments
		CREATE TABLE IF NOT EXISTS np_github_issue_comments (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			issue_number    INTEGER NOT NULL DEFAULT 0,
			issue_id        BIGINT,
			pull_request_number INTEGER,
			user_login      TEXT,
			user_id         BIGINT,
			body            TEXT NOT NULL DEFAULT '',
			reactions       JSONB DEFAULT '{}',
			html_url        TEXT,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_issue_comments_repo ON np_github_issue_comments (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_issue_comments_issue ON np_github_issue_comments (issue_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_issue_comments_source ON np_github_issue_comments (source_account_id);

		-- 8. PR Review Comments
		CREATE TABLE IF NOT EXISTS np_github_pr_review_comments (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			pull_request_id BIGINT,
			pull_request_number INTEGER NOT NULL DEFAULT 0,
			review_id       BIGINT,
			user_login      TEXT,
			user_id         BIGINT,
			body            TEXT NOT NULL DEFAULT '',
			path            TEXT,
			position        INTEGER,
			original_position INTEGER,
			diff_hunk       TEXT,
			commit_id       TEXT,
			original_commit_id TEXT,
			in_reply_to_id  BIGINT,
			reactions       JSONB DEFAULT '{}',
			html_url        TEXT,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_pr_review_comments_repo ON np_github_pr_review_comments (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_pr_review_comments_pr ON np_github_pr_review_comments (pull_request_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_pr_review_comments_source ON np_github_pr_review_comments (source_account_id);

		-- 9. Commit Comments
		CREATE TABLE IF NOT EXISTS np_github_commit_comments (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			commit_sha      TEXT NOT NULL,
			user_login      TEXT,
			user_id         BIGINT,
			body            TEXT NOT NULL DEFAULT '',
			path            TEXT,
			position        INTEGER,
			line            INTEGER,
			reactions       JSONB DEFAULT '{}',
			html_url        TEXT,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_commit_comments_repo ON np_github_commit_comments (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_commit_comments_commit ON np_github_commit_comments (commit_sha);
		CREATE INDEX IF NOT EXISTS idx_np_github_commit_comments_source ON np_github_commit_comments (source_account_id);

		-- 10. Commits
		CREATE TABLE IF NOT EXISTS np_github_commits (
			sha             TEXT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			message         TEXT,
			author_name     TEXT,
			author_email    TEXT,
			author_login    TEXT,
			author_date     TIMESTAMPTZ,
			committer_name  TEXT,
			committer_email TEXT,
			committer_login TEXT,
			committer_date  TIMESTAMPTZ,
			tree_sha        TEXT,
			parents         JSONB DEFAULT '[]',
			additions       INTEGER NOT NULL DEFAULT 0,
			deletions       INTEGER NOT NULL DEFAULT 0,
			total           INTEGER NOT NULL DEFAULT 0,
			html_url        TEXT,
			verified        BOOLEAN NOT NULL DEFAULT FALSE,
			verification_reason TEXT,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_commits_repo ON np_github_commits (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_commits_author ON np_github_commits (author_login);
		CREATE INDEX IF NOT EXISTS idx_np_github_commits_date ON np_github_commits (author_date);
		CREATE INDEX IF NOT EXISTS idx_np_github_commits_source ON np_github_commits (source_account_id);

		-- 11. Releases
		CREATE TABLE IF NOT EXISTS np_github_releases (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			tag_name        TEXT NOT NULL,
			target_commitish TEXT,
			name            TEXT,
			body            TEXT,
			draft           BOOLEAN NOT NULL DEFAULT FALSE,
			prerelease      BOOLEAN NOT NULL DEFAULT FALSE,
			author_login    TEXT,
			html_url        TEXT,
			tarball_url     TEXT,
			zipball_url     TEXT,
			assets          JSONB DEFAULT '[]',
			created_at      TIMESTAMPTZ,
			published_at    TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_releases_repo ON np_github_releases (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_releases_tag ON np_github_releases (tag_name);
		CREATE INDEX IF NOT EXISTS idx_np_github_releases_source ON np_github_releases (source_account_id);

		-- 12. Tags
		CREATE TABLE IF NOT EXISTS np_github_tags (
			id              TEXT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			repo_id         BIGINT,
			name            TEXT NOT NULL,
			sha             TEXT NOT NULL,
			message         TEXT,
			tagger_name     TEXT,
			tagger_email    TEXT,
			tagger_date     TIMESTAMPTZ,
			zipball_url     TEXT,
			tarball_url     TEXT,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_tags_repo ON np_github_tags (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_tags_source ON np_github_tags (source_account_id);

		-- 13. Milestones
		CREATE TABLE IF NOT EXISTS np_github_milestones (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			number          INTEGER NOT NULL,
			title           TEXT NOT NULL,
			description     TEXT,
			state           TEXT NOT NULL DEFAULT 'open',
			creator_login   TEXT,
			open_issues     INTEGER NOT NULL DEFAULT 0,
			closed_issues   INTEGER NOT NULL DEFAULT 0,
			html_url        TEXT,
			due_on          TIMESTAMPTZ,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			closed_at       TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_milestones_repo ON np_github_milestones (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_milestones_source ON np_github_milestones (source_account_id);

		-- 14. Labels
		CREATE TABLE IF NOT EXISTS np_github_labels (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			name            TEXT NOT NULL,
			color           TEXT NOT NULL DEFAULT '',
			description     TEXT,
			is_default      BOOLEAN NOT NULL DEFAULT FALSE,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_labels_repo ON np_github_labels (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_labels_source ON np_github_labels (source_account_id);

		-- 15. Workflows
		CREATE TABLE IF NOT EXISTS np_github_workflows (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			name            TEXT NOT NULL,
			path            TEXT NOT NULL DEFAULT '',
			state           TEXT NOT NULL DEFAULT 'active',
			badge_url       TEXT,
			html_url        TEXT,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_workflows_repo ON np_github_workflows (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_workflows_source ON np_github_workflows (source_account_id);

		-- 16. Workflow Runs
		CREATE TABLE IF NOT EXISTS np_github_workflow_runs (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			workflow_id     BIGINT,
			workflow_name   TEXT,
			name            TEXT,
			head_branch     TEXT,
			head_sha        TEXT,
			run_number      INTEGER,
			run_attempt     INTEGER DEFAULT 1,
			event           TEXT,
			status          TEXT,
			conclusion      TEXT,
			actor_login     TEXT,
			triggering_actor_login TEXT,
			html_url        TEXT,
			jobs_url        TEXT,
			logs_url        TEXT,
			run_started_at  TIMESTAMPTZ,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_repo ON np_github_workflow_runs (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_status ON np_github_workflow_runs (status);
		CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_conclusion ON np_github_workflow_runs (conclusion);
		CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_created ON np_github_workflow_runs (created_at);
		CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_source ON np_github_workflow_runs (source_account_id);

		-- 17. Workflow Jobs
		CREATE TABLE IF NOT EXISTS np_github_workflow_jobs (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			run_id          BIGINT,
			run_attempt     INTEGER,
			workflow_name   TEXT,
			name            TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT '',
			conclusion      TEXT,
			head_sha        TEXT,
			html_url        TEXT,
			runner_id       BIGINT,
			runner_name     TEXT,
			runner_group_id BIGINT,
			runner_group_name TEXT,
			labels          JSONB DEFAULT '[]',
			steps           JSONB DEFAULT '[]',
			started_at      TIMESTAMPTZ,
			completed_at    TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_workflow_jobs_repo ON np_github_workflow_jobs (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_workflow_jobs_run ON np_github_workflow_jobs (run_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_workflow_jobs_source ON np_github_workflow_jobs (source_account_id);

		-- 18. Check Suites
		CREATE TABLE IF NOT EXISTS np_github_check_suites (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			head_branch     TEXT,
			head_sha        TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT '',
			conclusion      TEXT,
			app_id          BIGINT,
			app_slug        TEXT,
			pull_requests   JSONB DEFAULT '[]',
			before_sha      TEXT,
			after_sha       TEXT,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_check_suites_repo ON np_github_check_suites (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_check_suites_source ON np_github_check_suites (source_account_id);

		-- 19. Check Runs
		CREATE TABLE IF NOT EXISTS np_github_check_runs (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			check_suite_id  BIGINT,
			head_sha        TEXT NOT NULL,
			name            TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT '',
			conclusion      TEXT,
			external_id     TEXT,
			html_url        TEXT,
			details_url     TEXT,
			app_id          BIGINT,
			app_slug        TEXT,
			output          JSONB,
			pull_requests   JSONB DEFAULT '[]',
			started_at      TIMESTAMPTZ,
			completed_at    TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_check_runs_repo ON np_github_check_runs (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_check_runs_suite ON np_github_check_runs (check_suite_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_check_runs_source ON np_github_check_runs (source_account_id);

		-- 20. Deployments
		CREATE TABLE IF NOT EXISTS np_github_deployments (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			repo_id         BIGINT,
			sha             TEXT,
			ref             TEXT,
			task            TEXT DEFAULT 'deploy',
			environment     TEXT,
			description     TEXT,
			creator_login   TEXT,
			statuses        JSONB DEFAULT '[]',
			current_status  TEXT,
			production_environment BOOLEAN NOT NULL DEFAULT FALSE,
			transient_environment BOOLEAN NOT NULL DEFAULT FALSE,
			payload         JSONB DEFAULT '{}',
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_deployments_repo ON np_github_deployments (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_deployments_env ON np_github_deployments (environment);
		CREATE INDEX IF NOT EXISTS idx_np_github_deployments_source ON np_github_deployments (source_account_id);

		-- 21. Teams
		CREATE TABLE IF NOT EXISTS np_github_teams (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			node_id         TEXT,
			org_login       TEXT NOT NULL,
			name            TEXT NOT NULL,
			slug            TEXT NOT NULL,
			description     TEXT,
			privacy         TEXT,
			permission      TEXT,
			parent_id       BIGINT,
			members_count   INTEGER NOT NULL DEFAULT 0,
			repos_count     INTEGER NOT NULL DEFAULT 0,
			html_url        TEXT,
			created_at      TIMESTAMPTZ,
			updated_at      TIMESTAMPTZ,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_teams_org ON np_github_teams (org_login);
		CREATE INDEX IF NOT EXISTS idx_np_github_teams_source ON np_github_teams (source_account_id);

		-- 22. Collaborators
		CREATE TABLE IF NOT EXISTS np_github_collaborators (
			id              BIGINT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			repo_id         BIGINT,
			login           TEXT NOT NULL,
			type            TEXT,
			site_admin      BOOLEAN NOT NULL DEFAULT FALSE,
			permissions     JSONB DEFAULT '{}',
			role_name       TEXT,
			synced_at       TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_collaborators_repo ON np_github_collaborators (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_collaborators_login ON np_github_collaborators (login);
		CREATE INDEX IF NOT EXISTS idx_np_github_collaborators_source ON np_github_collaborators (source_account_id);

		-- 23. Webhook Events
		CREATE TABLE IF NOT EXISTS np_github_webhook_events (
			id              TEXT PRIMARY KEY,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			event           TEXT NOT NULL,
			action          TEXT,
			repo_id         BIGINT,
			repo_full_name  TEXT,
			sender_login    TEXT,
			data            JSONB NOT NULL DEFAULT '{}',
			processed       BOOLEAN NOT NULL DEFAULT FALSE,
			processed_at    TIMESTAMPTZ,
			error           TEXT,
			received_at     TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_np_github_webhook_events_event ON np_github_webhook_events (event);
		CREATE INDEX IF NOT EXISTS idx_np_github_webhook_events_repo ON np_github_webhook_events (repo_id);
		CREATE INDEX IF NOT EXISTS idx_np_github_webhook_events_processed ON np_github_webhook_events (processed);
		CREATE INDEX IF NOT EXISTS idx_np_github_webhook_events_received ON np_github_webhook_events (received_at);
		CREATE INDEX IF NOT EXISTS idx_np_github_webhook_events_source ON np_github_webhook_events (source_account_id);
	`)
	return err
}

// --- Upsert functions --------------------------------------------------------

// UpsertOrganization inserts or updates an organization record.
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
func UpsertIssue(ctx context.Context, pool *pgxpool.Pool, i *Issue) error {
	labels := defaultJSONB(i.Labels)
	assignees := defaultJSONB(i.Assignees)
	reactions := defaultJSONB(i.Reactions)
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_issues (
			id, source_account_id, node_id, repo_id, number, title, body, state,
			state_reason, locked, user_login, user_id, labels, assignees, milestone,
			comments, reactions, html_url, closed_at, closed_by_login,
			created_at, updated_at, synced_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			node_id = EXCLUDED.node_id,
			repo_id = EXCLUDED.repo_id,
			title = EXCLUDED.title,
			body = EXCLUDED.body,
			state = EXCLUDED.state,
			state_reason = EXCLUDED.state_reason,
			locked = EXCLUDED.locked,
			user_login = EXCLUDED.user_login,
			labels = EXCLUDED.labels,
			assignees = EXCLUDED.assignees,
			milestone = EXCLUDED.milestone,
			comments = EXCLUDED.comments,
			reactions = EXCLUDED.reactions,
			html_url = EXCLUDED.html_url,
			closed_at = EXCLUDED.closed_at,
			closed_by_login = EXCLUDED.closed_by_login,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, i.ID, i.SourceAccountID, i.NodeID, i.RepoID, i.Number, i.Title,
		i.Body, i.State, i.StateReason, i.Locked, i.UserLogin, i.UserID,
		labels, assignees, i.Milestone, i.Comments, reactions, i.HTMLURL,
		i.ClosedAt, i.ClosedByLogin, i.CreatedAt, i.UpdatedAt)
	return err
}

// UpsertPullRequest inserts or updates a pull request record.
func UpsertPullRequest(ctx context.Context, pool *pgxpool.Pool, p *PullRequest) error {
	labels := defaultJSONB(p.Labels)
	assignees := defaultJSONB(p.Assignees)
	reviewers := defaultJSONB(p.Reviewers)
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_pull_requests (
			id, source_account_id, node_id, repo_id, number, title, body, state,
			draft, locked, user_login, user_id, head_ref, head_sha, head_repo_id,
			base_ref, base_sha, merged, mergeable, mergeable_state, merged_by_login,
			merged_at, merge_commit_sha, labels, assignees, reviewers, milestone,
			comments, review_comments, commits, additions, deletions, changed_files,
			html_url, diff_url, closed_at, created_at, updated_at, synced_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27,
			$28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			node_id = EXCLUDED.node_id,
			repo_id = EXCLUDED.repo_id,
			title = EXCLUDED.title,
			body = EXCLUDED.body,
			state = EXCLUDED.state,
			draft = EXCLUDED.draft,
			locked = EXCLUDED.locked,
			user_login = EXCLUDED.user_login,
			head_ref = EXCLUDED.head_ref,
			head_sha = EXCLUDED.head_sha,
			base_ref = EXCLUDED.base_ref,
			base_sha = EXCLUDED.base_sha,
			merged = EXCLUDED.merged,
			mergeable = EXCLUDED.mergeable,
			mergeable_state = EXCLUDED.mergeable_state,
			merged_by_login = EXCLUDED.merged_by_login,
			merged_at = EXCLUDED.merged_at,
			merge_commit_sha = EXCLUDED.merge_commit_sha,
			labels = EXCLUDED.labels,
			assignees = EXCLUDED.assignees,
			reviewers = EXCLUDED.reviewers,
			milestone = EXCLUDED.milestone,
			comments = EXCLUDED.comments,
			review_comments = EXCLUDED.review_comments,
			commits = EXCLUDED.commits,
			additions = EXCLUDED.additions,
			deletions = EXCLUDED.deletions,
			changed_files = EXCLUDED.changed_files,
			html_url = EXCLUDED.html_url,
			diff_url = EXCLUDED.diff_url,
			closed_at = EXCLUDED.closed_at,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, p.ID, p.SourceAccountID, p.NodeID, p.RepoID, p.Number, p.Title,
		p.Body, p.State, p.Draft, p.Locked, p.UserLogin, p.UserID,
		p.HeadRef, p.HeadSHA, p.HeadRepoID, p.BaseRef, p.BaseSHA,
		p.Merged, p.Mergeable, p.MergeableState, p.MergedByLogin,
		p.MergedAt, p.MergeCommitSHA, labels, assignees, reviewers,
		p.MilestonePR, p.CommentCount, p.ReviewComments, p.Commits,
		p.Additions, p.Deletions, p.ChangedFiles, p.HTMLURL, p.DiffURL,
		p.ClosedAt, p.CreatedAt, p.UpdatedAt)
	return err
}

// UpsertCommit inserts or updates a commit record.
func UpsertCommit(ctx context.Context, pool *pgxpool.Pool, c *Commit) error {
	parents := defaultJSONB(c.Parents)
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_commits (
			sha, source_account_id, node_id, repo_id, message, author_name,
			author_email, author_login, author_date, committer_name, committer_email,
			committer_login, committer_date, tree_sha, parents, additions, deletions,
			total, html_url, verified, verification_reason, synced_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, NOW()
		)
		ON CONFLICT (sha) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			node_id = EXCLUDED.node_id,
			repo_id = EXCLUDED.repo_id,
			message = EXCLUDED.message,
			author_name = EXCLUDED.author_name,
			author_email = EXCLUDED.author_email,
			author_login = EXCLUDED.author_login,
			committer_name = EXCLUDED.committer_name,
			committer_email = EXCLUDED.committer_email,
			committer_login = EXCLUDED.committer_login,
			tree_sha = EXCLUDED.tree_sha,
			parents = EXCLUDED.parents,
			additions = EXCLUDED.additions,
			deletions = EXCLUDED.deletions,
			total = EXCLUDED.total,
			html_url = EXCLUDED.html_url,
			verified = EXCLUDED.verified,
			verification_reason = EXCLUDED.verification_reason,
			synced_at = NOW()
	`, c.SHA, c.SourceAccountID, c.NodeID, c.RepoID, c.Message,
		c.AuthorName, c.AuthorEmail, c.AuthorLogin, c.AuthorDate,
		c.CommitterName, c.CommitterEmail, c.CommitterLogin, c.CommitterDate,
		c.TreeSHA, parents, c.CommitAdditions, c.CommitDeletions, c.Total,
		c.HTMLURL, c.Verified, c.VerificationReason)
	return err
}

// UpsertRelease inserts or updates a release record.
func UpsertRelease(ctx context.Context, pool *pgxpool.Pool, r *Release) error {
	assets := defaultJSONB(r.Assets)
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_releases (
			id, source_account_id, node_id, repo_id, tag_name, target_commitish,
			name, body, draft, prerelease, author_login, html_url, tarball_url,
			zipball_url, assets, created_at, published_at, synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			tag_name = EXCLUDED.tag_name,
			target_commitish = EXCLUDED.target_commitish,
			name = EXCLUDED.name,
			body = EXCLUDED.body,
			draft = EXCLUDED.draft,
			prerelease = EXCLUDED.prerelease,
			author_login = EXCLUDED.author_login,
			html_url = EXCLUDED.html_url,
			tarball_url = EXCLUDED.tarball_url,
			zipball_url = EXCLUDED.zipball_url,
			assets = EXCLUDED.assets,
			published_at = EXCLUDED.published_at,
			synced_at = NOW()
	`, r.ID, r.SourceAccountID, r.NodeID, r.RepoID, r.TagName,
		r.TargetCommitish, r.Name, r.Body, r.Draft, r.Prerelease,
		r.AuthorLogin, r.HTMLURL, r.TarballURL, r.ZipballURL,
		assets, r.CreatedAt, r.PublishedAt)
	return err
}

// UpsertWorkflow inserts or updates a workflow record.
func UpsertWorkflow(ctx context.Context, pool *pgxpool.Pool, w *Workflow) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_workflows (
			id, source_account_id, node_id, repo_id, name, path, state,
			badge_url, html_url, created_at, updated_at, synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			node_id = EXCLUDED.node_id,
			repo_id = EXCLUDED.repo_id,
			name = EXCLUDED.name,
			path = EXCLUDED.path,
			state = EXCLUDED.state,
			badge_url = EXCLUDED.badge_url,
			html_url = EXCLUDED.html_url,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, w.ID, w.SourceAccountID, w.NodeID, w.RepoID, w.Name, w.Path,
		w.State, w.BadgeURL, w.HTMLURL, w.CreatedAt, w.UpdatedAt)
	return err
}

// UpsertWorkflowRun inserts or updates a workflow run record.
func UpsertWorkflowRun(ctx context.Context, pool *pgxpool.Pool, r *WorkflowRun) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_workflow_runs (
			id, source_account_id, node_id, repo_id, workflow_id, workflow_name,
			name, head_branch, head_sha, run_number, run_attempt, event, status,
			conclusion, actor_login, triggering_actor_login, html_url, jobs_url,
			logs_url, run_started_at, created_at, updated_at, synced_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19, $20, $21, $22, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			node_id = EXCLUDED.node_id,
			repo_id = EXCLUDED.repo_id,
			workflow_id = EXCLUDED.workflow_id,
			workflow_name = EXCLUDED.workflow_name,
			name = EXCLUDED.name,
			head_branch = EXCLUDED.head_branch,
			head_sha = EXCLUDED.head_sha,
			run_number = EXCLUDED.run_number,
			run_attempt = EXCLUDED.run_attempt,
			event = EXCLUDED.event,
			status = EXCLUDED.status,
			conclusion = EXCLUDED.conclusion,
			actor_login = EXCLUDED.actor_login,
			triggering_actor_login = EXCLUDED.triggering_actor_login,
			html_url = EXCLUDED.html_url,
			jobs_url = EXCLUDED.jobs_url,
			logs_url = EXCLUDED.logs_url,
			run_started_at = EXCLUDED.run_started_at,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, r.ID, r.SourceAccountID, r.NodeID, r.RepoID, r.WorkflowID,
		r.WorkflowName, r.Name, r.HeadBranch, r.HeadSHA, r.RunNumber,
		r.RunAttempt, r.Event, r.Status, r.Conclusion, r.ActorLogin,
		r.TriggeringActorLogin, r.HTMLURL, r.JobsURL, r.LogsURL,
		r.RunStartedAt, r.CreatedAt, r.UpdatedAt)
	return err
}

// UpsertDeployment inserts or updates a deployment record.
func UpsertDeployment(ctx context.Context, pool *pgxpool.Pool, d *Deployment) error {
	statuses := defaultJSONB(d.Statuses)
	payload := defaultJSONB(d.Payload)
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_deployments (
			id, source_account_id, node_id, repo_id, sha, ref, task, environment,
			description, creator_login, statuses, current_status,
			production_environment, transient_environment, payload,
			created_at, updated_at, synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			sha = EXCLUDED.sha,
			ref = EXCLUDED.ref,
			task = EXCLUDED.task,
			environment = EXCLUDED.environment,
			description = EXCLUDED.description,
			creator_login = EXCLUDED.creator_login,
			statuses = EXCLUDED.statuses,
			current_status = EXCLUDED.current_status,
			production_environment = EXCLUDED.production_environment,
			transient_environment = EXCLUDED.transient_environment,
			payload = EXCLUDED.payload,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, d.ID, d.SourceAccountID, d.NodeID, d.RepoID, d.SHA, d.Ref,
		d.Task, d.Environment, d.Description, d.CreatorLogin, statuses,
		d.CurrentStatus, d.ProductionEnvironment, d.TransientEnvironment,
		payload, d.CreatedAt, d.UpdatedAt)
	return err
}

// UpsertTeam inserts or updates a team record.
func UpsertTeam(ctx context.Context, pool *pgxpool.Pool, t *Team) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_teams (
			id, source_account_id, node_id, org_login, name, slug, description,
			privacy, permission, parent_id, members_count, repos_count, html_url,
			created_at, updated_at, synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			org_login = EXCLUDED.org_login,
			name = EXCLUDED.name,
			slug = EXCLUDED.slug,
			description = EXCLUDED.description,
			privacy = EXCLUDED.privacy,
			permission = EXCLUDED.permission,
			parent_id = EXCLUDED.parent_id,
			members_count = EXCLUDED.members_count,
			repos_count = EXCLUDED.repos_count,
			html_url = EXCLUDED.html_url,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, t.ID, t.SourceAccountID, t.NodeID, t.OrgLogin, t.Name, t.Slug,
		t.Description, t.Privacy, t.Permission, t.ParentID,
		t.MembersCount, t.ReposCount, t.HTMLURL, t.CreatedAt, t.UpdatedAt)
	return err
}

// UpsertCollaborator inserts or updates a collaborator record.
func UpsertCollaborator(ctx context.Context, pool *pgxpool.Pool, c *Collaborator) error {
	permissions := defaultJSONB(c.Permissions)
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_collaborators (
			id, source_account_id, repo_id, login, type, site_admin,
			permissions, role_name, synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			repo_id = EXCLUDED.repo_id,
			login = EXCLUDED.login,
			type = EXCLUDED.type,
			site_admin = EXCLUDED.site_admin,
			permissions = EXCLUDED.permissions,
			role_name = EXCLUDED.role_name,
			synced_at = NOW()
	`, c.ID, c.SourceAccountID, c.RepoID, c.Login, c.Type,
		c.SiteAdmin, permissions, c.RoleName)
	return err
}

// InsertWebhookEvent inserts a webhook event record.
func InsertWebhookEvent(ctx context.Context, pool *pgxpool.Pool, e *WebhookEvent) error {
	data := defaultJSONB(e.Data)
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_webhook_events (
			id, source_account_id, event, action, repo_id, repo_full_name,
			sender_login, data, processed, processed_at, error, received_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, COALESCE($12, NOW()))
		ON CONFLICT (id) DO NOTHING
	`, e.ID, e.SourceAccountID, e.Event, e.Action, e.RepoID, e.RepoFullName,
		e.SenderLogin, data, e.Processed, e.ProcessedAt, e.Error, e.ReceivedAt)
	return err
}

// --- Query functions ---------------------------------------------------------

// ListRepositories returns repositories with pagination.
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
func ListIssues(ctx context.Context, pool *pgxpool.Pool, repoID *int64, state string, limit, offset int) ([]Issue, error) {
	query := `SELECT id, source_account_id, node_id, repo_id, number, title, body,
		state, state_reason, locked, user_login, user_id, labels, assignees, milestone,
		comments, reactions, html_url, closed_at, closed_by_login, created_at, updated_at, synced_at
		FROM np_github_issues WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if repoID != nil {
		query += fmt.Sprintf(" AND repo_id = $%d", argIdx)
		args = append(args, *repoID)
		argIdx++
	}
	if state != "" {
		query += fmt.Sprintf(" AND state = $%d", argIdx)
		args = append(args, state)
		argIdx++
	}

	query += " ORDER BY created_at DESC NULLS LAST"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Issue
	for rows.Next() {
		var i Issue
		if err := rows.Scan(
			&i.ID, &i.SourceAccountID, &i.NodeID, &i.RepoID, &i.Number,
			&i.Title, &i.Body, &i.State, &i.StateReason, &i.Locked,
			&i.UserLogin, &i.UserID, &i.Labels, &i.Assignees, &i.Milestone,
			&i.Comments, &i.Reactions, &i.HTMLURL, &i.ClosedAt,
			&i.ClosedByLogin, &i.CreatedAt, &i.UpdatedAt, &i.SyncedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, i)
	}
	return results, rows.Err()
}

// GetIssue returns a single issue by ID.
func GetIssue(ctx context.Context, pool *pgxpool.Pool, id int64) (*Issue, error) {
	var i Issue
	err := pool.QueryRow(ctx, `
		SELECT id, source_account_id, node_id, repo_id, number, title, body,
			state, state_reason, locked, user_login, user_id, labels, assignees, milestone,
			comments, reactions, html_url, closed_at, closed_by_login, created_at, updated_at, synced_at
		FROM np_github_issues WHERE id = $1
	`, id).Scan(
		&i.ID, &i.SourceAccountID, &i.NodeID, &i.RepoID, &i.Number,
		&i.Title, &i.Body, &i.State, &i.StateReason, &i.Locked,
		&i.UserLogin, &i.UserID, &i.Labels, &i.Assignees, &i.Milestone,
		&i.Comments, &i.Reactions, &i.HTMLURL, &i.ClosedAt,
		&i.ClosedByLogin, &i.CreatedAt, &i.UpdatedAt, &i.SyncedAt,
	)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// ListPullRequests returns pull requests with optional filtering and pagination.
func ListPullRequests(ctx context.Context, pool *pgxpool.Pool, repoID *int64, state string, limit, offset int) ([]PullRequest, error) {
	query := `SELECT id, source_account_id, node_id, repo_id, number, title, body,
		state, draft, locked, user_login, user_id, head_ref, head_sha, head_repo_id,
		base_ref, base_sha, merged, mergeable, mergeable_state, merged_by_login,
		merged_at, merge_commit_sha, labels, assignees, reviewers, milestone,
		comments, review_comments, commits, additions, deletions, changed_files,
		html_url, diff_url, closed_at, created_at, updated_at, synced_at
		FROM np_github_pull_requests WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if repoID != nil {
		query += fmt.Sprintf(" AND repo_id = $%d", argIdx)
		args = append(args, *repoID)
		argIdx++
	}
	if state != "" {
		query += fmt.Sprintf(" AND state = $%d", argIdx)
		args = append(args, state)
		argIdx++
	}

	query += " ORDER BY created_at DESC NULLS LAST"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PullRequest
	for rows.Next() {
		var p PullRequest
		if err := rows.Scan(
			&p.ID, &p.SourceAccountID, &p.NodeID, &p.RepoID, &p.Number,
			&p.Title, &p.Body, &p.State, &p.Draft, &p.Locked, &p.UserLogin,
			&p.UserID, &p.HeadRef, &p.HeadSHA, &p.HeadRepoID, &p.BaseRef,
			&p.BaseSHA, &p.Merged, &p.Mergeable, &p.MergeableState,
			&p.MergedByLogin, &p.MergedAt, &p.MergeCommitSHA, &p.Labels,
			&p.Assignees, &p.Reviewers, &p.MilestonePR, &p.CommentCount,
			&p.ReviewComments, &p.Commits, &p.Additions, &p.Deletions,
			&p.ChangedFiles, &p.HTMLURL, &p.DiffURL, &p.ClosedAt,
			&p.CreatedAt, &p.UpdatedAt, &p.SyncedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// GetPullRequest returns a single pull request by ID.
func GetPullRequest(ctx context.Context, pool *pgxpool.Pool, id int64) (*PullRequest, error) {
	var p PullRequest
	err := pool.QueryRow(ctx, `
		SELECT id, source_account_id, node_id, repo_id, number, title, body,
			state, draft, locked, user_login, user_id, head_ref, head_sha, head_repo_id,
			base_ref, base_sha, merged, mergeable, mergeable_state, merged_by_login,
			merged_at, merge_commit_sha, labels, assignees, reviewers, milestone,
			comments, review_comments, commits, additions, deletions, changed_files,
			html_url, diff_url, closed_at, created_at, updated_at, synced_at
		FROM np_github_pull_requests WHERE id = $1
	`, id).Scan(
		&p.ID, &p.SourceAccountID, &p.NodeID, &p.RepoID, &p.Number,
		&p.Title, &p.Body, &p.State, &p.Draft, &p.Locked, &p.UserLogin,
		&p.UserID, &p.HeadRef, &p.HeadSHA, &p.HeadRepoID, &p.BaseRef,
		&p.BaseSHA, &p.Merged, &p.Mergeable, &p.MergeableState,
		&p.MergedByLogin, &p.MergedAt, &p.MergeCommitSHA, &p.Labels,
		&p.Assignees, &p.Reviewers, &p.MilestonePR, &p.CommentCount,
		&p.ReviewComments, &p.Commits, &p.Additions, &p.Deletions,
		&p.ChangedFiles, &p.HTMLURL, &p.DiffURL, &p.ClosedAt,
		&p.CreatedAt, &p.UpdatedAt, &p.SyncedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListWebhookEvents returns webhook events with pagination.
func ListWebhookEvents(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]WebhookEvent, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, source_account_id, event, action, repo_id, repo_full_name,
			sender_login, data, processed, processed_at, error, received_at
		FROM np_github_webhook_events
		ORDER BY received_at DESC NULLS LAST
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []WebhookEvent
	for rows.Next() {
		var e WebhookEvent
		if err := rows.Scan(
			&e.ID, &e.SourceAccountID, &e.Event, &e.Action, &e.RepoID,
			&e.RepoFullName, &e.SenderLogin, &e.Data, &e.Processed,
			&e.ProcessedAt, &e.Error, &e.ReceivedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// GetSyncStats returns counts for all synced entity types.
func GetSyncStats(ctx context.Context, pool *pgxpool.Pool) (*SyncStats, error) {
	var s SyncStats
	err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM np_github_repositories),
			(SELECT COUNT(*) FROM np_github_branches),
			(SELECT COUNT(*) FROM np_github_issues),
			(SELECT COUNT(*) FROM np_github_pull_requests),
			(SELECT COUNT(*) FROM np_github_pr_reviews),
			(SELECT COUNT(*) FROM np_github_issue_comments),
			(SELECT COUNT(*) FROM np_github_pr_review_comments),
			(SELECT COUNT(*) FROM np_github_commit_comments),
			(SELECT COUNT(*) FROM np_github_commits),
			(SELECT COUNT(*) FROM np_github_releases),
			(SELECT COUNT(*) FROM np_github_tags),
			(SELECT COUNT(*) FROM np_github_milestones),
			(SELECT COUNT(*) FROM np_github_labels),
			(SELECT COUNT(*) FROM np_github_workflows),
			(SELECT COUNT(*) FROM np_github_workflow_runs),
			(SELECT COUNT(*) FROM np_github_workflow_jobs),
			(SELECT COUNT(*) FROM np_github_check_suites),
			(SELECT COUNT(*) FROM np_github_check_runs),
			(SELECT COUNT(*) FROM np_github_deployments),
			(SELECT COUNT(*) FROM np_github_teams),
			(SELECT COUNT(*) FROM np_github_collaborators),
			(SELECT MAX(synced_at) FROM np_github_repositories)
	`).Scan(
		&s.Repositories, &s.Branches, &s.Issues, &s.PullRequests,
		&s.PRReviews, &s.IssueComments, &s.PRReviewComments,
		&s.CommitComments, &s.Commits, &s.Releases, &s.Tags,
		&s.Milestones, &s.Labels, &s.Workflows, &s.WorkflowRuns,
		&s.WorkflowJobs, &s.CheckSuites, &s.CheckRuns, &s.Deployments,
		&s.Teams, &s.Collaborators, &s.LastSyncedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// --- Helpers -----------------------------------------------------------------

// defaultJSONB returns a default JSON value if the input is nil.
func defaultJSONB(v *json.RawMessage) json.RawMessage {
	if v == nil || len(*v) == 0 {
		return json.RawMessage("{}")
	}
	return *v
}
