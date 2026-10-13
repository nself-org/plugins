-- Boot-kit legacy fixture (tests/boot/tables-upgrade.sh): the github_* tables an install from before the np_ prefix had,
-- with one row each (fixed primary keys, different from seed.sql). Same columns as the current tables. Test data only.

CREATE TABLE github_organizations (
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
INSERT INTO github_organizations (id, login) VALUES (9101, 'seed');

CREATE TABLE github_repositories (
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
INSERT INTO github_repositories (id, name, full_name, owner_login) VALUES (9101, 'seed', 'seed', 'seed');

CREATE TABLE github_branches (
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
INSERT INTO github_branches (id, name, sha) VALUES ('legacy-github_branches', 'seed', 'seed');

CREATE TABLE github_issues (
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
INSERT INTO github_issues (id, number, title) VALUES (9101, 1, 'seed');

CREATE TABLE github_pull_requests (
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
INSERT INTO github_pull_requests (id, number, title) VALUES (9101, 1, 'seed');

CREATE TABLE github_pr_reviews (
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
INSERT INTO github_pr_reviews (id) VALUES (9101);

CREATE TABLE github_issue_comments (
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
INSERT INTO github_issue_comments (id) VALUES (9101);

CREATE TABLE github_pr_review_comments (
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
INSERT INTO github_pr_review_comments (id) VALUES (9101);

CREATE TABLE github_commit_comments (
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
INSERT INTO github_commit_comments (id, commit_sha) VALUES (9101, 'seed');

CREATE TABLE github_commits (
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
INSERT INTO github_commits (sha) VALUES ('legacy-github_commits');

CREATE TABLE github_releases (
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
INSERT INTO github_releases (id, tag_name) VALUES (9101, 'seed');

CREATE TABLE github_tags (
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
INSERT INTO github_tags (id, name, sha) VALUES ('legacy-github_tags', 'seed', 'seed');

CREATE TABLE github_milestones (
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
INSERT INTO github_milestones (id, number, title) VALUES (9101, 1, 'seed');

CREATE TABLE github_labels (
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
INSERT INTO github_labels (id, name) VALUES (9101, 'seed');

CREATE TABLE github_workflows (
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
INSERT INTO github_workflows (id, name) VALUES (9101, 'seed');

CREATE TABLE github_workflow_runs (
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
INSERT INTO github_workflow_runs (id) VALUES (9101);

CREATE TABLE github_workflow_jobs (
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
INSERT INTO github_workflow_jobs (id, name) VALUES (9101, 'seed');

CREATE TABLE github_check_suites (
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
INSERT INTO github_check_suites (id, head_sha) VALUES (9101, 'seed');

CREATE TABLE github_check_runs (
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
INSERT INTO github_check_runs (id, head_sha, name) VALUES (9101, 'seed', 'seed');

CREATE TABLE github_deployments (
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
INSERT INTO github_deployments (id) VALUES (9101);

CREATE TABLE github_teams (
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
INSERT INTO github_teams (id, org_login, name, slug) VALUES (9101, 'seed', 'seed', 'seed');

CREATE TABLE github_collaborators (
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
INSERT INTO github_collaborators (id, login) VALUES (9101, 'seed');

CREATE TABLE github_webhook_events (
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
INSERT INTO github_webhook_events (id, event) VALUES ('legacy-github_webhook_events', 'seed');

CREATE INDEX idx_github_organizations_login ON github_organizations (login);
