# Game Metadata Plugin

Game metadata service with IGDB (Internet Game Database) integration, ROM hash matching, tier-based metadata requirements, and artwork management for retro gaming collections.

| Property | Value |
|----------|-------|
| **Port** | `3211` |
| **Category** | `media` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run game-metadata init
nself plugin run game-metadata server
```

---

## Features

- **IGDB Integration** - Rich metadata from Twitch's Internet Game Database
- **ROM Hash Matching** - Identify games by MD5, SHA1, SHA256, or CRC32 hash
- **Multi-Platform Support** - Covers all major gaming platforms (NES, SNES, Genesis, PlayStation, etc.)
- **Artwork Management** - Download and serve cover art, screenshots, and banners
- **Tier System** - Configurable metadata completeness levels (Bronze, Silver, Gold, Platinum)
- **Genre & Platform Taxonomy** - Normalized categorization across all games
- **Offline Catalog** - Cache metadata locally for fast lookups

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GAME_METADATA_PLUGIN_PORT` | `3211` | Server port |
| `IGDB_CLIENT_ID` | - | Twitch/IGDB client ID for API access |
| `IGDB_CLIENT_SECRET` | - | Twitch/IGDB client secret |
| `GAME_METADATA_ARTWORK_PATH` | `./artwork` | Local path for artwork storage |

### IGDB API Setup

