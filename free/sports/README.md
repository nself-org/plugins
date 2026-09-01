# sports

Comprehensive sports data plugin with live scores, schedules, standings, team rosters, player stats, and real-time game updates

## Installation

```bash
nself plugin install sports
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself-tv backend `.env.dev`, map variables as follows:

### Backend → Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `SPORTS_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `SPORTS_PLUGIN_PORT` | `PORT` or `SPORTS_PLUGIN_PORT` | Server port | `3035` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `SPORTS_APP_IDS` | `SPORTS_APP_IDS` | App identifiers | `nself-tv` |
| `SPORTS_PROVIDER` | `SPORTS_PROVIDER` | Primary provider | `espn` |
| `SPORTS_PROVIDERS` | `SPORTS_PROVIDERS` | All enabled providers | `espn,sportsdata` |
| `SPORTS_ESPN_API_KEY` | `SPORTS_ESPN_API_KEY` | ESPN API key | `your_key` |
| `SPORTS_ESPN_API_URL` | `SPORTS_ESPN_API_URL` | ESPN API base URL | `https://site.api.espn.com/...` |
| `SPORTS_SPORTSDATA_API_KEY` | `SPORTS_SPORTSDATA_API_KEY` | SportsData API key | `your_key` |
| `SPORTS_AUTO_TRIGGER_RECORDINGS` | `SPORTS_AUTO_TRIGGER_RECORDINGS` | Auto-trigger recordings | `true` |
| `SPORTS_RECORDING_PLUGIN_URL` | `SPORTS_RECORDING_PLUGIN_URL` | Recording plugin URL | `http://localhost:3602` |

### Configuration Helper Script

```bash
#!/bin/bash
# generate-sports-env.sh

BACKEND_ENV="$HOME/Sites/nself-tv/backend/.env.dev"
PLUGIN_ENV="$HOME/.nself/plugins/sports/ts/.env"

# Source backend variables
source "$BACKEND_ENV"

# Create plugin .env
cat > "$PLUGIN_ENV" <<EOF
# Auto-generated from backend .env.dev
DATABASE_URL=$DATABASE_URL
PORT=$SPORTS_PLUGIN_PORT

# Sports settings
SPORTS_APP_IDS=${SPORTS_APP_IDS:-nself-tv}
SPORTS_PROVIDER=${SPORTS_PROVIDER:-espn}
SPORTS_PROVIDERS=${SPORTS_PROVIDERS:-espn,sportsdata}

# API credentials
SPORTS_ESPN_API_KEY=$SPORTS_ESPN_API_KEY
SPORTS_ESPN_API_URL=${SPORTS_ESPN_API_URL:-https://site.api.espn.com/apis/site/v2}
SPORTS_SPORTSDATA_API_KEY=$SPORTS_SPORTSDATA_API_KEY

# Recording integration
SPORTS_AUTO_TRIGGER_RECORDINGS=${SPORTS_AUTO_TRIGGER_RECORDINGS:-true}
SPORTS_RECORDING_PLUGIN_URL=${SPORTS_RECORDING_PLUGIN_URL:-http://localhost:3602}
SPORTS_RECORDING_LEAD_TIME_MINUTES=${SPORTS_RECORDING_LEAD_TIME_MINUTES:-5}
SPORTS_RECORDING_TRAIL_TIME_MINUTES=${SPORTS_RECORDING_TRAIL_TIME_MINUTES:-15}

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
