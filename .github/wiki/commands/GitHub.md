# GitHub Commands

## Entry Points

- Shell actions: `nself plugin github <action> [args...]`
- TypeScript CLI: `npx nself-github <command> [args...]`

## Shell Action Matrix

| Action | Syntax | Subcommands | Options |
|---|---|---|---|
| `sync` | `nself plugin github sync [--initial] [--full] [--repos-only]` | none | `--initial`, `--full`, `--repos-only` |
| `repos` | `nself plugin github repos <subcommand> [options]` | `list`, `show <owner/repo>`, `stats`, `sync` | `--org`, `--language`, `--archived`, `--format <table|json|csv>` |
| `issues` | `nself plugin github issues <subcommand> [options]` | `list`, `show <id>`, `open`, `closed`, `stats` | `--state`, `--repo`, `--author`, `--label`, `--limit` |
| `prs` | `nself plugin github prs <subcommand> [options]` | `list`, `show <id>`, `open`, `merged`, `stats` | `--state`, `--repo`, `--author`, `--draft`, `--limit` |
| `webhook` | `nself plugin github webhook <subcommand> [options]` | `list`, `show <id>`, `pending`, `retry <id>`, `stats` | `--event`, `--repo`, `--limit` |
| `actions` | `nself plugin github actions <subcommand> [options]` | `list`, `show <id>`, `failed`, `stats` | `--repo`, `--workflow`, `--status`, `--limit` |

## TS CLI Matrix (`nself-github`)

| Command | Syntax | Options |
|---|---|---|
| `sync` | `npx nself-github sync` | `-r, --resources <resources>`, `--repos <repos>`, `--since <date>` |
| `server` | `npx nself-github server` | `-p, --port <port>`, `-h, --host <host>` |
| `init` | `npx nself-github init` | none |
| `status` | `npx nself-github status` | none |
| `repos` | `npx nself-github repos` | `-l, --limit <limit>` |
| `issues` | `npx nself-github issues` | `-l, --limit <limit>`, `-s, --state <state>` |
| `prs` | `npx nself-github prs` | `-l, --limit <limit>`, `-s, --state <state>` |

## Common Usage

```bash
nself plugin github sync --repos-only
nself plugin github repos list --org acamarata --format json
nself plugin github issues open --repo acamarata/nself-plugins --limit 25
nself plugin github actions failed --repo acamarata/nself-plugins
```

## Source Files

- `plugins/github/actions/*.sh`
- `plugins/github/ts/src/cli.ts`
