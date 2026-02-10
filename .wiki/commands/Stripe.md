# Stripe Commands

## Entry Points

- Shell actions: `nself plugin stripe <action> [args...]`
- TypeScript CLI: `pnpm exec nself-stripe <command> [args...]` (run from `plugins/stripe/ts`)

## Shell Action Matrix

| Action | Syntax | Subcommands | Notes |
|---|---|---|---|
| `sync` | `nself plugin stripe sync [target]` | `all`, `customers`, `products`, `prices`, `subscriptions` | default target: `all` |
| `customers` | `nself plugin stripe customers <command> [args]` | `list [limit] [offset]`, `get <id>`, `search <query>`, `count` | `count` also accepts `stats` alias |
| `subscriptions` | `nself plugin stripe subscriptions <command> [args]` | `list [status] [limit]`, `get <id>`, `stats`, `mrr` | status examples: `active`, `trialing`, `past_due`, `canceled`, `all` |
| `invoices` | `nself plugin stripe invoices <command> [args]` | `list [status] [limit]`, `get <id>`, `stats`, `recent [limit]`, `failed [limit]` | status examples: `draft`, `open`, `paid`, `uncollectible`, `void`, `all` |
| `webhook` | `nself plugin stripe webhook <command> [args]` | `status`, `events [limit] [type]`, `get <event_id>`, `errors [limit]`, `retry <event_id>`, `retry-all [limit]` | webhook inspection/retry controls |

## TS CLI Matrix (`nself-stripe`)

| Command | Syntax | Options |
|---|---|---|
| `sync` | `pnpm exec nself-stripe sync` | `-r, --resources <resources>`, `-i, --incremental` |
| `server` | `pnpm exec nself-stripe server` | `-p, --port <port>`, `-h, --host <host>` |
| `init` | `pnpm exec nself-stripe init` | none |
| `status` | `pnpm exec nself-stripe status` | none |
| `customers` | `pnpm exec nself-stripe customers [action] [id]` | `-l, --limit <limit>` |
| `subscriptions` | `pnpm exec nself-stripe subscriptions [action] [id]` | `-l, --limit <limit>`, `-s, --status <status>` |
| `invoices` | `pnpm exec nself-stripe invoices [action] [id]` | `-l, --limit <limit>`, `-s, --status <status>` |
| `products` | `pnpm exec nself-stripe products [action] [id]` | `-l, --limit <limit>` |
| `prices` | `pnpm exec nself-stripe prices [action] [id]` | `-l, --limit <limit>` |

## Multi-Account Unified Sync

Both shell and TS sync commands can aggregate multiple Stripe accounts into one sync run when these env vars are set:

1. `STRIPE_API_KEYS` (comma-separated API keys)
2. `STRIPE_ACCOUNT_LABELS` (optional comma-separated labels in the same order)
3. `STRIPE_WEBHOOK_SECRETS` (optional comma-separated webhook secrets in the same order)

If not provided, the plugin falls back to single-account mode with `STRIPE_API_KEY`.
Synced rows are tagged with `source_account_id`; CLI list output also prefixes records with `[account-id]`.

Action values in TS CLI:

1. `customers`: `list`, `show`, `sync`
2. `subscriptions`: `list`, `show`, `stats`
3. `invoices`: `list`, `show`
4. `products`: `list`, `show`
5. `prices`: `list`, `show`

## Common Usage

```bash
nself plugin stripe sync subscriptions
nself plugin stripe customers search user@example.com
nself plugin stripe invoices list paid 50
nself plugin stripe webhook events 100 customer
pnpm exec nself-stripe subscriptions stats
```

## Source Files

- `plugins/stripe/actions/*.sh`
- `plugins/stripe/ts/src/cli.ts`
