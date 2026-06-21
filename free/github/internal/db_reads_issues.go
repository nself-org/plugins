package internal

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
