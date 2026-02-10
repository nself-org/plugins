# ID.me Commands

## Entry Points

- Shell actions: `nself plugin idme <action> [args...]`
- TypeScript CLI: `npx nself-idme <command> [args...]`

## Shell Action Matrix

| Action | Syntax | Subcommands |
|---|---|---|
| `init` | `nself plugin idme init [command]` | `config`, `auth`, `groups` |
| `verify` | `nself plugin idme verify [command] [args]` | `user <email>`, `list [limit]`, `stats` |
| `groups` | `nself plugin idme groups [command] [args]` | `list`, `user <email>`, `type <group_type>`, `types` |
| `test` | `nself plugin idme test [command]` | `config`, `database`, `api`, `all` |

## TS CLI Matrix (`nself-idme`)

| Command | Syntax | Options |
|---|---|---|
| `init` | `npx nself-idme init` | none |
| `verify` | `npx nself-idme verify <email>` | none |
| `server` | `npx nself-idme server` | `-p, --port <port>`, `-h, --host <host>` |
| `test` | `npx nself-idme test` | none |

## Common Usage

```bash
nself plugin idme init auth
nself plugin idme verify user user@example.com
nself plugin idme groups type veteran
nself plugin idme test all
```

## Source Files

- `plugins/idme/actions/*.sh`
- `plugins/idme/ts/src/cli.ts`
