package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
