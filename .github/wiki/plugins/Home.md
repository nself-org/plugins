# Home Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install home` |
| **Requirements** | nSelf >=1.0.0 |

<!-- END GENERATED:catalog -->

> Home automation bridge.

## Install

```bash
nself plugin install home
```

No license key required.

## Description

Home automation bridge. Connects Home Assistant and MQTT to ɳSelf, enabling smart device control, state monitoring, scene activation, and command logging.

Category: `integrations`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `PLUGIN_INTERNAL_SECRET` | `(required)` | Shared secret for plugin-to-plugin HTTP calls (`X-Internal-Token` header) |
| `HOME_PORT` | `3127` | - |
| `HOME_ASSISTANT_URL` | `-` | - |
| `HOME_ASSISTANT_TOKEN` | `-` | - |
| `MQTT_BROKER_URL` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3127 | Home service port |

## Database Schema

3 table(s) added to your Postgres database:

- `np_home_connections`
- `np_home_devices`
- `np_home_command_log`

## REST API

```
GET    /health
GET    /home/automations
POST   /home/automations/{id}/toggle
POST   /home/command
GET    /home/devices
POST   /home/scene/{scene_id}
GET    /home/state/{entity_id}
GET    /ready
```

## Examples

### Health check

```bash
curl http://localhost:3127/health
```

## Source

[`plugins/home/`](https://github.com/nself-org/plugins/tree/main/home)

Manifest: [`plugins/home/plugin.json`](https://github.com/nself-org/plugins/tree/main/home/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
