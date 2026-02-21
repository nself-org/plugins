# GitHub Plugin for nself

Sync GitHub repository data to PostgreSQL with real-time webhook support.

## Features

- **Full Data Sync** - Repositories, issues, PRs, commits, releases, workflows, deployments
- **Real-time Webhooks** - 12+ webhook event handlers
- **REST API** - Query synced data via HTTP endpoints
- **CLI Tools** - Command-line interface for management
- **Analytics Views** - Repository summary, activity feed, PR metrics

## Installation

### TypeScript Implementation

```bash
# Install shared utilities first
cd shared
npm install
npm run build
cd ..

# Install the GitHub plugin
cd plugins/github/ts
npm install
npm run build
```

## Configuration

Create a `.env` file in `plugins/github/ts/`:

```bash
# Required
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx
DATABASE_URL=postgresql://user:pass@localhost:5432/nself

# Optional - Organization mode
GITHUB_ORG=your-organization

# Optional - Specific repos mode
GITHUB_REPOS=owner/repo1,owner/repo2

# Optional - Webhooks
GITHUB_WEBHOOK_SECRET=your_webhook_secret

# Server options
PORT=3002
HOST=0.0.0.0
```

### Getting GitHub Credentials

1. Go to [GitHub Settings > Tokens](https://github.com/settings/tokens)
2. Generate new token (classic or fine-grained)
3. Required scopes:
   - `repo` - Full repository access
   - `read:org` - For organization repositories (optional)

## Usage

### CLI Commands

```bash
# Initialize database schema
npx nself-github init

# Sync all data
npx nself-github sync

# Sync specific resources
npx nself-github sync --resources repositories,issues

# Sync specific repos
npx nself-github sync --repos owner/repo1,owner/repo2

# Start webhook server
npx nself-github server --port 3002

# Show sync status
npx nself-github status

# List data
npx nself-github repos --limit 20
npx nself-github issues --state open
npx nself-github prs --state open
```

### REST API

Start the server and access endpoints at `http://localhost:3002`:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/webhook` | GitHub webhook receiver |
| POST | `/api/sync` | Trigger data sync |
| GET | `/api/status` | Get sync status |
| GET | `/api/repositories` | List repositories |
| GET | `/api/repositories/:owner/:repo` | Get repository |
| GET | `/api/issues` | List issues |
| GET | `/api/issues/:id` | Get issue |
| GET | `/api/pull-requests` | List pull requests |
| GET | `/api/pull-requests/:id` | Get pull request |
| GET | `/api/commits` | List commits |
| GET | `/api/releases` | List releases |
| GET | `/api/workflow-runs` | List workflow runs |
| GET | `/api/deployments` | List deployments |
| GET | `/api/webhook-events` | List webhook events |

## Webhook Setup

1. Go to your repo/org Settings > Webhooks
2. Add webhook:
   - URL: `https://your-domain.com/webhook`
   - Content type: `application/json`
   - Secret: Your `GITHUB_WEBHOOK_SECRET`
3. Select events to receive

### Supported Webhook Events

| Event | Description |
|-------|-------------|
| `push` | Code pushed to repository |
| `pull_request` | PR opened, closed, merged |
| `pull_request_review` | PR review submitted |
| `issues` | Issue opened, closed, edited |
| `issue_comment` | Comment on issue/PR |
| `workflow_run` | GitHub Actions workflow run |
| `release` | Release published |
| `deployment` | Deployment created |
| `deployment_status` | Deployment status updated |
| `create` | Branch or tag created |
| `delete` | Branch or tag deleted |
| `repository` | Repository created, deleted |

## Database Schema

### Tables

| Table | Description |
|-------|-------------|
| `github_repositories` | Repository metadata and stats |
| `github_issues` | Issues with labels, assignees |
| `github_pull_requests` | PRs with merge info, review stats |
| `github_commits` | Commit history |
| `github_releases` | Release tags and assets |
| `github_workflow_runs` | GitHub Actions runs |
| `github_deployments` | Deployment status |
| `github_webhook_events` | Webhook event log |

### Analytics Views

```sql
-- Open issues and PRs by repository
SELECT * FROM github_open_items;

-- Recent activity feed
SELECT * FROM github_recent_activity;

-- Workflow success statistics
SELECT * FROM github_workflow_stats;
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GITHUB_TOKEN` | Yes | - | GitHub personal access token |
| `GITHUB_WEBHOOK_SECRET` | No | - | Webhook signing secret |
| `GITHUB_ORG` | No | - | GitHub organization to sync |
| `GITHUB_REPOS` | No | - | Comma-separated list of repos |
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `PORT` | No | 3002 | Server port |
| `HOST` | No | 0.0.0.0 | Server host |

## Sync Modes

### Organization Mode

Set `GITHUB_ORG` to sync all repositories from an organization:

```bash
GITHUB_ORG=my-organization
```

### Specific Repos Mode

Set `GITHUB_REPOS` to sync specific repositories:

```bash
GITHUB_REPOS=owner/repo1,owner/repo2,other-owner/repo3
```

### Combined Mode

Both can be used together - org repos plus additional repos.

## Architecture

```
plugins/github/ts/
├── src/
│   ├── types.ts        # GitHub-specific type definitions
│   ├── client.ts       # Octokit API client wrapper
│   ├── database.ts     # Database operations
│   ├── sync.ts         # Full sync service
│   ├── webhooks.ts     # Webhook event handlers
│   ├── config.ts       # Configuration loading
│   ├── server.ts       # Fastify HTTP server
│   ├── cli.ts          # Commander.js CLI
│   └── index.ts        # Module exports
├── package.json
└── tsconfig.json
```

## Development

```bash
# Watch mode
npm run watch

# Type checking
npm run typecheck

# Development server
npm run dev
```

## Support

- [GitHub Issues](https://github.com/acamarata/nself-plugins/issues)
- [GitHub API Documentation](https://docs.github.com/en/rest)
