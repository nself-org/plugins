# Home Plugin

> Home automation bridge connecting Home Assistant and MQTT to ɳSelf for smart device control, state monitoring, and scene activation.

**Tier:** Free (MIT) — no license required.

## Install

```bash
nself plugin install home
nself build
```

After install, `nself build` regenerates the docker-compose stack with the plugin service included.

## Description

The home plugin is an HTTP gateway to Home Assistant. It proxies live device state from the Home Assistant REST API and exposes endpoints for issuing commands, activating scenes, listing devices, and managing automations. Commands issued are logged to `np_home_command_log` for audit. A MQTT broker URL may be configured (via `MQTT_BROKER_URL`) but MQTT message processing is not implemented in the current version; the plugin operates as a pure Home Assistant REST client.

The plugin is designed to be a back-end gateway, not a UI. Build your own dashboards (or use ɳClaw as the conversational front) and let `home` handle the underlying device protocol translation.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `PLUGIN_INTERNAL_SECRET` | Yes | — | Shared secret for inter-plugin auth |
| `HOME_PORT` | No | `3127` | Listen port |
| `HOME_ASSISTANT_URL` | No | — | Base URL of a Home Assistant instance |
| `HOME_ASSISTANT_TOKEN` | No | — | Long-lived access token for Home Assistant |
| `MQTT_BROKER_URL` | No | — | MQTT broker URL (e.g., `mqtt://broker:1883`) |

## Ports

- Default port: `3127` (override via `HOME_PORT`)
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly.

## Database Schema

Tables created (prefix `np_home_`):

- `np_home_connections`: per-account Home Assistant or MQTT connection config
- `np_home_devices`: known devices, types, capabilities, last-seen state
- `np_home_command_log`: audit trail for issued commands

All tables use `source_account_id` for multi-app isolation.

## REST API

Account identity is passed via the `X-Source-Account-ID` header (defaults to `primary`).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/ready` | Readiness probe |
| POST | `/home/command` | Issue a command to a Home Assistant entity |
| GET | `/home/devices` | List known devices |
| GET | `/home/state/{entity_id}` | Read live state for an entity from Home Assistant |
| POST | `/home/scene/{scene_id}` | Activate a scene |
| GET | `/home/automations` | List automations |
| POST | `/home/automations/{id}/toggle` | Toggle an automation on or off |

## Examples

Send a command to a device:

```bash
curl -X POST https://api.your-domain.tld/home/command \
  -H "X-Source-Account-ID: primary" \
  -H "Content-Type: application/json" \
  -d '{"entity_id":"light.porch","action":"turn_off"}'
```

Read live state:

```bash
curl https://api.your-domain.tld/home/state/sensor.temperature \
  -H "X-Source-Account-ID: primary"
```

Activate a scene:

```bash
curl -X POST https://api.your-domain.tld/home/scene/scene.evening \
  -H "X-Source-Account-ID: primary"
```

## Source

MIT licensed, source included in this repository: [`free/home/`](https://github.com/nself-org/plugins/tree/main/free/home)

## See Also

- ɳClaw bundle plugins (`ai`, `claw`, `voice`) for conversational device control
- `notify` for alerting on device state changes
- `.github/docs/licensing/bundles.md` for bundle membership reference
- `.github/docs/licensing.md` for the pricing matrix
