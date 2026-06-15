-- Rollback: Rename np_ prefix back to github_ for github tables
-- Reverse of 001_add_np_prefix.sql

BEGIN;

ALTER TABLE IF EXISTS np_github_collaborators RENAME TO github_collaborators;
ALTER TABLE IF EXISTS np_github_teams RENAME TO github_teams;
ALTER TABLE IF EXISTS np_github_deployments RENAME TO github_deployments;
ALTER TABLE IF EXISTS np_github_check_runs RENAME TO github_check_runs;
ALTER TABLE IF EXISTS np_github_check_suites RENAME TO github_check_suites;
ALTER TABLE IF EXISTS np_github_workflow_jobs RENAME TO github_workflow_jobs;
ALTER TABLE IF EXISTS np_github_workflow_runs RENAME TO github_workflow_runs;
ALTER TABLE IF EXISTS np_github_workflows RENAME TO github_workflows;
ALTER TABLE IF EXISTS np_github_labels RENAME TO github_labels;
ALTER TABLE IF EXISTS np_github_milestones RENAME TO github_milestones;
ALTER TABLE IF EXISTS np_github_tags RENAME TO github_tags;
ALTER TABLE IF EXISTS np_github_releases RENAME TO github_releases;
ALTER TABLE IF EXISTS np_github_commits RENAME TO github_commits;
ALTER TABLE IF EXISTS np_github_commit_comments RENAME TO github_commit_comments;
ALTER TABLE IF EXISTS np_github_pr_review_comments RENAME TO github_pr_review_comments;
ALTER TABLE IF EXISTS np_github_issue_comments RENAME TO github_issue_comments;
ALTER TABLE IF EXISTS np_github_pr_reviews RENAME TO github_pr_reviews;
ALTER TABLE IF EXISTS np_github_pull_requests RENAME TO github_pull_requests;
ALTER TABLE IF EXISTS np_github_issues RENAME TO github_issues;
ALTER TABLE IF EXISTS np_github_branches RENAME TO github_branches;
ALTER TABLE IF EXISTS np_github_repositories RENAME TO github_repositories;
ALTER TABLE IF EXISTS np_github_organizations RENAME TO github_organizations;
ALTER TABLE IF EXISTS np_github_webhook_events RENAME TO github_webhook_events;

-- Rename indexes back to old convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_np_github_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_np_github_', 'idx_github_'));
  END LOOP;
END $$;

COMMIT;
