# Shopify Commands

## Entry Points

- Shell actions: `nself plugin shopify <action> [args...]`
- TypeScript CLI: `npx nself-shopify <command> [args...]`

## Shell Action Matrix

| Action | Syntax | Subcommands | Options |
|---|---|---|---|
| `sync` | `nself plugin shopify sync [--initial] [--products-only]` | none | `--initial`, `--products-only` |
| `products` | `nself plugin shopify products <subcommand> [options]` | `list`, `show <id>`, `stats`, `low-stock` | `--vendor`, `--status`, `--limit` |
| `customers` | `nself plugin shopify customers <subcommand> [options]` | `list`, `show <id>`, `top`, `stats` | `--limit` |
| `orders` | `nself plugin shopify orders <subcommand> [options]` | `list`, `show <id>`, `pending`, `unfulfilled`, `stats` | `--status`, `--fulfillment`, `--limit` |
| `webhook` | `nself plugin shopify webhook <subcommand> [options]` | `list`, `show <id>`, `pending`, `retry <id>`, `stats` | `--topic`, `--limit` |

## TS CLI Matrix (`nself-shopify`)

| Command | Syntax | Options |
|---|---|---|
| `sync` | `npx nself-shopify sync` | `-r, --resources <resources>` |
| `server` | `npx nself-shopify server` | `-p, --port <port>`, `-h, --host <host>` |
| `init` | `npx nself-shopify init` | none |
| `status` | `npx nself-shopify status` | none |
| `products` | `npx nself-shopify products` | `-l, --limit <limit>` |
| `customers` | `npx nself-shopify customers` | `-l, --limit <limit>` |
| `orders` | `npx nself-shopify orders` | `-l, --limit <limit>`, `-s, --status <status>` |
| `collections` | `npx nself-shopify collections` | `-l, --limit <limit>` |
| `inventory` | `npx nself-shopify inventory` | `-l, --limit <limit>` |
| `webhooks` | `npx nself-shopify webhooks` | `-l, --limit <limit>`, `-t, --topic <topic>` |
| `analytics` | `npx nself-shopify analytics` | none |

## Common Usage

```bash
nself plugin shopify sync --products-only
nself plugin shopify products list --status active --limit 100
nself plugin shopify orders pending --limit 50
nself plugin shopify webhook stats
npx nself-shopify analytics
```

## Source Files

- `plugins/shopify/actions/*.sh`
- `plugins/shopify/ts/src/cli.ts`
