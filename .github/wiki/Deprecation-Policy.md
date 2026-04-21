# DEPRECATION-POLICY — Free Plugins

Governs end-of-life (EOL) notice periods and procedures for plugins in
`plugins/` (MIT-licensed, free tier). (S58-T07)

For paid plugin policy see `plugins-pro/DEPRECATION-POLICY.md`.
Full doctrine: `.claude/docs/doctrines/plugin-lifecycle.md`.

---

## Minimum Notice Periods

| Tier | Min notice | Extra requirements |
|------|------------|--------------------|
| **Free (MIT)** | 6 months from `announcedDate` to `eolDate` | deprecation block in plugin.json |
| **Security-critical removal** | 0-day allowed | announcement must state "security-critical removal" |

## Transition Process

1. Open PR setting `status: deprecated` in `plugin.json` and `registry.json`.
2. Include complete `deprecation` block: `announcedDate`, `eolDate` (min 6 months
   from announcedDate), `migrationGuide` (URL that resolves HTTP 200), and
   optionally `replacedBy` + `migrationScript`.
3. CI gate validates the block and checks URL resolution — PR is blocked otherwise.
4. After `eolDate`, status may be changed to `eol`. The plugin is never physically
   deleted from the registry during the 24-month archive window; old tarballs are
   preserved at `releases/archive/<plugin>-<version>.tgz`.

## Announcement Channels

- GitHub Release notes on the `plugins` repo.
- CHANGELOG entry with `[DEPRECATED]` tag.
- Migration guide published at the URL in `migrationGuide` before the PR lands.

## Security Exception

An active, exploited vulnerability with no available patch allows immediate
removal (0-day). Removal announcement must:
- Explicitly state "security-critical removal".
- List the CVE or equivalent vulnerability identifier.
- Provide a mitigation workaround in the announcement body.

Non-critical disclosures still require the standard 6-month notice period.
