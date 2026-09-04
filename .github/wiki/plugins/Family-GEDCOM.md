# Family-GEDCOM Plugin

> Imports any GEDCOM 5.5.1/7.0 family-tree export into the (planned) family plugin. **Free — MIT licensed.**

## Install

```bash
nself plugin install family-gedcom
```

No license key required.

## Description

Generic GEDCOM file importer for the family plugin. Accepts any GEDCOM 5.5.1 / 7.0 file from any provider with optional photo-folder upload. PLANNED: requires the 'family' plugin (ɳFamily bundle, planned v1.1.0) and 'object-storage' + 'photos' plugins. Not installable until the family plugin ships.

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `content`. Current version: `0.0.1`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | *(required)* | Required. |

## Ports

| Port | Purpose |
|------|---------|
| 3108 | Family-GEDCOM service |

## REST API

```
POST /import              — Upload and parse a GEDCOM 5.5.1/7.0 file
GET  /import/{id}/status  — Import job status
```

## Nginx Routes

| Route | Target |
|-------|--------|
| `/family-gedcom/` | GEDCOM import REST API |

## Examples

### Check health

```bash
curl http://localhost:3108/health
```

## Source

[`plugins/family-gedcom/`](https://github.com/nself-org/plugins/tree/main/family-gedcom)

Manifest: [`plugins/family-gedcom/plugin.json`](https://github.com/nself-org/plugins/tree/main/family-gedcom/plugin.json)

## See Also

- [[Notify]] — multi-channel notification service

← [[Home]] →
