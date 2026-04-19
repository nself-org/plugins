# File Processing Commands

## Entry Points

- Shell actions: `nself plugin file-processing <action> [args...]`
- TypeScript CLI: `npx nself-file-processing <command> [args...]`

## Shell Action Matrix

| Action | Syntax | Arguments | Options/Defaults |
|---|---|---|---|
| `init` | `nself plugin file-processing init` | none | Installs/builds TS layer when needed |
| `server` | `nself plugin file-processing server` | none | Uses `PORT` (default `3104`) |
| `worker` | `nself plugin file-processing worker` | none | Uses `FILE_QUEUE_CONCURRENCY` (default `3`) |
| `process` | `nself plugin file-processing process <file-id> <file-path>` | `file-id`, `file-path` | Sends `POST /api/jobs` |
| `stats` | `nself plugin file-processing stats` | none | DB-backed stats output |
| `cleanup` | `nself plugin file-processing cleanup [retention_days]` | optional `retention_days` | Default retention `30` days |

## TS CLI Matrix (`nself-file-processing`)

| Command | Syntax | Arguments | Options |
|---|---|---|---|
| `init` | `npx nself-file-processing init` | none | none |
| `process` | `npx nself-file-processing process <file-id> <file-path>` | `file-id`, `file-path` | none |
| `stats` | `npx nself-file-processing stats` | none | none |
| `cleanup` | `npx nself-file-processing cleanup` | none | `-d, --days <days>` |

## Typical Flows

```bash
# Boot
nself plugin file-processing init
nself plugin file-processing server
nself plugin file-processing worker

# Queue one file for processing
nself plugin file-processing process file_123 uploads/image.jpg

# Maintenance
nself plugin file-processing stats
nself plugin file-processing cleanup 45
```

## Source Files

- `plugins/file-processing/actions/*.sh`
- `plugins/file-processing/ts/src/cli.ts`
