# nself-tenant

Multi-tenant operations and per-tenant billing, moved out of the CLI core under
CLI-R11.

This plugin provides two commands: `nself tenant` and `nself billing`.

## Install

```bash
nself install tenant
```

## Commands

```bash
nself tenant create acme
nself tenant list
nself tenant suspend acme
nself tenant upgrade acme --plan pro
nself tenant destroy acme

nself billing usage --tenant acme
nself billing invoice-preview --tenant acme
nself billing report
nself billing retry-event <id>
```

## Configuration

Project settings come from the environment. nself resolves the `.env` cascade
and passes in every variable listed under `envVars` in `plugin.json` — the
Postgres connection, the MinIO settings and the Prometheus settings. The plugin
does not read `.env` files itself, so there is only one implementation of the
cascade order.

## Note

`internal/tenant` also stays in the CLI, because `nself db` uses it. The copy
here is the plugin's own.

## License

MIT.
