# Retro Gaming Plugin

> Retro gaming ROM library management, emulator core serving, save states, play sessions, and controller configuration for nTV. **Pro plugin — requires license.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| Any bundle | $0.99/mo | $9.99/yr | If in bundle |
| ɳSelf+ | $3.99/mo | $39.99/yr | Yes |

**Minimum tier:** Basic (this is a `tier: pro` plugin per F07-PRICING-TIERS).

## Bundle membership

Not currently included in any of the five product bundles (ɳClaw, ClawDE, nTV, nFamily, nChat). Typically paired with the nTV bundle plugins (`media-processing`, `streaming`, `epg`, `tmdb`) to build the full nTV retro gaming stack.

Or get all bundles + all apps via **ɳSelf+** ($49.99/yr).

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install retro-gaming
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

## Description

A ROM library and emulator backend for nTV and other nSelf-powered media servers. The plugin tracks ROM files, their platform metadata, and serves libretro-compatible emulator cores on demand. Save states are synchronised across devices so a game paused on one client resumes on another.

Play sessions record start and end times, platform, and duration for library statistics. Controller configurations are stored per user and keyed by platform so a gamepad mapping follows the user across devices. Metadata enrichment uses IGDB and MobyGames when API keys are provided, with results cached locally so repeat lookups do not hit upstream APIs.

Emulator cores are versioned and tracked per installation, so client devices can fetch the right binary for their platform and architecture. Cores can be served directly or fronted by a CDN for distribution to many clients.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | Main Postgres connection |
| `RETRO_GAMING_PLUGIN_PORT` | No | `3033` | Service port |
| `IGDB_CLIENT_ID` | No | — | IGDB client id for metadata enrichment |
| `IGDB_CLIENT_SECRET` | No | — | IGDB client secret |
| `MOBYGAMES_API_KEY` | No | — | MobyGames API key |
| `RETRO_GAMING_ROM_PATH_PREFIX` | No | `/roms` | Storage prefix for ROM files |
| `RETRO_GAMING_SAVE_STATE_PATH_PREFIX` | No | `/save-states` | Storage prefix for save states |
| `RETRO_GAMING_CORE_PATH_PREFIX` | No | `/cores` | Storage prefix for emulator cores |
| `RETRO_GAMING_CDN_URL` | No | — | Optional CDN for core downloads |
| `RETRO_GAMING_STORAGE_BUCKET` | No | — | Object storage bucket |
| `LOG_LEVEL` | No | `info` | Log verbosity |

Reference vault credentials. Never hardcode secrets.

## Ports

- Default port: `3124`
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly.

## Database Schema

Tables created (prefix `np_retrogame_`):

- `np_retrogame_roms` — ROM file index with platform metadata
- `np_retrogame_save_states` — per-ROM save state slots
- `np_retrogame_play_sessions` — start/end timing for library stats
- `np_retrogame_emulator_cores` — libretro core registry
- `np_retrogame_controller_configs` — per-user gamepad mappings
- `np_retrogame_core_installations` — installed core tracking per device

All tables use `source_account_id` for multi-app isolation.

## REST API

Public endpoints. Internal admin routes are excluded from this surface.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/games/roms` | List ROMs |
| GET | `/api/games/roms/:id` | Get ROM details |
| POST | `/api/games/roms` | Create ROM entry |
| PATCH | `/api/games/roms/:id` | Update ROM metadata |
| DELETE | `/api/games/roms/:id` | Delete ROM |
| POST | `/api/games/roms/scan` | Scan/import ROMs |
| POST | `/api/games/roms/:id/enrich` | Trigger metadata enrichment |
| GET | `/api/games/save-states/:rom_id` | List save states |
| POST | `/api/games/save-states/:rom_id` | Create save state |
| GET | `/api/games/save-states/:rom_id/:slot` | Get save state |
| DELETE | `/api/games/save-states/:rom_id/:slot` | Delete save state |
| GET | `/api/games/cores` | List emulator cores |
| GET | `/api/games/cores/:platform` | Recommended core for platform |
| GET | `/api/games/cores/:core_name/download` | Get core download URL |
| POST | `/api/games/cores/installed` | Record core installation |
| GET | `/api/games/cores/installed` | List installed cores |
| POST | `/api/games/sessions/start` | Start play session |
| POST | `/api/games/sessions/:session_id/end` | End play session |
| GET | `/api/games/sessions/recent` | Recent play sessions |
| GET | `/api/games/controllers` | List controller configs |
| POST | `/api/games/controllers` | Create controller config |
| DELETE | `/api/games/controllers/:id` | Delete controller config |

## Examples

List every ROM in the library:

```bash
curl -H 'Authorization: Bearer $TOKEN' https://api.example.com/retro-gaming/api/games/roms
```

Start a play session:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/retro-gaming/api/games/sessions/start \
  -d '{"rom_id":"rom_xxx","core_id":"core_xxx"}'
```

Save a state to slot 3:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/retro-gaming/api/games/save-states/rom_xxx \
  -d '{"slot":3,"data_url":"s3://bucket/states/abc.state"}'
```

Trigger metadata enrichment for a ROM:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/retro-gaming/api/games/roms/rom_xxx/enrich
```

## Source

Source-available (license required to run): [`plugins-pro/paid/retro-gaming/`](https://github.com/nself-org/plugins-pro/tree/main/paid/retro-gaming)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- [[plugin-rom-discovery]] — companion plugin for ROM metadata scraping and download orchestration
- [[plugin-game-metadata]] — IGDB-backed game metadata enrichment service
- [[plugin-tmdb]] — broader media metadata for nTV libraries
- [[plugin-streaming]] — live streaming infrastructure used alongside the nTV stack
- [[Pricing]] — tier comparison
- [[Plugins]] — full plugin index

← [[Plugins]] | [[Home]] →
