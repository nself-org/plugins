# Realtime Commands

## Entry Points

- Shell actions: `nself plugin realtime <action> [args...]`
- TypeScript CLI: `npx nself-realtime <command> [args...]`

## Shell Action Matrix

| Action | Syntax | Subcommands/Args |
|---|---|---|
| `init` | `nself plugin realtime init` | none |
| `server` | `nself plugin realtime server <command>` | `start`, `stop`, `restart`, `status`, `logs [lines]` |
| `status` | `nself plugin realtime status` | none |
| `rooms` | `nself plugin realtime rooms <command> [args]` | `list`, `create <name> [type] [visibility]`, `delete <name>`, `info <name>`, `add-member <room> <user_id> [role]`, `remove-member <room> <user_id>` |

## TS CLI Matrix (`nself-realtime`)

| Command | Syntax | Options |
|---|---|---|
| `init` | `npx nself-realtime init` | none |
| `stats` | `npx nself-realtime stats` | none |
| `rooms` | `npx nself-realtime rooms` | none |
| `create-room` | `npx nself-realtime create-room <name>` | `-t, --type <type>`, `-v, --visibility <visibility>` |
| `connections` | `npx nself-realtime connections` | none |
| `events` | `npx nself-realtime events` | `-n, --number <number>` |

## Common Usage

```bash
nself plugin realtime init
nself plugin realtime server start
nself plugin realtime rooms create general channel public
nself plugin realtime rooms add-member general user_123 member
npx nself-realtime events --number 100
```

## Source Files

- `plugins/realtime/actions/*.sh`
- `plugins/realtime/ts/src/cli.ts`
