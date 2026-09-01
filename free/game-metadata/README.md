# game-metadata

Game metadata service with IGDB integration, ROM hash matching, tier requirements, and artwork management

## Installation

```bash
nself plugin install game-metadata
```

## Configuration

See plugin.json for environment variables and configuration options.

### Environment Variables

| Variable | Required | Description | Default |
|----------|----------|-------------|---------|
| `DATABASE_URL` | Yes | PostgreSQL connection URL | - |
| `GAME_METADATA_PLUGIN_PORT` | No | Server port | `3211` |
| `IGDB_CLIENT_ID` | No | Twitch/IGDB client ID for API access | - |
| `IGDB_CLIENT_SECRET` | No | Twitch/IGDB client secret | - |
| `GAME_METADATA_ARTWORK_PATH` | No | Path for artwork storage | `./artwork` |

## Usage

See plugin.json for available CLI commands and API endpoints.

## License

See LICENSE file in repository root.
