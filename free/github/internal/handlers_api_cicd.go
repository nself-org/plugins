package internal

import (
	"fmt"
	"net/http"
)

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

// Size-cap exception: single-responsibility HTTP route handler — 51L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
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