1. Create a Twitch developer account at [dev.twitch.tv](https://dev.twitch.tv)
2. Register an application to get Client ID and Client Secret
3. Configure the plugin with your credentials:
   ```bash
   export IGDB_CLIENT_ID=your-client-id
   export IGDB_CLIENT_SECRET=your-client-secret
   ```

---

## Installation

```bash
# Install plugin
nself plugin install game-metadata

# Initialize database
nself plugin run game-metadata init

# Start server
nself plugin run game-metadata server
```

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (5 tables) |
| `server` | Start the HTTP API server (`-p`/`--port`) |
| `lookup` | Lookup game by name or hash (`--name`, `--hash`, `--hash-type`) |
| `enrich` | Enrich metadata from IGDB for a game (`--id`, `--igdb-id`) |
| `tiers` | Show tier requirements and counts |
| `platforms` | List supported platforms with game counts |
| `stats` | Show plugin statistics (total games, coverage, artwork) |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |

### Game Lookup

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/games/search` | Search games (query: `q` (name), `platform?`, `limit?`, `offset?`) |
| `GET` | `/api/games/:id` | Get game details by ID |
| `POST` | `/api/games/hash-lookup` | Lookup game by ROM hash (body: `hash`, `hash_type`, `platform?`) |
| `GET` | `/api/games/:id/metadata` | Get full metadata for a game |

### Metadata Enrichment

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/games/enrich` | Enrich game from IGDB (body: `game_id?`, `igdb_id?`, `name?`, `platform?`) |
| `POST` | `/api/games/:id/enrich` | Enrich specific game from IGDB |
| `GET` | `/api/games/:id/tier` | Get tier status for a game |

### Platforms

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/platforms` | List all platforms (query: `limit?`, `offset?`) |
| `GET` | `/api/platforms/:id` | Get platform details |
| `GET` | `/api/platforms/:id/games` | List games for a platform |

### Genres

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/genres` | List all genres |
| `GET` | `/api/genres/:id/games` | List games in a genre |

### Artwork

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/games/:id/artwork` | List artwork for a game (cover, screenshots, banner) |
| `GET` | `/api/artwork/:id` | Get artwork file (returns image) |
| `POST` | `/api/games/:id/artwork/download` | Download artwork from IGDB |

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `game.added` | New game added to catalog |
| `game.enriched` | Game metadata enriched from IGDB |
| `game.tier.upgraded` | Game achieved higher tier status |
| `artwork.downloaded` | Artwork downloaded successfully |
| `hash.matched` | ROM hash matched to game |

---

## Database Schema

### `np_game_catalog`

Master catalog of all games.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Game ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `title` | `VARCHAR(500)` | Game title |
| `normalized_title` | `VARCHAR(500)` | Lowercase, no special chars (for matching) |
| `platform_id` | `UUID` (FK) | References `np_game_platforms` |
| `release_date` | `DATE` | Release date |
| `developer` | `VARCHAR(255)` | Developer name |
| `publisher` | `VARCHAR(255)` | Publisher name |
| `igdb_id` | `INTEGER` | IGDB game ID |
| `tier` | `VARCHAR(20)` | Metadata tier (`bronze`, `silver`, `gold`, `platinum`) |
| `rom_hashes` | `JSONB` | `{md5, sha1, sha256, crc32}` |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_game_metadata`

Extended metadata for games.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Metadata ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `game_id` | `UUID` (FK) | References `np_game_catalog` |
| `summary` | `TEXT` | Game description |
| `storyline` | `TEXT` | Detailed storyline |
| `rating` | `DECIMAL(3,1)` | Average rating (0-100) |
| `rating_count` | `INTEGER` | Number of ratings |
| `genres` | `TEXT[]` | Array of genre IDs |
| `game_modes` | `TEXT[]` | `single-player`, `multiplayer`, `co-op`, etc. |
| `player_perspectives` | `TEXT[]` | `first-person`, `third-person`, `side-view`, etc. |
| `themes` | `TEXT[]` | Game themes |
| `franchises` | `TEXT[]` | Franchise names |
| `similar_games` | `UUID[]` | IDs of similar games |
| `videos` | `JSONB` | YouTube video IDs, trailers |
| `websites` | `JSONB` | Official site, wikis, etc. |
| `metadata` | `JSONB` | Raw IGDB response |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_game_artwork`

Game artwork (covers, screenshots, banners).

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Artwork ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `game_id` | `UUID` (FK) | References `np_game_catalog` |
| `artwork_type` | `VARCHAR(50)` | `cover`, `screenshot`, `banner`, `logo`, `artwork` |
| `url` | `TEXT` | Original IGDB URL |
| `local_path` | `TEXT` | Local file path |
| `width` | `INTEGER` | Image width |
| `height` | `INTEGER` | Image height |
| `size_bytes` | `INTEGER` | File size |
| `is_primary` | `BOOLEAN` | Whether this is the main image for its type |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `np_game_platforms`

Gaming platforms.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Platform ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(255)` | Platform name |
| `slug` | `VARCHAR(100)` | URL-friendly identifier |
| `igdb_id` | `INTEGER` | IGDB platform ID |
| `abbreviation` | `VARCHAR(20)` | Short name (e.g., `NES`, `PS1`) |
| `generation` | `INTEGER` | Console generation |
| `manufacturer` | `VARCHAR(128)` | Nintendo, Sony, Sega, etc. |
| `release_date` | `DATE` | Platform release date |
| `game_count` | `INTEGER` | Number of games in catalog |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `np_game_genres`

Game genres.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Genre ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(128)` | Genre name |
| `slug` | `VARCHAR(100)` | URL-friendly identifier |
| `igdb_id` | `INTEGER` | IGDB genre ID |
| `game_count` | `INTEGER` | Number of games in this genre |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

---

## Tier System

Games are assigned tiers based on metadata completeness:

| Tier | Requirements | Description |
|------|--------------|-------------|
| **Bronze** | Title, Platform, Release Year | Minimal identification data |
| **Silver** | + Developer, Publisher, Genres | Basic commercial information |
| **Gold** | + Summary, Rating, Cover Art | Rich presentation quality |
| **Platinum** | + Screenshots, Videos, Full Metadata | Complete archival record |

### Tier Benefits

- **Bronze**: Enough to identify and organize games
- **Silver**: Sufficient for browsing and discovery
- **Gold**: Ready for public-facing game libraries
- **Platinum**: Museum-quality preservation

---

## Hash Matching

The plugin supports four hash algorithms for ROM identification:

| Algorithm | Strength | Use Case |
|-----------|----------|----------|
| `MD5` | Good | Standard, widely used |
| `SHA1` | Better | More collision-resistant |
| `SHA256` | Best | Cryptographically secure |
| `CRC32` | Fast | Quick verification (less unique) |

### Hash Lookup Flow

```bash
# 1. Calculate ROM hash
md5sum game.nes
# Output: 3c4a6b8e2f9d1c5e7a0b4d8f6e2a9c1b

# 2. Lookup via API
curl -X POST http://localhost:3211/api/games/hash-lookup \
  -H "Content-Type: application/json" \
  -d '{
    "hash": "3c4a6b8e2f9d1c5e7a0b4d8f6e2a9c1b",
    "hash_type": "md5",
    "platform": "NES"
  }'

# 3. Returns game details if match found
```

---

## Usage Examples

### Search for Games

```bash
# Search by name
curl "http://localhost:3211/api/games/search?q=zelda&platform=SNES"

# CLI search
nself plugin run game-metadata lookup --name "Super Mario"
```

### Enrich Game Metadata

```bash
# Enrich from IGDB by game ID
curl -X POST http://localhost:3211/api/games/enrich \
  -H "Content-Type: application/json" \
  -d '{"game_id": "550e8400-e29b-41d4-a716-446655440000"}'

# CLI enrich
nself plugin run game-metadata enrich --id 550e8400-e29b-41d4-a716-446655440000
```

### Download Artwork

```bash
# Download all artwork for a game
curl -X POST http://localhost:3211/api/games/{id}/artwork/download

