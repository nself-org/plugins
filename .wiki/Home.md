# nself Plugins Wiki Home

This wiki is the **SPORT** for this repository: **Single Point of Reference and Truth**.

All public docs are authored in `/.wiki` and synced to GitHub Wiki via `.github/workflows/wiki-sync.yml`.

## Start Here

- [[Installation]]
- [[Quick Start|getting-started/Quick-Start]]
- [[Commands|COMMANDS]]
- [[Repository Structure|REPOSITORY-STRUCTURE]]

## Top-Level Categories

### Getting Started

- [[Installation]]
- [[Quick Start|getting-started/Quick-Start]]
- [[Configuration|guides/Configuration]]

### Commands (Complete Reference)

- [[Commands|COMMANDS]]
- [[File Processing Commands|commands/File-Processing]]
- [[GitHub Commands|commands/GitHub]]
- [[ID.me Commands|commands/IDme]]
- [[Jobs Commands|commands/Jobs]]
- [[Notifications Commands|commands/Notifications]]
- [[Realtime Commands|commands/Realtime]]
- [[Shopify Commands|commands/Shopify]]
- [[Stripe Commands|commands/Stripe]]

Command pages include action/subcommand syntax, argument shapes, and option flags from source.

### Plugin Documentation

- [[File Processing|plugins/FileProcessing]]
- [[GitHub|plugins/GitHub]]
- [[ID.me|plugins/IDme]]
- [[Jobs|plugins/Jobs]]
- [[Notifications|plugins/Notifications]]
- [[Realtime|plugins/Realtime]]
- [[Shopify|plugins/Shopify]]
- [[Stripe|plugins/Stripe]]

### Architecture and API

- [[Plugin System|architecture/Plugin-System]]
- [[REST API|api/REST-API]]

### Engineering Guides

- [[Plugin Development|DEVELOPMENT]]
- [[TypeScript Plugin Guide|TYPESCRIPT_PLUGIN_GUIDE]]
- [[Deployment|guides/Deployment]]
- [[Migration|guides/Migration]]
- [[Best Practices|guides/Best-Practices]]
- [[Troubleshooting FAQ|troubleshooting/FAQ]]

### Governance and Reference

- [[Repository Structure|REPOSITORY-STRUCTURE]]
- [[Security]]
- [[Contributing|CONTRIBUTING]]
- [[Planned Plugins|PLANNED]]
- [[Changelog|CHANGELOG]]
- [[License]]

## Root Structure Policy (Canonical)

Root should remain intentionally minimal:

- `.claude/` (private, gitignored)
- `.codex/` (private, gitignored)
- `.github/`
- `.wiki/`
- `plugins/`
- `shared/`
- `registry.json`
- `registry-schema.json`
- `README.md`
- `LICENSE`
- required meta files (for example `.gitignore`)

Allowed infrastructure exception:

- `.workers/` for registry publishing.

Legacy `docs/` is retired. Public docs belong in `/.wiki` only.

## SPORT Rules

1. If behavior changes, docs must be updated in `/.wiki` in the same change set.
2. Commands in `COMMANDS.md` and `commands/*.md` must match action/CLI source files.
3. Any drift between code and docs is treated as a defect.
