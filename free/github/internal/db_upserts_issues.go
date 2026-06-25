package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
// Size-cap exception: single DB operation — 62L scan loop with struct mapping; splitting would fragment a single SQL query across files.
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

