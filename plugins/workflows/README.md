# workflows

Automation engine providing trigger-action workflow chains, conditional logic, scheduled tasks, webhook integrations, and cross-plugin orchestration

## Installation

```bash
nself plugin install workflows
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself-tv backend `.env.dev`, map variables as follows:

### Backend → Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `WORKFLOWS_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `WORKFLOWS_PLUGIN_PORT` | `PORT` | Server port | `3712` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `WORKFLOWS_MAX_CONCURRENT_EXECUTIONS` | `WORKFLOWS_MAX_CONCURRENT_EXECUTIONS` | Max concurrent workflows | `10` |

### Configuration Helper Script

```bash
#!/bin/bash
# generate-workflows-env.sh

BACKEND_ENV="$HOME/Sites/nself-tv/backend/.env.dev"
PLUGIN_ENV="$HOME/.nself/plugins/workflows/ts/.env"

# Source backend variables
source "$BACKEND_ENV"

# Create plugin .env
cat > "$PLUGIN_ENV" <<EOF
# Auto-generated from backend .env.dev
DATABASE_URL=$DATABASE_URL
PORT=$WORKFLOWS_PLUGIN_PORT

# Workflow settings
WORKFLOWS_MAX_CONCURRENT_EXECUTIONS=${WORKFLOWS_MAX_CONCURRENT_EXECUTIONS:-10}

# Logging
LOG_LEVEL=info
EOF

echo "Created $PLUGIN_ENV"
```

See [CONFIGURATION.md](../../CONFIGURATION.md) for detailed mapping patterns and troubleshooting.

## Usage

See plugin.json for available CLI commands and API endpoints.

## License

See LICENSE file in repository root.
