# Contributing to nSelf Plugins

Thanks for your interest in contributing to the nSelf plugin ecosystem.

## What This Is

Free MIT plugins for the nSelf CLI. There are 25 free plugins available to everyone.

## Prerequisites

- nSelf CLI
- Node.js 22+ (for registry/SDK tools)
- Go 1.22+ (for Go plugins)

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

Plugins live in `free/`, `community/`, and `monitoring/` directories. Each plugin lives in `<directory>/<plugin-name>/` with this layout:

```
free/<plugin-name>/
  plugin.json          # Plugin manifest
  install.sh           # Installation script
  README.md            # Plugin-specific docs
  docker-compose.yml   # If adding a service
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
- All shell scripts must be Bash 3.2 compatible (no `echo -e`, no `${var,,}`, no `declare -A`)
- Follow existing code patterns; include comments for complex logic; use `printf` not `echo -e`

## Mandatory Standards

> Read [[Standards]] BEFORE contributing. Violations will cause automated build failures.

All plugins must comply with:

- Universal `np_` table prefix
- `source_account_id` multi-app isolation
- Official category assignment (1 of 13)
- Lowercase-with-hyphens naming (`my-service`, not `MyService`)

### Schema Design (Mandatory)

ALL tables must:

- Be prefixed with `np_` (e.g. `np_stripe_customers`, not `stripe_customers`)
- Include `source_account_id` for multi-tenant isolation
- Include standard columns: `created_at`, `synced_at`
- Use JSONB for flexible data
- Add indexes for common queries

Example:

```sql
CREATE TABLE np_myplugin_resources (
    id UUID PRIMARY KEY,
    source_account_id VARCHAR(255) NOT NULL,  -- REQUIRED
    name VARCHAR(255) NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    INDEX idx_np_myplugin_resources_account (source_account_id)
);
```

### Multi-App Configuration (Mandatory)

`plugin.json` must include:

```json
{
  "multiApp": {
    "supported": true,
    "isolationColumn": "source_account_id",
    "pkStrategy": "uuid",
    "defaultValue": "primary"
  }
}
```

### Category Assignment (Mandatory)

Choose ONE of the 13 official categories:

- authentication, automation, commerce, communication, content
- data, development, infrastructure, integrations, media
- streaming, sports, compliance

See [[Standards]] for category descriptions.

## Pre-Submission Validation

Before submitting, run these checks:

```bash
# 1. Validate JSON syntax
jq empty plugins/my-plugin/plugin.json

# 2. Check table prefixes (should return nothing)
jq -r '.tables[] | select(startswith("np_") | not)' plugins/my-plugin/plugin.json

# 3. Check multi-app isolation (should output: source_account_id)
jq -r '.multiApp.isolationColumn' plugins/my-plugin/plugin.json

# 4. Check category is valid
jq -r '.category' plugins/my-plugin/plugin.json
```

All checks must pass or your PR will fail automated validation.

## Pull Request Process

1. Fork the repository
2. Create a feature branch from `main`
3. New plugins need a `plugin.json`, `install.sh`, and README
4. Make your changes, following existing plugin patterns
5. Test with a local nSelf stack
6. Submit a PR with a clear description of what changed and why

## Commit Style

Conventional commits: `feat:`, `fix:`, `chore:`, `docs:`, `test:`

## Reporting Issues

Open an issue on GitHub with:
- nSelf CLI version (`nself version`)
- Plugin name and version
- Steps to reproduce
- Expected vs actual behavior

## Security Disclosures

Do not open a public issue for security vulnerabilities. Follow the process in [SECURITY.md](https://github.com/nself-org/plugins/blob/main/.github/SECURITY.md).

## Community and Questions

- [GitHub Discussions](https://github.com/nself-org/plugins/discussions) — preferred for questions about plugin development
- Community: [nself.org](https://nself.org)

## Related

- [GOVERNANCE.md](https://github.com/nself-org/plugins/blob/main/.github/GOVERNANCE.md) — decision model
- [ENFORCEMENT.md](https://github.com/nself-org/plugins/blob/main/.github/ENFORCEMENT.md) — code of conduct enforcement
- [CODEOWNERS](https://github.com/nself-org/plugins/blob/main/.github/CODEOWNERS) — who reviews what
- [[Plugin-Development]] — detailed plugin dev guide
- [[Plugin-Overview]] — all 25 free plugins

---
← [[Home]] | [[_Sidebar]]