# Access artwork
curl http://localhost:3211/api/artwork/{artwork_id} > cover.jpg
```

### Platform Information

```bash
# List all platforms
curl http://localhost:3211/api/platforms

# Get games for a specific platform
curl http://localhost:3211/api/platforms/{platform_id}/games

# CLI platforms
nself plugin run game-metadata platforms
```

### Hash Matching

```bash
# Match ROM by hash
curl -X POST http://localhost:3211/api/games/hash-lookup \
  -H "Content-Type: application/json" \
  -d '{
    "hash": "abc123...",
    "hash_type": "md5",
    "platform": "NES"
  }'

# CLI hash lookup
nself plugin run game-metadata lookup --hash abc123... --hash-type md5
```

---

## IGDB Rate Limiting

IGDB enforces rate limits:

- **4 requests per second** (default)
- Plugin automatically throttles requests
- Configurable via `igdbRateLimitPerSecond` in plugin.json

When enriching large catalogs:
```bash
# Enrich games in batches
for id in $(cat game_ids.txt); do
  nself plugin run game-metadata enrich --id $id
  sleep 0.25  # Stay under rate limit
done
```

---

## Artwork Storage

Artwork is downloaded to `GAME_METADATA_ARTWORK_PATH` (default: `./artwork`):

```
artwork/
├── covers/
│   ├── {game_id}_cover.jpg
│   └── ...
├── screenshots/
│   ├── {game_id}_screenshot_1.jpg
│   └── ...
└── banners/
    ├── {game_id}_banner.jpg
    └── ...
```

Maximum artwork size: **50 MB per file** (configurable in plugin.json)

---

## Troubleshooting

**"IGDB credentials not configured"** -- Set `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` environment variables. Get credentials from [dev.twitch.tv](https://dev.twitch.tv).

**"Rate limit exceeded"** -- IGDB allows 4 requests/second. The plugin auto-throttles but you may need to slow down bulk enrichment operations.

**"Game not found by hash"** -- Hash matching requires exact ROM matches. Different ROM versions (regions, revisions) have different hashes. Try searching by name instead.

**"Artwork download failed"** -- Check that `GAME_METADATA_ARTWORK_PATH` exists and is writable. Verify network access to IGDB CDN.

**"Tier not upgrading"** -- Check tier requirements with `nself plugin run game-metadata tiers`. Missing fields prevent tier advancement.

**"Platform not recognized"** -- Platforms must exist in `np_game_platforms` table. Initialize with `nself plugin run game-metadata init` or add manually.

---

## Performance

- **Local catalog** reduces IGDB API calls
- **Hash lookups** use database indexes (sub-millisecond)
- **Artwork caching** serves files from local filesystem
- **Batch operations** should throttle to 4 req/sec for IGDB compliance

---

## Advanced Configuration

### Using with nself Backend

Add to your `.env.dev`:

```bash
# Enable game metadata plugin
GAME_METADATA_PLUGIN_ENABLED=true
GAME_METADATA_PLUGIN_PORT=3211

# IGDB credentials
IGDB_CLIENT_ID=your-client-id
IGDB_CLIENT_SECRET=your-client-secret

# Artwork storage
GAME_METADATA_ARTWORK_PATH=/var/game-artwork
```

### Custom Tier Definitions

While tiers are currently hardcoded, you can query games by tier:

```sql
-- Get all Gold tier games
SELECT title, platform_id, tier
FROM np_game_catalog
WHERE tier = 'gold';

-- Count games by tier
SELECT tier, COUNT(*)
FROM np_game_catalog
GROUP BY tier;
```

### Building a Game Library

```bash
# 1. Import ROM collection (calculate hashes)
for rom in roms/*.nes; do
  hash=$(md5sum "$rom" | awk '{print $1}')
  nself plugin run game-metadata lookup --hash "$hash" --hash-type md5
done

# 2. Enrich all unmatched games
nself plugin run game-metadata stats  # Check coverage

# 3. Download artwork for presentation
curl -X POST http://localhost:3211/api/games/{id}/artwork/download
```

---

## Data Sources

- **IGDB** - Primary metadata source (games, platforms, genres, artwork)
- **ROM Hashes** - User-contributed hash databases (No-Intro, Redump)
- **Local Catalog** - Cached metadata for offline operation

---

## Privacy & Licensing

- IGDB API requires Twitch account
- IGDB data subject to [Twitch Developer Agreement](https://www.twitch.tv/p/legal/developer-agreement/)
- Artwork copyrights belong to publishers/developers
- Use for personal/archival purposes only
