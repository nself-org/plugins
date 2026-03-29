package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server holds all dependencies for the HTTP server.
type Server struct {
	db             *DB
	pool           *pgxpool.Pool
	client         *GitHubClient
	syncService    *SyncService
	webhookHandler *WebhookHandler
	webhookSecret  string
	startTime      time.Time
}

// NewServer creates a new Server instance with all dependencies wired up.
func NewServer(pool *pgxpool.Pool, cfg *Config) *Server {
	db := NewDB(pool, "primary")
	client := NewGitHubClient(cfg.Token)
	syncService := NewSyncService(pool, client, cfg, "primary")
	webhookHandler := NewWebhookHandler(db)

	return &Server{
		db:             db,
		pool:           pool,
		client:         client,
		syncService:    syncService,
		webhookHandler: webhookHandler,
		webhookSecret:  cfg.WebhookSecret,
		startTime:      time.Now(),
	}
}

// Router builds the chi router with all endpoints registered.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health / readiness / liveness
	r.Get("/health", s.handleHealth)
	r.Get("/ready", s.handleReady)
	r.Get("/live", s.handleLive)
	r.Get("/status", s.handleStatus)

	// Webhook endpoint
	r.Post("/webhooks/github", s.handleWebhook)

	// Sync endpoint
	r.Post("/sync", s.handleSync)

	// API endpoints
	r.Route("/api", func(r chi.Router) {
		r.Get("/repos", s.handleListRepos)
		r.Get("/repos/{fullName}", s.handleGetRepo)

		r.Get("/issues", s.handleListIssues)
		r.Get("/prs", s.handleListPRs)
		r.Get("/commits", s.handleListCommits)
		r.Get("/releases", s.handleListReleases)
		r.Get("/branches", s.handleListBranches)
		r.Get("/tags", s.handleListTags)
		r.Get("/milestones", s.handleListMilestones)
		r.Get("/labels", s.handleListLabels)

		r.Get("/workflows", s.handleListWorkflows)
		r.Get("/workflow-runs", s.handleListWorkflowRuns)
		r.Get("/workflow-jobs", s.handleListWorkflowJobs)

		r.Get("/check-suites", s.handleListCheckSuites)
		r.Get("/check-runs", s.handleListCheckRuns)

		r.Get("/deployments", s.handleListDeployments)
		r.Get("/teams", s.handleListTeams)
		r.Get("/collaborators", s.handleListCollaborators)

		r.Get("/pr-reviews", s.handleListPRReviews)
		r.Get("/issue-comments", s.handleListIssueComments)
		r.Get("/pr-review-comments", s.handleListPRReviewComments)
		r.Get("/commit-comments", s.handleListCommitComments)

		r.Get("/events", s.handleListEvents)
		r.Get("/stats", s.handleStats)
	})

	return r
}

