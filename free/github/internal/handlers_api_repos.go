package internal

import (
	"fmt"
	"net/http"
	"net/url"
	"github.com/go-chi/chi/v5"
)

// --- API endpoints -----------------------------------------------------------

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	repos, err := s.db.ListRepositories(ctx, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list repositories", err)
		return
	}
	total, _ := s.db.CountRepositories(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: repos, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fullName := chi.URLParam(r, "fullName")
	decoded, err := url.PathUnescape(fullName)
	if err != nil {
		decoded = fullName
	}

	repo, err := s.db.GetRepositoryByFullName(ctx, decoded)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Repository not found"})
		return
	}

	writeJSON(w, http.StatusOK, repo)
}

func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)
	state := r.URL.Query().Get("state")
	repoID := parseOptionalInt64(r, "repo_id")

	issues, err := ListIssues(ctx, s.pool, repoID, state, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list issues", err)
		return
	}
	total, _ := s.db.CountIssues(ctx, state)

	writeJSON(w, http.StatusOK, ListResponse{Data: issues, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListPRs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)
	state := r.URL.Query().Get("state")
	repoID := parseOptionalInt64(r, "repo_id")

	prs, err := ListPullRequests(ctx, s.pool, repoID, state, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list pull requests", err)
		return
	}
	total, _ := s.db.CountPullRequests(ctx, state)

	writeJSON(w, http.StatusOK, ListResponse{Data: prs, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListCommits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT sha, source_account_id, node_id, repo_id, message, author_name, author_email,
			author_login, author_date, committer_name, committer_email, committer_login,
			committer_date, tree_sha, parents, additions, deletions, total,
			html_url, verified, verification_reason, synced_at
		FROM np_github_commits
		ORDER BY author_date DESC NULLS LAST
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list commits", err)
		return
	}
	defer rows.Close()

	var commits []Commit
	for rows.Next() {
		var c Commit
		if err := rows.Scan(
			&c.SHA, &c.SourceAccountID, &c.NodeID, &c.RepoID, &c.Message,
			&c.AuthorName, &c.AuthorEmail, &c.AuthorLogin, &c.AuthorDate,
			&c.CommitterName, &c.CommitterEmail, &c.CommitterLogin, &c.CommitterDate,
			&c.TreeSHA, &c.Parents, &c.CommitAdditions, &c.CommitDeletions, &c.Total,
			&c.HTMLURL, &c.Verified, &c.VerificationReason, &c.SyncedAt,
		); err != nil {
			writeErr(w, "Failed to scan commit", err)
			return
		}
		commits = append(commits, c)
	}
	total, _ := s.db.CountCommits(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: commits, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListReleases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT id, source_account_id, node_id, repo_id, tag_name, target_commitish,
			name, body, draft, prerelease, author_login, html_url, tarball_url,
			zipball_url, assets, created_at, published_at, synced_at
		FROM np_github_releases
		ORDER BY published_at DESC NULLS LAST
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list releases", err)
		return
	}
	defer rows.Close()

	var releases []Release
	for rows.Next() {
		var rel Release
		if err := rows.Scan(
			&rel.ID, &rel.SourceAccountID, &rel.NodeID, &rel.RepoID,
			&rel.TagName, &rel.TargetCommitish, &rel.Name, &rel.Body,
			&rel.Draft, &rel.Prerelease, &rel.AuthorLogin, &rel.HTMLURL,
			&rel.TarballURL, &rel.ZipballURL, &rel.Assets,
			&rel.CreatedAt, &rel.PublishedAt, &rel.SyncedAt,
		); err != nil {
			writeErr(w, "Failed to scan release", err)
			return
		}
		releases = append(releases, rel)
	}
	total, _ := s.db.CountReleases(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: releases, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListBranches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)
	repoID := parseOptionalInt64(r, "repo_id")

	query := `SELECT id, source_account_id, repo_id, name, sha, protected, protection_enabled, protection, updated_at
		FROM np_github_branches WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if repoID != nil {
		query += fmt.Sprintf(" AND repo_id = $%d", argIdx)
		args = append(args, *repoID)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY name LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		writeErr(w, "Failed to list branches", err)
		return
	}
	defer rows.Close()

	var branches []Branch
	for rows.Next() {
		var b Branch
		if err := rows.Scan(&b.ID, &b.SourceAccountID, &b.RepoID, &b.Name, &b.SHA,
			&b.Protected, &b.ProtectionEnabled, &b.Protection, &b.UpdatedAt); err != nil {
			writeErr(w, "Failed to scan branch", err)
			return
		}
		branches = append(branches, b)
	}
	total, _ := s.db.CountBranches(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: branches, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT id, source_account_id, repo_id, name, sha, message, tagger_name,
			tagger_email, tagger_date, zipball_url, tarball_url, synced_at
		FROM np_github_tags ORDER BY name LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list tags", err)
		return
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.SourceAccountID, &t.RepoID, &t.Name, &t.SHA,
			&t.Message, &t.TaggerName, &t.TaggerEmail, &t.TaggerDate,
			&t.ZipballURL, &t.TarballURL, &t.SyncedAt); err != nil {
			writeErr(w, "Failed to scan tag", err)
			return
		}
		tags = append(tags, t)
	}
	total, _ := s.db.CountTags(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: tags, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListMilestones(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT id, source_account_id, node_id, repo_id, number, title, description,
			state, creator_login, open_issues, closed_issues, html_url, due_on,
			created_at, updated_at, closed_at, synced_at
		FROM np_github_milestones ORDER BY due_on ASC NULLS LAST LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list milestones", err)
		return
	}
	defer rows.Close()

	var milestones []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.SourceAccountID, &m.NodeID, &m.RepoID, &m.Number,
			&m.Title, &m.Description, &m.State, &m.CreatorLogin, &m.OpenIssues,
			&m.ClosedIssues, &m.HTMLURL, &m.DueOn, &m.CreatedAt, &m.UpdatedAt,
			&m.ClosedAt, &m.SyncedAt); err != nil {
			writeErr(w, "Failed to scan milestone", err)
			return
		}
		milestones = append(milestones, m)
	}
	total, _ := s.db.CountMilestones(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: milestones, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)
	repoID := parseOptionalInt64(r, "repo_id")

	query := `SELECT id, source_account_id, node_id, repo_id, name, color, description, is_default, synced_at
		FROM np_github_labels WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if repoID != nil {
		query += fmt.Sprintf(" AND repo_id = $%d", argIdx)
		args = append(args, *repoID)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY name LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		writeErr(w, "Failed to list labels", err)
		return
	}
	defer rows.Close()

	var labels []Label
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.ID, &l.SourceAccountID, &l.NodeID, &l.RepoID, &l.Name,
			&l.Color, &l.Description, &l.IsDefault, &l.SyncedAt); err != nil {
			writeErr(w, "Failed to scan label", err)
			return
		}
		labels = append(labels, l)
	}
	total, _ := s.db.CountLabels(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: labels, Total: total, Limit: limit, Offset: offset})
}

