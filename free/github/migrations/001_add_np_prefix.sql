-- Migration: Add np_ prefix to github tables
-- Run this on existing installations before upgrading to the latest plugin version.
-- These tables used the `github_` prefix without the required `np_` namespace wrapper.

BEGIN;

ALTER TABLE IF EXISTS github_organizations RENAME TO np_github_organizations;
ALTER TABLE IF EXISTS github_repositories RENAME TO np_github_repositories;
ALTER TABLE IF EXISTS github_branches RENAME TO np_github_branches;
ALTER TABLE IF EXISTS github_issues RENAME TO np_github_issues;
ALTER TABLE IF EXISTS github_pull_requests RENAME TO np_github_pull_requests;
ALTER TABLE IF EXISTS github_pr_reviews RENAME TO np_github_pr_reviews;
ALTER TABLE IF EXISTS github_issue_comments RENAME TO np_github_issue_comments;
ALTER TABLE IF EXISTS github_pr_review_comments RENAME TO np_github_pr_review_comments;
ALTER TABLE IF EXISTS github_commit_comments RENAME TO np_github_commit_comments;
ALTER TABLE IF EXISTS github_commits RENAME TO np_github_commits;
ALTER TABLE IF EXISTS github_releases RENAME TO np_github_releases;
ALTER TABLE IF EXISTS github_tags RENAME TO np_github_tags;
ALTER TABLE IF EXISTS github_milestones RENAME TO np_github_milestones;
ALTER TABLE IF EXISTS github_labels RENAME TO np_github_labels;
ALTER TABLE IF EXISTS github_workflows RENAME TO np_github_workflows;
ALTER TABLE IF EXISTS github_workflow_runs RENAME TO np_github_workflow_runs;
ALTER TABLE IF EXISTS github_workflow_jobs RENAME TO np_github_workflow_jobs;
ALTER TABLE IF EXISTS github_check_suites RENAME TO np_github_check_suites;
ALTER TABLE IF EXISTS github_check_runs RENAME TO np_github_check_runs;
ALTER TABLE IF EXISTS github_deployments RENAME TO np_github_deployments;
ALTER TABLE IF EXISTS github_teams RENAME TO np_github_teams;
ALTER TABLE IF EXISTS github_collaborators RENAME TO np_github_collaborators;
ALTER TABLE IF EXISTS github_webhook_events RENAME TO np_github_webhook_events;

-- Rename indexes to match new convention
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_github_%'
  LOOP
    EXECUTE format('ALTER INDEX IF EXISTS %I RENAME TO %I',
      idx.indexname,
      replace(idx.indexname, 'idx_github_', 'idx_np_github_'));
  END LOOP;
END $$;

COMMIT;