// --- Health endpoints --------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"plugin":    "github",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.db.Ping(ctx); err != nil {
		log.Printf("[github:server] Readiness check failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"ready":     false,
			"plugin":    "github",
			"error":     "Database unavailable",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ready":     true,
		"plugin":    "github",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := s.db.GetStats(ctx)
	if err != nil {
		log.Printf("[github:server] Live check stats error: %v", err)
		stats = &SyncStats{}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"alive":   true,
		"plugin":  "github",
		"version": "1.0.0",
		"uptime":  time.Since(s.startTime).Seconds(),
		"memory": map[string]interface{}{
			"alloc":      mem.Alloc,
			"totalAlloc": mem.TotalAlloc,
			"sys":        mem.Sys,
			"heapInuse":  mem.HeapInuse,
		},
		"stats": map[string]interface{}{
			"repositories": stats.Repositories,
			"issues":       stats.Issues,
			"pullRequests": stats.PullRequests,
			"lastSync":     stats.LastSyncedAt,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := s.db.GetStats(ctx)
	if err != nil {
		log.Printf("[github:server] Status stats error: %v", err)
		stats = &SyncStats{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"plugin":    "github",
		"version":   "1.0.0",
		"status":    "running",
		"stats":     stats,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// --- Webhook endpoint --------------------------------------------------------

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("X-Hub-Signature-256")
	event := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")

	if event == "" || deliveryID == "" {
		log.Printf("[github:server] Missing GitHub event headers")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing event headers"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read body"})
		return
	}
	defer r.Body.Close()

	if s.webhookSecret != "" && signature != "" {
		if !VerifySignature(body, signature, s.webhookSecret) {
			log.Printf("[github:server] Invalid GitHub signature")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid signature"})
			return
		}
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}

	ctx := r.Context()
	if err := s.webhookHandler.Handle(ctx, deliveryID, event, payload); err != nil {
		log.Printf("[github:server] Webhook processing failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Processing failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"received": true})
}

// --- Sync endpoint -----------------------------------------------------------

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	var since *time.Time
	if req.Since != "" {
		t, err := time.Parse(time.RFC3339, req.Since)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid since format, expected RFC3339"})
			return
		}
		since = &t
	}

	ctx := r.Context()

	var result *SyncResult
	if len(req.Resources) == 0 {
		result = s.syncService.SyncAll(ctx)
	} else {
		// Sync each requested resource and merge results
		merged := &SyncResult{Success: true}
		for _, res := range req.Resources {
			partial := s.syncService.SyncResource(ctx, res)
			merged.Errors = append(merged.Errors, partial.Errors...)
			merged.Duration += partial.Duration
			merged.Stats = partial.Stats
		}
		merged.Success = len(merged.Errors) == 0
		result = merged
	}
	_ = since // reserved for future incremental sync support

	writeJSON(w, http.StatusOK, result)
}

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

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT id, source_account_id, node_id, repo_id, name, path, state,
			badge_url, html_url, created_at, updated_at, synced_at
		FROM np_github_workflows ORDER BY name LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list workflows", err)
		return
	}
	defer rows.Close()

	var workflows []Workflow
	for rows.Next() {
		var wf Workflow
		if err := rows.Scan(&wf.ID, &wf.SourceAccountID, &wf.NodeID, &wf.RepoID,
			&wf.Name, &wf.Path, &wf.State, &wf.BadgeURL, &wf.HTMLURL,
			&wf.CreatedAt, &wf.UpdatedAt, &wf.SyncedAt); err != nil {
			writeErr(w, "Failed to scan workflow", err)
			return
		}
		workflows = append(workflows, wf)
	}
	total, _ := s.db.CountWorkflows(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: workflows, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")
	conclusion := r.URL.Query().Get("conclusion")

	query := `SELECT id, source_account_id, node_id, repo_id, workflow_id, workflow_name,
		name, head_branch, head_sha, run_number, run_attempt, event, status, conclusion,
		actor_login, triggering_actor_login, html_url, jobs_url, logs_url,
		run_started_at, created_at, updated_at, synced_at
		FROM np_github_workflow_runs WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if conclusion != "" {
		query += fmt.Sprintf(" AND conclusion = $%d", argIdx)
		args = append(args, conclusion)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		writeErr(w, "Failed to list workflow runs", err)
		return
	}
	defer rows.Close()

	var runs []WorkflowRun
	for rows.Next() {
		var wr WorkflowRun
		if err := rows.Scan(&wr.ID, &wr.SourceAccountID, &wr.NodeID, &wr.RepoID,
			&wr.WorkflowID, &wr.WorkflowName, &wr.Name, &wr.HeadBranch, &wr.HeadSHA,
			&wr.RunNumber, &wr.RunAttempt, &wr.Event, &wr.Status, &wr.Conclusion,
			&wr.ActorLogin, &wr.TriggeringActorLogin, &wr.HTMLURL, &wr.JobsURL, &wr.LogsURL,
			&wr.RunStartedAt, &wr.CreatedAt, &wr.UpdatedAt, &wr.SyncedAt); err != nil {
			writeErr(w, "Failed to scan workflow run", err)
			return
		}
		runs = append(runs, wr)
	}
	total, _ := s.db.CountWorkflowRuns(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: runs, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListWorkflowJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)
	runID := parseOptionalInt64(r, "run_id")

	query := `SELECT id, source_account_id, node_id, repo_id, run_id, run_attempt,
		workflow_name, name, status, conclusion, head_sha, html_url,
		runner_id, runner_name, runner_group_id, runner_group_name,
		labels, steps, started_at, completed_at, synced_at
		FROM np_github_workflow_jobs WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if runID != nil {
		query += fmt.Sprintf(" AND run_id = $%d", argIdx)
		args = append(args, *runID)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY started_at DESC NULLS LAST LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		writeErr(w, "Failed to list workflow jobs", err)
		return
	}
	defer rows.Close()

	var jobs []WorkflowJob
	for rows.Next() {
		var j WorkflowJob
		if err := rows.Scan(&j.ID, &j.SourceAccountID, &j.NodeID, &j.RepoID, &j.RunID,
			&j.RunAttempt, &j.WorkflowName, &j.Name, &j.Status, &j.Conclusion,
			&j.HeadSHA, &j.HTMLURL, &j.RunnerID, &j.RunnerName, &j.RunnerGroupID,
			&j.RunnerGroupName, &j.JobLabels, &j.Steps, &j.StartedAt,
			&j.CompletedAt, &j.SyncedAt); err != nil {
			writeErr(w, "Failed to scan workflow job", err)
			return
		}
		jobs = append(jobs, j)
	}
	total, _ := s.db.CountWorkflowJobs(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: jobs, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListCheckSuites(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT id, source_account_id, node_id, repo_id, head_branch, head_sha,
			status, conclusion, app_id, app_slug, pull_requests,
			before_sha, after_sha, created_at, updated_at, synced_at
		FROM np_github_check_suites ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list check suites", err)
		return
	}
	defer rows.Close()

	var suites []CheckSuite
	for rows.Next() {
		var cs CheckSuite
		if err := rows.Scan(&cs.ID, &cs.SourceAccountID, &cs.NodeID, &cs.RepoID,
			&cs.HeadBranch, &cs.HeadSHA, &cs.Status, &cs.Conclusion, &cs.AppID,
			&cs.AppSlug, &cs.PullRequests, &cs.BeforeSHA, &cs.AfterSHA,
			&cs.CreatedAt, &cs.UpdatedAt, &cs.SyncedAt); err != nil {
			writeErr(w, "Failed to scan check suite", err)
			return
		}
		suites = append(suites, cs)
	}
	total, _ := s.db.CountCheckSuites(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: suites, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListCheckRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)

	rows, err := s.pool.Query(ctx,
		`SELECT id, source_account_id, node_id, repo_id, check_suite_id, head_sha,
			name, status, conclusion, external_id, html_url, details_url,
			app_id, app_slug, output, pull_requests, started_at, completed_at, synced_at
		FROM np_github_check_runs ORDER BY started_at DESC NULLS LAST LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, "Failed to list check runs", err)
		return
	}
	defer rows.Close()

	var checkRuns []CheckRun
	for rows.Next() {
		var cr CheckRun
		if err := rows.Scan(&cr.ID, &cr.SourceAccountID, &cr.NodeID, &cr.RepoID,
			&cr.CheckSuiteID, &cr.HeadSHA, &cr.Name, &cr.Status, &cr.Conclusion,
			&cr.ExternalID, &cr.HTMLURL, &cr.DetailsURL, &cr.AppID, &cr.AppSlug,
			&cr.Output, &cr.PullRequests, &cr.StartedAt, &cr.CompletedAt, &cr.SyncedAt); err != nil {
			writeErr(w, "Failed to scan check run", err)
			return
		}
		checkRuns = append(checkRuns, cr)
	}
	total, _ := s.db.CountCheckRuns(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: checkRuns, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset := parsePagination(r)
	environment := r.URL.Query().Get("environment")

	query := `SELECT id, source_account_id, node_id, repo_id, sha, ref, task,
		environment, description, creator_login, statuses, current_status,
		production_environment, transient_environment, payload, created_at, updated_at, synced_at
		FROM np_github_deployments WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if environment != "" {
		query += fmt.Sprintf(" AND environment = $%d", argIdx)
		args = append(args, environment)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		writeErr(w, "Failed to list deployments", err)
		return
	}
	defer rows.Close()

	var deployments []Deployment
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(&d.ID, &d.SourceAccountID, &d.NodeID, &d.RepoID, &d.SHA,
			&d.Ref, &d.Task, &d.Environment, &d.Description, &d.CreatorLogin,
			&d.Statuses, &d.CurrentStatus, &d.ProductionEnvironment,
			&d.TransientEnvironment, &d.Payload, &d.CreatedAt, &d.UpdatedAt, &d.SyncedAt); err != nil {
			writeErr(w, "Failed to scan deployment", err)
			return
		}
		deployments = append(deployments, d)
	}
	total, _ := s.db.CountDeployments(ctx)

	writeJSON(w, http.StatusOK, ListResponse{Data: deployments, Total: total, Limit: limit, Offset: offset})
}

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

// --- Helpers -----------------------------------------------------------------

// parsePagination extracts limit and offset from query parameters with defaults.
func parsePagination(r *http.Request) (int, int) {
	limit := 100
	offset := 0

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

// parseOptionalInt64 extracts an optional int64 query parameter.
func parseOptionalInt64(r *http.Request, param string) *int64 {
	v := r.URL.Query().Get(param)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[github:server] Failed to encode response: %v", err)
	}
}

// writeErr writes an error response and logs it.
func writeErr(w http.ResponseWriter, msg string, err error) {
	log.Printf("[github:server] %s: %v", msg, err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg})
}
