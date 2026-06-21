package internal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

