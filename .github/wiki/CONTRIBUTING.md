# Contributing to nSelf Plugins

Thanks for your interest in contributing to the nSelf plugin ecosystem.

## Development Setup

1. Clone the repo and the CLI:
   ```bash
   git clone https://github.com/nself-org/plugins.git
   git clone https://github.com/nself-org/cli.git
   ```

2. Install the nSelf CLI:
   ```bash
   cd cli && make install
   ```

3. Start a local nSelf stack:
   ```bash
   mkdir testproject && cd testproject
   nself init
   nself start
   ```

4. Install a plugin for development:
   ```bash
   nself plugin install <plugin-name> --dev
   ```

## Plugin Structure

Each plugin lives in `free/<plugin-name>/` with this layout:

```
free/<plugin-name>/
  plugin.json          # Plugin manifest
  ts/src/              # TypeScript source
    types.ts
    client.ts
    database.ts
    server.ts
    index.ts
  migrations/          # SQL migrations (optional)
```

## Code Style

- TypeScript strict mode
- All tables use the `np_` prefix (e.g., `np_notify_subscriptions`)
- Every handler includes `source_account_id` for multi-tenancy
- pnpm only (no npm or yarn)

## Pull Request Process

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes, following existing plugin patterns
4. Test with a local nSelf stack
5. Submit a PR with a clear description of what changed and why

## Reporting Issues

Open an issue on GitHub with:
- nSelf CLI version (`nself version`)
- Plugin name and version
- Steps to reproduce
- Expected vs actual behavior
