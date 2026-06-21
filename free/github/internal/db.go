package internal

import (
	"context"
	"fmt"

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
