# nself-sentry

ɳSentry operations, moved out of the CLI core under CLI-R11.

This plugin provides two commands: `nself sentry` and `nself sentry-server`.

## Install

```bash
nself install sentry-cli
```

Both commands then work exactly as before — the CLI proxies each to its own
binary.

## Commands

```bash
nself sentry login --api-url https://sentry.example.com
nself sentry monitors list
nself sentry incidents list
nself sentry pages list
nself sentry alerts list
nself sentry status
```

`nself sentry status` exits non-zero when the instance is unhealthy, which is
how it behaved in the CLI. Scripts checking that exit code keep working.

```bash
nself sentry-server provision
```

## Requirements

`nself sentry-server provision` chains into `nself build` and `nself start`, so
the nself CLI must be on your `PATH`. Provisioning against Hetzner reads
`HETZNER_NSELF_TOKEN` (copied to `HCLOUD_TOKEN` when that is unset).

## Note on `nself mcp`

The CLI keeps its ɳSentry MCP tools and its own copy of the API client. `mcp` is
a core command, and there is no mechanism for a plugin to contribute MCP tools,
so the two coexist rather than one importing the other.

## License

MIT.
