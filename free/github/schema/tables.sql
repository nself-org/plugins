-- =============================================================================
-- GitHub Plugin Schema
-- Tables for storing synced GitHub data
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Repositories
-- =============================================================================

CREATE TABLE IF NOT EXISTS github_repositories (
    id BIGINT PRIMARY KEY,                          -- GitHub repo ID
    node_id VARCHAR(255),
    name VARCHAR(255) NOT NULL,
    full_name VARCHAR(255) NOT NULL,                -- owner/repo
    owner_login VARCHAR(255) NOT NULL,
    owner_type VARCHAR(50),                         -- User, Organization
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
    pushed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_github_repos_owner ON github_repositories(owner_login);
CREATE INDEX IF NOT EXISTS idx_github_repos_name ON github_repositories(full_name);
CREATE INDEX IF NOT EXISTS idx_github_repos_language ON github_repositories(language);

-- =============================================================================
-- Issues
-- =============================================================================

CREATE TABLE IF NOT EXISTS github_issues (
    id BIGINT PRIMARY KEY,                          -- GitHub issue ID
    node_id VARCHAR(255),
    repo_id BIGINT REFERENCES github_repositories(id),
    number INTEGER NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    state VARCHAR(20) NOT NULL,                     -- open, closed
    state_reason VARCHAR(50),                       -- completed, not_planned, reopened
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_github_issues_repo ON github_issues(repo_id);
CREATE INDEX IF NOT EXISTS idx_github_issues_state ON github_issues(state);
CREATE INDEX IF NOT EXISTS idx_github_issues_user ON github_issues(user_login);
CREATE INDEX IF NOT EXISTS idx_github_issues_created ON github_issues(created_at);

-- =============================================================================
-- Pull Requests
-- =============================================================================

CREATE TABLE IF NOT EXISTS github_pull_requests (
    id BIGINT PRIMARY KEY,                          -- GitHub PR ID
    node_id VARCHAR(255),
    repo_id BIGINT REFERENCES github_repositories(id),
    number INTEGER NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    state VARCHAR(20) NOT NULL,                     -- open, closed
    draft BOOLEAN DEFAULT FALSE,
    locked BOOLEAN DEFAULT FALSE,
    user_login VARCHAR(255),
    user_id BIGINT,
    head_ref VARCHAR(255),                          -- Source branch
    head_sha VARCHAR(40),
    base_ref VARCHAR(255),                          -- Target branch
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_github_prs_repo ON github_pull_requests(repo_id);
CREATE INDEX IF NOT EXISTS idx_github_prs_state ON github_pull_requests(state);
CREATE INDEX IF NOT EXISTS idx_github_prs_user ON github_pull_requests(user_login);
CREATE INDEX IF NOT EXISTS idx_github_prs_merged ON github_pull_requests(merged);
CREATE INDEX IF NOT EXISTS idx_github_prs_created ON github_pull_requests(created_at);

-- =============================================================================
-- Commits
-- =============================================================================

CREATE TABLE IF NOT EXISTS github_commits (
    sha VARCHAR(40) PRIMARY KEY,
    node_id VARCHAR(255),
    repo_id BIGINT REFERENCES github_repositories(id),
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_github_commits_repo ON github_commits(repo_id);
CREATE INDEX IF NOT EXISTS idx_github_commits_author ON github_commits(author_login);
CREATE INDEX IF NOT EXISTS idx_github_commits_date ON github_commits(author_date);

-- =============================================================================
-- Releases
-- =============================================================================

CREATE TABLE IF NOT EXISTS github_releases (
    id BIGINT PRIMARY KEY,                          -- GitHub release ID
    node_id VARCHAR(255),
    repo_id BIGINT REFERENCES github_repositories(id),
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_github_releases_repo ON github_releases(repo_id);
CREATE INDEX IF NOT EXISTS idx_github_releases_tag ON github_releases(tag_name);
CREATE INDEX IF NOT EXISTS idx_github_releases_published ON github_releases(published_at);

-- =============================================================================
-- Workflow Runs (GitHub Actions)
-- =============================================================================

CREATE TABLE IF NOT EXISTS github_workflow_runs (
    id BIGINT PRIMARY KEY,                          -- GitHub workflow run ID
    node_id VARCHAR(255),
    repo_id BIGINT REFERENCES github_repositories(id),
    workflow_id BIGINT,
    workflow_name VARCHAR(255),
    name VARCHAR(255),                              -- Run name
    head_branch VARCHAR(255),
    head_sha VARCHAR(40),
    run_number INTEGER,
    run_attempt INTEGER DEFAULT 1,
    event VARCHAR(50),                              -- push, pull_request, etc.
    status VARCHAR(50),                             -- queued, in_progress, completed
    conclusion VARCHAR(50),                         -- success, failure, cancelled, etc.
    actor_login VARCHAR(255),
    triggering_actor_login VARCHAR(255),
    html_url VARCHAR(2048),
    jobs_url VARCHAR(2048),
    logs_url VARCHAR(2048),
    run_started_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_github_workflows_repo ON github_workflow_runs(repo_id);
CREATE INDEX IF NOT EXISTS idx_github_workflows_status ON github_workflow_runs(status);
CREATE INDEX IF NOT EXISTS idx_github_workflows_conclusion ON github_workflow_runs(conclusion);
CREATE INDEX IF NOT EXISTS idx_github_workflows_event ON github_workflow_runs(event);
CREATE INDEX IF NOT EXISTS idx_github_workflows_created ON github_workflow_runs(created_at);

-- =============================================================================
-- Deployments
-- =============================================================================

CREATE TABLE IF NOT EXISTS github_deployments (
    id BIGINT PRIMARY KEY,                          -- GitHub deployment ID
    node_id VARCHAR(255),
    repo_id BIGINT REFERENCES github_repositories(id),
    sha VARCHAR(40),
    ref VARCHAR(255),
    task VARCHAR(255) DEFAULT 'deploy',
    environment VARCHAR(255),
    description TEXT,
    creator_login VARCHAR(255),
    statuses JSONB DEFAULT '[]',                    -- Latest status updates
    current_status VARCHAR(50),                     -- success, failure, pending, etc.
    production_environment BOOLEAN DEFAULT FALSE,
    transient_environment BOOLEAN DEFAULT FALSE,
    payload JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_github_deployments_repo ON github_deployments(repo_id);
CREATE INDEX IF NOT EXISTS idx_github_deployments_env ON github_deployments(environment);
CREATE INDEX IF NOT EXISTS idx_github_deployments_status ON github_deployments(current_status);

-- =============================================================================
-- Webhook Events
-- =============================================================================

CREATE TABLE IF NOT EXISTS github_webhook_events (
    id VARCHAR(255) PRIMARY KEY,                    -- GitHub delivery ID
    event VARCHAR(100) NOT NULL,                    -- Event type
    action VARCHAR(100),                            -- Event action
    repo_id BIGINT,
    repo_full_name VARCHAR(255),
    sender_login VARCHAR(255),
    data JSONB NOT NULL,
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_github_events_event ON github_webhook_events(event);
CREATE INDEX IF NOT EXISTS idx_github_events_repo ON github_webhook_events(repo_id);
CREATE INDEX IF NOT EXISTS idx_github_events_processed ON github_webhook_events(processed);
CREATE INDEX IF NOT EXISTS idx_github_events_received ON github_webhook_events(received_at);

-- =============================================================================
-- Views
-- =============================================================================

-- Open issues and PRs by repo
CREATE OR REPLACE VIEW github_open_items AS
SELECT
    r.full_name AS repo,
    COUNT(DISTINCT i.id) FILTER (WHERE i.state = 'open') AS open_issues,
    COUNT(DISTINCT p.id) FILTER (WHERE p.state = 'open') AS open_prs
FROM github_repositories r
LEFT JOIN github_issues i ON r.id = i.repo_id
LEFT JOIN github_pull_requests p ON r.id = p.repo_id
GROUP BY r.full_name
ORDER BY open_issues + open_prs DESC;

-- Recent activity
CREATE OR REPLACE VIEW github_recent_activity AS
SELECT
    'commit' AS type,
    c.sha AS id,
    c.message AS title,
    c.author_login AS user,
    r.full_name AS repo,
    c.author_date AS created_at
FROM github_commits c
JOIN github_repositories r ON c.repo_id = r.id
WHERE c.author_date > NOW() - INTERVAL '7 days'
UNION ALL
SELECT
    'pr' AS type,
    p.id::text AS id,
    p.title,
    p.user_login AS user,
    r.full_name AS repo,
    p.created_at
FROM github_pull_requests p
JOIN github_repositories r ON p.repo_id = r.id
WHERE p.created_at > NOW() - INTERVAL '7 days'
UNION ALL
SELECT
    'issue' AS type,
    i.id::text AS id,
    i.title,
    i.user_login AS user,
    r.full_name AS repo,
    i.created_at
FROM github_issues i
JOIN github_repositories r ON i.repo_id = r.id
WHERE i.created_at > NOW() - INTERVAL '7 days'
ORDER BY created_at DESC
LIMIT 100;

-- Workflow success rate
CREATE OR REPLACE VIEW github_workflow_stats AS
SELECT
    r.full_name AS repo,
    w.workflow_name,
    COUNT(*) AS total_runs,
    COUNT(*) FILTER (WHERE w.conclusion = 'success') AS success,
    COUNT(*) FILTER (WHERE w.conclusion = 'failure') AS failure,
    ROUND(
        COUNT(*) FILTER (WHERE w.conclusion = 'success')::numeric /
        NULLIF(COUNT(*), 0) * 100, 2
    ) AS success_rate
FROM github_workflow_runs w
JOIN github_repositories r ON w.repo_id = r.id
WHERE w.created_at > NOW() - INTERVAL '30 days'
GROUP BY r.full_name, w.workflow_name
ORDER BY total_runs DESC;
