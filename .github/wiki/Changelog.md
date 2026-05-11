# Changelog

All notable changes to the `nself-org/plugins` repository — both plugin releases and
documentation/structure changes.

---

## Plugin Releases

### v1.2.0 — PENDING (v1.1.0 ecosystem release)

Aligns free plugin library with nSelf CLI v1.1.0. 4 new free plugins added; total free plugins: 29.

**New free plugins:**

- **invitations** (communication): Invitation management with email/SMS delivery. MIT licensed.
- **mdns** (networking): mDNS/Bonjour service discovery. MIT licensed.
- **subtitle-manager** (media): Subtitle search, download, and sync. MIT licensed.
- **tokens** (media): Secure content delivery tokens with expiry and scope. MIT licensed.

**What changed:**

- 25 → 29 free plugins. All MIT licensed. Install with `nself plugin install <name>`.
- All 4 new plugins use `source_account_id` for multi-app isolation per plugin hard rules.
- Table prefix: `np_<abbrev>_*` for each new plugin.
- Registry schema: `registry.json` updated with 4 new entries; `registry-schema.json` unchanged (no new top-level fields).
- Cloudflare Worker (`plugins.nself.org`) auto-deploys with updated registry on merge to main.
- Plugin author notice: no breaking changes to existing free plugin API surface. All 25 existing plugins install unchanged on CLI v1.1.0.

### v1.1.1 — 2026-04-18

**Plugin author notice: no action required.**

v1.1.1 contains no breaking changes to the free plugin API surface. All existing free plugins
install and run without modification on nSelf CLI v1.0.9.

What changed:

- Internal registry schema aligned with CLI v1.0.9 plugin loader.
- No changes to `plugin.yaml` signature fields, lifecycle hooks, or env var names.
- No new required fields.

If you maintain a free plugin: no migration is needed. Your plugin continues to work. See the
pinned issue on this repo for the full compatibility notice.

### v1.0.5 — 2026-04-10

Initial changelog entry. Free plugins track CLI minor versions.

25 free plugins ship with the nSelf ecosystem: analytics, auth-helpers, backup, cache, cdn, ci,
cms-lite, contact, cron, dashboard, dns, email-basic, feature-flags, github, healthcheck, i18n,
monitoring-lite, notify-basic, oauth, queue, rate-limit, scheduler, search-lite, stripe-basic,
webhooks.

All MIT licensed. Install with `nself plugin install <name>`.

---

## Documentation & Structure

### 2026-02-10

- Migrated public documentation source from `docs/` to `/.wiki/`.
- Updated GitHub Actions wiki sync to publish directly from `/.wiki`.
- Added strict repository structure policy and public wiki navigation pages.
- Moved planning-only content (`NCHAT-PLUGINS-PLAN.md`) out of public docs into private planning storage.
- Added private control-plane scaffolding (GO-style) for planning and QA governance.
- Added plugin command reference matrices with explicit action/subcommand/option syntax in `/.wiki/commands/*`.
- Clarified license/changelog placement policy (`LICENSE` at root, discoverability mirrors in wiki).
