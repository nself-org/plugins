package internal

// schemaSQL contains the full CREATE TABLE IF NOT EXISTS DDL for all 23 np_github_* tables,
// their indexes, and the analytical views. Ported directly from the TypeScript database.ts.
const schemaSQL = `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Organizations
CREATE TABLE IF NOT EXISTS np_github_organizations (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  login VARCHAR(255) NOT NULL,
  name VARCHAR(255),
  description TEXT,
  company VARCHAR(255),
  blog VARCHAR(2048),
  location VARCHAR(255),
  email VARCHAR(255),
  twitter_username VARCHAR(255),
  is_verified BOOLEAN DEFAULT FALSE,
  html_url VARCHAR(2048),
  avatar_url VARCHAR(2048),
  public_repos INTEGER DEFAULT 0,
  public_gists INTEGER DEFAULT 0,
  followers INTEGER DEFAULT 0,
  following INTEGER DEFAULT 0,
  type VARCHAR(50) DEFAULT 'Organization',
  total_private_repos INTEGER,
  owned_private_repos INTEGER,
  plan JSONB,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_orgs_login ON np_github_organizations(login);
CREATE INDEX IF NOT EXISTS idx_np_github_orgs_source ON np_github_organizations(source_account_id);

-- Repositories
CREATE TABLE IF NOT EXISTS np_github_repositories (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  name VARCHAR(255) NOT NULL,
  full_name VARCHAR(255) NOT NULL,
  owner_login VARCHAR(255) NOT NULL,
  owner_type VARCHAR(50),
  private BOOLEAN DEFAULT FALSE,
  description TEXT,
  fork BOOLEAN DEFAULT FALSE,
  url VARCHAR(2048),
  html_url VARCHAR(2048),
  clone_url VARCHAR(2048),
  ssh_url VARCHAR(2048),
  homepage VARCHAR(2048),
  language VARCHAR(100),
  languages JSONB DEFAULT '{}',
  default_branch VARCHAR(255) DEFAULT 'main',
  size INTEGER DEFAULT 0,
  stargazers_count INTEGER DEFAULT 0,
  watchers_count INTEGER DEFAULT 0,
  forks_count INTEGER DEFAULT 0,
  open_issues_count INTEGER DEFAULT 0,
  topics JSONB DEFAULT '[]',
  visibility VARCHAR(50) DEFAULT 'public',
  archived BOOLEAN DEFAULT FALSE,
  disabled BOOLEAN DEFAULT FALSE,
  has_issues BOOLEAN DEFAULT TRUE,
  has_projects BOOLEAN DEFAULT TRUE,
  has_wiki BOOLEAN DEFAULT TRUE,
  has_pages BOOLEAN DEFAULT FALSE,
  has_downloads BOOLEAN DEFAULT TRUE,
  has_discussions BOOLEAN DEFAULT FALSE,
  allow_forking BOOLEAN DEFAULT TRUE,
  is_template BOOLEAN DEFAULT FALSE,
  license JSONB,
  pushed_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_repos_owner ON np_github_repositories(owner_login);
CREATE INDEX IF NOT EXISTS idx_np_github_repos_name ON np_github_repositories(full_name);
CREATE INDEX IF NOT EXISTS idx_np_github_repos_language ON np_github_repositories(language);
CREATE INDEX IF NOT EXISTS idx_np_github_repos_source ON np_github_repositories(source_account_id);

-- Branches
CREATE TABLE IF NOT EXISTS np_github_branches (
  id VARCHAR(500) NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  repo_id BIGINT,
  name VARCHAR(255) NOT NULL,
  sha VARCHAR(40) NOT NULL,
  protected BOOLEAN DEFAULT FALSE,
  protection_enabled BOOLEAN DEFAULT FALSE,
  protection JSONB,
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_branches_repo ON np_github_branches(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_branches_name ON np_github_branches(name);
CREATE INDEX IF NOT EXISTS idx_np_github_branches_source ON np_github_branches(source_account_id);

-- Issues
CREATE TABLE IF NOT EXISTS np_github_issues (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  number INTEGER NOT NULL,
  title TEXT NOT NULL,
  body TEXT,
  state VARCHAR(20) NOT NULL,
  state_reason VARCHAR(50),
  locked BOOLEAN DEFAULT FALSE,
  user_login VARCHAR(255),
  user_id BIGINT,
  labels JSONB DEFAULT '[]',
  assignees JSONB DEFAULT '[]',
  milestone JSONB,
  comments INTEGER DEFAULT 0,
  reactions JSONB DEFAULT '{}',
  html_url VARCHAR(2048),
  closed_at TIMESTAMP WITH TIME ZONE,
  closed_by_login VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_issues_repo ON np_github_issues(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_issues_state ON np_github_issues(state);
CREATE INDEX IF NOT EXISTS idx_np_github_issues_user ON np_github_issues(user_login);
CREATE INDEX IF NOT EXISTS idx_np_github_issues_created ON np_github_issues(created_at);
CREATE INDEX IF NOT EXISTS idx_np_github_issues_source ON np_github_issues(source_account_id);

-- Pull Requests
CREATE TABLE IF NOT EXISTS np_github_pull_requests (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  number INTEGER NOT NULL,
  title TEXT NOT NULL,
  body TEXT,
  state VARCHAR(20) NOT NULL,
  draft BOOLEAN DEFAULT FALSE,
  locked BOOLEAN DEFAULT FALSE,
  user_login VARCHAR(255),
  user_id BIGINT,
  head_ref VARCHAR(255),
  head_sha VARCHAR(40),
  head_repo_id BIGINT,
  base_ref VARCHAR(255),
  base_sha VARCHAR(40),
  merged BOOLEAN DEFAULT FALSE,
  mergeable BOOLEAN,
  mergeable_state VARCHAR(50),
  merged_by_login VARCHAR(255),
  merged_at TIMESTAMP WITH TIME ZONE,
  merge_commit_sha VARCHAR(40),
  labels JSONB DEFAULT '[]',
  assignees JSONB DEFAULT '[]',
  reviewers JSONB DEFAULT '[]',
  milestone JSONB,
  comments INTEGER DEFAULT 0,
  review_comments INTEGER DEFAULT 0,
  commits INTEGER DEFAULT 0,
  additions INTEGER DEFAULT 0,
  deletions INTEGER DEFAULT 0,
  changed_files INTEGER DEFAULT 0,
  html_url VARCHAR(2048),
  diff_url VARCHAR(2048),
  closed_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_prs_repo ON np_github_pull_requests(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_prs_state ON np_github_pull_requests(state);
CREATE INDEX IF NOT EXISTS idx_np_github_prs_user ON np_github_pull_requests(user_login);
CREATE INDEX IF NOT EXISTS idx_np_github_prs_merged ON np_github_pull_requests(merged);
CREATE INDEX IF NOT EXISTS idx_np_github_prs_created ON np_github_pull_requests(created_at);
CREATE INDEX IF NOT EXISTS idx_np_github_prs_source ON np_github_pull_requests(source_account_id);

-- Pull Request Reviews
CREATE TABLE IF NOT EXISTS np_github_pr_reviews (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  pull_request_id BIGINT,
  pull_request_number INTEGER NOT NULL,
  user_login VARCHAR(255),
  user_id BIGINT,
  body TEXT,
  state VARCHAR(50) NOT NULL,
  html_url VARCHAR(2048),
  commit_id VARCHAR(40),
  submitted_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_pr_reviews_pr ON np_github_pr_reviews(pull_request_id);
CREATE INDEX IF NOT EXISTS idx_np_github_pr_reviews_user ON np_github_pr_reviews(user_login);
CREATE INDEX IF NOT EXISTS idx_np_github_pr_reviews_state ON np_github_pr_reviews(state);
CREATE INDEX IF NOT EXISTS idx_np_github_pr_reviews_source ON np_github_pr_reviews(source_account_id);

-- Issue Comments
CREATE TABLE IF NOT EXISTS np_github_issue_comments (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  issue_number INTEGER NOT NULL,
  issue_id BIGINT,
  pull_request_number INTEGER,
  user_login VARCHAR(255),
  user_id BIGINT,
  body TEXT NOT NULL,
  reactions JSONB DEFAULT '{}',
  html_url VARCHAR(2048),
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_issue_comments_repo ON np_github_issue_comments(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_issue_comments_issue ON np_github_issue_comments(issue_number);
CREATE INDEX IF NOT EXISTS idx_np_github_issue_comments_user ON np_github_issue_comments(user_login);
CREATE INDEX IF NOT EXISTS idx_np_github_issue_comments_source ON np_github_issue_comments(source_account_id);

-- Pull Request Review Comments
CREATE TABLE IF NOT EXISTS np_github_pr_review_comments (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  pull_request_id BIGINT,
  pull_request_number INTEGER NOT NULL,
  review_id BIGINT,
  user_login VARCHAR(255),
  user_id BIGINT,
  body TEXT NOT NULL,
  path VARCHAR(1024),
  position INTEGER,
  original_position INTEGER,
  diff_hunk TEXT,
  commit_id VARCHAR(40),
  original_commit_id VARCHAR(40),
  in_reply_to_id BIGINT,
  reactions JSONB DEFAULT '{}',
  html_url VARCHAR(2048),
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_pr_review_comments_pr ON np_github_pr_review_comments(pull_request_id);
CREATE INDEX IF NOT EXISTS idx_np_github_pr_review_comments_review ON np_github_pr_review_comments(review_id);
CREATE INDEX IF NOT EXISTS idx_np_github_pr_review_comments_source ON np_github_pr_review_comments(source_account_id);

-- Commit Comments
CREATE TABLE IF NOT EXISTS np_github_commit_comments (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  commit_sha VARCHAR(40) NOT NULL,
  user_login VARCHAR(255),
  user_id BIGINT,
  body TEXT NOT NULL,
  path VARCHAR(1024),
  position INTEGER,
  line INTEGER,
  reactions JSONB DEFAULT '{}',
  html_url VARCHAR(2048),
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_commit_comments_repo ON np_github_commit_comments(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_commit_comments_commit ON np_github_commit_comments(commit_sha);
CREATE INDEX IF NOT EXISTS idx_np_github_commit_comments_source ON np_github_commit_comments(source_account_id);

-- Commits
CREATE TABLE IF NOT EXISTS np_github_commits (
  sha VARCHAR(40) NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  message TEXT,
  author_name VARCHAR(255),
  author_email VARCHAR(255),
  author_login VARCHAR(255),
  author_date TIMESTAMP WITH TIME ZONE,
  committer_name VARCHAR(255),
  committer_email VARCHAR(255),
  committer_login VARCHAR(255),
  committer_date TIMESTAMP WITH TIME ZONE,
  tree_sha VARCHAR(40),
  parents JSONB DEFAULT '[]',
  additions INTEGER DEFAULT 0,
  deletions INTEGER DEFAULT 0,
  total INTEGER DEFAULT 0,
  html_url VARCHAR(2048),
  verified BOOLEAN DEFAULT FALSE,
  verification_reason VARCHAR(50),
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (sha, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_commits_repo ON np_github_commits(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_commits_author ON np_github_commits(author_login);
CREATE INDEX IF NOT EXISTS idx_np_github_commits_date ON np_github_commits(author_date);
CREATE INDEX IF NOT EXISTS idx_np_github_commits_source ON np_github_commits(source_account_id);

-- Releases
CREATE TABLE IF NOT EXISTS np_github_releases (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  tag_name VARCHAR(255) NOT NULL,
  target_commitish VARCHAR(255),
  name VARCHAR(255),
  body TEXT,
  draft BOOLEAN DEFAULT FALSE,
  prerelease BOOLEAN DEFAULT FALSE,
  author_login VARCHAR(255),
  html_url VARCHAR(2048),
  tarball_url VARCHAR(2048),
  zipball_url VARCHAR(2048),
  assets JSONB DEFAULT '[]',
  created_at TIMESTAMP WITH TIME ZONE,
  published_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_releases_repo ON np_github_releases(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_releases_tag ON np_github_releases(tag_name);
CREATE INDEX IF NOT EXISTS idx_np_github_releases_published ON np_github_releases(published_at);
CREATE INDEX IF NOT EXISTS idx_np_github_releases_source ON np_github_releases(source_account_id);

-- Tags
CREATE TABLE IF NOT EXISTS np_github_tags (
  id VARCHAR(500) NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  repo_id BIGINT,
  name VARCHAR(255) NOT NULL,
  sha VARCHAR(40) NOT NULL,
  message TEXT,
  tagger_name VARCHAR(255),
  tagger_email VARCHAR(255),
  tagger_date TIMESTAMP WITH TIME ZONE,
  zipball_url VARCHAR(2048),
  tarball_url VARCHAR(2048),
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_tags_repo ON np_github_tags(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_tags_name ON np_github_tags(name);
CREATE INDEX IF NOT EXISTS idx_np_github_tags_source ON np_github_tags(source_account_id);

-- Milestones
CREATE TABLE IF NOT EXISTS np_github_milestones (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  number INTEGER NOT NULL,
  title VARCHAR(255) NOT NULL,
  description TEXT,
  state VARCHAR(20) NOT NULL,
  creator_login VARCHAR(255),
  open_issues INTEGER DEFAULT 0,
  closed_issues INTEGER DEFAULT 0,
  html_url VARCHAR(2048),
  due_on TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  closed_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_milestones_repo ON np_github_milestones(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_milestones_state ON np_github_milestones(state);
CREATE INDEX IF NOT EXISTS idx_np_github_milestones_source ON np_github_milestones(source_account_id);

-- Labels
CREATE TABLE IF NOT EXISTS np_github_labels (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  name VARCHAR(255) NOT NULL,
  color VARCHAR(10) NOT NULL,
  description TEXT,
  is_default BOOLEAN DEFAULT FALSE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_labels_repo ON np_github_labels(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_labels_name ON np_github_labels(name);
CREATE INDEX IF NOT EXISTS idx_np_github_labels_source ON np_github_labels(source_account_id);

-- Workflows
CREATE TABLE IF NOT EXISTS np_github_workflows (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  name VARCHAR(255) NOT NULL,
  path VARCHAR(1024) NOT NULL,
  state VARCHAR(50) NOT NULL,
  badge_url VARCHAR(2048),
  html_url VARCHAR(2048),
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_workflows_repo ON np_github_workflows(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_workflows_state ON np_github_workflows(state);
CREATE INDEX IF NOT EXISTS idx_np_github_workflows_source ON np_github_workflows(source_account_id);

-- Workflow Runs
CREATE TABLE IF NOT EXISTS np_github_workflow_runs (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  workflow_id BIGINT,
  workflow_name VARCHAR(255),
  name VARCHAR(255),
  head_branch VARCHAR(255),
  head_sha VARCHAR(40),
  run_number INTEGER,
  run_attempt INTEGER DEFAULT 1,
  event VARCHAR(50),
  status VARCHAR(50),
  conclusion VARCHAR(50),
  actor_login VARCHAR(255),
  triggering_actor_login VARCHAR(255),
  html_url VARCHAR(2048),
  jobs_url VARCHAR(2048),
  logs_url VARCHAR(2048),
  run_started_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_repo ON np_github_workflow_runs(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_workflow ON np_github_workflow_runs(workflow_id);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_status ON np_github_workflow_runs(status);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_conclusion ON np_github_workflow_runs(conclusion);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_event ON np_github_workflow_runs(event);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_created ON np_github_workflow_runs(created_at);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_runs_source ON np_github_workflow_runs(source_account_id);

-- Workflow Jobs
CREATE TABLE IF NOT EXISTS np_github_workflow_jobs (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  run_id BIGINT,
  run_attempt INTEGER DEFAULT 1,
  workflow_name VARCHAR(255),
  name VARCHAR(255) NOT NULL,
  status VARCHAR(50) NOT NULL,
  conclusion VARCHAR(50),
  head_sha VARCHAR(40),
  html_url VARCHAR(2048),
  runner_id BIGINT,
  runner_name VARCHAR(255),
  runner_group_id BIGINT,
  runner_group_name VARCHAR(255),
  labels JSONB DEFAULT '[]',
  steps JSONB DEFAULT '[]',
  started_at TIMESTAMP WITH TIME ZONE,
  completed_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_jobs_run ON np_github_workflow_jobs(run_id);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_jobs_status ON np_github_workflow_jobs(status);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_jobs_conclusion ON np_github_workflow_jobs(conclusion);
CREATE INDEX IF NOT EXISTS idx_np_github_workflow_jobs_source ON np_github_workflow_jobs(source_account_id);

-- Check Suites
CREATE TABLE IF NOT EXISTS np_github_check_suites (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  head_branch VARCHAR(255),
  head_sha VARCHAR(40) NOT NULL,
  status VARCHAR(50) NOT NULL,
  conclusion VARCHAR(50),
  app_id BIGINT,
  app_slug VARCHAR(255),
  pull_requests JSONB DEFAULT '[]',
  before_sha VARCHAR(40),
  after_sha VARCHAR(40),
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_check_suites_repo ON np_github_check_suites(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_check_suites_sha ON np_github_check_suites(head_sha);
CREATE INDEX IF NOT EXISTS idx_np_github_check_suites_status ON np_github_check_suites(status);
CREATE INDEX IF NOT EXISTS idx_np_github_check_suites_source ON np_github_check_suites(source_account_id);

-- Check Runs
CREATE TABLE IF NOT EXISTS np_github_check_runs (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  check_suite_id BIGINT,
  head_sha VARCHAR(40) NOT NULL,
  name VARCHAR(255) NOT NULL,
  status VARCHAR(50) NOT NULL,
  conclusion VARCHAR(50),
  external_id VARCHAR(255),
  html_url VARCHAR(2048),
  details_url VARCHAR(2048),
  app_id BIGINT,
  app_slug VARCHAR(255),
  output JSONB,
  pull_requests JSONB DEFAULT '[]',
  started_at TIMESTAMP WITH TIME ZONE,
  completed_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_check_runs_suite ON np_github_check_runs(check_suite_id);
CREATE INDEX IF NOT EXISTS idx_np_github_check_runs_sha ON np_github_check_runs(head_sha);
CREATE INDEX IF NOT EXISTS idx_np_github_check_runs_status ON np_github_check_runs(status);
CREATE INDEX IF NOT EXISTS idx_np_github_check_runs_name ON np_github_check_runs(name);
CREATE INDEX IF NOT EXISTS idx_np_github_check_runs_source ON np_github_check_runs(source_account_id);

-- Deployments
CREATE TABLE IF NOT EXISTS np_github_deployments (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  repo_id BIGINT,
  sha VARCHAR(40),
  ref VARCHAR(255),
  task VARCHAR(255) DEFAULT 'deploy',
  environment VARCHAR(255),
  description TEXT,
  creator_login VARCHAR(255),
  statuses JSONB DEFAULT '[]',
  current_status VARCHAR(50),
  production_environment BOOLEAN DEFAULT FALSE,
  transient_environment BOOLEAN DEFAULT FALSE,
  payload JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_deployments_repo ON np_github_deployments(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_deployments_env ON np_github_deployments(environment);
CREATE INDEX IF NOT EXISTS idx_np_github_deployments_status ON np_github_deployments(current_status);
CREATE INDEX IF NOT EXISTS idx_np_github_deployments_source ON np_github_deployments(source_account_id);

-- Teams
CREATE TABLE IF NOT EXISTS np_github_teams (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  node_id VARCHAR(255),
  org_login VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  slug VARCHAR(255) NOT NULL,
  description TEXT,
  privacy VARCHAR(50),
  permission VARCHAR(50),
  parent_id BIGINT,
  members_count INTEGER DEFAULT 0,
  repos_count INTEGER DEFAULT 0,
  html_url VARCHAR(2048),
  created_at TIMESTAMP WITH TIME ZONE,
  updated_at TIMESTAMP WITH TIME ZONE,
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_teams_org ON np_github_teams(org_login);
CREATE INDEX IF NOT EXISTS idx_np_github_teams_slug ON np_github_teams(slug);
CREATE INDEX IF NOT EXISTS idx_np_github_teams_source ON np_github_teams(source_account_id);

-- Collaborators
CREATE TABLE IF NOT EXISTS np_github_collaborators (
  id BIGINT NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  repo_id BIGINT,
  login VARCHAR(255) NOT NULL,
  type VARCHAR(50),
  site_admin BOOLEAN DEFAULT FALSE,
  permissions JSONB DEFAULT '{}',
  role_name VARCHAR(50),
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (repo_id, id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_collaborators_login ON np_github_collaborators(login);
CREATE INDEX IF NOT EXISTS idx_np_github_collaborators_source ON np_github_collaborators(source_account_id);

-- Webhook Events
CREATE TABLE IF NOT EXISTS np_github_webhook_events (
  id VARCHAR(255) NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event VARCHAR(100) NOT NULL,
  action VARCHAR(100),
  repo_id BIGINT,
  repo_full_name VARCHAR(255),
  sender_login VARCHAR(255),
  data JSONB NOT NULL,
  processed BOOLEAN DEFAULT FALSE,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (id, source_account_id)
);
CREATE INDEX IF NOT EXISTS idx_np_github_events_event ON np_github_webhook_events(event);
CREATE INDEX IF NOT EXISTS idx_np_github_events_repo ON np_github_webhook_events(repo_id);
CREATE INDEX IF NOT EXISTS idx_np_github_events_processed ON np_github_webhook_events(processed);
CREATE INDEX IF NOT EXISTS idx_np_github_events_received ON np_github_webhook_events(received_at);
CREATE INDEX IF NOT EXISTS idx_np_github_events_source ON np_github_webhook_events(source_account_id);

-- Views
CREATE OR REPLACE VIEW np_github_open_items AS
SELECT
  r.source_account_id,
  r.full_name AS repo,
  COUNT(DISTINCT i.id) FILTER (WHERE i.state = 'open') AS open_issues,
  COUNT(DISTINCT p.id) FILTER (WHERE p.state = 'open') AS open_prs,
  COUNT(DISTINCT p.id) FILTER (WHERE p.state = 'open' AND p.draft = TRUE) AS draft_prs
FROM np_github_repositories r
LEFT JOIN np_github_issues i ON r.id = i.repo_id AND r.source_account_id = i.source_account_id
LEFT JOIN np_github_pull_requests p ON r.id = p.repo_id AND r.source_account_id = p.source_account_id
GROUP BY r.source_account_id, r.full_name
ORDER BY open_issues + open_prs DESC;

CREATE OR REPLACE VIEW np_github_recent_activity AS
SELECT
  c.source_account_id,
  'commit' AS type,
  c.sha AS id,
  LEFT(c.message, 100) AS title,
  c.author_login AS user_login,
  r.full_name AS repo,
  c.author_date AS created_at
FROM np_github_commits c
JOIN np_github_repositories r ON c.repo_id = r.id AND c.source_account_id = r.source_account_id
WHERE c.author_date > NOW() - INTERVAL '7 days'
UNION ALL
SELECT
  p.source_account_id,
  'pr' AS type,
  p.id::text AS id,
  p.title,
  p.user_login,
  r.full_name AS repo,
  p.created_at
FROM np_github_pull_requests p
JOIN np_github_repositories r ON p.repo_id = r.id AND p.source_account_id = r.source_account_id
WHERE p.created_at > NOW() - INTERVAL '7 days'
UNION ALL
SELECT
  i.source_account_id,
  'issue' AS type,
  i.id::text AS id,
  i.title,
  i.user_login,
  r.full_name AS repo,
  i.created_at
FROM np_github_issues i
JOIN np_github_repositories r ON i.repo_id = r.id AND i.source_account_id = r.source_account_id
WHERE i.created_at > NOW() - INTERVAL '7 days'
ORDER BY created_at DESC
LIMIT 100;

CREATE OR REPLACE VIEW np_github_workflow_stats AS
SELECT
  w.source_account_id,
  r.full_name AS repo,
  w.workflow_name,
  COUNT(*) AS total_runs,
  COUNT(*) FILTER (WHERE w.conclusion = 'success') AS success,
  COUNT(*) FILTER (WHERE w.conclusion = 'failure') AS failure,
  COUNT(*) FILTER (WHERE w.conclusion = 'cancelled') AS cancelled,
  ROUND(
    COUNT(*) FILTER (WHERE w.conclusion = 'success')::numeric /
    NULLIF(COUNT(*) FILTER (WHERE w.conclusion IS NOT NULL), 0) * 100, 2
  ) AS success_rate,
  AVG(EXTRACT(EPOCH FROM (w.updated_at - w.run_started_at))) FILTER (WHERE w.conclusion IS NOT NULL) AS avg_duration_seconds
FROM np_github_workflow_runs w
JOIN np_github_repositories r ON w.repo_id = r.id AND w.source_account_id = r.source_account_id
WHERE w.created_at > NOW() - INTERVAL '30 days'
GROUP BY w.source_account_id, r.full_name, w.workflow_name
ORDER BY total_runs DESC;

CREATE OR REPLACE VIEW np_github_pr_review_stats AS
SELECT
  pr.source_account_id,
  r.full_name AS repo,
  COUNT(DISTINCT pr.id) AS total_prs,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.merged = TRUE) AS merged_prs,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.state = 'closed' AND pr.merged = FALSE) AS closed_without_merge,
  AVG(EXTRACT(EPOCH FROM (COALESCE(pr.merged_at, pr.closed_at) - pr.created_at)) / 3600) FILTER (WHERE pr.state = 'closed') AS avg_hours_to_close,
  ROUND(AVG(pr.review_comments)::numeric, 1) AS avg_review_comments,
  ROUND(AVG(pr.commits)::numeric, 1) AS avg_commits
FROM np_github_pull_requests pr
JOIN np_github_repositories r ON pr.repo_id = r.id AND pr.source_account_id = r.source_account_id
WHERE pr.created_at > NOW() - INTERVAL '90 days'
GROUP BY pr.source_account_id, r.full_name
ORDER BY total_prs DESC;

CREATE OR REPLACE VIEW np_github_contributor_stats AS
SELECT
  c.source_account_id,
  c.author_login AS contributor,
  r.full_name AS repo,
  COUNT(DISTINCT c.sha) AS commit_count,
  SUM(c.additions) AS total_additions,
  SUM(c.deletions) AS total_deletions,
  COUNT(DISTINCT pr.id) AS prs_opened,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.merged = TRUE) AS prs_merged
FROM np_github_commits c
JOIN np_github_repositories r ON c.repo_id = r.id AND c.source_account_id = r.source_account_id
LEFT JOIN np_github_pull_requests pr ON pr.repo_id = r.id AND pr.user_login = c.author_login AND pr.source_account_id = c.source_account_id
WHERE c.author_date > NOW() - INTERVAL '90 days'
  AND c.author_login IS NOT NULL
GROUP BY c.source_account_id, c.author_login, r.full_name
ORDER BY commit_count DESC;

CREATE OR REPLACE VIEW np_github_milestone_progress AS
SELECT
  m.source_account_id,
  r.full_name AS repo,
  m.title AS milestone,
  m.state,
  m.open_issues,
  m.closed_issues,
  m.open_issues + m.closed_issues AS total_issues,
  ROUND(
    m.closed_issues::numeric / NULLIF(m.open_issues + m.closed_issues, 0) * 100, 2
  ) AS completion_percent,
  m.due_on
FROM np_github_milestones m
JOIN np_github_repositories r ON m.repo_id = r.id AND m.source_account_id = r.source_account_id
ORDER BY m.due_on ASC NULLS LAST;

CREATE OR REPLACE VIEW np_github_check_status AS
SELECT
  cs.source_account_id,
  r.full_name AS repo,
  cs.head_branch AS branch,
  cs.head_sha,
  cs.status,
  cs.conclusion,
  COUNT(cr.id) AS total_checks,
  COUNT(cr.id) FILTER (WHERE cr.conclusion = 'success') AS passed,
  COUNT(cr.id) FILTER (WHERE cr.conclusion = 'failure') AS failed,
  cs.updated_at
FROM np_github_check_suites cs
JOIN np_github_repositories r ON cs.repo_id = r.id AND cs.source_account_id = r.source_account_id
LEFT JOIN np_github_check_runs cr ON cr.check_suite_id = cs.id AND cr.source_account_id = cs.source_account_id
WHERE cs.created_at > NOW() - INTERVAL '7 days'
GROUP BY cs.source_account_id, r.full_name, cs.head_branch, cs.head_sha, cs.status, cs.conclusion, cs.updated_at
ORDER BY cs.updated_at DESC;
`
