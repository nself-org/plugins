# podcast

Podcast service with RSS feed parsing, episode management, playback position sync, and subscription management

## Installation

```bash
nself plugin install podcast
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself backend `.env.dev`, map variables as follows:

### Backend -> Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `PODCAST_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `PODCAST_PLUGIN_PORT` | `PORT` or `PODCAST_PLUGIN_PORT` | Server port | `3210` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `PODCAST_RSS_POLL_INTERVAL` | `PODCAST_RSS_POLL_INTERVAL` | RSS poll interval in minutes | `60` |
| `PODCAST_MAX_EPISODES` | `PODCAST_MAX_EPISODES` | Max episodes per feed | `500` |
| `PODCAST_STORAGE_BACKEND` | `PODCAST_STORAGE_BACKEND` | Storage backend type | `database` |

## Usage

See plugin.json for available CLI commands and API endpoints.

## License

See LICENSE file in repository root.
