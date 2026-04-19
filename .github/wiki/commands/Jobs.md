# Jobs Commands

## Entry Points

- Shell actions: `nself plugin jobs <action> [args...]`
- TypeScript CLI: `npx nself-jobs <command> [args...]`

## Shell Action Matrix

| Action | Syntax | Arguments/Subcommands | Options |
|---|---|---|---|
| `init` | `nself plugin jobs init` | none | none |
| `server` | `nself plugin jobs server` | none | none (env-driven) |
| `worker` | `nself plugin jobs worker [QUEUE]` | optional `QUEUE` | `-c, --concurrency <n>` |
| `stats` | `nself plugin jobs stats` | none | `-q, --queue`, `-t, --time`, `-p, --performance`, `-w, --watch` |
| `retry` | `nself plugin jobs retry` | none | `-i, --id`, `-q, --queue`, `-t, --type`, `-l, --limit`, `-s, --show` |
| `schedule` | `nself plugin jobs schedule <command> [args]` | `list`, `show <name>`, `create`, `enable <name>`, `disable <name>`, `delete <name>` | create: `-n`, `-t`, `-c`, `-p`, `-q`, `-d` |

## TS CLI Matrix (`nself-jobs`)

| Command | Syntax | Options |
|---|---|---|
| `retry` | `npx nself-jobs retry` | `--queue <queue>`, `--type <type>`, `--id <id>`, `--limit <limit>` |
| `add` | `npx nself-jobs add -t <type> -p <json>` | `-q, --queue <queue>`, `--priority <priority>`, `--delay <ms>` |

## Common Usage

```bash
nself plugin jobs init
nself plugin jobs worker default --concurrency 10
nself plugin jobs retry --queue default --limit 20
nself plugin jobs schedule create -n daily-backup -t database-backup -c '0 2 * * *' -p '{"database":"production"}'
```

## Source Files

- `plugins/jobs/actions/*.sh`
- `plugins/jobs/ts/src/cli.ts`
