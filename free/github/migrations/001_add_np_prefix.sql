-- Migration 001: add the np_ prefix to the github tables (guarded renames only).
-- Renames the tables (and their indexes) an older release created without the np_ prefix.
-- Old names: github_*. New names: np_github_*.
-- Guarded, idempotent, no transaction control (sdk/go/migrate refuses it, and applies each file itself):
--   * a table is renamed only when the old name exists in public AND the new name is free;
--   * when both names exist, both are left untouched (nothing merged, nothing dropped);
--   * an index is renamed only when it sits on a renamed np_ table and the new index name is free.
-- A fresh database has none of the old tables, so this file is a no-op there; the tables themselves are created by the next migration file.

DO $$
DECLARE
  t RECORD;
  i RECORD;
  new_idx TEXT;
BEGIN
  FOR t IN
    SELECT old_name, new_name FROM (VALUES
      ('github_organizations', 'np_github_organizations')
      , ('github_repositories', 'np_github_repositories')
      , ('github_branches', 'np_github_branches')
      , ('github_issues', 'np_github_issues')
      , ('github_pull_requests', 'np_github_pull_requests')
      , ('github_pr_reviews', 'np_github_pr_reviews')
      , ('github_issue_comments', 'np_github_issue_comments')
      , ('github_pr_review_comments', 'np_github_pr_review_comments')
      , ('github_commit_comments', 'np_github_commit_comments')
      , ('github_commits', 'np_github_commits')
      , ('github_releases', 'np_github_releases')
      , ('github_tags', 'np_github_tags')
      , ('github_milestones', 'np_github_milestones')
      , ('github_labels', 'np_github_labels')
      , ('github_workflows', 'np_github_workflows')
      , ('github_workflow_runs', 'np_github_workflow_runs')
      , ('github_workflow_jobs', 'np_github_workflow_jobs')
      , ('github_check_suites', 'np_github_check_suites')
      , ('github_check_runs', 'np_github_check_runs')
      , ('github_deployments', 'np_github_deployments')
      , ('github_teams', 'np_github_teams')
      , ('github_collaborators', 'np_github_collaborators')
      , ('github_webhook_events', 'np_github_webhook_events')
    ) AS v(old_name, new_name)
  LOOP
    IF to_regclass('public.' || quote_ident(t.old_name)) IS NOT NULL
       AND to_regclass('public.' || quote_ident(t.new_name)) IS NULL THEN
      EXECUTE format('ALTER TABLE public.%I RENAME TO %I', t.old_name, t.new_name);
    END IF;
  END LOOP;

  FOR i IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND tablename LIKE 'np\_github\_%'
      AND starts_with(indexname, 'idx_github_')
  LOOP
    new_idx := 'idx_np_github_' || substr(i.indexname, length('idx_github_') + 1);
    IF to_regclass('public.' || quote_ident(new_idx)) IS NULL THEN
      EXECUTE format('ALTER INDEX public.%I RENAME TO %I', i.indexname, new_idx);
    END IF;
  END LOOP;
END $$;
