# Commands

This page is the command SPORT index for `nself-plugins`.

## Command Surfaces

1. Shell actions:

```bash
nself plugin <plugin> <action> [subcommand] [args...] [options...]
```

2. TypeScript CLIs:

```bash
npx nself-<plugin> <command> [args...] [options...]
```

## Plugin Command Index

| Plugin | Shell Action Family | TS CLI Binary | Command Reference |
|---|---|---|---|
| File Processing | `nself plugin file-processing ...` | `nself-file-processing` | [[File Processing Commands|commands/File-Processing]] |
| GitHub | `nself plugin github ...` | `nself-github` | [[GitHub Commands|commands/GitHub]] |
| ID.me | `nself plugin idme ...` | `nself-idme` | [[ID.me Commands|commands/IDme]] |
| Jobs | `nself plugin jobs ...` | `nself-jobs` | [[Jobs Commands|commands/Jobs]] |
| Notifications | `nself plugin notifications ...` | `nself-notifications` | [[Notifications Commands|commands/Notifications]] |
| Realtime | `nself plugin realtime ...` | `nself-realtime` | [[Realtime Commands|commands/Realtime]] |
| Shopify | `nself plugin shopify ...` | `nself-shopify` | [[Shopify Commands|commands/Shopify]] |
| Stripe | `nself plugin stripe ...` | `nself-stripe` | [[Stripe Commands|commands/Stripe]] |

## Quick Examples

```bash
nself plugin stripe sync subscriptions
nself plugin github repos list --org acamarata --format json
nself plugin notifications stats delivery 30
nself plugin realtime rooms create general channel public
npx nself-shopify orders --limit 50 --status paid
```

## Source of Truth

Command docs are derived from:

1. `plugins/*/actions/*.sh` (shell actions)
2. `plugins/*/ts/src/cli.ts` (TypeScript CLIs)
3. `plugins/*/plugin.json` (metadata declarations)

If implementation and docs diverge, treat it as a defect and update both in the same change set.
Executable syntax in this wiki follows actual script/CLI behavior first.
