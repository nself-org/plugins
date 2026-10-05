# Plugin Dev Plugin

> Plugin author tools: scaffold, link, run with hot reload, debug and test a plugin under development. **Free — MIT licensed.**

## Install

```bash
nself add plugin-dev
```

No license key required.

## Description

The author commands that used to live in core as `nself plugin init|new|dev|debug|link|unlink|test` now ship as this free
CLI plugin. Authors add the plugin, then run the commands as `nself plugin-dev <command>`. Behaviour is the same as core:
same flags, messages, exit codes and file effects.

This is a CLI plugin: it installs the `nself-plugin-dev` binary into your plugin path and runs as a command, not a
background service. Category: `development`.

## Commands

| Command | What it does |
|---------|--------------|
| `nself plugin-dev init <name>` | Scaffold a plugin project. Flags: `--template go\|rust\|node\|static`, `--tier free\|pro`, `--bundle`, `--description`, `--author`, `--category`, `--min-cli`, `--min-sdk`, `--port`, `--out`, `--force`, `--tenancy`, `--no-interactive`. Alias: `scaffold`. |
| `nself plugin-dev new <name>` | Deprecated alias of `init`; prints the deprecation notice. |
| `nself plugin-dev link <path>` | Record a local plugin directory (it must hold a `plugin.yaml`) in `~/.nself/plugin-links.json` so `nself build` prefers it. `--host`, `--list`. |
| `nself plugin-dev unlink <name>` | Remove the link. Unlinking an unlinked plugin is a no-op. |
| `nself plugin-dev dev <name>` | Auto-link, then run the hot-reload watcher (air, or fswatch). `--no-link`, `--debug`, `--entrypoint`. |
| `nself plugin-dev debug <name>` | Build the plugin with debug flags and run it under a headless `dlv` on a port in 2345-2399. `--port`, `--port-only`. |
| `nself plugin-dev test <name>` | Unit tests, then a smoke install and uninstall. `--phase unit\|smoke\|both`, `--host`, `--no-cleanup`. |

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_LOCAL_URL` | `http://localhost:8080` | Base URL `test` polls for `/healthz` during the smoke phase. |

## Examples

```bash
nself plugin-dev init my-plugin --tier free --no-interactive
nself plugin-dev link ./my-plugin
nself plugin-dev dev my-plugin
nself plugin-dev test my-plugin --phase unit --host
```

## Differences from the core commands

- `--help` shows `nself plugin-dev <command>` where core shows `nself plugin <command>`.
- `dev --debug` re-runs this binary as `debug`, and `test` finds `nself` on `PATH` for the smoke phases.
- `link --list` is sorted by name.

## Source

[`free/plugin-dev/`](https://github.com/nself-org/plugins/tree/main/free/plugin-dev)
