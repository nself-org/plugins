# Changelog — nSelf Free Plugins

All notable changes to the free plugin ecosystem (`nself-org/plugins`).

---

## v1.1.1 — 2026-04-18

**Plugin author notice: no action required.**

v1.1.1 contains no breaking changes to the free plugin API surface. All existing free plugins
install and run without modification on nSelf CLI v1.0.9.

### What changed

- Internal registry schema aligned with CLI v1.0.9 plugin loader.
- No changes to `plugin.yaml` signature fields, lifecycle hooks, or env var names.
- No new required fields.

### If you maintain a free plugin

No migration is needed. Your plugin continues to work. See the pinned issue on this repo for
the full compatibility notice.

---

## v1.0.5 — 2026-04-10

Initial changelog entry. Free plugins track CLI minor versions.

25 free plugins ship with the nSelf ecosystem: analytics, auth-helpers, backup, cache, cdn, ci,
cms-lite, contact, cron, dashboard, dns, email-basic, feature-flags, github, healthcheck, i18n,
monitoring-lite, notify-basic, oauth, queue, rate-limit, scheduler, search-lite, stripe-basic,
webhooks.

All MIT licensed. Install with `nself plugin install <name>`.
