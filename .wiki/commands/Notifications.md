# Notifications Commands

## Entry Points

- Shell actions: `nself plugin notifications <action> [args...]`
- TypeScript CLI: `npx nself-notifications <command> [args...]`

## Shell Action Matrix

| Action | Syntax | Subcommands/Args | Options |
|---|---|---|---|
| `init` | `nself plugin notifications init` | none | none |
| `server` | `nself plugin notifications server` | none | `--port <port>`, `--host <host>` |
| `worker` | `nself plugin notifications worker` | none | `--concurrency <n>`, `--poll-interval <ms>` |
| `template` | `nself plugin notifications template <action> [args]` | `list [json]`, `show <name>`, `create`, `update <name>`, `delete <name>` | none |
| `test` | `nself plugin notifications test <type> [args]` | `email <recipient>`, `template <name> <recipient>`, `providers` | none |
| `stats` | `nself plugin notifications stats <type> [args]` | `overview`, `delivery [days]`, `engagement [days]`, `providers`, `templates [limit]`, `failures [limit]`, `hourly [hours]`, `export [format] [file]` | none |

## TS CLI Matrix (`nself-notifications`)

| Command | Syntax |
|---|---|
| `init` | `npx nself-notifications init` |
| `templates` | `npx nself-notifications templates` |
| `status` | `npx nself-notifications status` |

## Common Usage

```bash
nself plugin notifications init
nself plugin notifications template list
nself plugin notifications test template welcome_email user@example.com
nself plugin notifications stats delivery 30
nself plugin notifications worker --concurrency 10
```

## Source Files

- `plugins/notifications/actions/*.sh`
- `plugins/notifications/ts/src/cli.ts`
