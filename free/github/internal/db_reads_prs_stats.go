package internal

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
