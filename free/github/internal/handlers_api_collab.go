package internal

import (
	"fmt"
	"net/http"
)

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT id, source_account_id, node_id, org_login, name, slug, description,
			privacy, permission, parent_id, members_count, repos_count, html_url,
			created_at, updated_at, synced_at
		FROM np_github_teams ORDER BY name LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list teams", err)
		return
	}
	defer rows.Close()

	var teams []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.SourceAccountID, &t.NodeID, &t.OrgLogin, &t.Name,
			&t.Slug, &t.Description, &t.Privacy, &t.Permission, &t.ParentID,
			&t.MembersCount, &t.ReposCount, &t.HTMLURL, &t.CreatedAt,
			&t.UpdatedAt, &t.SyncedAt); err != nil {
			writeErr(w, "Failed to scan team", err)
			return
		}
		teams = append(teams, t)
	}
	total, _ := s.db.CountTeams(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: teams, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListCollaborators(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)
	repoID := parseOptionalInt64(r, "repo_id")

	query := `SELECT id, source_account_id, repo_id, login, type, site_admin, permissions, role_name, synced_at
		FROM np_github_collaborators WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if repoID != nil {
		query += fmt.Sprintf(" AND repo_id = $%d", argIdx)
		args = append(args, *repoID)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY login LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		writeErr(w, "Failed to list collaborators", err)
		return
	}
	defer rows.Close()

	var collaborators []Collaborator
	for rows.Next() {
		var c Collaborator
		if err := rows.Scan(&c.ID, &c.SourceAccountID, &c.RepoID, &c.Login, &c.Type,
			&c.SiteAdmin, &c.Permissions, &c.RoleName, &c.SyncedAt); err != nil {
			writeErr(w, "Failed to scan collaborator", err)
			return
		}
		collaborators = append(collaborators, c)
	}
	total, _ := s.db.CountCollaborators(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: collaborators, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListPRReviews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)
	prID := parseOptionalInt64(r, "pr_id")

	query := `SELECT id, source_account_id, node_id, repo_id, pull_request_id,
		pull_request_number, user_login, user_id, body, state, html_url,
		commit_id, submitted_at, synced_at
		FROM np_github_pr_reviews WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if prID != nil {
		query += fmt.Sprintf(" AND pull_request_id = $%d", argIdx)
		args = append(args, *prID)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY submitted_at DESC NULLS LAST LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		writeErr(w, "Failed to list PR reviews", err)
		return
	}
	defer rows.Close()

	var reviews []PRReview
	for rows.Next() {
		var rev PRReview
		if err := rows.Scan(&rev.ID, &rev.SourceAccountID, &rev.NodeID, &rev.RepoID,
			&rev.PullRequestID, &rev.PullRequestNumber, &rev.UserLogin, &rev.UserID,
			&rev.Body, &rev.State, &rev.HTMLURL, &rev.CommitID,
			&rev.SubmittedAt, &rev.SyncedAt); err != nil {
			writeErr(w, "Failed to scan PR review", err)
			return
		}
		reviews = append(reviews, rev)
	}
	total, _ := s.db.CountPRReviews(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: reviews, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListIssueComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT id, source_account_id, node_id, repo_id, issue_number, issue_id,
			pull_request_number, user_login, user_id, body, reactions, html_url,
			created_at, updated_at, synced_at
		FROM np_github_issue_comments ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list issue comments", err)
		return
	}
	defer rows.Close()

	var comments []IssueComment
	for rows.Next() {
		var c IssueComment
		if err := rows.Scan(&c.ID, &c.SourceAccountID, &c.NodeID, &c.RepoID, &c.IssueNumber,
			&c.IssueID, &c.PullRequestNumber, &c.UserLogin, &c.UserID, &c.Body,
			&c.Reactions, &c.HTMLURL, &c.CreatedAt, &c.UpdatedAt, &c.SyncedAt); err != nil {
			writeErr(w, "Failed to scan issue comment", err)
			return
		}
		comments = append(comments, c)
	}
	total, _ := s.db.CountIssueComments(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: comments, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListPRReviewComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT id, source_account_id, node_id, repo_id, pull_request_id, pull_request_number,
			review_id, user_login, user_id, body, path, position, original_position,
			diff_hunk, commit_id, original_commit_id, in_reply_to_id, reactions, html_url,
			created_at, updated_at, synced_at
		FROM np_github_pr_review_comments ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list PR review comments", err)
		return
	}
	defer rows.Close()

	var comments []PRReviewComment
	for rows.Next() {
		var c PRReviewComment
		if err := rows.Scan(&c.ID, &c.SourceAccountID, &c.NodeID, &c.RepoID,
			&c.PullRequestID, &c.PullRequestNumber, &c.ReviewID, &c.UserLogin,
			&c.UserID, &c.Body, &c.Path, &c.Position, &c.OriginalPosition,
			&c.DiffHunk, &c.CommitID, &c.OriginalCommitID, &c.InReplyToID,
			&c.Reactions, &c.HTMLURL, &c.CreatedAt, &c.UpdatedAt, &c.SyncedAt); err != nil {
			writeErr(w, "Failed to scan PR review comment", err)
			return
		}
		comments = append(comments, c)
	}
	total, _ := s.db.CountPRReviewComments(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: comments, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListCommitComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT id, source_account_id, node_id, repo_id, commit_sha, user_login, user_id,
			body, path, position, line, reactions, html_url, created_at, updated_at, synced_at
		FROM np_github_commit_comments ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list commit comments", err)
		return
	}
	defer rows.Close()

	var comments []CommitComment
	for rows.Next() {
		var c CommitComment
		if err := rows.Scan(&c.ID, &c.SourceAccountID, &c.NodeID, &c.RepoID, &c.CommitSHA,
			&c.UserLogin, &c.UserID, &c.Body, &c.Path, &c.Position, &c.Line,
			&c.Reactions, &c.HTMLURL, &c.CreatedAt, &c.UpdatedAt, &c.SyncedAt); err != nil {
			writeErr(w, "Failed to scan commit comment", err)
			return
		}
		comments = append(comments, c)
	}
	total, _ := s.db.CountCommitComments(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: comments, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)
	eventFilter := r.URL.Query().Get("event")

	query := `SELECT id, source_account_id, event, action, repo_id, repo_full_name,
		sender_login, data, processed, processed_at, error, received_at
		FROM np_github_webhook_events WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if eventFilter != "" {
		query += fmt.Sprintf(" AND event = $%d", argIdx)
		args = append(args, eventFilter)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY received_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		writeErr(w, "Failed to list events", err)
		return
	}
	defer rows.Close()

	var events []WebhookEvent
	for rows.Next() {
		var e WebhookEvent
		if err := rows.Scan(&e.ID, &e.SourceAccountID, &e.Event, &e.Action, &e.RepoID,
			&e.RepoFullName, &e.SenderLogin, &e.Data, &e.Processed, &e.ProcessedAt,
			&e.Error, &e.ReceivedAt); err != nil {
			writeErr(w, "Failed to scan event", err)
			return
		}
		events = append(events, e)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":   events,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := s.db.GetStats(ctx)
	if err != nil {
		writeErr(w, "Failed to get stats", err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}


