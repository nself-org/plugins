# Contributing to nSelf Plugins

## What This Is

Free MIT plugins for the nSelf CLI. There are 25 free plugins available to everyone.

## Prerequisites

- nSelf CLI
- Node.js 22+ (for registry/SDK tools)
- Go 1.22+ (for Go plugins)

## Plugin Development

Plugins live in `free/`, `community/`, and `monitoring/` directories. Each plugin has its own directory with:

- `plugin.json` — manifest
- `install.sh` — installation script
- `docker-compose.yml` (if adding a service)

## Setup

```bash
nself plugin install <name>   # install any free plugin
```

## Pull Requests

1. Fork and create a branch
2. New plugins need a `plugin.json`, `install.sh`, and README
3. All shell scripts must be Bash 3.2 compatible
4. Submit PR against `main`

## Commit Style

Conventional commits: `feat:`, `fix:`, `chore:`, `docs:`, `test:`
