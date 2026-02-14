# epg

Electronic program guide with XMLTV import, channel management, and schedule queries

## Installation

```bash
nself plugin install epg
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself-tv backend `.env.dev`, map variables as follows:

### Backend → Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `EPG_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `EPG_PLUGIN_PORT` | `PORT` or `EPG_PLUGIN_PORT` | Server port | `3031` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `EPG_APP_IDS` | `EPG_APP_IDS` | App identifiers | `nself-tv` |
| `EPG_XMLTV_URLS` | `EPG_XMLTV_URLS` | XMLTV data source URLs | `https://...` |
| `EPG_SCHEDULES_DIRECT_USERNAME` | `EPG_SCHEDULES_DIRECT_USERNAME` | Schedules Direct username | `user@example.com` |
| `EPG_SCHEDULES_DIRECT_PASSWORD` | `EPG_SCHEDULES_DIRECT_PASSWORD` | Schedules Direct password | `password` |
| `EPG_DEFAULT_TIMEZONE` | `EPG_DEFAULT_TIMEZONE` | Default timezone | `America/New_York` |
| `EPG_GUIDE_DAYS_AHEAD` | `EPG_GUIDE_DAYS_AHEAD` | Days of guide data | `7` |

### Configuration Helper Script

```bash
#!/bin/bash
# generate-epg-env.sh

BACKEND_ENV="$HOME/Sites/nself-tv/backend/.env.dev"
PLUGIN_ENV="$HOME/.nself/plugins/epg/ts/.env"

# Source backend variables
source "$BACKEND_ENV"

# Create plugin .env
cat > "$PLUGIN_ENV" <<EOF
# Auto-generated from backend .env.dev
DATABASE_URL=$DATABASE_URL
PORT=$EPG_PLUGIN_PORT

# EPG settings
EPG_APP_IDS=${EPG_APP_IDS:-nself-tv}
EPG_XMLTV_URLS=$EPG_XMLTV_URLS
EPG_DEFAULT_TIMEZONE=${EPG_DEFAULT_TIMEZONE:-America/New_York}
EPG_GUIDE_DAYS_AHEAD=${EPG_GUIDE_DAYS_AHEAD:-7}

# Schedules Direct (optional)
EPG_SCHEDULES_DIRECT_USERNAME=$EPG_SCHEDULES_DIRECT_USERNAME
EPG_SCHEDULES_DIRECT_PASSWORD=$EPG_SCHEDULES_DIRECT_PASSWORD

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
