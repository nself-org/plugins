# GitHub Plugin for nself

Comprehensive GitHub integration that syncs repositories, issues, pull requests, commits, releases, and workflow data to your local PostgreSQL database with real-time webhook support.

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhooks](#webhooks)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [Use Cases](#use-cases)
- [TypeScript Implementation](#typescript-implementation)
- [Troubleshooting](#troubleshooting)

---

## Overview

The GitHub plugin provides complete synchronization between GitHub and your local database. It captures all aspects of your GitHub workflow including repositories, issues, pull requests, commits, releases, deployments, and GitHub Actions workflow runs.

### Why Sync GitHub Data Locally?

1. **Faster Queries** - Query your entire GitHub history without API calls or rate limits
2. **Cross-Repository Analytics** - Aggregate data across all repos in a single SQL query
3. **Custom Dashboards** - Build engineering metrics dashboards with your synced data
4. **Offline Access** - Your GitHub data is always available, even without internet
5. **Real-Time Updates** - Webhooks keep your local data in sync as changes happen
6. **Historical Analysis** - Track trends over time with full historical data

---

## Features

### Data Synchronization

| Resource | Synced Data | Incremental Sync |
|----------|-------------|------------------|
| Repositories | All metadata, settings, topics | Yes |
| Issues | Full issue data with labels, assignees | Yes |
| Pull Requests | PR data, reviews, comments | Yes |
| Commits | Commit history with diffs | Yes |
| Releases | Release versions with assets | Yes |
| Branches | Branch list and protection rules | Yes |
| Tags | Tag list with commit refs | Yes |
| Milestones | Milestone tracking | Yes |
| Labels | All repository labels | Yes |
| Workflow Runs | GitHub Actions run history | Yes |
| Workflow Jobs | Individual job details | Yes |
| Check Suites | Check suite results | Yes |
| Check Runs | Individual check results | Yes |
| Deployments | Deployment history | Yes |
| Teams | Organization team data | Yes |
| Collaborators | Repository collaborators | Yes |

### Real-Time Webhooks

Supported webhook events for instant updates:

- `push` - Code pushed to any branch
- `pull_request` - PR opened, closed, merged, synchronized
- `pull_request_review` - PR review submitted, approved, rejected
- `pull_request_review_comment` - Comments on PR diffs
- `issues` - Issue created, updated, labeled, closed
- `issue_comment` - Comments on issues and PRs
- `release` - New release published
- `workflow_run` - GitHub Actions workflow completed
- `workflow_job` - Individual job completed
- `check_suite` - Check suite completed
- `check_run` - Individual check completed
- `deployment` - New deployment created
- `deployment_status` - Deployment status changed
- `create` - Branch or tag created
- `delete` - Branch or tag deleted
- `repository` - Repository created, deleted, settings changed
- `star` - Repository starred/unstarred
- `fork` - Repository forked
- `branch_protection_rule` - Branch protection changed
- `label` - Label created, updated, deleted
- `milestone` - Milestone created, updated, deleted
- `team` - Team created, updated, deleted
- `member` - Collaborator added/removed
- `commit_comment` - Comment on a commit

---

## Installation

### Via nself CLI

```bash
# Install the plugin
nself plugin install github

# Verify installation
nself plugin status github
```

### Manual Installation

```bash
# Clone the repository
git clone https://github.com/acamarata/nself-plugins.git
cd nself-plugins/plugins/github/ts

# Install dependencies
npm install

# Build
npm run build

# Link for CLI access
npm link
```

---

## Configuration

### Environment Variables

Create a `.env` file in the plugin directory or add to your project's `.env`:

```bash
# Required - GitHub Personal Access Token
# Generate at: https://github.com/settings/tokens
# Required scopes: repo, read:org, workflow, read:user
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# Required - PostgreSQL connection string
DATABASE_URL=postgresql://user:password@localhost:5432/nself

# Optional - Webhook signing secret
# Get from: Repository Settings > Webhooks > Edit > Secret
GITHUB_WEBHOOK_SECRET=your_webhook_secret

# Optional - Organization to sync (syncs all accessible repos if not set)
GITHUB_ORG=your-organization

# Optional - Specific repos to sync (comma-separated)
# Format: owner/repo,owner/repo2
GITHUB_REPOS=acamarata/nself,acamarata/nself-plugins

# Optional - Server configuration
PORT=3002
HOST=0.0.0.0

# Optional - Sync interval in seconds (default: 3600)
GITHUB_SYNC_INTERVAL=3600
```

### GitHub Token Permissions

When creating a Personal Access Token (PAT), enable these scopes:

| Scope | Purpose |
|-------|---------|
| `repo` | Full access to repositories (issues, PRs, commits) |
| `read:org` | Read organization data (teams, members) |
| `workflow` | Access to GitHub Actions workflow data |
| `read:user` | Read user profile data |
| `admin:repo_hook` | Required for webhook management |

For fine-grained PATs, select:
- **Repository access**: All repositories or specific ones
- **Permissions**: Issues (read), Pull requests (read), Contents (read), Metadata (read), Workflows (read), Deployments (read)

---

## Usage

### Initialize Database Schema

```bash
# Create all required tables
nself-github init

# Or via nself CLI
nself plugin github init
```

### Sync Data

```bash
# Sync all data from GitHub
nself-github sync

# Sync specific resources
nself-github sync --resources repos,issues,prs

# Incremental sync (only changes since last sync)
nself-github sync --incremental

# Sync a specific repository
nself-github sync --repo acamarata/nself
```

### Start Webhook Server

```bash
# Start the server
nself-github server

# Custom port
nself-github server --port 3002

# The server exposes:
# - POST /webhook - GitHub webhook endpoint
# - GET /health - Health check
# - GET /api/* - REST API endpoints
```

---

## CLI Commands

### Repository Commands

```bash
# List all synced repositories
nself-github repos list

# List with details
nself-github repos list --details

# Search repositories
nself-github repos search "keyword"

# Get repository details
nself-github repos get owner/repo

# Show repository statistics
nself-github repos stats owner/repo
```

### Issue Commands

```bash
# List all issues
nself-github issues list

# Filter by state
nself-github issues list --state open
nself-github issues list --state closed

# Filter by repository
nself-github issues list --repo owner/repo

# Filter by labels
nself-github issues list --labels bug,urgent

# Filter by assignee
nself-github issues list --assignee username

# Get issue details
nself-github issues get owner/repo 123
```

### Pull Request Commands

```bash
# List all pull requests
nself-github prs list

# Filter by state
nself-github prs list --state open
nself-github prs list --state merged
nself-github prs list --state closed

# Filter by repository
nself-github prs list --repo owner/repo

# Filter by author
nself-github prs list --author username

# Get PR details
nself-github prs get owner/repo 456
```

### Release Commands

```bash
# List all releases
nself-github releases list

# Filter by repository
nself-github releases list --repo owner/repo

# Get latest release
nself-github releases latest owner/repo
```

### Workflow Commands

```bash
# List workflow runs
nself-github actions list

# Filter by status
nself-github actions list --status success
nself-github actions list --status failure

# Filter by repository
nself-github actions list --repo owner/repo

# Get run details
nself-github actions get 12345678
```

### Status Command

```bash
# Show sync status and statistics
nself-github status

# Output:
# Repositories: 25
# Issues: 1,234 (456 open)
# Pull Requests: 789 (23 open)
# Commits: 45,678
# Releases: 156
# Workflow Runs: 3,456
# Last Sync: 2026-01-24 12:00:00
```

---

## REST API

The plugin exposes a REST API when running in server mode.

### Endpoints

#### Health Check

```http
GET /health
```

Response:
```json
{
  "status": "ok",
  "version": "1.0.0",
  "uptime": 3600
}
```

#### Sync Trigger

```http
POST /api/sync
Content-Type: application/json

{
  "resources": ["repos", "issues", "prs"],
  "incremental": true
}
```

Response:
```json
{
  "results": [
    { "resource": "repos", "synced": 25, "duration": 1234 },
    { "resource": "issues", "synced": 156, "duration": 5678 },
    { "resource": "prs", "synced": 45, "duration": 2345 }
  ]
}
```

#### Sync Status

```http
GET /api/status
```

Response:
```json
{
  "stats": {
    "repositories": 25,
    "issues": 1234,
    "pull_requests": 789,
    "commits": 45678,
    "releases": 156,
    "workflow_runs": 3456,
    "deployments": 234
  },
  "last_sync": "2026-01-24T12:00:00Z"
}
```

#### Repositories

```http
GET /api/repositories
GET /api/repositories?limit=50&offset=0
GET /api/repositories/:owner/:repo
GET /api/repositories/:owner/:repo/issues
GET /api/repositories/:owner/:repo/pull-requests
GET /api/repositories/:owner/:repo/commits
GET /api/repositories/:owner/:repo/releases
```

#### Issues

```http
GET /api/issues
GET /api/issues?state=open&repo=owner/repo
GET /api/issues/:owner/:repo/:number
GET /api/issues/:owner/:repo/:number/comments
```

#### Pull Requests

```http
GET /api/pull-requests
GET /api/pull-requests?state=open&repo=owner/repo
GET /api/pull-requests/:owner/:repo/:number
GET /api/pull-requests/:owner/:repo/:number/reviews
GET /api/pull-requests/:owner/:repo/:number/comments
```

#### Commits

```http
GET /api/commits
GET /api/commits?repo=owner/repo&since=2026-01-01
GET /api/commits/:owner/:repo/:sha
GET /api/commits/:owner/:repo/:sha/comments
```

#### Releases

```http
GET /api/releases
GET /api/releases?repo=owner/repo
GET /api/releases/:owner/:repo/:tag
```

#### Workflow Runs

```http
GET /api/workflow-runs
GET /api/workflow-runs?repo=owner/repo&status=success
GET /api/workflow-runs/:id
GET /api/workflow-runs/:id/jobs
```

#### Deployments

```http
GET /api/deployments
GET /api/deployments?repo=owner/repo
GET /api/deployments/:id
```

#### Teams & Collaborators

```http
GET /api/teams
GET /api/teams/:org/:team_slug
GET /api/collaborators?repo=owner/repo
```

---

## Webhooks

### Webhook Setup

1. Go to your repository or organization settings
2. Navigate to **Webhooks** > **Add webhook**
3. Configure:
   - **Payload URL**: `https://your-domain.com/webhook`
   - **Content type**: `application/json`
   - **Secret**: Your `GITHUB_WEBHOOK_SECRET` value
   - **Events**: Select events or "Send me everything"

### Webhook Endpoint

```http
POST /webhook
X-GitHub-Event: push
X-Hub-Signature-256: sha256=...
X-GitHub-Delivery: uuid

{
  "action": "...",
  "repository": { ... },
  ...
}
```

### Signature Verification

The plugin verifies all incoming webhooks using HMAC-SHA256:

```typescript
// Verification uses X-Hub-Signature-256 header
const signature = request.headers['x-hub-signature-256'];
const expected = 'sha256=' + hmac('sha256', secret, rawBody);
```

### Event Handling

Each webhook event is:
1. Verified for signature
2. Stored in `github_webhook_events` table
3. Processed by appropriate handler
4. Used to update synced data

---

## Database Schema

### Tables

#### github_repositories

```sql
CREATE TABLE github_repositories (
    id BIGINT PRIMARY KEY,
    node_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    full_name VARCHAR(511) NOT NULL UNIQUE,
    owner_login VARCHAR(255) NOT NULL,
    owner_type VARCHAR(50),
    private BOOLEAN DEFAULT FALSE,
    description TEXT,
    fork BOOLEAN DEFAULT FALSE,
    homepage VARCHAR(255),
    language VARCHAR(100),
    forks_count INTEGER DEFAULT 0,
    stargazers_count INTEGER DEFAULT 0,
    watchers_count INTEGER DEFAULT 0,
    open_issues_count INTEGER DEFAULT 0,
    default_branch VARCHAR(255) DEFAULT 'main',
    topics JSONB DEFAULT '[]',
    visibility VARCHAR(50),
    archived BOOLEAN DEFAULT FALSE,
    disabled BOOLEAN DEFAULT FALSE,
    pushed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### github_issues

```sql
CREATE TABLE github_issues (
    id BIGINT PRIMARY KEY,
    node_id VARCHAR(255) NOT NULL,
    repo_id BIGINT REFERENCES github_repositories(id) ON DELETE CASCADE,
    number INTEGER NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    state VARCHAR(50) NOT NULL,
    state_reason VARCHAR(100),
    user_login VARCHAR(255),
    user_id BIGINT,
    assignees JSONB DEFAULT '[]',
    labels JSONB DEFAULT '[]',
    milestone_id BIGINT,
    milestone_title VARCHAR(255),
    comments INTEGER DEFAULT 0,
    locked BOOLEAN DEFAULT FALSE,
    closed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(repo_id, number)
);
```

#### github_pull_requests

```sql
CREATE TABLE github_pull_requests (
    id BIGINT PRIMARY KEY,
    node_id VARCHAR(255) NOT NULL,
    repo_id BIGINT REFERENCES github_repositories(id) ON DELETE CASCADE,
    number INTEGER NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    state VARCHAR(50) NOT NULL,
    user_login VARCHAR(255),
    user_id BIGINT,
    head_ref VARCHAR(255),
    head_sha VARCHAR(40),
    base_ref VARCHAR(255),
    base_sha VARCHAR(40),
    merged BOOLEAN DEFAULT FALSE,
    mergeable BOOLEAN,
    merged_at TIMESTAMP WITH TIME ZONE,
    merged_by_login VARCHAR(255),
    merge_commit_sha VARCHAR(40),
    assignees JSONB DEFAULT '[]',
    labels JSONB DEFAULT '[]',
    milestone_id BIGINT,
    draft BOOLEAN DEFAULT FALSE,
    additions INTEGER DEFAULT 0,
    deletions INTEGER DEFAULT 0,
    changed_files INTEGER DEFAULT 0,
    comments INTEGER DEFAULT 0,
    review_comments INTEGER DEFAULT 0,
    commits INTEGER DEFAULT 0,
    closed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(repo_id, number)
);
```

#### github_commits

```sql
CREATE TABLE github_commits (
    sha VARCHAR(40) PRIMARY KEY,
    repo_id BIGINT REFERENCES github_repositories(id) ON DELETE CASCADE,
    message TEXT NOT NULL,
    author_name VARCHAR(255),
    author_email VARCHAR(255),
    author_date TIMESTAMP WITH TIME ZONE,
    committer_name VARCHAR(255),
    committer_email VARCHAR(255),
    committer_date TIMESTAMP WITH TIME ZONE,
    tree_sha VARCHAR(40),
    parents JSONB DEFAULT '[]',
    additions INTEGER DEFAULT 0,
    deletions INTEGER DEFAULT 0,
    total INTEGER DEFAULT 0,
    files JSONB DEFAULT '[]',
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### github_releases

```sql
CREATE TABLE github_releases (
    id BIGINT PRIMARY KEY,
    repo_id BIGINT REFERENCES github_repositories(id) ON DELETE CASCADE,
    tag_name VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    body TEXT,
    draft BOOLEAN DEFAULT FALSE,
    prerelease BOOLEAN DEFAULT FALSE,
    target_commitish VARCHAR(255),
    author_login VARCHAR(255),
    assets JSONB DEFAULT '[]',
    created_at TIMESTAMP WITH TIME ZONE,
    published_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### github_workflow_runs

```sql
CREATE TABLE github_workflow_runs (
    id BIGINT PRIMARY KEY,
    repo_id BIGINT REFERENCES github_repositories(id) ON DELETE CASCADE,
    name VARCHAR(255),
    workflow_id BIGINT,
    head_branch VARCHAR(255),
    head_sha VARCHAR(40),
    status VARCHAR(50),
    conclusion VARCHAR(50),
    event VARCHAR(100),
    run_number INTEGER,
    run_attempt INTEGER DEFAULT 1,
    actor_login VARCHAR(255),
    triggering_actor_login VARCHAR(255),
    run_started_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### github_webhook_events

```sql
CREATE TABLE github_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    action VARCHAR(100),
    repo_id BIGINT,
    repo_name VARCHAR(511),
    sender_login VARCHAR(255),
    data JSONB NOT NULL,
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### Additional Tables

- `github_branches` - Branch information with protection status
- `github_tags` - Git tags
- `github_milestones` - Milestone tracking
- `github_labels` - Repository labels
- `github_workflow_jobs` - Individual workflow job details
- `github_check_suites` - Check suite results
- `github_check_runs` - Individual check run results
- `github_deployments` - Deployment records
- `github_pr_reviews` - Pull request reviews
- `github_issue_comments` - Issue and PR comments
- `github_pr_review_comments` - PR diff comments
- `github_commit_comments` - Commit comments
- `github_teams` - Organization teams
- `github_collaborators` - Repository collaborators

---

## Analytics Views

Pre-built SQL views for common queries:

### github_open_items

```sql
CREATE VIEW github_open_items AS
SELECT
    r.full_name AS repository,
    'issue' AS type,
    i.number,
    i.title,
    i.user_login AS author,
    i.created_at,
    i.updated_at
FROM github_issues i
JOIN github_repositories r ON i.repo_id = r.id
WHERE i.state = 'open'
UNION ALL
SELECT
    r.full_name,
    'pr' AS type,
    p.number,
    p.title,
    p.user_login,
    p.created_at,
    p.updated_at
FROM github_pull_requests p
JOIN github_repositories r ON p.repo_id = r.id
WHERE p.state = 'open'
ORDER BY updated_at DESC;
```

### github_recent_activity

```sql
CREATE VIEW github_recent_activity AS
SELECT
    r.full_name AS repository,
    c.sha,
    c.message,
    c.author_name,
    c.author_date AS activity_date,
    'commit' AS activity_type
FROM github_commits c
JOIN github_repositories r ON c.repo_id = r.id
WHERE c.author_date > NOW() - INTERVAL '7 days'
ORDER BY c.author_date DESC;
```

### github_workflow_stats

```sql
CREATE VIEW github_workflow_stats AS
SELECT
    r.full_name AS repository,
    w.name AS workflow,
    COUNT(*) AS total_runs,
    COUNT(*) FILTER (WHERE w.conclusion = 'success') AS successful,
    COUNT(*) FILTER (WHERE w.conclusion = 'failure') AS failed,
    ROUND(
        COUNT(*) FILTER (WHERE w.conclusion = 'success')::NUMERIC /
        NULLIF(COUNT(*), 0) * 100, 2
    ) AS success_rate,
    AVG(
        EXTRACT(EPOCH FROM (w.updated_at - w.run_started_at))
    ) AS avg_duration_seconds
FROM github_workflow_runs w
JOIN github_repositories r ON w.repo_id = r.id
WHERE w.created_at > NOW() - INTERVAL '30 days'
GROUP BY r.full_name, w.name;
```

---

## Performance Considerations

### Rate Limiting

GitHub has different rate limits depending on authentication type:

- **Authenticated requests**: 5,000 requests per hour
- **Search API**: 30 requests per minute
- **GraphQL API**: 5,000 points per hour

The plugin includes built-in rate limiting to prevent hitting GitHub's API limits:

```typescript
// Configured automatically in client.ts
const rateLimiter = new RateLimiter(5000 / 3600); // ~1.4 req/sec to stay under hourly limit
```

**Rate Limit Best Practices:**
- Use incremental sync to minimize API calls
- Enable webhooks for real-time updates (no API calls required)
- Monitor rate limit headers: `X-RateLimit-Remaining`, `X-RateLimit-Reset`
- Use conditional requests with ETags when possible

### Database Performance

For optimal performance with large repositories:

```sql
-- Create additional indexes for common query patterns
CREATE INDEX CONCURRENTLY idx_github_issues_labels
    ON github_issues USING GIN(labels);

CREATE INDEX CONCURRENTLY idx_github_prs_merged_at
    ON github_pull_requests(merged_at DESC) WHERE merged = TRUE;

CREATE INDEX CONCURRENTLY idx_github_commits_author_email
    ON github_commits(author_email);

CREATE INDEX CONCURRENTLY idx_github_workflow_runs_conclusion
    ON github_workflow_runs(conclusion) WHERE conclusion != 'success';

-- Partial index for open items only
CREATE INDEX CONCURRENTLY idx_github_issues_open
    ON github_issues(created_at DESC) WHERE state = 'open';

CREATE INDEX CONCURRENTLY idx_github_prs_open
    ON github_pull_requests(created_at DESC) WHERE state = 'open';

-- Analyze tables for query optimization
ANALYZE github_repositories;
ANALYZE github_issues;
ANALYZE github_pull_requests;
ANALYZE github_commits;
ANALYZE github_workflow_runs;
```

### Sync Optimization

**Incremental Sync Strategy:**
```bash
# Full sync (first time only)
nself-github sync

# Incremental sync (hourly via cron)
0 * * * * nself-github sync --incremental --since "1 hour ago"

# Sync specific repositories only
*/15 * * * * nself-github sync --repo owner/critical-repo --incremental
```

**Parallel Sync:**
```typescript
// Sync multiple resources in parallel
await Promise.all([
  syncRepositories({ incremental: true }),
  syncIssues({ incremental: true, since: lastSync }),
  syncPullRequests({ incremental: true, since: lastSync }),
  syncWorkflowRuns({ incremental: true, since: lastSync }),
]);
```

**Pagination Strategy:**
```typescript
// Use cursor-based pagination for large datasets
async function* paginateIssues(owner: string, repo: string) {
  let cursor: string | undefined;

  while (true) {
    const response = await octokit.issues.listForRepo({
      owner,
      repo,
      per_page: 100,
      page: cursor ? parseInt(cursor) : 1,
    });

    if (response.data.length === 0) break;

    yield response.data;
    cursor = (parseInt(cursor || '1') + 1).toString();
  }
}
```

### Connection Pooling

For high-traffic deployments:

```bash
# Increase PostgreSQL connection pool
DATABASE_URL="postgresql://user:pass@localhost:5432/nself?pool_max=20&pool_min=5"
```

### Memory Management

Monitor memory usage for large repositories:

```bash
# Set Node.js heap size for large syncs
NODE_OPTIONS="--max-old-space-size=4096" nself-github sync

# Use streaming for commit history
nself-github sync commits --stream
```

---

## Security Notes

### Token Management

**Production Best Practices:**

1. **Use Fine-Grained Personal Access Tokens (PAT)**: Limit access to specific repositories and permissions
2. **Token Rotation**: Rotate tokens every 90 days
3. **Minimal Permissions**: Only grant read permissions required for sync
4. **Secret Storage**: Never commit tokens to git; use environment variables or secret managers

```bash
# Set via environment variable
export GITHUB_TOKEN="ghp_..."

# Or use a secret manager
aws secretsmanager get-secret-value --secret-id github-token

# Or use GitHub Actions secrets
- uses: actions/checkout@v4
  with:
    token: ${{ secrets.GITHUB_TOKEN }}
```

**Fine-Grained Token Setup:**
1. GitHub Settings → Developer Settings → Personal Access Tokens → Fine-grained tokens
2. Set expiration (max 1 year recommended)
3. Select specific repositories or all repositories
4. Grant permissions:
   - Issues: Read-only
   - Pull requests: Read-only
   - Contents: Read-only
   - Metadata: Read-only (automatically included)
   - Workflows: Read-only
   - Deployments: Read-only

### Webhook Security

**Signature Verification:**
The plugin automatically verifies all incoming webhooks using HMAC-SHA256:

```typescript
// Automatic verification in webhooks.ts
function verifyWebhookSignature(
  payload: string,
  signature: string,
  secret: string
): boolean {
  const hmac = crypto.createHmac('sha256', secret);
  const digest = 'sha256=' + hmac.update(payload).digest('hex');
  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(digest)
  );
}
```

**Webhook Security Checklist:**
- [ ] HTTPS endpoint (required by GitHub)
- [ ] Signature verification enabled (GITHUB_WEBHOOK_SECRET set)
- [ ] Raw request body preserved (no parsing before verification)
- [ ] Webhook events logged for audit trail
- [ ] IP allowlist configured (optional but recommended)
- [ ] Rate limiting on webhook endpoint

**GitHub Webhook IP Ranges:**
```bash
# Get current GitHub webhook IPs
curl https://api.github.com/meta | jq -r '.hooks[]'

# Example firewall rules
iptables -A INPUT -p tcp --dport 3002 -s 192.30.252.0/22 -j ACCEPT
iptables -A INPUT -p tcp --dport 3002 -s 185.199.108.0/22 -j ACCEPT
iptables -A INPUT -p tcp --dport 3002 -s 140.82.112.0/20 -j ACCEPT
```

### Access Control

**Database Permissions:**
```sql
-- Create read-only user for analytics
CREATE USER github_readonly WITH PASSWORD 'secure_password';
GRANT CONNECT ON DATABASE nself TO github_readonly;
GRANT USAGE ON SCHEMA public TO github_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO github_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT ON TABLES TO github_readonly;

-- Create restricted user for plugin (no DELETE)
CREATE USER github_plugin WITH PASSWORD 'secure_password';
GRANT CONNECT ON DATABASE nself TO github_plugin;
GRANT USAGE ON SCHEMA public TO github_plugin;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO github_plugin;
```

**API Access Logging:**
```typescript
// Log all API requests for security audit
const logger = createLogger('github:api');

octokit.hook.before('request', async (options) => {
  logger.info('API request', {
    method: options.method,
    url: options.url,
    timestamp: new Date().toISOString(),
  });
});

octokit.hook.after('request', async (response, options) => {
  logger.info('API response', {
    status: response.status,
    rateLimit: response.headers['x-ratelimit-remaining'],
  });
});
```

### Data Privacy

**Sensitive Data Handling:**

```sql
-- Audit log for sensitive operations
CREATE TABLE github_audit_log (
    id SERIAL PRIMARY KEY,
    action VARCHAR(100) NOT NULL,
    user_id VARCHAR(255),
    resource_type VARCHAR(50),
    resource_id VARCHAR(255),
    details JSONB,
    ip_address INET,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Log webhook processing
INSERT INTO github_audit_log (action, resource_type, resource_id, details)
VALUES ('webhook_received', 'pull_request', $1, $2);
```

**GDPR Compliance:**
```sql
-- Anonymize user data for GDPR compliance
UPDATE github_issues
SET user_login = 'anonymized_user_' || id,
    assignees = '[]'::jsonb
WHERE user_id = $1;

UPDATE github_pull_requests
SET user_login = 'anonymized_user_' || id,
    assignees = '[]'::jsonb
WHERE user_id = $1;

UPDATE github_commits
SET author_email = 'anonymized@example.com',
    committer_email = 'anonymized@example.com'
WHERE author_email = $1 OR committer_email = $1;
```

### Network Security

**Rate Limiting:**
```nginx
# Nginx rate limiting for webhook endpoint
limit_req_zone $binary_remote_addr zone=github_webhook:10m rate=10r/s;

location /webhook {
    limit_req zone=github_webhook burst=20;
    proxy_pass http://localhost:3002;
}
```

**TLS Configuration:**
```nginx
# Strong TLS configuration
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers HIGH:!aNULL:!MD5;
ssl_prefer_server_ciphers on;

# HTTPS only
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
```

---

## Advanced Code Examples

### CI/CD Pipeline Metrics

Track build success rates and durations:

```typescript
import { DatabaseService } from '@nself/github-plugin';

async function calculateCICDMetrics(repoId: bigint, days: number = 30) {
  const db = new DatabaseService();

  const metrics = await db.query(`
    SELECT
      w.name AS workflow_name,
      COUNT(*) AS total_runs,
      COUNT(*) FILTER (WHERE w.conclusion = 'success') AS successful,
      COUNT(*) FILTER (WHERE w.conclusion = 'failure') AS failed,
      COUNT(*) FILTER (WHERE w.conclusion = 'cancelled') AS cancelled,
      ROUND(
        COUNT(*) FILTER (WHERE w.conclusion = 'success')::NUMERIC /
        NULLIF(COUNT(*), 0) * 100, 2
      ) AS success_rate,
      AVG(
        EXTRACT(EPOCH FROM (w.updated_at - w.run_started_at))
      )::INTEGER AS avg_duration_seconds,
      MIN(
        EXTRACT(EPOCH FROM (w.updated_at - w.run_started_at))
      )::INTEGER AS min_duration_seconds,
      MAX(
        EXTRACT(EPOCH FROM (w.updated_at - w.run_started_at))
      )::INTEGER AS max_duration_seconds,
      PERCENTILE_CONT(0.5) WITHIN GROUP (
        ORDER BY EXTRACT(EPOCH FROM (w.updated_at - w.run_started_at))
      )::INTEGER AS median_duration_seconds
    FROM github_workflow_runs w
    WHERE w.repo_id = $1
      AND w.created_at > NOW() - INTERVAL '${days} days'
      AND w.status = 'completed'
    GROUP BY w.name
    ORDER BY total_runs DESC
  `, [repoId]);

  return metrics.rows;
}
```

### Team Productivity Dashboard

Analyze team contributions and velocity:

```sql
-- Developer productivity metrics
CREATE VIEW github_developer_metrics AS
WITH developer_commits AS (
  SELECT
    c.author_name,
    c.author_email,
    DATE_TRUNC('week', c.author_date) AS week,
    COUNT(*) AS commits,
    SUM(c.additions) AS lines_added,
    SUM(c.deletions) AS lines_deleted,
    SUM(c.additions + c.deletions) AS lines_changed,
    COUNT(DISTINCT c.repo_id) AS repos_contributed
  FROM github_commits c
  WHERE c.author_date > NOW() - INTERVAL '90 days'
  GROUP BY c.author_name, c.author_email, DATE_TRUNC('week', c.author_date)
),
developer_prs AS (
  SELECT
    p.user_login,
    DATE_TRUNC('week', p.created_at) AS week,
    COUNT(*) AS prs_created,
    COUNT(*) FILTER (WHERE p.merged = TRUE) AS prs_merged,
    AVG(
      EXTRACT(EPOCH FROM (p.merged_at - p.created_at)) / 3600
    ) AS avg_merge_time_hours
  FROM github_pull_requests p
  WHERE p.created_at > NOW() - INTERVAL '90 days'
  GROUP BY p.user_login, DATE_TRUNC('week', p.created_at)
),
developer_reviews AS (
  SELECT
    r.user_login,
    DATE_TRUNC('week', r.submitted_at) AS week,
    COUNT(*) AS reviews_submitted,
    COUNT(*) FILTER (WHERE r.state = 'approved') AS reviews_approved,
    COUNT(*) FILTER (WHERE r.state = 'changes_requested') AS reviews_requested_changes
  FROM github_pr_reviews r
  WHERE r.submitted_at > NOW() - INTERVAL '90 days'
  GROUP BY r.user_login, DATE_TRUNC('week', r.submitted_at)
)
SELECT
  COALESCE(dc.author_name, dp.user_login, dr.user_login) AS developer,
  COALESCE(dc.week, dp.week, dr.week) AS week,
  COALESCE(dc.commits, 0) AS commits,
  COALESCE(dc.lines_changed, 0) AS lines_changed,
  COALESCE(dc.repos_contributed, 0) AS repos_contributed,
  COALESCE(dp.prs_created, 0) AS prs_created,
  COALESCE(dp.prs_merged, 0) AS prs_merged,
  COALESCE(dp.avg_merge_time_hours, 0) AS avg_pr_merge_hours,
  COALESCE(dr.reviews_submitted, 0) AS reviews_submitted,
  COALESCE(dr.reviews_approved, 0) AS reviews_approved
FROM developer_commits dc
FULL OUTER JOIN developer_prs dp
  ON dc.author_name = dp.user_login AND dc.week = dp.week
FULL OUTER JOIN developer_reviews dr
  ON COALESCE(dc.author_name, dp.user_login) = dr.user_login
  AND COALESCE(dc.week, dp.week) = dr.week
ORDER BY week DESC, commits DESC;
```

### Code Review Analytics

Track PR review quality and speed:

```typescript
async function analyzeCodeReviewMetrics(repoId: bigint) {
  const db = new DatabaseService();

  return db.query(`
    WITH pr_metrics AS (
      SELECT
        p.id,
        p.number,
        p.title,
        p.user_login AS author,
        p.created_at,
        p.merged_at,
        p.closed_at,
        EXTRACT(EPOCH FROM (p.merged_at - p.created_at)) / 3600 AS hours_to_merge,
        p.additions,
        p.deletions,
        p.changed_files,
        (
          SELECT COUNT(*)
          FROM github_pr_reviews r
          WHERE r.pull_request_id = p.id
        ) AS review_count,
        (
          SELECT COUNT(DISTINCT user_login)
          FROM github_pr_reviews r
          WHERE r.pull_request_id = p.id
        ) AS unique_reviewers,
        (
          SELECT MIN(submitted_at)
          FROM github_pr_reviews r
          WHERE r.pull_request_id = p.id
        ) AS first_review_at,
        (
          SELECT COUNT(*)
          FROM github_pr_review_comments c
          WHERE c.pull_request_id = p.id
        ) AS review_comments
      FROM github_pull_requests p
      WHERE p.repo_id = $1
        AND p.merged = TRUE
        AND p.merged_at > NOW() - INTERVAL '90 days'
    )
    SELECT
      author,
      COUNT(*) AS total_prs,
      ROUND(AVG(hours_to_merge), 2) AS avg_hours_to_merge,
      ROUND(AVG(review_count), 2) AS avg_reviews_per_pr,
      ROUND(AVG(unique_reviewers), 2) AS avg_reviewers_per_pr,
      ROUND(AVG(review_comments), 2) AS avg_comments_per_pr,
      ROUND(AVG(
        EXTRACT(EPOCH FROM (first_review_at - created_at)) / 3600
      ), 2) AS avg_hours_to_first_review,
      ROUND(AVG(additions + deletions), 0) AS avg_lines_changed
    FROM pr_metrics
    GROUP BY author
    ORDER BY total_prs DESC
  `, [repoId]);
}
```

### Deployment Tracking

Monitor deployment frequency and success:

```typescript
async function trackDeploymentMetrics() {
  const db = new DatabaseService();

  return db.query(`
    WITH deployment_stats AS (
      SELECT
        r.full_name AS repository,
        d.environment,
        DATE_TRUNC('week', d.created_at) AS week,
        COUNT(*) AS total_deployments,
        COUNT(*) FILTER (WHERE d.status = 'success') AS successful,
        COUNT(*) FILTER (WHERE d.status = 'failure') AS failed,
        AVG(
          EXTRACT(EPOCH FROM (d.updated_at - d.created_at))
        )::INTEGER AS avg_duration_seconds
      FROM github_deployments d
      JOIN github_repositories r ON d.repo_id = r.id
      WHERE d.created_at > NOW() - INTERVAL '90 days'
      GROUP BY r.full_name, d.environment, DATE_TRUNC('week', d.created_at)
    )
    SELECT
      repository,
      environment,
      week,
      total_deployments,
      successful,
      failed,
      ROUND(successful::NUMERIC / NULLIF(total_deployments, 0) * 100, 2) AS success_rate,
      avg_duration_seconds,
      ROUND(avg_duration_seconds / 60.0, 2) AS avg_duration_minutes
    FROM deployment_stats
    ORDER BY week DESC, repository, environment
  `);
}
```

### Issue Triage Automation

Automatically categorize and prioritize issues:

```typescript
async function autoTriageIssues() {
  const db = new DatabaseService();

  // Identify stale issues
  await db.query(`
    UPDATE github_issues
    SET metadata = metadata || jsonb_build_object('triage_status', 'stale')
    WHERE state = 'open'
      AND updated_at < NOW() - INTERVAL '90 days'
      AND NOT (metadata->>'triage_status' = 'stale')
  `);

  // Identify high-priority bugs
  await db.query(`
    UPDATE github_issues
    SET metadata = metadata || jsonb_build_object('priority', 'high')
    WHERE state = 'open'
      AND EXISTS (
        SELECT 1
        FROM jsonb_array_elements(labels) AS label
        WHERE label->>'name' IN ('bug', 'critical', 'urgent')
      )
      AND NOT (metadata->>'priority' IS NOT NULL)
  `);

  // Calculate SLA breach risk
  const slaRisk = await db.query(`
    SELECT
      i.id,
      i.number,
      i.title,
      r.full_name AS repository,
      i.created_at,
      EXTRACT(DAYS FROM NOW() - i.created_at) AS days_open,
      CASE
        WHEN EXISTS (
          SELECT 1 FROM jsonb_array_elements(i.labels) AS l
          WHERE l->>'name' = 'critical'
        ) THEN 1  -- 1 day SLA
        WHEN EXISTS (
          SELECT 1 FROM jsonb_array_elements(i.labels) AS l
          WHERE l->>'name' = 'bug'
        ) THEN 7  -- 7 day SLA
        ELSE 30   -- 30 day SLA
      END AS sla_days,
      CASE
        WHEN EXTRACT(DAYS FROM NOW() - i.created_at) > (
          CASE
            WHEN EXISTS (
              SELECT 1 FROM jsonb_array_elements(i.labels) AS l
              WHERE l->>'name' = 'critical'
            ) THEN 1
            WHEN EXISTS (
              SELECT 1 FROM jsonb_array_elements(i.labels) AS l
              WHERE l->>'name' = 'bug'
            ) THEN 7
            ELSE 30
          END
        ) THEN 'BREACHED'
        ELSE 'OK'
      END AS sla_status
    FROM github_issues i
    JOIN github_repositories r ON i.repo_id = r.id
    WHERE i.state = 'open'
    ORDER BY days_open DESC
  `);

  return slaRisk.rows;
}
```

### Release Notes Generation

Automatically generate release notes from commits and PRs:

```typescript
async function generateReleaseNotes(
  repoId: bigint,
  fromTag: string,
  toTag: string
) {
  const db = new DatabaseService();

  const notes = await db.query(`
    WITH release_commits AS (
      SELECT c.*
      FROM github_commits c
      WHERE c.repo_id = $1
        AND c.author_date >= (
          SELECT created_at FROM github_releases
          WHERE repo_id = $1 AND tag_name = $2
        )
        AND c.author_date <= (
          SELECT created_at FROM github_releases
          WHERE repo_id = $1 AND tag_name = $3
        )
    ),
    release_prs AS (
      SELECT DISTINCT p.*
      FROM github_pull_requests p
      JOIN release_commits c ON p.merge_commit_sha = c.sha
      WHERE p.repo_id = $1
        AND p.merged = TRUE
    )
    SELECT
      'feat' AS type,
      p.title,
      p.number,
      p.user_login AS author,
      p.merged_at
    FROM release_prs p
    WHERE p.title ~* '^feat:|^feature:'

    UNION ALL

    SELECT
      'fix' AS type,
      p.title,
      p.number,
      p.user_login AS author,
      p.merged_at
    FROM release_prs p
    WHERE p.title ~* '^fix:|^bugfix:'

    UNION ALL

    SELECT
      'chore' AS type,
      p.title,
      p.number,
      p.user_login AS author,
      p.merged_at
    FROM release_prs p
    WHERE p.title ~* '^chore:|^refactor:|^docs:'

    ORDER BY type, merged_at DESC
  `, [repoId, fromTag, toTag]);

  // Format as Markdown
  const features = notes.rows.filter(n => n.type === 'feat');
  const fixes = notes.rows.filter(n => n.type === 'fix');
  const chores = notes.rows.filter(n => n.type === 'chore');

  let markdown = `# Release Notes\n\n`;

  if (features.length > 0) {
    markdown += `## Features\n\n`;
    features.forEach(f => {
      markdown += `- ${f.title} (#${f.number}) @${f.author}\n`;
    });
    markdown += `\n`;
  }

  if (fixes.length > 0) {
    markdown += `## Bug Fixes\n\n`;
    fixes.forEach(f => {
      markdown += `- ${f.title} (#${f.number}) @${f.author}\n`;
    });
    markdown += `\n`;
  }

  if (chores.length > 0) {
    markdown += `## Other Changes\n\n`;
    chores.forEach(f => {
      markdown += `- ${f.title} (#${f.number}) @${f.author}\n`;
    });
  }

  return markdown;
}
```

---

## Monitoring & Alerting

### Health Checks

```bash
# Monitor sync health
*/5 * * * * curl -s http://localhost:3002/health | jq -e '.status == "ok"' || alert-team

# Monitor webhook processing
*/10 * * * * psql $DATABASE_URL -c "SELECT COUNT(*) FROM github_webhook_events WHERE processed = FALSE AND received_at < NOW() - INTERVAL '1 hour'" | grep -q "^0$" || alert-team

# Monitor rate limit
*/15 * * * * curl -s http://localhost:3002/api/status | jq -e '.rate_limit.remaining > 1000' || alert-team
```

### Key Metrics to Monitor

```sql
-- Failed webhook events
SELECT COUNT(*) AS failed_webhooks
FROM github_webhook_events
WHERE processed = FALSE
  AND received_at > NOW() - INTERVAL '24 hours';

-- Sync lag (time since last successful sync)
SELECT
  'repositories' AS resource,
  MAX(synced_at) AS last_sync,
  NOW() - MAX(synced_at) AS lag
FROM github_repositories
UNION ALL
SELECT
  'issues' AS resource,
  MAX(synced_at) AS last_sync,
  NOW() - MAX(synced_at) AS lag
FROM github_issues
UNION ALL
SELECT
  'pull_requests' AS resource,
  MAX(synced_at) AS last_sync,
  NOW() - MAX(synced_at) AS lag
FROM github_pull_requests;

-- Failed workflow runs (last 24h)
SELECT COUNT(*) AS failed_workflows
FROM github_workflow_runs
WHERE conclusion = 'failure'
  AND created_at > NOW() - INTERVAL '24 hours';

-- Open issues aging report
SELECT
  CASE
    WHEN created_at > NOW() - INTERVAL '7 days' THEN '0-7 days'
    WHEN created_at > NOW() - INTERVAL '30 days' THEN '7-30 days'
    WHEN created_at > NOW() - INTERVAL '90 days' THEN '30-90 days'
    ELSE '90+ days'
  END AS age_bucket,
  COUNT(*) AS count
FROM github_issues
WHERE state = 'open'
GROUP BY age_bucket
ORDER BY age_bucket;

-- PR merge time SLA
WITH pr_merge_times AS (
  SELECT
    EXTRACT(EPOCH FROM (merged_at - created_at)) / 3600 AS hours_to_merge
  FROM github_pull_requests
  WHERE merged = TRUE
    AND merged_at > NOW() - INTERVAL '30 days'
)
SELECT
  COUNT(*) AS total_prs,
  ROUND(AVG(hours_to_merge), 2) AS avg_hours,
  ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY hours_to_merge), 2) AS median_hours,
  ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY hours_to_merge), 2) AS p95_hours,
  COUNT(*) FILTER (WHERE hours_to_merge > 24) AS over_24h,
  ROUND(
    COUNT(*) FILTER (WHERE hours_to_merge > 24)::NUMERIC / COUNT(*) * 100, 2
  ) AS pct_over_24h
FROM pr_merge_times;
```

### Prometheus Metrics

```typescript
import { Registry, Counter, Gauge, Histogram } from 'prom-client';

const registry = new Registry();

// Define metrics
const webhookCounter = new Counter({
  name: 'github_webhooks_total',
  help: 'Total GitHub webhooks received',
  labelNames: ['event_type', 'status'],
  registers: [registry]
});

const syncDuration = new Histogram({
  name: 'github_sync_duration_seconds',
  help: 'GitHub sync duration',
  labelNames: ['resource'],
  buckets: [1, 5, 10, 30, 60, 120, 300, 600],
  registers: [registry]
});

const rateLimitGauge = new Gauge({
  name: 'github_rate_limit_remaining',
  help: 'GitHub API rate limit remaining',
  registers: [registry]
});

const openIssuesGauge = new Gauge({
  name: 'github_open_issues_total',
  help: 'Total open issues across all repositories',
  registers: [registry]
});

const openPRsGauge = new Gauge({
  name: 'github_open_prs_total',
  help: 'Total open pull requests across all repositories',
  registers: [registry]
});

const workflowFailuresCounter = new Counter({
  name: 'github_workflow_failures_total',
  help: 'Total failed workflow runs',
  labelNames: ['repository', 'workflow'],
  registers: [registry]
});

// Update gauges periodically
async function updateMetrics() {
  const db = new DatabaseService();

  // Update rate limit
  const rateLimit = await octokit.rateLimit.get();
  rateLimitGauge.set(rateLimit.data.rate.remaining);

  // Update open issues
  const { rows: issueCount } = await db.query(
    'SELECT COUNT(*) FROM github_issues WHERE state = $1',
    ['open']
  );
  openIssuesGauge.set(parseInt(issueCount[0].count));

  // Update open PRs
  const { rows: prCount } = await db.query(
    'SELECT COUNT(*) FROM github_pull_requests WHERE state = $1',
    ['open']
  );
  openPRsGauge.set(parseInt(prCount[0].count));
}

// Export metrics endpoint
app.get('/metrics', async (req, reply) => {
  await updateMetrics();
  reply.header('Content-Type', registry.contentType);
  return registry.metrics();
});
```

### Grafana Dashboard

Example queries for Grafana:

```promql
# Webhook processing rate
rate(github_webhooks_total[5m])

# Failed webhooks percentage
sum(rate(github_webhooks_total{status="failed"}[5m])) /
sum(rate(github_webhooks_total[5m])) * 100

# Average sync duration by resource
avg(github_sync_duration_seconds) by (resource)

# Rate limit usage
100 - (github_rate_limit_remaining / 5000 * 100)

# Open issues trend
github_open_issues_total

# Workflow failure rate
rate(github_workflow_failures_total[1h])
```

### Alerting Rules

```yaml
# Prometheus alerting rules
groups:
  - name: github_plugin
    interval: 1m
    rules:
      - alert: GitHubRateLimitLow
        expr: github_rate_limit_remaining < 500
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "GitHub API rate limit low"
          description: "Only {{ $value }} requests remaining"

      - alert: GitHubSyncStale
        expr: time() - max(github_sync_last_timestamp) > 3600
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "GitHub sync is stale"
          description: "Last sync was {{ $value }}s ago"

      - alert: GitHubWebhookFailures
        expr: rate(github_webhooks_total{status="failed"}[15m]) > 0.1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High GitHub webhook failure rate"
          description: "{{ $value }} webhooks failing per second"

      - alert: GitHubWorkflowFailures
        expr: increase(github_workflow_failures_total[1h]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Multiple workflow failures detected"
          description: "{{ $value }} workflows failed in the last hour"
```

---

## Use Cases

### 1. Engineering Metrics Dashboard

Track development velocity and team performance:

```sql
-- Commits per developer per week
SELECT
    author_name,
    DATE_TRUNC('week', author_date) AS week,
    COUNT(*) AS commits,
    SUM(additions) AS lines_added,
    SUM(deletions) AS lines_deleted
FROM github_commits
WHERE author_date > NOW() - INTERVAL '3 months'
GROUP BY author_name, week
ORDER BY week DESC, commits DESC;

-- PR merge time (time from open to merge)
SELECT
    r.full_name,
    AVG(EXTRACT(EPOCH FROM (p.merged_at - p.created_at)) / 3600) AS avg_hours_to_merge,
    PERCENTILE_CONT(0.5) WITHIN GROUP (
      ORDER BY EXTRACT(EPOCH FROM (p.merged_at - p.created_at)) / 3600
    ) AS median_hours_to_merge
FROM github_pull_requests p
JOIN github_repositories r ON p.repo_id = r.id
WHERE p.merged = TRUE
  AND p.merged_at > NOW() - INTERVAL '30 days'
GROUP BY r.full_name;
```

### 2. Release Tracking

Monitor releases across repositories:

```sql
-- Recent releases with download counts
SELECT
    r.full_name,
    rel.tag_name,
    rel.name,
    rel.published_at,
    rel.prerelease,
    jsonb_array_length(rel.assets) AS asset_count,
    (
      SELECT SUM((asset->>'download_count')::INTEGER)
      FROM jsonb_array_elements(rel.assets) AS asset
    ) AS total_downloads
FROM github_releases rel
JOIN github_repositories r ON rel.repo_id = r.id
ORDER BY rel.published_at DESC
LIMIT 20;

-- Release frequency by repository
SELECT
    r.full_name,
    COUNT(*) AS total_releases,
    COUNT(*) FILTER (WHERE rel.prerelease = FALSE) AS stable_releases,
    MAX(rel.published_at) AS latest_release,
    ROUND(
      COUNT(*)::NUMERIC /
      NULLIF(EXTRACT(DAYS FROM NOW() - MIN(rel.published_at)), 0) * 30, 2
    ) AS avg_releases_per_month
FROM github_releases rel
JOIN github_repositories r ON rel.repo_id = r.id
WHERE rel.published_at > NOW() - INTERVAL '1 year'
GROUP BY r.full_name
ORDER BY avg_releases_per_month DESC;
```

### 3. CI/CD Monitoring

Track GitHub Actions performance:

```sql
-- Failed workflows in last 24 hours with error details
SELECT
    r.full_name,
    w.name,
    w.head_branch,
    w.actor_login,
    w.conclusion,
    w.run_number,
    w.created_at,
    (
      SELECT COUNT(*)
      FROM github_workflow_jobs j
      WHERE j.run_id = w.id AND j.conclusion = 'failure'
    ) AS failed_jobs
FROM github_workflow_runs w
JOIN github_repositories r ON w.repo_id = r.id
WHERE w.conclusion = 'failure'
  AND w.created_at > NOW() - INTERVAL '24 hours'
ORDER BY w.created_at DESC;

-- Workflow reliability over time
SELECT
    r.full_name,
    w.name AS workflow_name,
    DATE_TRUNC('day', w.created_at) AS day,
    COUNT(*) AS total_runs,
    COUNT(*) FILTER (WHERE w.conclusion = 'success') AS successful,
    ROUND(
      COUNT(*) FILTER (WHERE w.conclusion = 'success')::NUMERIC /
      NULLIF(COUNT(*), 0) * 100, 2
    ) AS success_rate
FROM github_workflow_runs w
JOIN github_repositories r ON w.repo_id = r.id
WHERE w.created_at > NOW() - INTERVAL '30 days'
  AND w.status = 'completed'
GROUP BY r.full_name, w.name, DATE_TRUNC('day', w.created_at)
ORDER BY day DESC, success_rate;
```

### 4. Issue Tracking Analytics

Analyze issue patterns:

```sql
-- Open issues by label with average age
SELECT
    label->>'name' AS label,
    COUNT(*) AS count,
    ROUND(AVG(EXTRACT(DAYS FROM NOW() - i.created_at)), 1) AS avg_age_days,
    COUNT(*) FILTER (WHERE i.created_at > NOW() - INTERVAL '7 days') AS new_this_week
FROM github_issues i,
     LATERAL jsonb_array_elements(i.labels) AS label
WHERE i.state = 'open'
GROUP BY label->>'name'
ORDER BY count DESC;

-- Issue resolution time by label
SELECT
    label->>'name' AS label,
    COUNT(*) AS closed_issues,
    ROUND(AVG(EXTRACT(EPOCH FROM (i.closed_at - i.created_at)) / 3600), 2) AS avg_hours_to_close,
    ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (
      ORDER BY EXTRACT(EPOCH FROM (i.closed_at - i.created_at)) / 3600
    ), 2) AS median_hours_to_close
FROM github_issues i,
     LATERAL jsonb_array_elements(i.labels) AS label
WHERE i.state = 'closed'
  AND i.closed_at > NOW() - INTERVAL '90 days'
GROUP BY label->>'name'
HAVING COUNT(*) >= 5
ORDER BY avg_hours_to_close;
```

### 5. Code Review Quality Metrics

Analyze PR review patterns:

```sql
-- Review thoroughness by reviewer
SELECT
    r.user_login AS reviewer,
    COUNT(DISTINCT r.pull_request_id) AS prs_reviewed,
    COUNT(*) FILTER (WHERE r.state = 'approved') AS approvals,
    COUNT(*) FILTER (WHERE r.state = 'changes_requested') AS changes_requested,
    ROUND(
      COUNT(*) FILTER (WHERE r.state = 'approved')::NUMERIC /
      NULLIF(COUNT(*), 0) * 100, 2
    ) AS approval_rate,
    (
      SELECT AVG(comment_count)::INTEGER
      FROM (
        SELECT COUNT(*) AS comment_count
        FROM github_pr_review_comments c
        WHERE c.user_login = r.user_login
        GROUP BY c.pull_request_id
      ) AS comment_counts
    ) AS avg_comments_per_pr
FROM github_pr_reviews r
WHERE r.submitted_at > NOW() - INTERVAL '90 days'
GROUP BY r.user_login
HAVING COUNT(DISTINCT r.pull_request_id) >= 10
ORDER BY prs_reviewed DESC;
```

### 6. Repository Health Score

Calculate repository health metrics:

```sql
CREATE VIEW github_repository_health AS
WITH repo_metrics AS (
  SELECT
    r.id,
    r.full_name,
    r.language,
    r.stargazers_count,
    r.forks_count,
    r.open_issues_count,
    -- Recent activity
    (
      SELECT COUNT(*)
      FROM github_commits c
      WHERE c.repo_id = r.id
        AND c.author_date > NOW() - INTERVAL '30 days'
    ) AS commits_last_30d,
    (
      SELECT COUNT(*)
      FROM github_pull_requests p
      WHERE p.repo_id = r.id
        AND p.created_at > NOW() - INTERVAL '30 days'
    ) AS prs_last_30d,
    (
      SELECT COUNT(*)
      FROM github_issues i
      WHERE i.repo_id = r.id
        AND i.state = 'open'
        AND i.created_at < NOW() - INTERVAL '90 days'
    ) AS stale_issues,
    -- PR merge rate
    (
      SELECT
        COUNT(*) FILTER (WHERE merged = TRUE)::NUMERIC /
        NULLIF(COUNT(*), 0)
      FROM github_pull_requests p
      WHERE p.repo_id = r.id
        AND p.created_at > NOW() - INTERVAL '90 days'
    ) AS pr_merge_rate,
    -- Average PR review time
    (
      SELECT AVG(EXTRACT(EPOCH FROM (merged_at - created_at)) / 3600)
      FROM github_pull_requests p
      WHERE p.repo_id = r.id
        AND p.merged = TRUE
        AND p.merged_at > NOW() - INTERVAL '90 days'
    ) AS avg_pr_hours,
    -- CI success rate
    (
      SELECT
        COUNT(*) FILTER (WHERE conclusion = 'success')::NUMERIC /
        NULLIF(COUNT(*), 0)
      FROM github_workflow_runs w
      WHERE w.repo_id = r.id
        AND w.created_at > NOW() - INTERVAL '30 days'
    ) AS ci_success_rate
  FROM github_repositories r
)
SELECT
  *,
  -- Calculate health score (0-100)
  LEAST(100, GREATEST(0,
    (CASE WHEN commits_last_30d > 0 THEN 20 ELSE 0 END) +
    (CASE WHEN prs_last_30d > 0 THEN 15 ELSE 0 END) +
    (CASE WHEN stale_issues < 10 THEN 15 ELSE 0 END) +
    (COALESCE(pr_merge_rate, 0) * 20)::INTEGER +
    (CASE WHEN avg_pr_hours < 48 THEN 15 ELSE 5 END) +
    (COALESCE(ci_success_rate, 0) * 15)::INTEGER
  )) AS health_score
FROM repo_metrics
ORDER BY health_score DESC;
```

### 7. Developer Contribution Patterns

Identify contribution trends:

```sql
-- Developer activity heatmap (by day of week and hour)
SELECT
    author_name,
    EXTRACT(DOW FROM author_date) AS day_of_week,
    EXTRACT(HOUR FROM author_date) AS hour,
    COUNT(*) AS commits
FROM github_commits
WHERE author_date > NOW() - INTERVAL '90 days'
GROUP BY author_name, day_of_week, hour
ORDER BY author_name, day_of_week, hour;

-- First-time contributors
SELECT
    c.author_name,
    c.author_email,
    MIN(c.author_date) AS first_commit,
    COUNT(*) AS total_commits,
    COUNT(DISTINCT c.repo_id) AS repos_contributed
FROM github_commits c
WHERE c.author_date > NOW() - INTERVAL '90 days'
GROUP BY c.author_name, c.author_email
HAVING MIN(c.author_date) > NOW() - INTERVAL '30 days'
ORDER BY first_commit DESC;
```

### 8. Sprint Planning Analytics

Analyze issue velocity for sprint planning:

```sql
-- Issue completion velocity
WITH weekly_completed AS (
  SELECT
    DATE_TRUNC('week', closed_at) AS week,
    COUNT(*) AS issues_completed,
    AVG(EXTRACT(DAYS FROM (closed_at - created_at))) AS avg_days_to_close
  FROM github_issues
  WHERE state = 'closed'
    AND closed_at > NOW() - INTERVAL '90 days'
  GROUP BY DATE_TRUNC('week', closed_at)
)
SELECT
  week,
  issues_completed,
  ROUND(avg_days_to_close, 1) AS avg_days_to_close,
  AVG(issues_completed) OVER (
    ORDER BY week
    ROWS BETWEEN 3 PRECEDING AND CURRENT ROW
  )::INTEGER AS moving_avg_4w
FROM weekly_completed
ORDER BY week DESC;
```

### 9. Security Vulnerability Tracking

Monitor security-related issues and PRs:

```sql
-- Security issues and fixes
SELECT
  r.full_name,
  CASE
    WHEN i.id IS NOT NULL THEN 'issue'
    WHEN p.id IS NOT NULL THEN 'pr'
  END AS type,
  COALESCE(i.number, p.number) AS number,
  COALESCE(i.title, p.title) AS title,
  COALESCE(i.state, p.state) AS state,
  COALESCE(i.created_at, p.created_at) AS created_at,
  COALESCE(i.closed_at, p.merged_at) AS resolved_at,
  EXTRACT(DAYS FROM NOW() - COALESCE(i.created_at, p.created_at)) AS days_open
FROM github_repositories r
LEFT JOIN github_issues i ON r.id = i.repo_id
  AND EXISTS (
    SELECT 1 FROM jsonb_array_elements(i.labels) AS l
    WHERE l->>'name' IN ('security', 'vulnerability', 'cve')
  )
LEFT JOIN github_pull_requests p ON r.id = p.repo_id
  AND EXISTS (
    SELECT 1 FROM jsonb_array_elements(p.labels) AS l
    WHERE l->>'name' IN ('security', 'vulnerability', 'cve')
  )
WHERE i.id IS NOT NULL OR p.id IS NOT NULL
ORDER BY days_open DESC;
```

### 10. Dependency Update Tracking

Track dependency update PRs (e.g., from Dependabot):

```sql
-- Dependency update PR metrics
SELECT
  r.full_name,
  DATE_TRUNC('month', p.created_at) AS month,
  COUNT(*) AS dependency_prs,
  COUNT(*) FILTER (WHERE p.merged = TRUE) AS merged,
  COUNT(*) FILTER (WHERE p.state = 'closed' AND p.merged = FALSE) AS rejected,
  AVG(
    EXTRACT(EPOCH FROM (p.merged_at - p.created_at)) / 3600
  )::INTEGER AS avg_hours_to_merge
FROM github_pull_requests p
JOIN github_repositories r ON p.repo_id = r.id
WHERE p.user_login IN ('dependabot[bot]', 'renovate[bot]')
  AND p.created_at > NOW() - INTERVAL '6 months'
GROUP BY r.full_name, DATE_TRUNC('month', p.created_at)
ORDER BY month DESC, dependency_prs DESC;
```

### 11. Cross-Repository Impact Analysis

Analyze changes that affect multiple repositories:

```sql
-- Find developers working across multiple repos
SELECT
    c.author_name,
    COUNT(DISTINCT c.repo_id) AS repos_contributed,
    array_agg(DISTINCT r.full_name ORDER BY r.full_name) AS repositories,
    COUNT(*) AS total_commits,
    SUM(c.additions + c.deletions) AS total_lines_changed
FROM github_commits c
JOIN github_repositories r ON c.repo_id = r.id
WHERE c.author_date > NOW() - INTERVAL '30 days'
GROUP BY c.author_name
HAVING COUNT(DISTINCT c.repo_id) > 1
ORDER BY repos_contributed DESC, total_commits DESC;
```

### 12. Documentation Coverage

Track documentation updates relative to code changes:

```sql
-- Documentation to code change ratio
WITH doc_changes AS (
  SELECT
    repo_id,
    COUNT(*) AS doc_commits
  FROM github_commits c,
       LATERAL jsonb_array_elements(c.files) AS file
  WHERE file->>'filename' ~* '\.(md|rst|txt|adoc)$'
    AND c.author_date > NOW() - INTERVAL '90 days'
  GROUP BY repo_id
),
code_changes AS (
  SELECT
    repo_id,
    COUNT(*) AS code_commits
  FROM github_commits c,
       LATERAL jsonb_array_elements(c.files) AS file
  WHERE file->>'filename' ~* '\.(ts|js|py|go|java|rb|php)$'
    AND c.author_date > NOW() - INTERVAL '90 days'
  GROUP BY repo_id
)
SELECT
  r.full_name,
  COALESCE(dc.doc_commits, 0) AS doc_commits,
  COALESCE(cc.code_commits, 0) AS code_commits,
  ROUND(
    COALESCE(dc.doc_commits, 0)::NUMERIC /
    NULLIF(cc.code_commits, 0) * 100, 2
  ) AS doc_coverage_pct
FROM github_repositories r
LEFT JOIN doc_changes dc ON r.id = dc.repo_id
LEFT JOIN code_changes cc ON r.id = cc.repo_id
WHERE cc.code_commits > 0
ORDER BY doc_coverage_pct DESC;
```

---

## TypeScript Implementation

The plugin is built with TypeScript for type safety and maintainability.

### Key Files

| File | Purpose |
|------|---------|
| `types.ts` | All type definitions for GitHub resources |
| `client.ts` | GitHub API client with pagination and rate limiting |
| `database.ts` | PostgreSQL operations with upsert support |
| `sync.ts` | Orchestrates full and incremental syncs |
| `webhooks.ts` | Webhook event handlers |
| `server.ts` | Fastify HTTP server |
| `cli.ts` | Commander.js CLI |

### API Client Example

```typescript
import { Octokit } from '@octokit/rest';

export class GitHubClient {
  private octokit: Octokit;

  constructor(token: string) {
    this.octokit = new Octokit({ auth: token });
  }

  async listRepositories(org?: string): Promise<Repository[]> {
    const repos: Repository[] = [];

    if (org) {
      for await (const response of this.octokit.paginate.iterator(
        this.octokit.repos.listForOrg,
        { org, per_page: 100 }
      )) {
        repos.push(...response.data.map(this.mapRepository));
      }
    } else {
      for await (const response of this.octokit.paginate.iterator(
        this.octokit.repos.listForAuthenticatedUser,
        { per_page: 100 }
      )) {
        repos.push(...response.data.map(this.mapRepository));
      }
    }

    return repos;
  }
}
```

---

## Troubleshooting

### Common Issues

#### Rate Limiting

```
Error: API rate limit exceeded
```

**Solution**: GitHub allows 5,000 requests/hour for authenticated users. Use incremental sync to minimize API calls:

```bash
nself-github sync --incremental
```

#### Token Permissions

```
Error: Resource not accessible by integration
```

**Solution**: Ensure your token has the required scopes:
- `repo` for repository access
- `read:org` for organization data
- `workflow` for Actions data

#### Webhook Signature Invalid

```
Error: Webhook signature verification failed
```

**Solution**:
1. Verify `GITHUB_WEBHOOK_SECRET` matches the secret in GitHub settings
2. Ensure the webhook is sending `application/json` content type
3. Check that no proxy is modifying the request body

#### Database Connection

```
Error: Connection refused to PostgreSQL
```

**Solution**:
1. Verify `DATABASE_URL` is correct
2. Ensure PostgreSQL is running
3. Check firewall rules

### Debug Mode

Enable debug logging for troubleshooting:

```bash
DEBUG=github:* nself-github sync
```

### Support

- [GitHub Issues](https://github.com/acamarata/nself-plugins/issues)
- [GitHub API Documentation](https://docs.github.com/en/rest)
- [Octokit Documentation](https://octokit.github.io/rest.js)
